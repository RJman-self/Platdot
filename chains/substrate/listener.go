// Copyright 2020 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package substrate

import (
	"errors"
	"fmt"
	"github.com/rjman-self/go-polkadot-rpc-client/expand/polkadot"
	"strconv"

	"github.com/rjman-self/go-polkadot-rpc-client/client"

	"github.com/rjmand/go-substrate-rpc-client/v2/types"
	"math/big"
	"time"

	"github.com/rjman-self/platdot-utils/blockstore"
	metrics "github.com/rjman-self/platdot-utils/metrics/types"
	"github.com/rjman-self/platdot-utils/msg"
	"github.com/ChainSafe/log15"
	"github.com/rjman-self/Platdot/chains"
)

type listener struct {
	name           string
	chainId        msg.ChainId
	startBlock     uint64
	blockStore     blockstore.Blockstorer
	conn           *Connection
	depositNonce   map[DepositTarget][]DepositNonce
	router         chains.Router
	log            log15.Logger
	stop           <-chan int
	sysErr         chan<- error
	latestBlock    metrics.LatestBlock
	metrics        *metrics.ChainMetrics
	client         client.Client
	multiSignAddr  types.AccountID
	msTxStatistics MultiSignTxStatistics
	msTxAsMulti    map[MultiSignTx]MultiSigAsMulti
	resourceId     msg.ResourceId
	destId         msg.ChainId
}

// Frequency of polling for a new block
var BlockRetryInterval = time.Second * 5

func NewListener(conn *Connection, name string, id msg.ChainId, startBlock uint64, log log15.Logger, bs blockstore.Blockstorer,
	stop <-chan int, sysErr chan<- error, m *metrics.ChainMetrics, multiSignAddress types.AccountID, cli *client.Client,
	resource msg.ResourceId, dest msg.ChainId) *listener {
	return &listener{
		name:          name,
		chainId:       id,
		startBlock:    startBlock,
		blockStore:    bs,
		conn:          conn,
		depositNonce:  make(map[DepositTarget][]DepositNonce, 500),
		log:           log,
		stop:          stop,
		sysErr:        sysErr,
		latestBlock:   metrics.LatestBlock{LastUpdated: time.Now()},
		metrics:       m,
		client:        *cli,
		multiSignAddr: multiSignAddress,
		msTxAsMulti:   make(map[MultiSignTx]MultiSigAsMulti, 500),
		resourceId:    resource,
		destId:        dest,
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
	var currentBlock = l.startBlock
	for {
		select {
		case <-l.stop:
			return errors.New("terminated")
		default:
			/// Get finalized block hash
			finalizedHash, err := l.client.Api.RPC.Chain.GetFinalizedHead()
			if err != nil {
				l.log.Error("Failed to fetch finalized hash", "err", err)
				time.Sleep(BlockRetryInterval)
				continue
			}

			// Get finalized block header
			finalizedHeader, err := l.client.Api.RPC.Chain.GetHeader(finalizedHash)

			if err != nil {
				l.log.Error("Failed to fetch finalized header", "err", err)
				time.Sleep(BlockRetryInterval)
				continue
			}

			if l.metrics != nil {
				l.metrics.LatestKnownBlock.Set(float64(finalizedHeader.Number))
			}

			hash, err := l.client.Api.RPC.Chain.GetBlockHash(currentBlock)
			if err != nil && err.Error() == ErrBlockNotReady.Error() {
				time.Sleep(BlockRetryInterval)
				continue
			} else if err != nil {
				l.log.Error("Failed to query latest block", "block", currentBlock, "err", err)
				time.Sleep(BlockRetryInterval)
				continue
			}

			err = l.processBlock(hash)
			if err != nil {
				fmt.Printf("err is %v\n", err)
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
		var msTx = MultiSigAsMulti{}
		if e.Type == polkadot.AsMultiNew {
			l.log.Info("Find a MultiSign New extrinsic in block", "Block", currentBlock)
			// MultiSign New
			l.msTxStatistics.CurrentTx.MultiSignTxId = MultiSignTxId(e.ExtrinsicIndex)
			l.msTxStatistics.CurrentTx.BlockNumber = BlockNumber(currentBlock)
			msTx = MultiSigAsMulti{
				Executed:         false,
				Threshold:        e.MultiSigAsMulti.Threshold,
				OtherSignatories: e.MultiSigAsMulti.OtherSignatories,
				MaybeTimePoint:   e.MultiSigAsMulti.MaybeTimePoint,
				DestAddress:      e.MultiSigAsMulti.DestAddress,
				DestAmount:       e.MultiSigAsMulti.DestAmount,
				StoreCall:        e.MultiSigAsMulti.StoreCall,
				MaxWeight:        e.MultiSigAsMulti.MaxWeight,
				OriginMsTx:       l.msTxStatistics.CurrentTx,
			}

			l.msTxAsMulti[l.msTxStatistics.CurrentTx] = msTx
			l.msTxStatistics.TotalCount++
		}
		if e.Type == polkadot.AsMultiApprove {
			l.log.Info("Find a MultiSign Approve extrinsic in block #", currentBlock, "#")
		}
		if e.Type == polkadot.AsMultiExecuted {
			l.log.Info("Find a MultiSign Executed extrinsic in block #", currentBlock, "#")
			l.msTxStatistics.CurrentTx.MultiSignTxId = MultiSignTxId(e.ExtrinsicIndex)
			l.msTxStatistics.CurrentTx.BlockNumber = BlockNumber(currentBlock)
			msTx = MultiSigAsMulti{
				DestAddress: e.MultiSigAsMulti.DestAddress,
				DestAmount:  e.MultiSigAsMulti.DestAmount,
			}
			// Find An existing multi-signed transaction in the record, and marks for executed status
			l.markExecution(msTx)
		}
		if e.Type == polkadot.UtilityBatch {
			l.log.Info("Find a MultiSign Batch Extrinsic!", "CurrentBlock", currentBlock)
			// Construct parameters of message
			amount, err := strconv.ParseInt(e.Amount, 10, 64)
			if err != nil {
				return err
			}
			amount = (amount * 95) / 100
			recipient := []byte(e.Recipient)
			depositNonceA := strconv.FormatInt(currentBlock, 10)
			depositNonceB := strconv.FormatInt(int64(e.ExtrinsicIndex), 10)

			depositNonce, _ := strconv.ParseInt(depositNonceA+depositNonceB, 10, 64)

			m := msg.NewFungibleTransfer(
				l.chainId,
				l.destId,
				msg.Nonce(depositNonce),
				big.NewInt(amount),
				l.resourceId,
				recipient,
			)
			l.log.Info("Ready to send PDOT...", "Amount", amount, "Recipient", recipient)
			l.submitMessage(m, err)
			if err != nil {
				l.log.Error("Submit message to Alaya:", "Error", err)
				return err
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
			l.log.Info("Tx executed!", "BlockNumber", ms.OriginMsTx.BlockNumber, "Address", msTx.DestAddress, "Amount", msTx.DestAmount)
			exeMsTx := l.msTxAsMulti[k]
			exeMsTx.Executed = true
			l.msTxAsMulti[k] = exeMsTx
		}
	}
}
