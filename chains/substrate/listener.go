// Copyright 2020 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package substrate

import (
	"errors"
	"fmt"

	//_ "github.com/ChainSafe/chainbridge-utils/crypto"
	"github.com/JFJun/go-substrate-crypto/ss58"
	"github.com/ethereum/go-ethereum/common"
	"github.com/rjman-self/go-polkadot-rpc-client/client"
	"github.com/rjman-self/go-polkadot-rpc-client/models"
	"strconv"

	//"github.com/stafiprotocol/go-substrate-rpc-client"
	//"github.com/stafiprotocol/go-substrate-rpc-client/config"
	//"github.com/stafiprotocol/go-substrate-rpc-client/types"

	gsrpc "github.com/rjmand/go-substrate-rpc-client/v2"
	"github.com/rjmand/go-substrate-rpc-client/v2/config"
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
	subscriptions  map[eventName]eventHandler // Handlers for specific events
	depositNonce   map[types.AccountID]int    //记录每个账户交易的nonce
	router         chains.Router
	log            log15.Logger
	stop           <-chan int
	sysErr         chan<- error
	latestBlock    metrics.LatestBlock
	metrics        *metrics.ChainMetrics
	client         client.Client
	msTxStatistics MultiSignTxStatistics
	msTxAsMulti    map[MultiSignTx]MultiSigAsMulti
}

// Frequency of polling for a new block
var BlockRetryInterval = time.Second * 5
var BlockRetryLimit = 5
var AKSM = "0x0000000000000000000000000000000000000000000000000000000000000000"
var chainSub = 1
var chainAlaya = 0
var MultiSignAddress = "0xbc1d0c69609ecf7cf6513415502b96247cf1747bfde31427462b2406d2f13746"

func NewListener(conn *Connection, name string, id msg.ChainId, startBlock uint64, log log15.Logger, bs blockstore.Blockstorer,
	stop <-chan int, sysErr chan<- error, m *metrics.ChainMetrics) *listener {

	c, err := client.New(url)
	if err != nil {
		panic(err)
	}
	c.SetPrefix(ss58.PolkadotPrefix)

	var multiSigAsMulti map[MultiSignTx]MultiSigAsMulti
	return &listener{
		name:          name,
		chainId:       id,
		startBlock:    startBlock,
		blockStore:    bs,
		conn:          conn,
		subscriptions: make(map[eventName]eventHandler),
		depositNonce:  make(map[types.AccountID]int),
		log:           log,
		stop:          stop,
		sysErr:        sysErr,
		latestBlock:   metrics.LatestBlock{LastUpdated: time.Now()},
		metrics:       m,
		client:        *c,
		msTxAsMulti:   multiSigAsMulti,
	}
}

func (l *listener) setRouter(r chains.Router) {
	l.router = r
}

// start creates the initial subscription for all events
func (l *listener) start() error {
	// Check whether latest is less than starting block
	header, err := l.conn.api.RPC.Chain.GetHeaderLatest()
	if err != nil {
		return err
	}
	if uint64(header.Number) < l.startBlock {
		//return fmt.Errorf("starting block (%d) is greater than latest known block (%d)", l.startBlock, header.Number)
	}

	for _, sub := range Subscriptions {
		err := l.registerEventHandler(sub.name, sub.handler)
		if err != nil {
			return err
		}
	}

	go func() {
		err := l.pollBlocks()
		if err != nil {
			l.log.Error("Polling blocks failed", "err", err)
		}
	}()
	return nil
}

// registerEventHandler enables a handler for a given event. This cannot be used after Start is called.
func (l *listener) registerEventHandler(name eventName, handler eventHandler) error {
	if l.subscriptions[name] != nil {
		return fmt.Errorf("event %s already registered", name)
	}
	l.subscriptions[name] = handler
	return nil
}

var ErrBlockNotReady = errors.New("required result to be 32 bytes, but got 0")

