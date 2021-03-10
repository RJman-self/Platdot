// Copyright 2020 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package substrate

import (
	log "github.com/ChainSafe/log15"
	"github.com/centrifuge/go-substrate-rpc-client/v2/types"
	"strconv"
	"time"

	"github.com/ChainSafe/chainbridge-utils/core"
)

var MultiSignThreshold = 2
var RelayerName = "Sss"
var RelayerRoundTotal = int64(3)
var RelayerRound = map[string]uint64{"Sss": 0, "Hhh": 2, "Alice": 1}
var RelayerRoundInterval = time.Second * 2
var MaxWeight = 2269800000

var url = "ws://127.0.0.1:9944"
var AKSM = "0x0000000000000000000000000000000000000000000000000000000000000000"
var chainSub = 1
var chainAlaya = 222

func parseStartBlock(cfg *core.ChainConfig) uint64 {
	if blk, ok := cfg.Opts["startBlock"]; ok {
		res, err := strconv.ParseUint(blk, 10, 32)
		if err != nil {
			panic(err)
		}
		return res
	}
	return 0
}

func parseUseExtended(cfg *core.ChainConfig) bool {
	if b, ok := cfg.Opts["useExtendedCall"]; ok {
		res, err := strconv.ParseBool(b)
		if err != nil {
			panic(err)
		}
		return res
	}
	return false
}

func parseOtherRelayer(cfg *core.ChainConfig) []types.AccountID {
	var otherSignatories []types.AccountID
	if relayer, ok := cfg.Opts["RelayerA"]; ok {
		address, _ := types.NewAddressFromHexAccountID(relayer)
		otherSignatories = append(otherSignatories, address.AsAccountID)
	} else {
		log.Error("Polkadot OtherRelayers Not Found")
	}
	if relayer, ok := cfg.Opts["RelayerB"]; ok {
		address, _ := types.NewAddressFromHexAccountID(relayer)
		otherSignatories = append(otherSignatories, address.AsAccountID)
	}
	if relayer, ok := cfg.Opts["RelayerC"]; ok {
		address, _ := types.NewAddressFromHexAccountID(relayer)
		otherSignatories = append(otherSignatories, address.AsAccountID)
	}
	if relayer, ok := cfg.Opts["RelayerD"]; ok {
		address, _ := types.NewAddressFromHexAccountID(relayer)
		otherSignatories = append(otherSignatories, address.AsAccountID)
	}
	if relayer, ok := cfg.Opts["RelayerE"]; ok {
		address, _ := types.NewAddressFromHexAccountID(relayer)
		otherSignatories = append(otherSignatories, address.AsAccountID)
	}
	return otherSignatories
}

func parseMultiSignAddress(cfg *core.ChainConfig) types.AccountID {
	if multisignAddress, ok := cfg.Opts["MultiSignAddress"]; ok {
		multiSignPk, _ := types.HexDecodeString(multisignAddress)
		multiSignAccount := types.NewAccountID(multiSignPk)
		return multiSignAccount
	} else {
		log.Error("Polkadot MultiAddress Not Found")
	}
	return types.AccountID{}
}
