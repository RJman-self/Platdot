// Copyright 2020 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package substrate

import (
	"errors"
	"fmt"
	"github.com/rjman-self/go-polkadot-rpc-client/models"
	"strconv"

	"github.com/rjman-self/go-polkadot-rpc-client/client"

	"github.com/rjmand/go-substrate-rpc-client/v2/types"
	"math/big"
	"time"

	"github.com/ChainSafe/chainbridge-utils/blockstore"
	metrics "github.com/ChainSafe/chainbridge-utils/metrics/types"
	"github.com/ChainSafe/chainbridge-utils/msg"
	"github.com/ChainSafe/log15"
	"github.com/rjman-self/Platdot/chains"
)

type listener struct {
	name           string
	chainId        msg.ChainId
	startBlock     uint64
	blockStore     blockstore.Blockstorer
	conn           *Connection
	depositNonce   map[DepositTarget]DepositNonce
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

//var BlockRetryLimit = 5

var MultiSignExtrinsicStatus = []int32{1, 2, 3}

func NewListener(conn *Connection, name string, id msg.ChainId, startBlock uint64, log log15.Logger, bs blockstore.Blockstorer,
	stop <-chan int, sysErr chan<- error, m *metrics.ChainMetrics, multiSignAddress types.AccountID, cli *client.Client,
	resource msg.ResourceId, dest msg.ChainId) *listener {
	return &listener{
		name:       name,
		chainId:    id,
		startBlock: startBlock,
		blockStore: bs,
		conn:       conn,
		//subscriptions: make(map[eventName]eventHandler),
		depositNonce:  make(map[DepositTarget]DepositNonce, 200),
		log:           log,
		stop:          stop,
		sysErr:        sysErr,
		latestBlock:   metrics.LatestBlock{LastUpdated: time.Now()},
		metrics:       m,
		client:        *cli,
		multiSignAddr: multiSignAddress,
		msTxAsMulti:   make(map[MultiSignTx]MultiSigAsMulti, 200),
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

	//var count = 0
	for {
		select {
		case <-l.stop:
			return errors.New("terminated")
		default:
			//fmt.Printf("Now deal with Block %d\n", currentBlock)
			/// Initialize the metadata
			/// Subscribe to system events via storage
			//fmt.Printf("current block is %v\n", currentBlock)

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
	//fmt.Printf("block hash is %v\n", hash)
	block, err := l.client.Api.RPC.Chain.GetBlock(hash)
	if err != nil {
		panic(err)
	}

	currentBlock := int64(block.Block.Header.Number)

	resp, err := l.client.GetBlockByNumber(currentBlock)
	if err != nil {
		panic(err)
	}

	for i, extrinsic := range resp.Extrinsic {
		var msTxAsMulti = MultiSigAsMulti{}
		if extrinsic.Type == "as_multi" {
			MsTxType := checkMultiSignExtrinsicStatus(extrinsic)
			switch MsTxType {
			case MultiSignExtrinsicStatus[0]:
				l.log.Info("find a MultiSign New extrinsic in block #", currentBlock, "#")
				///MultiSign New
				l.msTxStatistics.CurrentTx.MultiSignTxId = MultiSignTxId(extrinsic.ExtrinsicIndex)
				l.msTxStatistics.CurrentTx.BlockNumber = BlockNumber(currentBlock)
				msTxAsMulti.Executed = false
				msTxAsMulti.Threshold = extrinsic.MultiSigAsMulti.Threshold
				msTxAsMulti.OtherSignatories = extrinsic.MultiSigAsMulti.OtherSignatories
				msTxAsMulti.MaybeTimePoint = extrinsic.MultiSigAsMulti.MaybeTimePoint
				msTxAsMulti.DestAddress = extrinsic.MultiSigAsMulti.DestAddress
				msTxAsMulti.DestAmount = extrinsic.MultiSigAsMulti.DestAmount
				msTxAsMulti.StoreCall = extrinsic.MultiSigAsMulti.StoreCall
				msTxAsMulti.MaxWeight = extrinsic.MultiSigAsMulti.MaxWeight
				msTxAsMulti.OriginMsTx = l.msTxStatistics.CurrentTx
				l.msTxAsMulti[l.msTxStatistics.CurrentTx] = msTxAsMulti
				l.msTxStatistics.TotalCount++
			case MultiSignExtrinsicStatus[1]:
				l.log.Info("find a MultiSign Approve extrinsic in block #", currentBlock, "#")
				///MultiSign Approve
			case MultiSignExtrinsicStatus[2]:
				l.log.Info("find a MultiSign Executed extrinsic in block #", currentBlock, "#")
				/////MultiSign Executed
				l.msTxStatistics.CurrentTx.MultiSignTxId = MultiSignTxId(extrinsic.ExtrinsicIndex)
				l.msTxStatistics.CurrentTx.BlockNumber = BlockNumber(currentBlock)
				msTxAsMulti.Executed = false
				msTxAsMulti.Threshold = extrinsic.MultiSigAsMulti.Threshold
				msTxAsMulti.OtherSignatories = extrinsic.MultiSigAsMulti.OtherSignatories
				msTxAsMulti.MaybeTimePoint = extrinsic.MultiSigAsMulti.MaybeTimePoint
				msTxAsMulti.DestAddress = extrinsic.MultiSigAsMulti.DestAddress
				msTxAsMulti.DestAmount = extrinsic.MultiSigAsMulti.DestAmount
				msTxAsMulti.StoreCall = extrinsic.MultiSigAsMulti.StoreCall
				msTxAsMulti.MaxWeight = extrinsic.MultiSigAsMulti.MaxWeight
				//msTxAsMulti.OriginMsTx = l.msTxStatistics.CurrentTx
				///Find An existing multi-signed transaction in the record, and marks for executed status
				for k, ms := range l.msTxAsMulti {
					if !ms.Executed && ms.DestAddress == msTxAsMulti.DestAddress && ms.DestAmount == ms.DestAmount {
						exeMsTx := l.msTxAsMulti[k]
						exeMsTx.Executed = true

						depositTarget := DepositTarget{
							DestAddress: ms.DestAddress,
							DestAmount:  ms.DestAmount,
						}
						fmt.Printf("Executed: deposit Target is {destAddr: %s, destAmount: %s}\n", depositTarget.DestAddress, depositTarget.DestAmount)
						depositNonce := l.depositNonce[depositTarget]
						depositNonce.Status = true
						fmt.Printf("extrinsic of depositNonce %d has been executed\n", depositNonce.Nonce)
						l.depositNonce[depositTarget] = depositNonce
						l.msTxAsMulti[k] = exeMsTx
					}
				}
				//delete(l.msTxAsMulti, l.msTxStatistics.CurrentTx)
				l.msTxStatistics.TotalCount++
			}
		}
		if extrinsic.Type == "multiSignBatch" {
			fmt.Printf("find a MultiSign Batch extrinsic in block %v\n", currentBlock)
			/// 1. derive Extrinsic of Block
			/// 2. validate and get essential parameters of message

			amount, err := strconv.ParseInt(extrinsic.Amount, 10, 64)
			if err != nil {
				return err
			}
			recipient := []byte(extrinsic.Recipient)
			depositNonceA := strconv.FormatInt(currentBlock, 10)
			depositNonceB := strconv.FormatInt(int64(extrinsic.ExtrinsicIndex), 10)

			/// 3. construct parameters of message
			deposit := depositNonceA + depositNonceB
			depositNonce, _ := strconv.ParseInt(deposit, 10, 64)

			m := msg.NewFungibleTransfer(
				l.chainId,
				l.destId,
				msg.Nonce(depositNonce),
				big.NewInt(amount),
				l.resourceId,
				recipient,
			)
			fmt.Printf("ready to send %d PDOT to %s\n", amount, recipient)
			l.submitMessage(m, err)
			if err != nil {
				fmt.Printf("submit Message to Alaya meet a err: %v\n", err)
				return err
			}
			fmt.Printf("<---------------------- finish the No.%d MultiSignTransfer in block %v\n", i, currentBlock)
		}
	}
	return nil
}

func checkMultiSignExtrinsicStatus(extrinsic *models.ExtrinsicResponse) int32 {
	if extrinsic.Type == "as_multi" {
		if extrinsic.Status == "success" {
			///MultiSignTx Executed
			return MultiSignExtrinsicStatus[2]
		} else if extrinsic.MultiSigAsMulti.MaybeTimePoint.Index == 0 {
			///MultiSignTx New
			return MultiSignExtrinsicStatus[0]
		} else {
			///MultiSignTx Approve
			return MultiSignExtrinsicStatus[1]
		}
	} else if extrinsic.Type == "cancel_as_multi" {
		///MultiSignTx Cancel
		fmt.Printf("cancel_as_multi, nothing to do with it")
	}
	return 0
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
