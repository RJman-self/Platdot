// Copyright 2021 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package substrate

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/ChainSafe/chainbridge-utils/core"
	gsrpc "github.com/centrifuge/go-substrate-rpc-client/v2"
	"github.com/centrifuge/go-substrate-rpc-client/v2/config"
	"github.com/centrifuge/go-substrate-rpc-client/v2/rpc/author"
	"github.com/centrifuge/go-substrate-rpc-client/v2/scale"
	"github.com/centrifuge/go-substrate-rpc-client/v2/signature"
	"github.com/centrifuge/go-substrate-rpc-client/v2/types"
	"math/big"
	"time"

	metrics "github.com/ChainSafe/chainbridge-utils/metrics/types"
	"github.com/ChainSafe/chainbridge-utils/msg"
	"github.com/ChainSafe/log15"
	utils "github.com/rjman-self/Platdot/shared/substrate"
)

var _ core.Writer = &writer{}

var TerminatedError = errors.New("terminated")

type writer struct {
	conn             *Connection
	listener         *listener
	log              log15.Logger
	sysErr           chan<- error
	metrics          *metrics.ChainMetrics
	extendCall       bool // Extend extrinsic calls to substrate with ResourceID.Used for backward compatibility with example pallet.
	kr               signature.KeyringPair
	otherSignatories []types.AccountID
	msApi			 *gsrpc.SubstrateAPI
}

func NewWriter(conn *Connection, listener *listener, log log15.Logger, sysErr chan<- error, m *metrics.ChainMetrics, extendCall bool) *writer {

	OtherSignatureA, _ := types.NewAddressFromHexAccountID(OtherRelayerA)
	OtherSignatureB, _ := types.NewAddressFromHexAccountID(OtherRelayerB)

	api, err := gsrpc.NewSubstrateAPI(config.Default().RPCURL)
	if err != nil {
		panic(err)
	}

	return &writer{
		conn:       conn,
		listener:   listener,
		log:        log,
		sysErr:     sysErr,
		metrics:    m,
		extendCall: extendCall,
		kr: signature.KeyringPair{
			URI:       RelayerSeedOrSecret,
			Address:   RelayerAddress,
			PublicKey: types.MustHexDecodeString(RelayerPublicKey),
		},
		otherSignatories: []types.AccountID{
			OtherSignatureA.AsAccountID,
			OtherSignatureB.AsAccountID,
		},
		msApi:		api,
	}
}

