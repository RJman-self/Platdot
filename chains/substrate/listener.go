// Copyright 2020 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package substrate

import (
	"errors"
	"fmt"
	"github.com/JFJun/go-substrate-crypto/ss58"
	"github.com/rjman-self/go-polkadot-rpc-client/expand/polkadot"
	"github.com/rjman-self/go-polkadot-rpc-client/models"
	"strconv"

	"github.com/rjman-self/go-polkadot-rpc-client/client"

	"github.com/rjmand/go-substrate-rpc-client/v2/types"
	"math/big"
	"time"

	"github.com/ChainSafe/log15"
	"github.com/rjman-self/Platdot/chains"
	"github.com/rjman-self/platdot-utils/blockstore"
	metrics "github.com/rjman-self/platdot-utils/metrics/types"
	"github.com/rjman-self/platdot-utils/msg"
)

type listener struct {
	name          string
	chainId       msg.ChainId
	startBlock    uint64
	blockStore    blockstore.Blockstorer
	conn          *Connection
	router        chains.Router
	log           log15.Logger
	stop          <-chan int
	sysErr        chan<- error
	latestBlock   metrics.LatestBlock
	metrics       *metrics.ChainMetrics
	client        client.Client
	multiSignAddr types.AccountID
	currentTx     MultiSignTx
	msTxAsMulti   map[MultiSignTx]MultiSigAsMulti
	resourceId    msg.ResourceId
	destId        msg.ChainId
	relayer       Relayer
}

// Frequency of polling for a new block
const InitCapacity = 500

var BlockRetryInterval = time.Second * 5
var BlockRetryLimit = 10
var KSM int64 = 1e12
var FixedFee = KSM * 3 / 100
var FeeRate int64 = 1000

func NewListener(conn *Connection, name string, id msg.ChainId, startBlock uint64, log log15.Logger, bs blockstore.Blockstorer,
	stop <-chan int, sysErr chan<- error, m *metrics.ChainMetrics, multiSignAddress types.AccountID, cli *client.Client,
	resource msg.ResourceId, dest msg.ChainId, relayer Relayer) *listener {
	return &listener{
		name:          name,
		chainId:       id,
		startBlock:    startBlock,
		blockStore:    bs,
		conn:          conn,
		log:           log,
		stop:          stop,
		sysErr:        sysErr,
		latestBlock:   metrics.LatestBlock{LastUpdated: time.Now()},
		metrics:       m,
		client:        *cli,
		multiSignAddr: multiSignAddress,
		msTxAsMulti:   make(map[MultiSignTx]MultiSigAsMulti, InitCapacity),
		resourceId:    resource,
		destId:        dest,
		relayer:       relayer,
	}
}

func (l *listener) setRouter(r chains.Router) {
	l.router = r
}

// start creates the initial subscription for all events
func (l *listener) start() error {
	// Check whether latest is less than starting block
	header, err := l.client.Api.RPC.Chain.GetHeaderLatest()
	if err != nil {
		return err
	}
	if uint64(header.Number) < l.startBlock {
		return fmt.Errorf("starting block (%d) is greater than latest known block (%d)", l.startBlock, header.Number)
	}

	go func() {
		err := l.pollBlocks()
		if err != nil {
			l.log.Error("Polling blocks failed", "err", err)
		}
	}()
	return nil
}

var ErrBlockNotReady = errors.New("required result to be 32 bytes, but got 0")

