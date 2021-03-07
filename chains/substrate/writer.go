// Copyright 2020 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package substrate

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/centrifuge/go-substrate-rpc-client/v2/rpc/author"
	"github.com/centrifuge/go-substrate-rpc-client/v2/scale"
	"github.com/centrifuge/go-substrate-rpc-client/v2/signature"
	"math/big"
	"time"
	"unsafe"

	"github.com/ChainSafe/chainbridge-utils/core"

	metrics "github.com/ChainSafe/chainbridge-utils/metrics/types"
	"github.com/ChainSafe/chainbridge-utils/msg"
	"github.com/ChainSafe/log15"
	utils "github.com/rjman-self/Platdot/shared/substrate"

	"github.com/centrifuge/go-substrate-rpc-client/v2/types"
)

var _ core.Writer = &writer{}

var AcknowledgeProposal utils.Method = utils.BridgePalletName + ".acknowledge_proposal"
var TerminatedError = errors.New("terminated")
var MultisignThreshold = 2
var RelayerSeedOrSecret = "0x3c0c4fc26010d0512cd36a0f467375b3dbe2f207bbfda0c551b5e41ee495e909"
var RelayerAddress = "5FNTYUQwxjrVE5zRRH1hKh6fZ72AosHB7ThVnNnq9Bv9BFjm"
var url = "ws://127.0.0.1:9944"

type writer struct {
	conn       *Connection
	listener   *listener
	log        log15.Logger
	sysErr     chan<- error
	metrics    *metrics.ChainMetrics
	extendCall bool // Extend extrinsic calls to substrate with ResourceID.Used for backward compatibility with example pallet.

	//name          string
	//chainId       msg.ChainId
	//startBlock    uint64
	//blockStore    blockStore.Blockstorer
	//stop          <-chan int
	//latestBlock   metrics.LatestBlock
	//client        client.Client
}

func NewWriter(conn *Connection, listener *listener, log log15.Logger, sysErr chan<- error, m *metrics.ChainMetrics, extendCall bool) *writer {
	return &writer{
		conn:       conn,
		listener:   listener,
		log:        log,
		sysErr:     sysErr,
		metrics:    m,
		extendCall: extendCall,
	}
}

func (w *writer) ResolveMessage(m msg.Message) bool {
	fmt.Printf("--------------------------Writer try to make a MultiSignTransfer------------------------------------------\n")
	err := w.redeemTx(m)
	if err != nil {
		fmt.Printf("reemTx failed! Error is %v\n", err)
	}
	fmt.Printf("--------------------------Writer succeed made a MultiSignTransfer------------------------------------------\n")
	return true
}

