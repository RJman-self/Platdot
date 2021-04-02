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
const ProcessCount = 1

var NotExecuted = MultiSignTx{
	BlockNumber:   -1,
	MultiSignTxId: 0,
}

type writer struct {
	meta       *types.Metadata
	conn       *Connection
	listener   *listener
	log        log15.Logger
	sysErr     chan<- error
	metrics    *metrics.ChainMetrics
	extendCall bool // Extend extrinsic calls to substrate with ResourceID.Used for backward compatibility with example pallet.
	msApi      *gsrpc.SubstrateAPI
	relayer    Relayer
	maxWeight  uint64
	messages   map[Dest]bool
}

func NewWriter(conn *Connection, listener *listener, log log15.Logger, sysErr chan<- error,
	m *metrics.ChainMetrics, extendCall bool, weight uint64, relayer Relayer) *writer {

	msApi, err := gsrpc.NewSubstrateAPI(conn.url)
	if err != nil {
		panic(err)
	}

	meta, err := msApi.RPC.State.GetMetadataLatest()
	if err != nil {
		fmt.Printf("GetMetadataLatest err\n")
		panic(err)
	}

	return &writer{
		meta:       meta,
		conn:       conn,
		listener:   listener,
		log:        log,
		sysErr:     sysErr,
		metrics:    m,
		extendCall: extendCall,
		msApi:      msApi,
		relayer:    relayer,
		maxWeight:  weight,
		messages:   make(map[Dest]bool, InitCapacity),
	}
}

func (w *writer) ResolveMessage(m msg.Message) bool {
	w.log.Info("Start a redeemTx...")

	destMessage := Dest{
		DestAddress: string(m.Payload[1].([]byte)),
		DestAmount:  string(m.Payload[0].([]byte)),
	}

	for {
		if w.messages[destMessage] {
			repeatTime := RoundInterval * time.Duration(w.relayer.totalRelayers)
			fmt.Printf("Meet a Repeat Transaction, DepositNonce is %v, wait for %v Round\n", m.DepositNonce, repeatTime)
			time.Sleep(repeatTime)
		} else {
			break
		}
	}

	/// Mark Processing
	w.messages[destMessage] = true
	go func() {
		for {
			isFinished, currentTx := w.redeemTx(m)
			if isFinished {
				w.log.Info("finish a redeemTx", "DepositNonce", m.DepositNonce)
				if currentTx.BlockNumber != NotExecuted.BlockNumber && currentTx.MultiSignTxId != NotExecuted.MultiSignTxId {
					w.log.Info("MultiSig extrinsic executed!", "DepositNonce", m.DepositNonce, "OriginBlock", currentTx.BlockNumber)
					delete(w.listener.msTxAsMulti, currentTx)
					dm := Dest{
						DestAddress: string(m.Payload[1].([]byte)),
						DestAmount:  string(m.Payload[0].([]byte)),
					}
					delete(w.messages, dm)
					fmt.Printf("msg.DepositNonce %v is finished\n", m.DepositNonce)
				}
				break
			}
		}
	}()
	return true
}

