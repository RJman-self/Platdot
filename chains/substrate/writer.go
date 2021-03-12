// Copyright 2021 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package substrate

import (
	"errors"
	"fmt"
	"github.com/ChainSafe/chainbridge-utils/core"
	metrics "github.com/ChainSafe/chainbridge-utils/metrics/types"
	"github.com/ChainSafe/chainbridge-utils/msg"
	"github.com/ChainSafe/log15"
	gsrpc "github.com/centrifuge/go-substrate-rpc-client/v2"
	"github.com/centrifuge/go-substrate-rpc-client/v2/config"
	"github.com/centrifuge/go-substrate-rpc-client/v2/rpc/author"
	"github.com/centrifuge/go-substrate-rpc-client/v2/signature"
	"github.com/centrifuge/go-substrate-rpc-client/v2/types"
	utils "github.com/rjman-self/Platdot/shared/substrate"
	"math/big"
	"time"
)

var _ core.Writer = &writer{}

var TerminatedError = errors.New("terminated")

var RoundInterval = time.Second * 2

/// Processing concurrent transactions, the value of nonce increased
//const concurrent = 41
//var NonceUsed = [concurrent + 1]bool{false}
//var totalTx = uint64(0)
//var nonceIncreased = uint64(0)

const oneToken = 1000000
const Mod = 1

type writer struct {
	conn               *Connection
	listener           *listener
	log                log15.Logger
	sysErr             chan<- error
	metrics            *metrics.ChainMetrics
	extendCall         bool // Extend extrinsic calls to substrate with ResourceID.Used for backward compatibility with example pallet.
	kr                 signature.KeyringPair
	otherSignatories   []types.AccountID
	msApi              *gsrpc.SubstrateAPI
	totalRelayers      uint64
	currentRelayer     uint64
	multiSignThreshold uint16
	maxWeight          uint64
}

func NewWriter(conn *Connection, listener *listener, log log15.Logger, sysErr chan<- error,
	m *metrics.ChainMetrics, extendCall bool, krp *signature.KeyringPair, otherRelayers []types.AccountID,
	total uint64, current uint64, threshold uint16, weight uint64) *writer {

	api, err := gsrpc.NewSubstrateAPI(config.Default().RPCURL)
	if err != nil {
		panic(err)
	}

	return &writer{
		conn:               conn,
		listener:           listener,
		log:                log,
		sysErr:             sysErr,
		metrics:            m,
		extendCall:         extendCall,
		kr:                 *krp,
		otherSignatories:   otherRelayers,
		msApi:              api,
		totalRelayers:      total,
		currentRelayer:     current,
		multiSignThreshold: threshold,
		maxWeight:          weight,
	}
}

//func (w *writer) initNonceUsed() {
//	for i, _ := range NonceUsed {
//		if i < concurrent/2 {
//			/// Mark as finished nonce
//			NonceUsed[i] = true
//		} else {
//			/// Nonce waiting to be processed
//			NonceUsed[i] = false
//		}
//	}
//}
//
//func (w *writer) getNonceUnused() int {
//	mid := concurrent / 2
//	for i, nc := range NonceUsed {
//		/// Unused and past nonce, increase negative value
//		if !nc {
//			if i <= mid {
//				return i - mid
//			} else {
//				return mid - i
//			}
//		}
//	}
//	return -1
//}

func (w *writer) updateNonceUsed() {
	//for i, nc := range NonceUsed {
	//
	//}
}

func (w *writer) ResolveMessage(m msg.Message) bool {
	w.log.Info("start a redeemTx")
	//var mutex sync.Mutex
	go func() {
		//var RetryLimit = 5
		//mutex.Lock()
		for {
			//time.Sleep(RoundInterval)
			fmt.Printf("msg.DepositNonce is %v\n", m.DepositNonce)
			isFinished, currentTx := w.redeemTx(m)
			if isFinished {
				w.log.Info("finish a redeemTx")
				if currentTx.BlockNumber != -1 && currentTx.MultiSignTxId != 0 {
					delete(w.listener.msTxAsMulti, currentTx)
				}
				break
			}
		}
		//mutex.Unlock()
	}()
	return true
}