func (w *writer) redeemTx(m msg.Message) error {
	//var phrase = "outer spike flash urge bus text aim public drink pumpkin pretty loan"
	RelayerPublicKey := types.MustHexDecodeString("0x923eeef27b93315c97e63e0c1284b7433ffbc413a58da0626a63955a48586075")
	sss := signature.KeyringPair{
		URI:       RelayerSeedOrSecret,
		Address:   RelayerAddress,
		PublicKey: RelayerPublicKey,
	}
	fmt.Printf("============= relayer =====================\n")
	fmt.Printf("Relayer keyring: %v\n", sss)
	fmt.Printf("Relayer keyring.PublicKey: %v\n", RelayerPublicKey)
	fmt.Printf("=======================================\n")

	meta, err := w.conn.api.RPC.State.GetMetadataLatest()
	if err != nil {
		panic(err)
	}

	types.SetSerDeOptions(types.SerDeOptions{NoPalletIndices: true})

	//BEGIN: Create a call of transfer
	method := string(utils.BalancesTransferKeepAliveMethod)
	recipient := types.NewMultiAddressFromAccountID(m.Payload[1].([]byte))
	// convert PDOT amount to DOT amount
	bigAmt := big.NewInt(0).SetBytes(m.Payload[0].([]byte))
	oneToken := new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
	bigAmt.Div(bigAmt, oneToken)
	amount := types.NewUCompactFromUInt(bigAmt.Uint64())

	//create a transfer_keep_alive call
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
	mulMethod := string(utils.MultisigAsMulti)
	var threshold = uint16(MultisignThreshold)

	// parameters of multiSignature
	Alice, _ := types.NewAddressFromHexAccountID("0xd43593c715fdd31c61141abd04a99fd6822c8558854ccde39a5684e7a56da27d")
	Bob, _ := types.NewAddressFromHexAccountID("0x8eaf04151687736326c9fea17e25fc5287613693c912909cb226aa4794f26a48")

	var otherSignatories = []types.AccountID{Bob.AsAccountID, Alice.AsAccountID}

	destAddress := string(recipient.AsID[:])
	//find a exist MultiSignTxEvent
	var maybeTimePoint interface{}
	var maxWeight interface{}

	for k , ms := range w.listener.msTxAsMulti {
		fmt.Printf("k is %v\nv is %v\n", k, ms)
		if !ms.Executed && ms.DestAddress == destAddress && ms.DestAmount == bigAmt.String() {
			maybeTimePoint = ms.OriginMsTx.BlockNumber
			maxWeight = ms.MaxWeight
			break
		} else {
			maybeTimePoint = []byte{}
			maxWeight = types.Weight(0)
		}
	}

	if len(w.listener.msTxAsMulti) == 0 {
		maybeTimePoint = []byte{}
		maxWeight = types.Weight(0)
	}
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

	genesisHash, err := w.conn.api.RPC.Chain.GetBlockHash(0)
	if err != nil {
		panic(err)
	}

	rv, err := w.conn.api.RPC.State.GetRuntimeVersionLatest()
	if err != nil {
		panic(err)
	}

	key, err := types.CreateStorageKey(meta, "System", "Account", RelayerPublicKey, nil)
	if err != nil {
		panic(err)
	}

	var accountInfo types.AccountInfo
	//ok, err := api.RPC.State.GetStorageLatest(key, &accountInfo)
	ok, err := w.conn.api.RPC.State.GetStorageLatest(key, &accountInfo)
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

	err = ext.MultiSign(sss, o)
	if err != nil {
		panic(err)
	}

	// Do the transfer and track the actual status
	sub, err := w.conn.api.RPC.Author.SubmitAndWatchExtrinsic(ext)

	err = w.watchSubmission(sub)
	if err != nil {
		fmt.Printf("subWriter meet err: %v\n", err)
	}
	return err
}

