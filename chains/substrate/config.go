// Copyright 2020 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package substrate

import (
	log "github.com/ChainSafe/log15"
	"github.com/centrifuge/go-substrate-rpc-client/v2/types"
	"strconv"

	"github.com/ChainSafe/chainbridge-utils/core"
)

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
	if totalRelayer, ok := cfg.Opts["TotalRelayer"]; ok {
		total, _ := strconv.ParseUint(totalRelayer, 10, 32)
		for i := uint64(1); i < total; i++ {
			relayedKey := "OtherRelayer" + string(strconv.Itoa(int(i)))
			if relayer, ok := cfg.Opts[relayedKey]; ok {
				address, _ := types.NewAddressFromHexAccountID(relayer)
				otherSignatories = append(otherSignatories, address.AsAccountID)
			} else {
				log.Warn("Please set config 'OtherRelayer' from 1 to ...!")
				log.Error("Polkadot OtherRelayer Not Found", "OtherRelayerNumber", i)
			}
		}
	} else {
		log.Error("Please set config opts 'TotalRelayer'.")
	}
	return otherSignatories
}

func parseMultiSignConfig(cfg *core.ChainConfig) (uint64, uint64, uint16) {
	total := uint64(3)
	current := uint64(1)
	threshold := uint64(2)
	if totalRelayer, ok := cfg.Opts["TotalRelayer"]; ok {
		total, _ = strconv.ParseUint(totalRelayer, 10, 32)
	}
	if currentRelayerNumber, ok := cfg.Opts["CurrentRelayerNumber"]; ok {
		current, _ = strconv.ParseUint(currentRelayerNumber, 10, 32)
		if current == 0 {
			log.Error("Please set config opts 'CurrentRelayerNumber' from 1 to ...!")
		}
	}
	if multiSignThreshold, ok := cfg.Opts["MultiSignThreshold"]; ok {
		threshold, _ = strconv.ParseUint(multiSignThreshold, 10, 32)
	}
	return total, current, uint16(threshold)
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
