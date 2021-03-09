	// Copyright 2020 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package substrate

import (
	"strconv"
	"time"

	"github.com/ChainSafe/chainbridge-utils/core"
)

var MultiSignThreshold = 2
var RelayerName = "Sss"
var RelayerSeedOrSecret = "0x3c0c4fc26010d0512cd36a0f467375b3dbe2f207bbfda0c551b5e41ee495e909"
var RelayerPublicKey = "0x923eeef27b93315c97e63e0c1284b7433ffbc413a58da0626a63955a48586075"
var RelayerAddress = "5FNTYUQwxjrVE5zRRH1hKh6fZ72AosHB7ThVnNnq9Bv9BFjm"
var RelayerRoundTotal = int64(3)
var RelayerRound = map[string]uint64{"Sss": 0, "Hhh": 2, "Alice": 1}
var RelayerRoundInterval = time.Second * 2
var MaxWeight = 2269800000
var OtherRelayerA = "0x0a19674301c56a1721feb98dbe93cfab911a8c1bed127f598ef93b374bcc6e71"
var OtherRelayerB = "0xd43593c715fdd31c61141abd04a99fd6822c8558854ccde39a5684e7a56da27d"

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