func (w *writer) watchSubmission(sub *author.ExtrinsicStatusSubscription) error {
	for {
		select {
		case status := <-sub.Chan():
			switch {
			case status.IsInBlock:
				w.log.Trace("Extrinsic included in block", "block", status.AsInBlock.Hex())
				return nil
			case status.IsRetracted:
				fmt.Printf("extrinsic retracted: %s", status.AsRetracted.Hex())
			case status.IsDropped:
				fmt.Printf("extrinsic dropped from network")
			case status.IsInvalid:
				fmt.Printf("extrinsic invalid")
			}
		case err := <-sub.Err():
			w.log.Trace("Extrinsic subscription error", "err", err)
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

	meta, err := w.conn.api.RPC.State.GetMetadataLatest()
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

	var threshold = uint16(MultisignThreshold)

	// parameters of multiSignature
	Alice, _ := types.NewAddressFromHexAccountID("0xd43593c715fdd31c61141abd04a99fd6822c8558854ccde39a5684e7a56da27d")
	Bob, _ := types.NewAddressFromHexAccountID("0x8eaf04151687736326c9fea17e25fc5287613693c912909cb226aa4794f26a48")
	if err != nil {
		panic(err)
	}

	var otherSignatories = []types.AccountID{Bob.AsAccountID, Alice.AsAccountID}

	//type TimePointSafe32 struct {
	//	Height types.OptionU32
	//	Index  types.U32
	//}
	//
	//var value = types.NewOptionU32(15)

	//var maybeTimePoint = TimePointSafe32{
	//	Height: value,
	//	Index:  1,
	//}

	var maybeTimePoint = []byte{}
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

	genesisHash, err := w.conn.api.RPC.Chain.GetBlockHash(0)
	if err != nil {
		panic(err)
	}

	rv, err := w.conn.api.RPC.State.GetRuntimeVersionLatest()
	if err != nil {
		panic(err)
	}

	key, err := types.CreateStorageKey(meta, "System", "Account", krp, nil)
	if err != nil {
		panic(err)
	}

	var accountInfo types.AccountInfo
	//ok, err := api.RPC.State.GetStorageLatest(key, &accountInfo)
	ok, err := w.conn.api.RPC.State.GetStorageLatest(key, &accountInfo)
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
	sub, err := w.conn.api.RPC.Author.SubmitAndWatchExtrinsic(ext)
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

// 赎回：eth to sub (Alice to Fred)
func (w *writer) redeemSimpleTx(m msg.Message) bool {
	meta, err := w.conn.api.RPC.State.GetMetadataLatest()
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
	genesisHash, err := w.conn.api.RPC.Chain.GetBlockHash(0)
	if err != nil {
		panic(err)
	}

	//rv, err := api.RPC.State.GetRuntimeVersionLatest()
	rv, err := w.conn.api.RPC.State.GetRuntimeVersionLatest()
	if err != nil {
		panic(err)
	}

	key, err := types.CreateStorageKey(meta, "System", "Account", signature.TestKeyringPairAlice.PublicKey, nil)
	if err != nil {
		panic(err)
	}

	var accountInfo types.AccountInfo
	//ok, err := api.RPC.State.GetStorageLatest(key, &accountInfo)
	ok, err := w.conn.api.RPC.State.GetStorageLatest(key, &accountInfo)
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
	sub, err := w.conn.api.RPC.Author.SubmitAndWatchExtrinsic(ext)
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

func (w *writer) redeemMultiSignTx(m msg.Message) bool {

	//var multiSignPk, err = types.HexDecodeString("0x49daa32c7287890f38b7e1a8cd2961723d36d20baa0bf3b82e0c4bdda93b1c0a")
	//var multiSignAccount = types.NewAccountID(multiSignPk)

	nnnPk := types.MustHexDecodeString("0x3418f5e3f3e90db1e870bee7a2909d3ecb27623ed07b220aaf205f053c660c1e")
	//var nnnAccount = types.NewAccountID(nnnPk)

	meta, err := w.conn.api.RPC.State.GetMetadataLatest()
	if err != nil {
		panic(err)
	}

	//serialize signature data
	types.SetSerDeOptions(types.SerDeOptions{NoPalletIndices: true})

	//depositNonce := types.U64(m.DepositNonce)

	//BEGIN: Create a call of MultiSignTransfer
	//mulMethod := "Multisig.as_multi"
	//
	//var threshold = uint16(2)
	//// parameters of multiSignature
	//var Bob = types.NewAccountID([]byte("5FHneW46xGXgs5mUiveU4sbTyGBzmstUspZC92UhjJM694ty"))
	//var Charlie = types.NewAccountID([]byte("5FLSigC9HGRKVhB9FiEo4Y3koPsNmBmLJbpXg2mp1hXcS59Y"))

	//var otherSignatories = []types.AccountID{Bob, Charlie}
	//var maybeTimepoint = 0
	//var maxWeight = 0

	//BEGIN: Create a call of transfer
	method := "Balances.transfer_keep_alive"
	recipient := types.NewAddressFromAccountID(m.Payload[1].([]byte))
	bigAmt := big.NewInt(0).SetBytes(m.Payload[0].([]byte))
	oneToken := new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
	bigAmt.Div(bigAmt, oneToken)
	amount := types.NewUCompact(bigAmt)

	c, err := types.NewCall(
		meta,
		method,
		recipient,
		amount,
	)
	if err != nil {
		panic(err)
	}
	//END: Create a call of transfer

	//mc, err := types.NewCall(
	//	meta,
	//	mulMethod,
	//	threshold,
	//	otherSignatories,
	//	maybeTimepoint,
	//	c,
	//	false,
	//	maxWeight,
	//)

	//END: Create a call of MultiSignTransfer
	if err != nil {
		panic(err)
	}

	// Create the extrinsic
	ext := types.NewExtrinsic(c)

	//genesisHash, err := api.RPC.Chain.GetBlockHash(0)
	genesisHash, err := w.conn.api.RPC.Chain.GetBlockHash(0)
	if err != nil {
		panic(err)
	}

	//rv, err := api.RPC.State.GetRuntimeVersionLatest()
	rv, err := w.conn.api.RPC.State.GetRuntimeVersionLatest()
	if err != nil {
		panic(err)
	}
	//key, err := types.CreateStorageKey(meta, "System", "Account", signature.TestKeyringPairAlice.PublicKey, nil)
	key, err := types.CreateStorageKey(meta, "System", "Account", nnnPk, nil)
	if err != nil {
		panic(err)
	}

	var accountInfo types.AccountInfo
	ok, err := w.conn.api.RPC.State.GetStorageLatest(key, &accountInfo)
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

	fmt.Printf("===================================================================================")
	fmt.Printf("Multisign: Sending %v from %#x to %#x with nonce %v", amount, signature.TestKeyringPairAlice.PublicKey, recipient.AsAccountID, nonce)
	fmt.Printf("===================================================================================")

	// Sign the transaction using Alice's default account
	//nnn,err := signature.KeyringPairFromSecret(
	//	"0x294068dcf6f88d2ecad225c55560417cd93bbf78b551026495294575d6267fc7",
	//	1,
	//)
	var seed = "0x294068dcf6f88d2ecad225c55560417cd93bbf78b551026495294575d6267fc7"
	var addr = "5DF1m9a6vQwyoyrzj8JMwM1bVrwBRKXhnVP28eH95b7BhX7W"
	//var phrase = "outer spike flash urge bus text aim public drink pumpkin pretty loan"

	nnn := signature.KeyringPair{
		URI:       seed,
		Address:   addr,
		PublicKey: nnnPk,
	}
	err = ext.Sign(nnn, o)
	if err != nil {
		panic(err)
	}

	sub, err := w.conn.api.RPC.Author.SubmitAndWatchExtrinsic(ext)
	if err != nil {
		panic(err)
	}
	defer sub.Unsubscribe()

	for {
		status := <-sub.Chan()
		fmt.Printf("Transaction status: %#v\n", status)

		if status.IsFinalized {
			//w.conn.api.
			fmt.Printf("Completed at block hash: %#x\n", status.AsFinalized)
		}
	}
}

//func multiSignTransfer() {
//	// Instantiate the API
//	api, err := gsrpc.NewSubstrateAPI(config.Default().RPCURL)
//	if err != nil {
//		panic(err)
//	}
//
//	meta, err := api.RPC.State.GetMetadataLatest()
//	if err != nil {
//		panic(err)
//	}
//
//	//serialize signature data
//	types.SetSerDeOptions(types.SerDeOptions{NoPalletIndices: true})
//
//	// Create a call, transferring 12345 units to fred
//	fred, err := types.NewAddressFromHexAccountID("0x1cbd2d43530a44705ad088af313e18f80b53ef16b36177cd4b77b846f2a5f07c")
//	if err != nil {
//		panic(err)
//	}
//
//	var amount = types.NewUCompactFromUInt(333000000000000)
//
//	//c, err := types.NewCall(meta, "Balances.transfer", fred, amount)
//	//if err != nil {
//	//	panic(err)
//	//}
//
//
//	// parameters of multiSignature
//	//var threshold = 2
//	//var Bob = types.NewAccountID([]byte("5FHneW46xGXgs5mUiveU4sbTyGBzmstUspZC92UhjJM694ty"))
//	//var other_signatories = []types.AccountID{Bob,Bob}
//	//var maybe_timepoint = 0
//
//	//mc, err := types.NewCall(meta, "multisig.approveAsMulti", threshold, other_signatories, maybe_timepoint)
//
//	mc, err := types.NewCall(meta, "Balances.transfer", fred, amount)
//
//	// Create the extrinsic
//	ext := types.NewExtrinsic(mc)
//
//	genesisHash, err := api.RPC.Chain.GetBlockHash(0)
//	if err != nil {
//		panic(err)
//	}
//
//	rv, err := api.RPC.State.GetRuntimeVersionLatest()
//	if err != nil {
//		panic(err)
//	}
//
//	key, err := types.CreateStorageKey(meta, "System", "Account", signature.TestKeyringPairAlice.PublicKey, nil)
//	if err != nil {
//		panic(err)
//	}
//
//	var accountInfo types.AccountInfo
//	ok, err := api.RPC.State.GetStorageLatest(key, &accountInfo)
//	if err != nil || !ok {
//		panic(err)
//	}
//
//	nonce := uint32(accountInfo.Nonce)
//
//	o := types.SignatureOptions{
//		BlockHash:   genesisHash,
//		Era:         types.ExtrinsicEra{IsMortalEra: false},
//		GenesisHash: genesisHash,
//		Nonce:       types.NewUCompactFromUInt(uint64(nonce)),
//		SpecVersion: rv.SpecVersion,
//		Tip:         types.NewUCompactFromUInt(0),
//		TransactionVersion: rv.TransactionVersion,
//	}
//
//	fmt.Printf("Sending %v from %#x to %#x with nonce %v", amount, signature.TestKeyringPairAlice.PublicKey, fred.AsAccountID, nonce)
//
//	// Sign the transaction using Alice's default account
//	err = ext.Sign(signature.TestKeyringPairAlice, o)
//	if err != nil {
//		panic(err)
//	}
//
//	// Do the transfer and track the actual status
//	sub, err := api.RPC.Author.SubmitAndWatchExtrinsic(ext)
//
//	if err != nil {
//		panic(err)
//	}
//	defer sub.Unsubscribe()
//
//	for {
//		status := <-sub.Chan()
//		fmt.Printf("Transaction status: %#v\n", status)
//
//		if status.IsFinalized {
//			fmt.Printf("Completed at block hash: %#x\n", status.AsFinalized)
//			return
//		}
//	}
//}

func (w *writer) ResolveMessages(m msg.Message) bool {
	var prop *proposal
	var err error

	// Construct the proposal
	switch m.Type {
	case msg.FungibleTransfer:
		prop, err = w.createFungibleProposal(m)
	case msg.NonFungibleTransfer:
		prop, err = w.createNonFungibleProposal(m)
	case msg.GenericTransfer:
		prop, err = w.createGenericProposal(m)
	default:
		w.sysErr <- fmt.Errorf("unrecognized message type received (chain=%d, name=%s)", m.Destination, w.conn.name)
		return false
	}

	if err != nil {
		w.sysErr <- fmt.Errorf("failed to construct proposal (chain=%d, name=%s) Error: %w", m.Destination, w.conn.name, err)
		return false
	}

	for i := 0; i < BlockRetryLimit; i++ {
		// Ensure we only submit a vote if the proposal hasn't completed
		valid, reason, err := w.proposalValid(prop)
		if err != nil {
			w.log.Error("Failed to assert proposal state", "err", err)
			time.Sleep(BlockRetryInterval)
			continue
		}

		// If active submit call, otherrwise skip it. Retry on failue.
		if valid {
			w.log.Info("Acknowledging proposal on chain", "nonce", prop.depositNonce, "source", prop.sourceId, "resource", fmt.Sprintf("%x", prop.resourceId), "method", prop.method)

			err = w.conn.SubmitTx(AcknowledgeProposal, prop.depositNonce, prop.sourceId, prop.resourceId, prop.call)
			if err != nil && err.Error() == TerminatedError.Error() {
				return false
			} else if err != nil {
				w.log.Error("Failed to execute extrinsic", "err", err)
				time.Sleep(BlockRetryInterval)
				continue
			}
			if w.metrics != nil {
				w.metrics.VotesSubmitted.Inc()
			}
			return true
		} else {
			w.log.Info("Ignoring proposal", "reason", reason, "nonce", prop.depositNonce, "source", prop.sourceId, "resource", prop.resourceId)
			return true
		}
	}
	return true
}

func (w *writer) resolveResourceId(id [32]byte) (string, error) {
	var res []byte
	exists, err := w.conn.queryStorage(utils.BridgeStoragePrefix, "Resources", id[:], nil, &res)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("resource %x not found on chain", id)
	}
	return string(res), nil
}

// proposalValid asserts the state of a proposal. If the proposal is active and this relayer
// has not voted, it will return true. Otherwise, it will return false with a reason string.
func (w *writer) proposalValid(prop *proposal) (bool, string, error) {
	var voteRes voteState
	srcId, err := types.EncodeToBytes(prop.sourceId)
	if err != nil {
		return false, "", err
	}
	propBz, err := prop.encode()
	if err != nil {
		return false, "", err
	}
	exists, err := w.conn.queryStorage(utils.BridgeStoragePrefix, "Votes", srcId, propBz, &voteRes)
	if err != nil {
		return false, "", err
	}

	if !exists {
		return true, "", nil
	} else if voteRes.Status.IsActive {
		if containsVote(voteRes.VotesFor, types.NewAccountID(w.conn.key.PublicKey)) ||
			containsVote(voteRes.VotesAgainst, types.NewAccountID(w.conn.key.PublicKey)) {
			return false, "already voted", nil
		} else {
			return true, "", nil
		}
	} else {
		return false, "proposal complete", nil
	}
}

func containsVote(votes []types.AccountID, voter types.AccountID) bool {
	for _, v := range votes {
		if bytes.Equal(v[:], voter[:]) {
			return true
		}
	}
	return false
}

func ByteArrayToInt(arr []byte) int64 {
	val := int64(0)
	size := len(arr)
	for i := 0; i < size; i++ {
		*(*uint8)(unsafe.Pointer(uintptr(unsafe.Pointer(&val)) + uintptr(i))) = arr[i]
	}
	return val
}

func (w *writer) getParametersFromChain() {

}