func (w *writer) redeemTx(m msg.Message) (bool, MultiSignTx) {
	w.UpdateMetadate()
	types.SetSerDeOptions(types.SerDeOptions{NoPalletIndices: true})

	// BEGIN: Create a call of transfer
	method := string(utils.BalancesTransferKeepAliveMethod)

	// Convert AKSM amount to KSM amount
	amount := big.NewInt(0).SetBytes(m.Payload[0].([]byte))
	receiveAmount := big.NewInt(0).Div(amount, big.NewInt(oneToken))

	// calculate fee and sendAmount
	fixedFee := big.NewInt(FixedFee)
	additionalFee := big.NewInt(0).Div(receiveAmount, big.NewInt(FeeRate))
	fee := big.NewInt(0).Add(fixedFee, additionalFee)
	actualAmount := big.NewInt(0).Sub(receiveAmount, fee)
	sendAmount := types.NewUCompact(actualAmount)
	fmt.Printf("AKSM to KSM, Amount is %v, Fee is %v, Actual_KSM_Amount = %v\n", receiveAmount, fee, actualAmount)

	// Get recipient of Polkadot
	recipient, _ := types.NewMultiAddressFromHexAccountID(string(m.Payload[1].([]byte)))

	// Create a transfer_keep_alive call
	c, err := types.NewCall(
		w.meta,
		method,
		recipient,
		sendAmount,
	)

	if err != nil {
		fmt.Printf("NewCall err\n")
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
		processRound := (w.relayer.currentRelayer + uint64(m.DepositNonce)) % w.relayer.totalRelayers
		round := w.getRound()
		if round.blockRound.Uint64() == processRound {
			fmt.Printf("current %v transactions remain", len(w.listener.msTxAsMulti))
			fmt.Printf("process the message in block #%v, round #%v, depositnonce is %v\n", round.blockHeight, processRound, m.DepositNonce)
			//fmt.Printf("Round #%d , relayer to send a MultiSignTx, depositNonce #%d\n", round.Uint64(), m.DepositNonce)
			// Try to find a exist MultiSignTx
			var maybeTimePoint interface{}
			maxWeight := types.Weight(0)

			// Traverse all of matched Tx, included New、Approve、Executed
			for _, ms := range w.listener.msTxAsMulti {
				// Validate parameter
				if ms.DestAddress == destAddress[2:] && ms.DestAmount == actualAmount.String() {
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

			mc, err := types.NewCall(w.meta, mulMethod, threshold, w.relayer.otherSignatories, maybeTimePoint, EncodeCall(c), false, maxWeight)
			if err != nil {
				fmt.Printf("New MultiCall err\n")
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
	w.UpdateMetadate()
	retryTimes := BlockRetryLimit
	for {
		// No more retries, stop submitting Tx
		if retryTimes == 0 {
			fmt.Printf("submit Tx failed, check it\n")
		}
		genesisHash, err := w.msApi.RPC.Chain.GetBlockHash(0)
		if err != nil {
			fmt.Printf("GetBlockHash err\n")
			retryTimes--
			continue
		}
		rv, err := w.msApi.RPC.State.GetRuntimeVersionLatest()
		if err != nil {
			fmt.Printf("GetRuntimeVersionLatest err\n")
			retryTimes--
			continue
		}

		key, err := types.CreateStorageKey(w.meta, "System", "Account", w.relayer.kr.PublicKey, nil)
		if err != nil {
			fmt.Printf("CreateStorageKey err\n")
			retryTimes--
			continue
		}
		// END: Get the essential information

		// Validate account and get account information
		var accountInfo types.AccountInfo
		ok, err := w.msApi.RPC.State.GetStorageLatest(key, &accountInfo)
		if err != nil || !ok {
			fmt.Printf("GetStorageLatest err\n")
			retryTimes--
			continue
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
			fmt.Printf("MultiTx Sign err\n")
			panic(err)
		}

		// Do the transfer and track the actual status
		_, _ = w.msApi.RPC.Author.SubmitAndWatchExtrinsic(ext)
		fmt.Printf("submit Tx, Relayer nonce is %v\n", nonce)
		break
	}
}

func (w *writer) getRound() Round {
	finalizedHash, err := w.listener.client.Api.RPC.Chain.GetFinalizedHead()
	if err != nil {
		w.listener.log.Error("Writer Failed to fetch finalized hash", "err", err)
	}

	// Get finalized block header
	finalizedHeader, err := w.listener.client.Api.RPC.Chain.GetHeader(finalizedHash)
	if err != nil {
		w.listener.log.Error("Failed to fetch finalized header", "err", err)
	}

	blockHeight := big.NewInt(int64(finalizedHeader.Number))
	blockRound := big.NewInt(0)
	blockRound.Mod(blockHeight, big.NewInt(int64(w.relayer.totalRelayers))).Uint64()

	round := Round{
		blockHeight: blockHeight,
		blockRound:  blockRound,
	}

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

func (w *writer) UpdateMetadate() {
	meta, _ := w.msApi.RPC.State.GetMetadataLatest()
	if meta != nil {
		w.meta = meta
	}
}