func (w *writer) redeemTx(m msg.Message) (bool, MultiSignTx) {
	//var phrase = "outer spike flash urge bus text aim public drink pumpkin pretty loan"

	meta, err := w.msApi.RPC.State.GetMetadataLatest()
	if err != nil {
		panic(err)
	}

	types.SetSerDeOptions(types.SerDeOptions{NoPalletIndices: true})

	///BEGIN: Create a call of transfer
	method := string(utils.BalancesTransferKeepAliveMethod)

	// Convert Pdot amount to DOT amount
	bigAmt := big.NewInt(0).SetBytes(m.Payload[0].([]byte))
	bigAmt.Div(bigAmt, big.NewInt(oneToken))
	amount := types.NewUCompactFromUInt(bigAmt.Uint64())

	/// Get recipient of Polkadot
	recipient, _ := types.NewMultiAddressFromHexAccountID(string(m.Payload[1].([]byte)))

	/// Create a transfer_keep_alive call
	c, err := types.NewCall(
		meta,
		method,
		recipient,
		amount,
	)
	if err != nil {
		panic(err)
	}

	//BEGIN: Create a call of MultiSignTransfer
	mulMethod := string(utils.MultisigAsMulti)
	var threshold = w.multiSignThreshold

	// parameters of multiSignature
	destAddress := string(m.Payload[1].([]byte))

	for {
		round := w.getRound()
		if round.Uint64() == (w.currentRelayer*Mod - 1) {
			fmt.Printf("Round #%d , relayer to send a MultiSignTx, depositNonce #%d\n", round.Uint64(), m.DepositNonce)
			/// Try to find a exist MultiSignTx
			var maybeTimePoint interface{}
			maxWeight := types.Weight(0)

			/// Traverse all of matched Tx, included New、Approve、Executed
			for _, ms := range w.listener.msTxAsMulti {
				/// Once MultiSign Extrinsic is executed, stop sending Extrinsic to Polkadot
				/// Validate parameter
				var isVote = true
				if ms.DestAddress == destAddress[2:] && ms.DestAmount == bigAmt.String() {
					if ms.Executed {
						fmt.Printf("depositNonce %v done(Executed), block %d\n", m.DepositNonce, ms.OriginMsTx.BlockNumber)
						return true, ms.OriginMsTx
					}

					for _, signatory := range ms.OtherSignatories {
						voter, _ := types.NewAddressFromHexAccountID(signatory)
						relayer := types.NewAddressFromAccountID(w.kr.PublicKey)
						//fmt.Printf("voter = %v\nrelayer = %v\n", voter, relayer)
						if voter == relayer {
							isVote = false
						}
					}
					//For Each Tx of New、Approve、Executed，each relayer vote for one Tx
					if isVote {
						w.log.Info("relayer has vote, exit!")
						return true, MultiSignTx{
							BlockNumber:   -1,
							MultiSignTxId: 0,
						}
					}
					/// Match the correct TimePoint
					height := types.U32(ms.OriginMsTx.BlockNumber)
					value := types.NewOptionU32(height)
					maybeTimePoint = TimePointSafe32{
						Height: value,
						Index:  types.U32(ms.OriginMsTx.MultiSignTxId),
					}
					maxWeight = types.Weight(w.maxWeight)
					fmt.Printf("find the match MultiSign Tx, get TimePoint %v\n", maybeTimePoint)
					break
				} else {
					//fmt.Printf("Tx %d found, but not current Tx\n", ms.OriginMsTx)
					maybeTimePoint = []byte{}
					continue
				}
			}
			if len(w.listener.msTxAsMulti) == 0 {
				maybeTimePoint = []byte{}
			}

			mc, err := types.NewCall(meta, mulMethod, threshold, w.otherSignatories, maybeTimePoint, EncodeCall(c), false, maxWeight)
			if err != nil {
				panic(err)
			}
			///END: Create a call of MultiSignTransfer

			///BEGIN: Submit a MultiSignExtrinsic to Polkadot
			w.submitTx(mc)

			return false, MultiSignTx{
				BlockNumber:   -1,
				MultiSignTxId: 0,
			}
			//fmt.Printf("sleep a round for %fs\n", RoundInterval.Seconds())
			//time.Sleep(RoundInterval)
			///END: Submit a MultiSignExtrinsic to Polkadot

			///Round over, wait a RoundInterval
		}
		time.Sleep(RoundInterval)
	}
}