// pollBlocks will poll for the latest block and proceed to parse the associated events as it sees new blocks.
// Polling begins at the block defined in `l.startBlock`. Failed attempts to fetch the latest block or parse
// a block will be retried up to BlockRetryLimit times before returning with an error.
func (l *listener) pollBlocks() error {
	l.log.Info("Polling Blocks...")
	var currentBlock = l.startBlock
	var retry = BlockRetryLimit
	for {
		select {
		case <-l.stop:
			return errors.New("terminated")
		default:
			// No more retries, goto next block
			if retry == 0 {
				l.sysErr <- fmt.Errorf("event polling retries exceeded (chain=%d, name=%s)", l.chainId, l.name)
				return nil
			}

			/// Get finalized block hash
			finalizedHash, err := l.client.Api.RPC.Chain.GetFinalizedHead()
			if err != nil {
				l.log.Error("Failed to fetch finalized hash", "err", err)
				retry--
				time.Sleep(BlockRetryInterval)
				continue
			}

			// Get finalized block header
			finalizedHeader, err := l.client.Api.RPC.Chain.GetHeader(finalizedHash)
			if err != nil {
				l.log.Error("Failed to fetch finalized header", "err", err)
				retry--
				time.Sleep(BlockRetryInterval)
				continue
			}

			if l.metrics != nil {
				l.metrics.LatestKnownBlock.Set(float64(finalizedHeader.Number))
			}

			// Sleep if the block we want comes after the most recently finalized block
			if currentBlock > uint64(finalizedHeader.Number) {
				l.log.Trace("Block not yet finalized", "target", currentBlock, "latest", finalizedHeader.Number)
				time.Sleep(BlockRetryInterval)
				continue
			}

			/// Get hash for latest block, sleep and retry if not ready
			hash, err := l.client.Api.RPC.Chain.GetBlockHash(currentBlock)
			if err != nil && err.Error() == ErrBlockNotReady.Error() {
				time.Sleep(BlockRetryInterval)
				continue
			} else if err != nil {
				l.log.Error("Failed to query latest block", "block", currentBlock, "err", err)
				retry--
				time.Sleep(BlockRetryInterval)
				continue
			}

			err = l.processBlock(hash)
			if err != nil {
				l.log.Error("Failed to process current block", "block", currentBlock, "err", err)
				retry--
				continue
			}

			// Write to blockStore
			err = l.blockStore.StoreBlock(big.NewInt(0).SetUint64(currentBlock))
			if err != nil {
				l.log.Error("Failed to write to blockStore", "err", err)
			}

			if l.metrics != nil {
				l.metrics.BlocksProcessed.Inc()
				l.metrics.LatestProcessedBlock.Set(float64(currentBlock))
			}

			currentBlock++
			l.latestBlock.Height = big.NewInt(0).SetUint64(currentBlock)
			l.latestBlock.LastUpdated = time.Now()

			/// Succeed, reset retryLimit
			retry = BlockRetryLimit
		}
	}
}