func (w *writer) ResolveMessage(m msg.Message) bool {
	fmt.Printf("--------------------------Writer try to make a MultiSignTransfer------------------------------------------\n")
	var RetryLimit = 10
	for i := 0; i < RetryLimit; i++ {
		fmt.Printf("---", i ,"---\n")
		finish := w.redeemTx(m)
		if finish {
			fmt.Print(RelayerName, " succeed finish the MultiSignTransfer\n")
			break
		}
	}
	fmt.Printf("--------------------------Writer succeed made a MultiSignTransfer------------------------------------------\n")
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
	oneToken := new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
	bigAmt.Div(bigAmt, oneToken)
	amount := types.NewUCompactFromUInt(bigAmt.Uint64())
	fmt.Printf("call amount = %v\n", amount)

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
	var threshold = uint16(MultiSignThreshold)

	// parameters of multiSignature
	destAddress := string(m.Payload[1].([]byte))

	fmt.Printf("make a call, ready to send ms")

	for {
		round := big.NewInt(0)
		round.Mod(w.listener.latestBlock.Height, big.NewInt(RelayerRoundTotal)).Uint64()
		fmt.Printf("block is %v, round is %v\n", w.listener.latestBlock.Height, round)

		switch round.Uint64() {
		case RelayerRound["Sss"]:
			fmt.Printf("This Round is %v, Sss to do everything\n", RelayerRound["Sss"])

			/// Try to find a exist MultiSignTx
			var maybeTimePoint interface{}
			maxWeight := types.Weight(MaxWeight)

			/// Match the correct TimePoint
			for _, ms := range w.listener.msTxAsMulti {
				/// Once MultiSign Extrinsic is executed, stop sending Extrinsic to Polkadot
				if ms.Executed {
					return true
				}

				/// Validate parameter
				if ms.DestAddress == destAddress[2:] && ms.DestAmount == bigAmt.String() {
					height := types.U32(ms.OriginMsTx.BlockNumber)
					value := types.NewOptionU32(height)
					maybeTimePoint = TimePointSafe32{
						Height: value,
						Index:  types.U32(ms.OriginMsTx.MultiSignTxId),
					}
					fmt.Printf("find the match MultiSign Tx, get TimePoint %v", maybeTimePoint)
					break
				} else {
					maybeTimePoint = []byte{}
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
			///END: Submit a MultiSignExtrinsic to Polkadot

			return false
		case RelayerRound["Hhh"]:
			time.Sleep(RelayerRoundInterval)
			fmt.Printf("This Round is %v, Hhh to do everything\n", RelayerRound["Hhh"])
		case RelayerRound["Alice"]:
			time.Sleep(RelayerRoundInterval)
			fmt.Printf("This Round is %v, Alice to do everything\n", RelayerRound["Alice"])
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
	sub, err := w.msApi.RPC.Author.SubmitAndWatchExtrinsic(ext)


	/// Watch the Result
	err = w.watchSubmission(sub)
	if err != nil {
		fmt.Printf("subWriter meet err: %v\n", err)
	}
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

func (w *writer) redeemTxByAlice(m msg.Message) bool {
	kr := signature.TestKeyringPairAlice
	krp := signature.TestKeyringPairAlice.PublicKey

	fmt.Printf("============= relayer =====================\n")
	fmt.Printf("Relayer keyring: %v\n", kr)
	fmt.Printf("Relayer keyring.PublicKey: %v\n", krp)
	fmt.Printf("=======================================\n")

	meta, err := w.msApi.RPC.State.GetMetadataLatest()
	if err != nil {
		panic(err)
	}
	types.SetSerDeOptions(types.SerDeOptions{NoPalletIndices: true})

	//BEGIN: Create a call of transfer
	method := "Balances.transfer_keep_alive"
	recipient := types.NewMultiAddressFromAccountID(m.Payload[1].([]byte))
	// convert PDOT amount to DOT amount
	bigAmt := big.NewInt(0).SetBytes(m.Payload[0].([]byte))
	oneToken := new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
	bigAmt.Div(bigAmt, oneToken)
	amount := types.NewUCompactFromUInt(bigAmt.Uint64())
	//create a transfer call
	c, err := types.NewCall(
		meta,
		method,
		recipient,
		amount,
	)
	if err != nil {
		panic(err)
	}

	var buffer = bytes.Buffer{}
	encoderGoRPC := scale.NewEncoder(&buffer)
	_ = encoderGoRPC.Encode(c)
	callHash := buffer.Bytes()

	fmt.Printf("====================================\n")
	fmt.Printf("c_hash = %v\n", callHash)
	fmt.Printf("====================================\n")

	//BEGIN: Create a call of MultiSignTransfer
	mulMethod := "Multisig.as_multi"

	var threshold = uint16(MultiSignThreshold)

	// parameters of multiSignature
	Alice, _ := types.NewAddressFromHexAccountID("0xd43593c715fdd31c61141abd04a99fd6822c8558854ccde39a5684e7a56da27d")
	Bob, _ := types.NewAddressFromHexAccountID("0x8eaf04151687736326c9fea17e25fc5287613693c912909cb226aa4794f26a48")
	if err != nil {
		panic(err)
	}

	var otherSignatories = []types.AccountID{Bob.AsAccountID, Alice.AsAccountID}

	var maybeTimePoint []byte
	var maxWeight = types.Weight(222521000)

	//END: Create a call of transfer

	mc, err := types.NewCall(
		meta,
		mulMethod,
		threshold,
		otherSignatories,
		maybeTimePoint,
		callHash,
		false,
		maxWeight,
	)

	fmt.Printf("%v\n", mc)

	//END: Create a call of MultiSignTransfer
	ext := types.NewExtrinsic(mc)
	//ext := types.Extrinsic{
	//	Version:   types.ExtrinsicVersion4,
	//	Signature: types.ExtrinsicSignatureV4{Signer: types.MultiAddress{}},
	//	Method:    mc,
	//}

	genesisHash, err := w.msApi.RPC.Chain.GetBlockHash(0)
	if err != nil {
		panic(err)
	}

	rv, err := w.msApi.RPC.State.GetRuntimeVersionLatest()
	if err != nil {
		panic(err)
	}

	key, err := types.CreateStorageKey(meta, "System", "Account", krp, nil)
	if err != nil {
		panic(err)
	}

	var accountInfo types.AccountInfo
	//ok, err := api.RPC.State.GetStorageLatest(key, &accountInfo)
	ok, err := w.msApi.RPC.State.GetStorageLatest(key, &accountInfo)
	if err != nil || !ok {
		panic(err)
	}

	nonce := uint32(accountInfo.Nonce)

	o := types.SignatureOptions{
		BlockHash:          genesisHash,
		Era:                types.ExtrinsicEra{IsMortalEra: false},
		GenesisHash:        genesisHash,
		Nonce:              types.NewUCompactFromUInt(uint64(nonce)),
		SpecVersion:        rv.SpecVersion,
		Tip:                types.NewUCompactFromUInt(0),
		TransactionVersion: rv.TransactionVersion,
	}

	err = ext.MultiSign(kr, o)
	if err != nil {
		panic(err)
	}

	// Do the transfer and track the actual status
	sub, err := w.msApi.RPC.Author.SubmitAndWatchExtrinsic(ext)
	if err != nil {
		panic(err)
	}

	defer sub.Unsubscribe()

	for {
		status := <-sub.Chan()
		//fmt.Printf("Transaction status: %#v\n", status)
		if status.IsFinalized {
			fmt.Printf("Completed at block hash: %#x\n", status.AsFinalized)
		}
	}
}

func (w *writer) simpleTx(m msg.Message) bool {
	meta, err := w.msApi.RPC.State.GetMetadataLatest()
	if err != nil {
		panic(err)
	}

	//serialize signature data
	types.SetSerDeOptions(types.SerDeOptions{NoPalletIndices: true})
	//depositNonce := types.U64(m.DepositNonce)

	method := "Balances.transfer"
	recipient := types.NewAddressFromAccountID(m.Payload[1].([]byte))
	// convert PDOT amount to DOT amount
	bigAmt := big.NewInt(0).SetBytes(m.Payload[0].([]byte))
	oneToken := new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
	bigAmt.Div(bigAmt, oneToken)
	amount := types.NewUCompact(bigAmt)

	//simple transfer
	c, err := types.NewCall(
		meta,
		method,
		recipient,
		amount,
	)
	if err != nil {
		panic(err)
	}

	// Create the extrinsic
	ext := types.NewExtrinsic(c)

	//genesisHash, err := api.RPC.Chain.GetBlockHash(0)
	genesisHash, err := w.msApi.RPC.Chain.GetBlockHash(0)
	if err != nil {
		panic(err)
	}

	//rv, err := api.RPC.State.GetRuntimeVersionLatest()
	rv, err := w.msApi.RPC.State.GetRuntimeVersionLatest()
	if err != nil {
		panic(err)
	}

	key, err := types.CreateStorageKey(meta, "System", "Account", signature.TestKeyringPairAlice.PublicKey, nil)
	if err != nil {
		panic(err)
	}

	var accountInfo types.AccountInfo
	//ok, err := api.RPC.State.GetStorageLatest(key, &accountInfo)
	ok, err := w.msApi.RPC.State.GetStorageLatest(key, &accountInfo)
	if err != nil || !ok {
		panic(err)
	}

	nonce := uint32(accountInfo.Nonce)

	o := types.SignatureOptions{
		BlockHash:          genesisHash,
		Era:                types.ExtrinsicEra{IsMortalEra: false},
		GenesisHash:        genesisHash,
		Nonce:              types.NewUCompactFromUInt(uint64(nonce)),
		SpecVersion:        rv.SpecVersion,
		Tip:                types.NewUCompactFromUInt(0),
		TransactionVersion: rv.TransactionVersion,
	}
	fmt.Printf("Sending %v from %#x to %#x with nonce %v", amount, signature.TestKeyringPairAlice.PublicKey, recipient.AsAccountID, nonce)

	// Sign the transaction using Alice's default account
	err = ext.Sign(signature.TestKeyringPairAlice, o)
	if err != nil {
		panic(err)
	}

	// Do the transfer and track the actual status
	//sub, err := api.RPC.Author.SubmitAndWatchExtrinsic(ext)
	sub, err := w.msApi.RPC.Author.SubmitAndWatchExtrinsic(ext)
	if err != nil {
		panic(err)
	}
	defer sub.Unsubscribe()

	for {
		status := <-sub.Chan()
		fmt.Printf("Transaction status: %#v\n", status)

		if status.IsFinalized {
			fmt.Printf("Completed at block hash: %#x\n", status.AsFinalized)
		}
	}
}
