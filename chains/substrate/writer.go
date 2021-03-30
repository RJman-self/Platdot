// Copyright 2021 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package substrate

import (
	"errors"
	"fmt"
	"github.com/ChainSafe/log15"
	gsrpc "github.com/centrifuge/go-substrate-rpc-client/v2"
	"github.com/centrifuge/go-substrate-rpc-client/v2/rpc/author"
	"github.com/centrifuge/go-substrate-rpc-client/v2/types"
	utils "github.com/rjman-self/Platdot/shared/substrate"
	"github.com/rjman-self/platdot-utils/core"
	metrics "github.com/rjman-self/platdot-utils/metrics/types"
	"github.com/rjman-self/platdot-utils/msg"
	"math/big"
	"time"
)

var _ core.Writer = &writer{}

var TerminatedError = errors.New("terminated")

const RoundInterval = time.Second * 6
const oneToken = 1000000
const Mod = 1

var NotExecuted = MultiSignTx{
	BlockNumber:   -1,
	MultiSignTxId: 0,
}


type writer struct {
	conn               *Connection
	listener           *listener
	log                log15.Logger
	sysErr             chan<- error
	metrics            *metrics.ChainMetrics
	extendCall         bool // Extend extrinsic calls to substrate with ResourceID.Used for backward compatibility with example pallet.
	msApi              *gsrpc.SubstrateAPI
	relayer			   Relayer
	maxWeight          uint64
}

func NewWriter(conn *Connection, listener *listener, log log15.Logger, sysErr chan<- error,
	m *metrics.ChainMetrics, extendCall bool, weight uint64, relayer Relayer) *writer {

	api, err := gsrpc.NewSubstrateAPI(conn.url)
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
		msApi:              api,
		relayer: 			relayer,
		maxWeight:          weight,
	}
}

func (w *writer) ResolveMessage(m msg.Message) bool {
	w.log.Info("Start a redeemTx...")
	retryTime := 5
	go func() {
		for {
			isFinished, currentTx := w.redeemTx(m)
			if isFinished {
				w.log.Info("finish a redeemTx", "DepositNonce", m.DepositNonce)
				if currentTx.BlockNumber != NotExecuted.BlockNumber && currentTx.MultiSignTxId != NotExecuted.MultiSignTxId {
					w.log.Info("MultiSig extrinsic executed!", "DepositNonce", m.DepositNonce, "Block", currentTx.BlockNumber)
					delete(w.listener.msTxAsMulti, currentTx)
				}
				break
			}
			retryTime--
			if retryTime == 0 {
				w.log.Error("Can't finish the redeemTx, check it", "depositNonce", m.DepositNonce)
				break
			}
		}
	}()
	return true
}