func (l *listener) processBlock(hash types.Hash) error {
	block, err := l.client.Api.RPC.Chain.GetBlock(hash)
	if err != nil {
		panic(err)
	}

	currentBlock := int64(block.Block.Header.Number)

	resp, err := l.client.GetBlockByNumber(currentBlock)
	if err != nil {
		panic(err)
	}

	for _, e := range resp.Extrinsic {
		// Current Extrinsic { Block, Index }
		l.currentTx.BlockNumber = BlockNumber(currentBlock)
		l.currentTx.MultiSignTxId = MultiSignTxId(e.ExtrinsicIndex)
		msTx := MultiSigAsMulti{
			DestAddress: e.MultiSigAsMulti.DestAddress,
			DestAmount:  e.MultiSigAsMulti.DestAmount,
		}

		if e.Type == polkadot.AsMultiNew {
			l.log.Info("Find a MultiSign New extrinsic", "Block", currentBlock)
			/// Mark New a MultiSign Transfer
			l.markNew(e)
		}
		if e.Type == polkadot.AsMultiApprove {
			l.log.Info("Find a MultiSign Approve extrinsic", "Block", currentBlock)
			/// Mark Vote(Approve)
			l.markVote(msTx, e)
		}

		if e.Type == polkadot.AsMultiExecuted {
			l.log.Info("Find a MultiSign Executed extrinsic", "Block", currentBlock)
			// Find An existing multi-signed transaction in the record, and marks for executed status
			l.markVote(msTx, e)
			l.markExecution(msTx)
		}
		if e.Type == polkadot.UtilityBatch {
			l.log.Info("Find a MultiSign Batch Extrinsic", "Block", currentBlock)
			// Construct parameters of message
			amount, ok := big.NewInt(0).SetString(e.Amount, 10)
			if !ok {
				fmt.Printf("parse transfer amount %v, amount.string %v\n", amount, amount.String())
			}
			receiveAmount := amount

			fixedFee := big.NewInt(FixedFee)
			additionalFee := big.NewInt(0).Div(amount, big.NewInt(FeeRate))
			fee := big.NewInt(0).Add(fixedFee, additionalFee)

			actualAmount := big.NewInt(0).Sub(amount, fee)
			sendAmount := big.NewInt(0).Mul(actualAmount, big.NewInt(oneToken))

			recipient := []byte(e.Recipient)
			depositNonce, _ := strconv.ParseInt(strconv.FormatInt(currentBlock, 10)+strconv.FormatInt(int64(e.ExtrinsicIndex), 10), 10, 64)

			m := msg.NewFungibleTransfer(
				l.chainId,
				l.destId,
				msg.Nonce(depositNonce),
				sendAmount,
				l.resourceId,
				recipient,
			)
			/// Validate whether a cross-chain transaction
			receivePubAddress, _ := ss58.DecodeToPub(e.ToAddress)
			receiveAddress := types.NewAddressFromAccountID(receivePubAddress)
			if receiveAddress.AsAccountID == l.multiSignAddr {
				fmt.Printf("KSM to AKSM, Amount is %v, Fee is %v, Actual_AKSM_Amount = %v\n", receiveAmount, fee, sendAmount)
				l.log.Info("Ready to send AKSM...", "Amount", actualAmount, "Recipient", recipient)
				l.submitMessage(m, err)
				if err != nil {
					l.log.Error("Submit message to Writer", "Error", err)
					return err
				}
			}
		}
	}
	return nil
}

// submitMessage inserts the chainId into the msg and sends it to the router
func (l *listener) submitMessage(m msg.Message, err error) {
	if err != nil {
		log15.Error("Critical error processing event", "err", err)
		return
	}
	m.Source = l.chainId
	err = l.router.Send(m)
	if err != nil {
		log15.Error("failed to process event", "err", err)
	}
}

func (l *listener) markExecution(msTx MultiSigAsMulti) {
	for k, ms := range l.msTxAsMulti {
		if !ms.Executed && ms.DestAddress == msTx.DestAddress && ms.DestAmount == msTx.DestAmount {
			exeMsTx := l.msTxAsMulti[k]
			exeMsTx.Executed = true
			l.msTxAsMulti[k] = exeMsTx
		}
	}
}

func (l *listener) markVote(msTx MultiSigAsMulti, e *models.ExtrinsicResponse) {
	for k, ms := range l.msTxAsMulti {
		if !ms.Executed && ms.DestAddress == msTx.DestAddress && ms.DestAmount == msTx.DestAmount {
			//l.log.Info("relayer succeed vote", "Address", e.FromAddress)
			voteMsTx := l.msTxAsMulti[k]
			voteMsTx.Others = append(voteMsTx.Others, e.MultiSigAsMulti.OtherSignatories)
			l.msTxAsMulti[k] = voteMsTx
		}
	}
}

func (l *listener) markNew(e *models.ExtrinsicResponse) {
	msTx := MultiSigAsMulti{
		Executed:       false,
		Threshold:      e.MultiSigAsMulti.Threshold,
		MaybeTimePoint: e.MultiSigAsMulti.MaybeTimePoint,
		DestAddress:    e.MultiSigAsMulti.DestAddress,
		DestAmount:     e.MultiSigAsMulti.DestAmount,
		Others:         nil,
		StoreCall:      e.MultiSigAsMulti.StoreCall,
		MaxWeight:      e.MultiSigAsMulti.MaxWeight,
		OriginMsTx:     l.currentTx,
	}
	/// Mark voted
	msTx.Others = append(msTx.Others, e.MultiSigAsMulti.OtherSignatories)
	l.msTxAsMulti[l.currentTx] = msTx
}
