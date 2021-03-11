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
	"strconv"
	"time"
)

var _ core.Writer = &writer{}

var TerminatedError = errors.New("terminated")

var RoundInterval = time.Second * 1

const oneToken = 1000000

const Mod = 2

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
	multisignThreshold uint16
	maxweight          uint64
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
		multisignThreshold: threshold,
		maxweight:          weight,
	}
}

func (w *writer) ResolveMessage(m msg.Message) bool {
	fmt.Printf("--------------------------Writer try to make a MultiSignTransfer------------------------------------------\n")
	fmt.Printf("msg.DepositNonce is %v\n", m.DepositNonce)
	go func() {
		var RetryLimit = 5
		for i := 0; i < RetryLimit; i++ {
			if w.redeemTx(m) {
				break
			}
			time.Sleep(time.Second * 1)
		}
		fmt.Printf("--------------------------Writer succeed made a MultiSignTransfer------------------------------------------\n")
	}()
	return true
}

func (w *writer) redeemTx(m msg.Message) bool {
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
	fmt.Printf("call amount = %v\n", amount)

	/// Get recipient of Polkadot
	recipient, _ := types.NewMultiAddressFromHexAccountID(string(m.Payload[1].([]byte)))

	/// Record the depositNonce
	depositTarget := DepositTarget{
		DestAddress: string(m.Payload[1].([]byte)),
		DestAmount:  strconv.FormatInt(int64(bigAmt.Uint64()), 10),
	}
	fmt.Printf("=========deposit Target is {destAddr: %s, destAmount: %s}\n", depositTarget.DestAddress, depositTarget.DestAmount)

	_, exist := w.listener.depositNonce[depositTarget]
	if !exist {
		w.log.Trace("This Tx has been created")
		depositNonce := DepositNonce{
			Nonce:  m.DepositNonce,
			Status: false,
		}
		fmt.Printf(":::::::::::::::::New deal emerges, deposit Nonce is {destNonce: %v, destStatus: %v}\n", depositNonce.Nonce, depositNonce.Status)
		w.listener.depositNonce[depositTarget] = depositNonce
	} else if w.listener.depositNonce[depositTarget].Nonce != m.DepositNonce {
		fmt.Printf("Inconsistent with the nonce in the message, doesn't need to processe\n")
		return true
	} else if w.listener.depositNonce[depositTarget].Status {
		fmt.Printf("The message has been solved, skip it\n")
		return true
	} else {
		fmt.Printf("Deposit exist which is %v\n", w.listener.depositNonce[depositTarget])
	}

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
	var threshold = w.multisignThreshold

	// parameters of multiSignature
	destAddress := string(m.Payload[1].([]byte))

	fmt.Printf("make a call, ready to send ms\n")

	for {
		round := w.getRound()
		time.Sleep(RoundInterval)
		if round.Uint64() == (w.currentRelayer*Mod - 1) {
			/// Try to find a exist MultiSignTx
			var maybeTimePoint interface{}
			maxWeight := types.Weight(w.maxweight)

			/// Match the correct TimePoint
			for _, ms := range w.listener.msTxAsMulti {
				/// Once MultiSign Extrinsic is executed, stop sending Extrinsic to Polkadot
				/// Validate parameter
				if ms.DestAddress == destAddress[2:] && ms.DestAmount == bigAmt.String() {
					if ms.Executed {
						fmt.Printf("depositNonce %v done(Executed)", m.DepositNonce)
						return true
					}

					var isExecuted = false

					for _, relayer := range w.otherSignatories {
						if relayer == types.NewAccountID(w.kr.PublicKey) {
							isExecuted = true
						}
					}

					if isExecuted {
						return true
					}
					height := types.U32(ms.OriginMsTx.BlockNumber)
					value := types.NewOptionU32(height)
					maybeTimePoint = TimePointSafe32{
						Height: value,
						Index:  types.U32(ms.OriginMsTx.MultiSignTxId),
					}
					fmt.Printf("find the match MultiSign Tx, get TimePoint %v\n", maybeTimePoint)
				} else {
					maybeTimePoint = []byte{}
				}
			}
			mc, err := types.NewCall(meta, mulMethod, threshold, w.otherSignatories, maybeTimePoint, EncodeCall(c), false, maxWeight)
			if err != nil {
				panic(err)
			}
			///END: Create a call of MultiSignTransfer

			///BEGIN: Submit a MultiSignExtrinsic to Polkadot
			w.submitTx(mc)
			///END: Submit a MultiSignExtrinsic to Polkadot
		}
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
	_, err = w.msApi.RPC.Author.SubmitAndWatchExtrinsic(ext)

	/// Watch the Result
	//err = w.watchSubmission(sub)
	if err != nil {
		fmt.Printf("subWriter meet err: %v\n", err)
	}
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