func (w *writer) redeemTx(m msg.Message) (bool, MultiSignTx) {
	meta, err := w.msApi.RPC.State.GetMetadataLatest()
	if err != nil {
		panic(err)
	}

	types.SetSerDeOptions(types.SerDeOptions{NoPalletIndices: true})

	// BEGIN: Create a call of transfer
	method := string(utils.BalancesTransferKeepAliveMethod)

	// Convert PDOT amount to DOT amount
	bigAmt := big.NewInt(0).SetBytes(m.Payload[0].([]byte))
	bigAmt.Div(bigAmt, big.NewInt(oneToken))
	// calculate fee
	fee := uint64(FixedFee + float64(bigAmt.Uint64()) * FeeRate)
	actualAmount := bigAmt.Uint64() - fee
	amount := types.NewUCompactFromUInt(actualAmount)

	//fmt.Printf("Amount is %v, Fee is %v, ActualAmount = %v\n", bigAmt.Uint64(), fee, amount)

	// Get recipient of Polkadot
	recipient, _ := types.NewMultiAddressFromHexAccountID(string(m.Payload[1].([]byte)))

	// Create a transfer_keep_alive call
	c, err := types.NewCall(
		meta,
		method,
		recipient,
		amount,
	)
	if err != nil {
		panic(err)
	}

	// BEGIN: Create a call of MultiSignTransfer
	mulMethod := string(utils.MultisigAsMulti)
	var threshold = w.relayer.multiSignThreshold

	// Get parameters of multiSignature
	destAddress := string(m.Payload[1].([]byte))

	defer func() {
		/// Single thread send one time each round
		time.Sleep(RoundInterval)
	}()

	for {
		round := w.getRound()
		if round.Uint64() == (w.relayer.currentRelayer*Mod - 1) {
			//fmt.Printf("Round #%d , relayer to send a MultiSignTx, depositNonce #%d\n", round.Uint64(), m.DepositNonce)
			// Try to find a exist MultiSignTx
			var maybeTimePoint interface{}
			maxWeight := types.Weight(0)

			// Traverse all of matched Tx, included New、Approve、Executed
			for _, ms := range w.listener.msTxAsMulti {
				// Validate parameter
				if ms.DestAddress == destAddress[2:] && ms.DestAmount == big.NewInt(int64(actualAmount)).String() {
					/// Once MultiSign Extrinsic is executed, stop sending Extrinsic to Polkadot
					finished, executed := w.isFinish(ms)
					if finished {
						return finished, executed
					}

					/// Match the correct TimePoint
					height := types.U32(ms.OriginMsTx.BlockNumber)
					maybeTimePoint = TimePointSafe32{
						Height: types.NewOptionU32(height),
						Index:  types.U32(ms.OriginMsTx.MultiSignTxId),
					}
					maxWeight = types.Weight(w.maxWeight)
					break
				} else {
					maybeTimePoint = []byte{}
				}
			}

			if len(w.listener.msTxAsMulti) == 0 {
				maybeTimePoint = []byte{}
			}

			if maxWeight == 0 {
				w.log.Info("Try to make a New MultiSign Tx!", "depositNonce", m.DepositNonce)
			} else {
				_, height := maybeTimePoint.(TimePointSafe32).Height.Unwrap()
				w.log.Info("Try to Approve a MultiSignTx!", "Block", height, "Index", maybeTimePoint.(TimePointSafe32).Index, "depositNonce", m.DepositNonce)
			}

			mc, err := types.NewCall(meta, mulMethod, threshold, w.relayer.otherSignatories, maybeTimePoint, EncodeCall(c), false, maxWeight)
			if err != nil {
				panic(err)
			}
			///END: Create a call of MultiSignTransfer

			///BEGIN: Submit a MultiSignExtrinsic to Polkadot
			w.submitTx(mc)
			return false, NotExecuted
			///END: Submit a MultiSignExtrinsic to Polkadot
		} else {
			///Round over, wait a RoundInterval
			time.Sleep(RoundInterval)
		}
	}
}

func (w *writer) submitTx(c types.Call) {
	// BEGIN: Get the essential information first
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

	key, err := types.CreateStorageKey(meta, "System", "Account", w.relayer.kr.PublicKey, nil)
	if err != nil {
		panic(err)
	}
	// END: Get the essential information

	// Validate account and get account information
	var accountInfo types.AccountInfo
	ok, err := w.msApi.RPC.State.GetStorageLatest(key, &accountInfo)
	if err != nil || !ok {
		panic(err)
	}

	// Extrinsic nonce
	nonce := uint32(accountInfo.Nonce)

	// Construct signature option
	o := types.SignatureOptions{
		BlockHash:          genesisHash,
		Era:                types.ExtrinsicEra{IsMortalEra: false},
		GenesisHash:        genesisHash,
		Nonce:              types.NewUCompactFromUInt(uint64(nonce)),
		SpecVersion:        rv.SpecVersion,
		Tip:                types.NewUCompactFromUInt(0),
		TransactionVersion: rv.TransactionVersion,
	}

	// Create and Sign the MultiSign
	ext := types.NewExtrinsic(c)
	err = ext.MultiSign(w.relayer.kr, o)
	if err != nil {
		panic(err)
	}

	// Do the transfer and track the actual status
	_, _ = w.msApi.RPC.Author.SubmitAndWatchExtrinsic(ext)
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
	round.Mod(height, big.NewInt(int64(w.relayer.totalRelayers*Mod))).Uint64()
	return round
}

func (w *writer) isFinish(ms MultiSigAsMulti) (bool, MultiSignTx) {
	/// check isExecuted
	if ms.Executed {
		return true, ms.OriginMsTx
	}

	/// check isVoted
	for _, others := range ms.Others {
		var isVote = true
		for _, signatory := range others {
			voter, _ := types.NewAddressFromHexAccountID(signatory)
			relayer := types.NewAddressFromAccountID(w.relayer.kr.PublicKey)
			if voter == relayer {
				isVote = false
			}
		}
		if isVote {
			w.log.Info("relayer has vote, exit!", "Block", ms.OriginMsTx.BlockNumber, "Index", ms.OriginMsTx.MultiSignTxId)
			return true, NotExecuted
		}
	}

	/// check isApproved
	//for _, signatory := range ms.YesVote {
	//	relayer := types.NewAddressFromAccountID(w.relayer.kr.PublicKey).AsAccountID
	//	if signatory == relayer {
	//		isVote = true
	//		fmt.Printf("writer check relayer is Approved(vote)\n")
	//	}
	//}

	// For each Tx of New、Approve、Executed，relayer vote for one time

	return false, NotExecuted
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