func (w *writer) submitTx(c types.Call) {
	///BEGIN: Get the essential information first

	meta, err := w.msApi.RPC.State.GetMetadataLatest()
	if err != nil {
		panic(err)
	}

	genesisHash, err := w.msApi.RPC.Chain.GetBlockHash(0)
	if err != nil {
		panic(err)
	}

	rv, err := w.msApi.RPC.State.GetRuntimeVersionLatest()
	if err != nil {
		panic(err)
	}

	key, err := types.CreateStorageKey(meta, "System", "Account", w.kr.PublicKey, nil)
	if err != nil {
		panic(err)
	}
	///END: Get the essential information

	/// Validate account and get account information
	var accountInfo types.AccountInfo
	ok, err := w.msApi.RPC.State.GetStorageLatest(key, &accountInfo)
	if err != nil || !ok {
		panic(err)
	}

	/// Extrinsic nonce
	nonce := uint32(accountInfo.Nonce)
	//w.getNonceIncreased()

	/// Construct signature option
	o := types.SignatureOptions{
		BlockHash:          genesisHash,
		Era:                types.ExtrinsicEra{IsMortalEra: false},
		GenesisHash:        genesisHash,
		Nonce:              types.NewUCompactFromUInt(uint64(nonce)),
		SpecVersion:        rv.SpecVersion,
		Tip:                types.NewUCompactFromUInt(0),
		TransactionVersion: rv.TransactionVersion,
	}

	/// Create and Sign the MultiSign
	ext := types.NewExtrinsic(c)
	err = ext.MultiSign(w.kr, o)
	if err != nil {
		panic(err)
	}

	/// Do the transfer and track the actual status
	_, _ = w.msApi.RPC.Author.SubmitAndWatchExtrinsic(ext)

	/// Watch the Result
	//err = w.watchSubmission(sub)
	//fmt.Printf("succeed submitTx to Polkadot , meet err: %v\n", err)
}

func (w *writer) getRound() *big.Int {
	finalizedHash, err := w.listener.conn.api.RPC.Chain.GetFinalizedHead()
	if err != nil {
		w.listener.log.Error("Writer Failed to fetch finalized hash", "err", err)
	}

	// Get finalized block header
	finalizedHeader, err := w.listener.conn.api.RPC.Chain.GetHeader(finalizedHash)
	if err != nil {
		w.listener.log.Error("Failed to fetch finalized header", "err", err)
	}

	height := big.NewInt(int64(finalizedHeader.Number))
	round := big.NewInt(0)
	round.Mod(height, big.NewInt(int64(w.totalRelayers*Mod))).Uint64()
	//w.log.Info("block is ", height.Uint64(), ", round is ", round.Uint64())
	return round
}

func (w *writer) watchSubmission(sub *author.ExtrinsicStatusSubscription) error {
	for {
		select {
		case status := <-sub.Chan():
			switch {
			case status.IsInBlock:
				w.log.Info("Extrinsic included in block", status.AsInBlock.Hex())
				return nil
			case status.IsRetracted:
				fmt.Printf("extrinsic retracted: %s", status.AsRetracted.Hex())
			case status.IsDropped:
				fmt.Printf("extrinsic dropped from network\n")
			case status.IsInvalid:
				fmt.Printf("extrinsic invalid\n")
			}
		case err := <-sub.Err():
			w.log.Trace("Extrinsic subscription error\n", "err", err)
			return err
		}
	}
}

func (w *writer) recordDepositNonce(m msg.Message) {
	//// Record the depositNonce
	//depositTarget := DepositTarget{
	//	DestAddress: string(m.Payload[1].([]byte)),
	//	DestAmount:  strconv.FormatInt(int64(bigAmt.Uint64()), 10),
	//}
	//fmt.Printf("=========deposit Target is {destAddr: %s, destAmount: %s}\n", depositTarget.DestAddress, depositTarget.DestAmount)
	//
	//nonceIndex := w.listener.getDepositNonceIndex(depositTarget, m.DepositNonce)
	//fmt.Printf("nonceIndex is %v\n", nonceIndex)
	//if nonceIndex < 0 {
	//	depositNonce := DepositNonce{
	//		Nonce: m.DepositNonce,
	//		OriginMsTx: MultiSignTx{
	//			BlockNumber:   0,
	//			MultiSignTxId: 0,
	//		},
	//		Status: false,
	//	}
	//	fmt.Printf(":::::::::::::::::New deal emerges, deposit Nonce is {destNonce: %v, destStatus: %v}\n", depositNonce.Nonce, depositNonce.Status)
	//	w.listener.depositNonce[depositTarget] = append(w.listener.depositNonce[depositTarget], depositNonce)
	//} else if w.listener.depositNonce[depositTarget][nonceIndex].Nonce != m.DepositNonce {
	//	fmt.Printf("Inconsistent with the nonce in the message, doesn't need to processe\n")
	//	return true
	//} else if w.listener.depositNonce[depositTarget][nonceIndex].Status {
	//	fmt.Printf("The message has been solved, skip it\n")
	//	return true
	//} else {
	//	fmt.Printf("Deposit exist which is %v\n", w.listener.depositNonce[depositTarget][nonceIndex])
	//}
}
