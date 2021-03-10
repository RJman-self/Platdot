// Copyright 2020 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package substrate

import (
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"strconv"

	"github.com/JFJun/go-substrate-crypto/ss58"
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
}

// Frequency of polling for a new block
var BlockRetryInterval = time.Second * 5

func NewListener(conn *Connection, name string, id msg.ChainId, startBlock uint64, log log15.Logger, bs blockstore.Blockstorer,
	stop <-chan int, sysErr chan<- error, m *metrics.ChainMetrics, multiSignAddress types.AccountID) *listener {

	c, err := client.New(url)
	if err != nil {
		panic(err)
	}
	c.SetPrefix(ss58.PolkadotPrefix)

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
		client:        *c,
		multiSignAddr: multiSignAddress,
		msTxAsMulti:   make(map[MultiSignTx]MultiSigAsMulti, 200),
	}
}

func (l *listener) setRouter(r chains.Router) {
	l.router = r
}

// start creates the initial subscription for all events
func (l *listener) start() error {

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
			fmt.Printf("Now deal with Block %d\n", currentBlock)
			l.log.Trace("Now deal with Block ", currentBlock)
			/// Initialize the metadata
			/// Subscribe to system events via storage
			fmt.Printf("current block is %v\n", currentBlock)
			key, err := types.CreateStorageKey(l.client.Meta, "System", "Events", nil, nil)

			if err != nil {
				panic(err)
			}

			sub, err := l.client.Api.RPC.State.SubscribeStorageRaw([]types.StorageKey{key})
			if err != nil {
				panic(err)
			}

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

			//fmt.Printf("block hash is %v\n", hash)
			block, err := l.client.Api.RPC.Chain.GetBlock(hash)

			if err != nil {
				panic(err)
			}

			set := <-sub.Chan()
			// inner loop for the changes within one of those notifications
			for _, change := range set.Changes {
				if !types.Eq(change.StorageKey, key) || !change.HasStorageData {
					// skip, we are only interested in events with content
					continue
				}
				// Decode the event records
				events := types.EventRecords{}
				err = types.EventRecordsRaw(change.StorageData).DecodeEventRecords(l.client.Meta, &events)

				l.log.Trace("Block set change detected, handle events\n")

				for _, e := range events.Multisig_MultisigExecuted {
					fmt.Printf("\tSystem:detect new multisign Executed:: (phase=%#v)\n", e.Phase)
					fmt.Printf("\t\tFrom:%v,To: %v\n", e.Who, e.ID)

					// 1. derive Extrinsics of Block
					resp, err := l.client.GetBlockByNumber(int64(block.Block.Header.Number))
					if err != nil {
						panic(err)
					}

					var msTxAsMulti = MultiSigAsMulti{}

					for _, extrinsic := range resp.Extrinsic {
						if extrinsic.Type == "as_multi" {
							l.msTxStatistics.CurrentTx.MultiSignTxId = MultiSignTxId(extrinsic.ExtrinsicIndex)
							l.msTxStatistics.CurrentTx.BlockNumber = BlockNumber(block.Block.Header.Number)
							msTxAsMulti.Executed = false
							msTxAsMulti.Threshold = extrinsic.MultiSigAsMulti.Threshold
							msTxAsMulti.OtherSignatories = extrinsic.MultiSigAsMulti.OtherSignatories
							msTxAsMulti.MaybeTimePoint = extrinsic.MultiSigAsMulti.MaybeTimePoint
							msTxAsMulti.DestAddress = extrinsic.MultiSigAsMulti.DestAddress
							msTxAsMulti.DestAmount = extrinsic.MultiSigAsMulti.DestAmount
							msTxAsMulti.StoreCall = extrinsic.MultiSigAsMulti.StoreCall
							msTxAsMulti.MaxWeight = extrinsic.MultiSigAsMulti.MaxWeight
							//amount, err = strconv.ParseInt(extrinsic.Amount, 10, 64)
							//recipient = types.NewAccountID([]byte(extrinsic.Recipient))
						} else {
							continue
						}
					}
					msTxAsMulti.OriginMsTx = l.msTxStatistics.CurrentTx
					for k, ms := range l.msTxAsMulti {
						if !ms.Executed && ms.DestAddress == msTxAsMulti.DestAddress && ms.DestAmount == ms.DestAmount {
							exeMsTx := l.msTxAsMulti[k]
							exeMsTx.Executed = true
							/// TODO:remark
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

				for _, e := range events.Multisig_NewMultisig {
					fmt.Printf("\tSystem:detect new multisign request:: (phase=%#v)\n", e.Phase)
					fmt.Printf("\t\tFrom:%v,To: %v\n", e.Who, e.ID)

					// 1. derive Extrinsic of Block
					resp, err := l.client.GetBlockByNumber(int64(currentBlock))
					if err != nil {
						panic(err)
					}

					var msTxAsMulti = MultiSigAsMulti{}
					for _, extrinsic := range resp.Extrinsic {
						if extrinsic.Type == "as_multi" {
							l.msTxStatistics.CurrentTx.MultiSignTxId = MultiSignTxId(extrinsic.ExtrinsicIndex)
							l.msTxStatistics.CurrentTx.BlockNumber = BlockNumber(block.Block.Header.Number)
							msTxAsMulti.Executed = false
							msTxAsMulti.Threshold = extrinsic.MultiSigAsMulti.Threshold
							msTxAsMulti.OtherSignatories = extrinsic.MultiSigAsMulti.OtherSignatories
							msTxAsMulti.MaybeTimePoint = extrinsic.MultiSigAsMulti.MaybeTimePoint
							msTxAsMulti.DestAddress = extrinsic.MultiSigAsMulti.DestAddress
							msTxAsMulti.DestAmount = extrinsic.MultiSigAsMulti.DestAmount
							msTxAsMulti.StoreCall = extrinsic.MultiSigAsMulti.StoreCall
							msTxAsMulti.MaxWeight = extrinsic.MultiSigAsMulti.MaxWeight
							//amount, err = strconv.ParseInt(extrinsic.Amount, 10, 64)
							//recipient = types.NewAccountID([]byte(extrinsic.Recipient))
						} else {
							continue
						}
					}
					msTxAsMulti.OriginMsTx = l.msTxStatistics.CurrentTx
					l.msTxAsMulti[l.msTxStatistics.CurrentTx] = msTxAsMulti
					l.msTxStatistics.TotalCount++
				}

				if events.Utility_BatchCompleted != nil {
					l.log.Trace("<1>. Receive a batchCompleted")
					for _, e := range events.Balances_Transfer {
						l.log.Trace("<2>. verify there is a transfer event")
						if e.To == l.multiSignAddr {
							l.log.Trace("<3>. verify the balance of multiSign account changed")
							///BEGIN: Core process => DOT to PDOT
							/// 1. derive Extrinsic of Block
							resp, err := l.client.GetBlockByNumber(int64(currentBlock))
							if err != nil {
								panic(err)
							}

							// 2. validate and get essential parameters of message
							var recipient []byte
							var amount int64
							var depositNoceA, depositNoceB string

							//derive information from block
							for i, extrinsic := range resp.Extrinsic {
								//Only System.batch(transfer + remark) -> extrinsic can be parsed
								if extrinsic.Type == "multiSignBatch" {
									amount, err = strconv.ParseInt(extrinsic.Amount, 10, 64)
									recipient = []byte(extrinsic.Recipient)
									depositNoceA = strconv.FormatInt(int64(currentBlock), 10)
									depositNoceB = strconv.FormatInt(int64(extrinsic.ExtrinsicIndex), 10)

									// 3. construct parameters of message
									deposit := depositNoceA + depositNoceB
									depositNonce, _ := strconv.ParseInt(deposit, 10, 64)

									m := msg.NewFungibleTransfer(
										msg.ChainId(chainSub), // Unset
										msg.ChainId(chainAlaya),
										msg.Nonce(depositNonce),
										big.NewInt(amount),
										msg.ResourceIdFromSlice(common.FromHex(AKSM)),
										recipient,
									)
									fmt.Printf("ready to send %d PDOT to %s\n", amount, recipient)
									l.submitMessage(m, err)
									if err != nil {
										fmt.Printf("submit Message to Alaya meet a err: %v\n", err)
									}
									fmt.Printf("<---------------------- finish the No.%d MultiSignTransfer in currentBlock\n", i)
								} else {
									continue
								}
							}
						}
					}
				}
				l.log.Trace("Finished processing events, block", hash.Hex())
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

func (l *listener) handleEvent(events types.EventRecords, currentBlock uint64, block *types.SignedBlock) error {
	// Show what we are busy with

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