// pollBlocks will poll for the latest block and proceed to parse the associated events as it sees new blocks.
// Polling begins at the block defined in `l.startBlock`. Failed attempts to fetch the latest block or parse
// a block will be retried up to BlockRetryLimit times before returning with an error.
func (l *listener) pollBlocks() error {
	var currentBlock = l.startBlock
	// assume TestKeyringPairBob.PublicKey is a multisign address

	var multiSignPk, _ = types.HexDecodeString(MultiSignAddress)
	var multiSignAccount = types.NewAccountID(multiSignPk)

	// Instantiate the API
	api, err := gsrpc.NewSubstrateAPI(config.Default().RPCURL)
	if err != nil {
		panic(err)
	}

	meta, err := api.RPC.State.GetMetadataLatest()
	if err != nil {
		panic(err)
	}

	// Subscribe to system events via storage
	key, err := types.CreateStorageKey(meta, "System", "Events", nil, nil)
	if err != nil {
		panic(err)
	}

	sub, err := api.RPC.State.SubscribeStorageRaw([]types.StorageKey{key})
	if err != nil {
		panic(err)
	}

	defer sub.Unsubscribe()

	//var count = 0
	for {
		select {
		case <-l.stop:
			return errors.New("terminated")
		default:
			//count += 1
			//fmt.Printf("poll count = %d\n", count)

			// Get finalized block hash
			finalizedHash, err := api.RPC.Chain.GetFinalizedHead()
			if err != nil {
				l.log.Error("Failed to fetch finalized hash", "err", err)
				time.Sleep(BlockRetryInterval)
				continue
			}

			// Get finalized block header
			finalizedHeader, err := api.RPC.Chain.GetHeader(finalizedHash)
			if err != nil {
				l.log.Error("Failed to fetch finalized header", "err", err)
				time.Sleep(BlockRetryInterval)
				continue
			}

			if l.metrics != nil {
				l.metrics.LatestKnownBlock.Set(float64(finalizedHeader.Number))
			}

			hash, err := api.RPC.Chain.GetBlockHash(currentBlock)

			if err != nil && err.Error() == ErrBlockNotReady.Error() {
				time.Sleep(BlockRetryInterval)
				continue
			} else if err != nil {
				l.log.Error("Failed to query latest block", "block", currentBlock, "err", err)
				time.Sleep(BlockRetryInterval)
				continue
			}

			//fmt.Printf("block hash is %v\n", hash)
			block, err := api.RPC.Chain.GetBlock(hash)

			if err != nil {
				panic(err)
			}

			//var blockNumber = int64(block.Block.Header.Number)
			//fmt.Printf("block# %d\n", blockNumber)

			//blockFinalize, err := l.conn.api.RPC.Chain.SubscribeFinalizedHeads()
			//fmt.Printf("block:\n%v\n", blockFinalize)

			//block, _ := l.conn.api.RPC.Chain.GetBlockLatest()

			//fmt.Printf("block:\n%v\n", block)
			//number, _ := strconv.ParseInt(utils.RemoveHex0x(block.Block.Header.Number), 16, 64)
			// Extrinsics in the block

			//fmt.Printf("\tYes! Found %d extrinsic(s) in this block.\n", len(block.Block.Extrinsics))
			//var extrinsics = resp.Extrinsic
			//for index, extrinsic := range extrinsics {
			//	fmt.Printf("这是 #%.0f 块的第%d笔交易-------------------------------------------------------------------------\n", float64(block.Block.Header.Number), index)
			//
			//	//fmt.Printf("Unmarshal %v\n", unmarshal)
			//	//fmt.Printf("extrinsic MethodIndex is %d, extrinsic sectionIndex is %d\n", extrinsic.Method.CallIndex.MethodIndex, extrinsic.Method.CallIndex.SectionIndex)
			//	fmt.Printf("extrinsic CallIndex is %v\n", extrinsic.Method.CallIndex)
			//
			//	//var extDec types.Extrinsic
			//	//err = types.DecodeFromHexString(, &extDec)
			//
			//
			//	if extrinsic.Method.CallIndex.MethodIndex != 0 && extrinsic.Method.CallIndex.SectionIndex != 3 {
			//		//fmt.Printf("ext is %v\n", extrinsic)
			//		fmt.Printf("extArgs is %v\n", extrinsic.Method.Args[:])
			//
			//		//for i, arg := range extrinsic.Method.Args {
			//		//	var str = types.HexEncodeToString(extrinsic.Method.Args)
			//		//
			//		//	fmt.Printf("%v\n", str)
			//		//let [prefix, buffer] = parsePrefix(extrinsic.args[0]);
			//		//// Get sender address
			//		//let sender = extrinsic.signer.toString();
			//
			//		//fmt.Printf("Index: %d, Arg: %s", i, arg);
			//		//}
			//
			//		//extrinsic.Decode()
			//		fmt.Printf("extrinsic is Version %v\n", extrinsic.Version)
			//		//fmt.Printf("extrinsic is:---------------------------------------------\n")
			//		//fmt.Printf("%v\n", extrinsic.Method.Args)
			//		//fmt.Printf("extrinsic is over-----------------------------------------\n")
			//	}
			//
			//	//if extrinsic.Method.CallIndex.MethodIndex != 0 && extrinsic.Method.CallIndex.SectionIndex != 3{
			//	//
			//	//}
			//}

			set := <-sub.Chan()
			// inner loop for the changes within one of those notifications
			for _, change := range set.Changes {
				if !types.Eq(change.StorageKey, key) || !change.HasStorageData {
					// skip, we are only interested in events with countent
					continue
				}

				// Decode the event records
				events := types.EventRecords{}
				err = types.EventRecordsRaw(change.StorageData).DecodeEventRecords(meta, &events)
				if err != nil {
					//panic(err)
					fmt.Printf("\terr is %v\n", err)
				}

				// Show what we are busy with
				//for _, e := range events.Balances_Transfer {
				//	//fmt.Printf("\tBalances:Transfer:: (phase = %v)\n", e.Phase)
				//	//fmt.Printf("\t\t%v, %v, %v\n", e.From, e.To, e.Value)
				//}
				for _, e := range events.Balances_Deposit {
					fmt.Printf("\tBalances:Deposit:: (phase=%#v)\n", e.Phase)
					fmt.Printf("\t\t%v, %v\n", e.Who, e.Balance)
				}
				for _, e := range events.Multisig_NewMultisig {
					fmt.Printf("\tSystem:detect new multisign request:: (phase=%#v)\n", e.Phase)
					fmt.Printf("\t\tFrom:%v,To: %v\n", e.Who, e.ID)

					// 1. derive Extrinsics of Block
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

				for _, e := range events.Multisig_MultisigApproval {
					fmt.Printf("\tSystem:detect new multisign approval:: (phase=%#v)\n", e.Phase)
				}
				for _, e := range events.Multisig_MultisigExecuted {
					fmt.Printf("\tSystem:detect new multisign Executed:: (phase=%#v)\n", e.Phase)
					fmt.Printf("\t\tFrom:%v,To: %v\n", e.Who, e.ID)

					// 1. derive Extrinsics of Block
					resp, err := l.client.GetBlockByNumber(int64(currentBlock))
					if err != nil {
						panic(err)
					}

					for _, extrinsic := range resp.Extrinsic {
						if extrinsic.Type == "as_multi" {

						}
					}

				}
				for _, e := range events.Multisig_MultisigCancelled {
					fmt.Printf("\tSystem:detect new multisign request:: (phase=%#v)\n", e.Phase)
				}

				//Triggered a cross-chain transaction
				if events.Utility_BatchCompleted != nil {
					for bi, e := range events.Balances_Transfer {
						// If BatchCompleted and multiAccount Balance Changed => MultiSignTransfer
						if e.To == multiSignAccount {
							fmt.Printf("----------------------------------------Succeed catch a tx to mulsigAddress----------------------------------------\n")
							// 1. derive Extrinsic of Block
							resp, err := l.client.GetBlockByNumber(int64(currentBlock))
							if err != nil {
								panic(err)
							}

							// 2. validate and get essential parameters of message
							var fromChainId = msg.ChainId(chainSub)
							var toChainId = msg.ChainId(chainAlaya)
							var rId = msg.ResourceIdFromSlice(common.FromHex(AKSM))
							var recipient []byte
							var amount int64
							var depositNoceA, depositNoceB string

							//derive information from block
							for _, extrinsic := range resp.Extrinsic {
								//Only System.batch(transfer + remark) -> extrinsic can be parsed
								if extrinsic.Type == "multiSignBatch" {
									amount, err = strconv.ParseInt(extrinsic.Amount, 10, 64)
									recipient = []byte(extrinsic.Recipient)
									depositNoceA = strconv.FormatInt(int64(currentBlock), 10)
									depositNoceB = strconv.FormatInt(int64(extrinsic.ExtrinsicIndex), 10)
								} else {
									continue
								}
							}
							fmt.Printf("----------------------> Try to solve the No.%d MultiSignTransfer in currentBlock\n", bi)
							fmt.Printf("ready to send %d PDOT to %s\n", amount, recipient)

							//recipient := types.NewAccountID(common.FromHex("0xff93B45308FD417dF303D6515aB04D9e89a750Ca"))
							// 3. construct parameters of message
							//TODO:how to storage depositNonce
							//l.depositNonce[recipient]++
							//TODO: update msg.Nonce
							deposit := depositNoceA + depositNoceB
							depositNonce, _ := strconv.ParseInt(deposit, 10, 64)

							m := msg.NewFungibleTransfer(
								fromChainId, // Unset
								toChainId,
								msg.Nonce(depositNonce),
								big.NewInt(amount),
								rId,
								recipient,
							)
							l.submitMessage(m, err)
							if err != nil {
								fmt.Printf("submit Message to Alaya meet a err: %v\n", err)
							}
							fmt.Printf("<---------------------- finish the No.%d MultiSignTransfer in currentBlock\n", bi)
						}
					}
				}
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

			//header, err := l.conn.api.RPC.Chain.SubscribeNewHeads()
			//fmt.Printf("header:\n%v\n", header)
		}
	}
}

//func (l *listener) pollBlock() error {
//	var currentBlock = l.startBlock
//	var retry = BlockRetryLimit
//	for {
//		select {
//		case <-l.stop:
//			return errors.New("terminated")
//		default:
//			// No more retries, goto next block
//			if retry == 0 {
//				l.sysErr <- fmt.Errorf("event polling retries exceeded (chain=%d, name=%s)", l.chainId, l.name)
//				return nil
//			}
//
//			// Get finalized block hash
//			finalizedHash, err := api.RPC.Chain.GetFinalizedHead()
//			if err != nil {
//				l.log.Error("Failed to fetch finalized hash", "err", err)
//				retry--
//				time.Sleep(BlockRetryInterval)
//				continue
//			}
//
//			// Get finalized block header
//			finalizedHeader, err := api.RPC.Chain.GetHeader(finalizedHash)
//			if err != nil {
//				l.log.Error("Failed to fetch finalized header", "err", err)
//				retry--
//				time.Sleep(BlockRetryInterval)
//				continue
//			}
//
//			if l.metrics != nil {
//				l.metrics.LatestKnownBlock.Set(float64(finalizedHeader.Number))
//			}
//
//			// Sleep if the block we want comes after the most recently finalized block
//			if currentBlock > uint64(finalizedHeader.Number) {
//				l.log.Trace("Block not yet finalized", "target", currentBlock, "latest", finalizedHeader.Number)
//				time.Sleep(BlockRetryInterval)
//				continue
//			}
//
//			// Get hash for latest block, sleep and retry if not ready
//			hash, err := api.RPC.Chain.GetBlockHash(currentBlock)
//			if err != nil && err.Error() == ErrBlockNotReady.Error() {
//				time.Sleep(BlockRetryInterval)
//				continue
//			} else if err != nil {
//				l.log.Error("Failed to query latest block", "block", currentBlock, "err", err)
//				retry--
//				time.Sleep(BlockRetryInterval)
//				continue
//			}
//
//			err = l.processEvents(hash)
//			if err != nil {
//				l.log.Error("Failed to process events in block", "block", currentBlock, "err", err)
//				retry--
//				continue
//			}
//
//			// Write to blockStore
//			err = l.blockStore.StoreBlock(big.NewInt(0).SetUint64(currentBlock))
//			if err != nil {
//				l.log.Error("Failed to write to blockStore", "err", err)
//			}
//
//			if l.metrics != nil {
//				l.metrics.BlocksProcessed.Inc()
//				l.metrics.LatestProcessedBlock.Set(float64(currentBlock))
//			}
//
//			currentBlock++
//			l.latestBlock.Height = big.NewInt(0).SetUint64(currentBlock)
//			l.latestBlock.LastUpdated = time.Now()
//			retry = BlockRetryLimit
//		}
//	}
//}

//func (l *listener) processEvent(hash types.Hash) error {
//	l.log.Trace("Fetching block for events", "hash", hash.Hex())
//	meta := l.conn.getMetadata()
//	// Subscribe to system events via storage
//	key, err := types.CreateStorageKey(&meta, "System", "Events", nil, nil)
//	if err != nil {
//		return err
//	}
//
//	//var records types.EventRecordsRaw
//	//_, err = l.conn.api.RPC.State.GetStorage(key, &records, hash)
//	//if err != nil {
//	//	return err
//	//}
//
//	//f, err := os.OpenFile("watchfile.log", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
//	//if err != nil {
//	//	log.Fatal(err)
//	//}
//	////完成后，延迟关闭
//	////defer f.Close()
//	//// 设置日志输出到文件
//	//log.SetOutput(f)
//
//	var multiSignPk, errs = types.HexDecodeString("0x96255ecf5f66b58074da258ad20e6d74fedc900798687ff86547efe30ec2e7c6")
//	if errs != nil {
//		panic(err)
//	}
//	var multiSignAccount = types.NewAccountID(multiSignPk)
//
//	sub, err := l.conn.api.RPC.State.SubscribeStorageRaw([]types.StorageKey{key})
//	if err != nil {
//		panic(err)
//	}
//	defer sub.Unsubscribe()
//
//	set := <-sub.Chan()
//	//log.Println("\t~_~sadsadsadad---------")
//
//	for _, chng := range set.Changes {
//		if !types.Eq(chng.StorageKey, key) || !chng.HasStorageData {
//			// skip, we are only interested in events with countent
//			continue
//		}
//
//		// Decode the event records
//		events := types.EventRecords{}
//		err = types.EventRecordsRaw(chng.StorageData).DecodeEventRecords(&meta, &events)
//		if err != nil {
//			//panic(err)
//			fmt.Printf("\terr is %v\n", err)
//		}
//
//		// Show what we are busy with
//		for _, e := range events.Balances_Transfer {
//			fmt.Printf("\tBalances:Transfer:: (phase=%#v)\n", e.Phase)
//			fmt.Printf("\t\t%v, %v, %v\n", e.From, e.To, e.Value)
//			if e.To == multiSignAccount {
//				fmt.Printf("\t~_~成功捕捉到转账到多签地址的交易---------\n")
//
//				// 写入日志内容
//				//log.Println("\t~_~成功捕捉到转账到多签地址的交易---------")
//
//				//	var recipient, err = types.HexDecodeString("0xff93B45308FD417dF303D6515aB04D9e89a750Ca")
//				//	if err != nil {
//				//		panic(err)
//				//	}
//				//var recipientAccount = types.NewAccountID(recipient)
//				//
//
//				//msg.NewFungibleTransfer(
//				//	0, // Unset
//				//	msg.ChainId(evt.Destination),
//				//	msg.Nonce(evt.DepositNonce),
//				//	evt.Amount.Int,
//				//	resourceId,
//				//	evt.Recipient,
//				//	)
//
//			}
//		}
//		for _, e := range events.Balances_Deposit {
//			fmt.Printf("\tBalances:Deposit:: (phase=%#v)\n", e.Phase)
//			fmt.Printf("\t\t%v, %v\n", e.Who, e.Balance)
//		}
//		for _, e := range events.Grandpa_NewAuthorities {
//			fmt.Printf("\tGrandpa:NewAuthorities:: (phase=%#v)\n", e.Phase)
//			fmt.Printf("\t\t%v\n", e.NewAuthorities)
//		}
//		for _, e := range events.Grandpa_Paused {
//			fmt.Printf("\tGrandpa:Paused:: (phase=%#v)\n", e.Phase)
//		}
//		for _, e := range events.Grandpa_Resumed {
//			fmt.Printf("\tGrandpa:Resumed:: (phase=%#v)\n", e.Phase)
//		}
//		for _, e := range events.ImOnline_HeartbeatReceived {
//			fmt.Printf("\tImOnline:HeartbeatReceived:: (phase=%#v)\n", e.Phase)
//			fmt.Printf("\t\t%#x\n", e.AuthorityID)
//		}
//		for _, e := range events.Offences_Offence {
//			fmt.Printf("\tOffences:Offence:: (phase=%#v)\n", e.Phase)
//			fmt.Printf("\t\t%v%v\n", e.Kind, e.OpaqueTimeSlot)
//		}
//		for _, e := range events.Session_NewSession {
//			fmt.Printf("\tSession:NewSession:: (phase=%#v)\n", e.Phase)
//			fmt.Printf("\t\t%v\n", e.SessionIndex)
//		}
//		for _, e := range events.Staking_OldSlashingReportDiscarded {
//			fmt.Printf("\tStaking:OldSlashingReportDiscarded:: (phase=%#v)\n", e.Phase)
//			fmt.Printf("\t\t%v\n", e.SessionIndex)
//		}
//		for _, e := range events.Staking_Slash {
//			fmt.Printf("\tStaking:Slash:: (phase=%#v)\n", e.Phase)
//			fmt.Printf("\t\t%#x%v\n", e.AccountID, e.Balance)
//		}
//		for _, e := range events.System_ExtrinsicSuccess {
//			fmt.Printf("\tSystem:ExtrinsicSuccess:: (phase=%#v)\n", e.Phase)
//		}
//		for _, e := range events.System_ExtrinsicFailed {
//			fmt.Printf("\tSystem:ErtrinsicFailed:: (phase=%#v)\n", e.Phase)
//			fmt.Printf("\t\t%v\n", e.DispatchError)
//		}
//		for _, e := range events.Multisig_NewMultisig {
//			fmt.Printf("\tSystem:detect new multisign request:: (phase=%#v)\n", e.Phase)
//			fmt.Printf("\t\tFrom:%v,To: %v\n", e.Who, e.ID)
//		}
//		for _, e := range events.Multisig_MultisigApproval {
//			fmt.Printf("\tSystem:detect new multisign approval:: (phase=%#v)\n", e.Phase)
//		}
//		for _, e := range events.Multisig_MultisigExecuted {
//			fmt.Printf("\tSystem:detect new multisign Executed:: (phase=%#v)\n", e.Phase)
//			fmt.Printf("\t\tFrom:%v,To: %v\n", e.Who, e.ID)
//		}
//		for _, e := range events.Multisig_MultisigCancelled {
//			fmt.Printf("\tSystem:detect new multisign request:: (phase=%#v)\n", e.Phase)
//		}
//	}
//
//	//e := types.EventRecords{}
//	//err = records.DecodeEventRecords(&meta, &e)
//	l.log.Trace("Finished processing events", "block", hash.Hex())
//
//	return nil
//}
//
//// processEvents fetches a block and parses out the events, calling Listener.handleEvents()
//func (l *listener) processEvents(hash types.Hash) error {
//	l.log.Trace("Fetching block for events", "hash", hash.Hex())
//	meta := l.conn.getMetadata()
//	key, err := types.CreateStorageKey(&meta, "System", "Events", nil, nil)
//	if err != nil {
//		return err
//	}
//
//	var records types.EventRecordsRaw
//	_, err = l.conn.api.RPC.State.GetStorage(key, &records, hash)
//	if err != nil {
//		return err
//	}
//
//	e := utils.Events{}
//	err = records.DecodeEventRecords(&meta, &e)
//	if err != nil {
//		return err
//	}
//
//	l.handleEvents(e)
//	l.log.Trace("Finished processing events", "block", hash.Hex())
//
//	return nil
//}
//
//// handleEvents calls the associated handler for all registered event types
//func (l *listener) handleEvents(evts utils.Events) {
//	if l.subscriptions[FungibleTransfer] != nil {
//		for _, evt := range evts.ChainBridge_FungibleTransfer {
//			l.log.Trace("Handling FungibleTransfer event")
//			l.submitMessage(l.subscriptions[FungibleTransfer](evt, l.log))
//		}
//	}
//	if l.subscriptions[NonFungibleTransfer] != nil {
//		for _, evt := range evts.ChainBridge_NonFungibleTransfer {
//			l.log.Trace("Handling NonFungibleTransfer event")
//			l.submitMessage(l.subscriptions[NonFungibleTransfer](evt, l.log))
//		}
//	}
//	if l.subscriptions[GenericTransfer] != nil {
//		for _, evt := range evts.ChainBridge_GenericTransfer {
//			l.log.Trace("Handling GenericTransfer event")
//			l.submitMessage(l.subscriptions[GenericTransfer](evt, l.log))
//		}
//	}
//
//	if len(evts.System_CodeUpdated) > 0 {
//		l.log.Trace("Received CodeUpdated event")
//		err := l.conn.updateMetatdata()
//		if err != nil {
//			l.log.Error("Unable to update Metadata", "error", err)
//		}
//	}
//}

func getRemark(resp *models.BlockResponse, blockNumber int64) string {
	return ""
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
