// Copyright 2020 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package substrate

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/centrifuge/go-substrate-rpc-client/v2/signature"
	"math/big"
	"time"
	"unsafe"

	"github.com/ChainSafe/chainbridge-utils/core"

	utils "github.com/ChainSafe/ChainBridge/shared/substrate"
	metrics "github.com/ChainSafe/chainbridge-utils/metrics/types"
	"github.com/ChainSafe/chainbridge-utils/msg"
	"github.com/ChainSafe/log15"

	"github.com/centrifuge/go-substrate-rpc-client/v2/types"
)

var _ core.Writer = &writer{}

var AcknowledgeProposal utils.Method = utils.BridgePalletName + ".acknowledge_proposal"
var TerminatedError = errors.New("terminated")

type writer struct {
	conn       *Connection
	log        log15.Logger
	sysErr     chan<- error
	metrics    *metrics.ChainMetrics
	extendCall bool // Extend extrinsic calls to substrate with ResourceID.Used for backward compatibility with example pallet.
}

func NewWriter(conn *Connection, log log15.Logger, sysErr chan<- error, m *metrics.ChainMetrics, extendCall bool) *writer {
	return &writer{
		conn:       conn,
		log:        log,
		sysErr:     sysErr,
		metrics:    m,
		extendCall: extendCall,
	}
}

func (w *writer) ResolveMessage(m msg.Message) bool {
	fmt.Printf("--------------------------Writer try to make a simpleTransfer------------------------------------------\n")
	w.redeemTx(m)
	fmt.Printf("--------------------------Writer make a simpleTransfer over------------------------------------------\n")
	return true
}

func (w *writer) redeemTx(m msg.Message) bool {
	// This sample shows how to create a transaction to make a transfer from one an account to another.

	// Instantiate the API
	//api, err := gsrpc.NewSubstrateAPI(config.Default().RPCURL)
	//if err != nil {
	//	panic(err)
	//}

	//meta, err := api.RPC.State.GetMetadataLatest()

	meta, err := w.conn.api.RPC.State.GetMetadataLatest()
	if err != nil {
		panic(err)
	}

	//serialize signature data
	types.SetSerDeOptions(types.SerDeOptions{NoPalletIndices: true})

	////parameters with call
	//bigAmt := big.NewInt(0).SetBytes(m.Payload[0].([]byte))
	//amount := types.NewU128(*bigAmt)
	//recipient := types.NewAccountID(m.Payload[1].([]byte))
	////depositNonce := types.U64(m.DepositNonce)
	//
	//method := "Balances.transfer"
	//
	//if err != nil {
	//	panic(err)
	//}
	//
	//c, err := types.NewCall(
	//	meta,
	//	method,
	//	recipient,
	//	amount,
	//)

	// Create a call, transferring 123 units to fred
	//recipient, err := types.NewAddressFromHexAccountID("0x1cbd2d43530a44705ad088af313e18f80b53ef16b36177cd4b77b846f2a5f07c")
	//recipient := types.NewAccountID(m.Payload[1].([]byte))
	method := "Balances.transfer"
	//rcp := binary.BigEndian.Uint64(m.Payload[1].([]byte))
	//recipient, err := types.NewAddressFromHexAccountID(strconv.FormatUint(rcp, 10))
	//recipient, err := types.NewAddressFromAccountID(m.Payload[1].([]byte))
	//recipient := types.NewAccountID(m.Payload[1].([]byte))

	//var recipient []byte
	//copy(recipient, m.Payload[0].([]byte))

	//var rcp_str = string(rcp[:])
	//fmt.Printf("rcp_str is %v\n", rcp)

	//rcp := types.NewAccountID(m.Payload[1].([]byte))
	//var rcpByte []byte
	//copy(rcpByte, rcp[:])

	recipient := types.NewAddressFromAccountID(m.Payload[1].([]byte))

	bigAmt := big.NewInt(0).SetBytes(m.Payload[0].([]byte))

	oneEth := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	bigAmt.Div(bigAmt, oneEth)

	//amount := types.NewUCompactFromUInt()
	amount := types.NewUCompact(bigAmt)

	//bigAmt := big.NewInt(0).SetBytes(m.Payload[0].([]byte))
	//amount := types.NewUCompact(*bigAmt)
	//
	//amt := binary.BigEndian.Uint64(m.Payload[0].([]byte))
	//var amount = types.NewUCompactFromUInt(amt)

	//amt := binary.BigEndian.(m.Payload[0].([]byte))

	//u8 := m.Payload[0].([]byte)
	//amt := ByteArrayToInt(u8)
	//amtStr := strconv.FormatInt(amt, 10)

	//u64LE := binary.LittleEndian.Uint64(u8)
	//u64BE := binary.BigEndian.Uint64(u8)
	//fmt.Println("little-endian:",u8, "to", u64LE)
	//fmt.Println("big-endian:   ", u8, "to", u64BE)

	//var amount = types.NewUCompactFromUInt(uint64(amt))

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
