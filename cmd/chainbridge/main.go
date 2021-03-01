// Copyright 2020 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only
/*
Provides the command-line interface for the chainbridge application.

For configuration and CLI commands see the README: https://github.com/ChainSafe/ChainBridge.
*/
package main

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/ChainSafe/ChainBridge/chains/ethereum"
	gsrpc "github.com/centrifuge/go-substrate-rpc-client/v2"
	rpcConfig "github.com/centrifuge/go-substrate-rpc-client/v2/config"
	"github.com/centrifuge/go-substrate-rpc-client/v2/scale"
	"github.com/centrifuge/go-substrate-rpc-client/v2/signature"
	"github.com/centrifuge/go-substrate-rpc-client/v2/types"
	"net/http"
	"os"

	"strconv"

	"github.com/ChainSafe/ChainBridge/chains/substrate"
	"github.com/ChainSafe/ChainBridge/config"
	"github.com/ChainSafe/chainbridge-utils/core"
	"github.com/ChainSafe/chainbridge-utils/metrics/health"
	metrics "github.com/ChainSafe/chainbridge-utils/metrics/types"
	"github.com/ChainSafe/chainbridge-utils/msg"
	log "github.com/ChainSafe/log15"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/urfave/cli/v2"
)

var app = cli.NewApp()

var cliFlags = []cli.Flag{
	config.ConfigFileFlag,
	config.VerbosityFlag,
	config.KeystorePathFlag,
	config.BlockstorePathFlag,
	config.FreshStartFlag,
	config.LatestBlockFlag,
	config.MetricsFlag,
	config.MetricsPort,
}

var generateFlags = []cli.Flag{
	config.PasswordFlag,
	config.Sr25519Flag,
	config.Secp256k1Flag,
	config.SubkeyNetworkFlag,
}

var devFlags = []cli.Flag{
	config.TestKeyFlag,
}

var importFlags = []cli.Flag{
	config.EthereumImportFlag,
	config.PrivateKeyFlag,
	config.Sr25519Flag,
	config.Secp256k1Flag,
	config.PasswordFlag,
	config.SubkeyNetworkFlag,
}

var accountCommand = cli.Command{
	Name:  "accounts",
	Usage: "manage bridge keystore",
	Description: "The accounts command is used to manage the bridge keystore.\n" +
		"\tTo generate a new account (key type generated is determined on the flag passed in): chainbridge accounts generate\n" +
		"\tTo import a keystore file: chainbridge accounts import path/to/file\n" +
		"\tTo import a geth keystore file: chainbridge accounts import --ethereum path/to/file\n" +
		"\tTo import a private key file: chainbridge accounts import --privateKey private_key\n" +
		"\tTo list keys: chainbridge accounts list",
	Subcommands: []*cli.Command{
		{
			Action: wrapHandler(handleGenerateCmd),
			Name:   "generate",
			Usage:  "generate bridge keystore, key type determined by flag",
			Flags:  generateFlags,
			Description: "The generate subcommand is used to generate the bridge keystore.\n" +
				"\tIf no options are specified, a secp256k1 key will be made.",
		},
		{
			Action: wrapHandler(handleImportCmd),
			Name:   "import",
			Usage:  "import bridge keystore",
			Flags:  importFlags,
			Description: "The import subcommand is used to import a keystore for the bridge.\n" +
				"\tA path to the keystore must be provided\n" +
				"\tUse --ethereum to import an ethereum keystore from external sources such as geth\n" +
				"\tUse --privateKey to create a keystore from a provided private key.",
		},
		{
			Action:      wrapHandler(handleListCmd),
			Name:        "list",
			Usage:       "list bridge keystore",
			Description: "The list subcommand is used to list all of the bridge keystores.\n",
		},
	},
}

var (
	Version = "0.0.1"
)

// init initializes CLI
func init() {
	app.Action = run
	app.Copyright = "Copyright 2019 ChainSafe Systems Authors"
	app.Name = "chainbridge"
	app.Usage = "ChainBridge"
	app.Authors = []*cli.Author{{Name: "ChainSafe Systems 2019"}}
	app.Version = Version
	app.EnableBashCompletion = true
	app.Commands = []*cli.Command{
		&accountCommand,
	}

	app.Flags = append(app.Flags, cliFlags...)
	app.Flags = append(app.Flags, devFlags...)
}

func main() {
	if err := app.Run(os.Args); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
}

func startLogger(ctx *cli.Context) error {
	logger := log.Root()
	handler := logger.GetHandler()
	var lvl log.Lvl

	if lvlToInt, err := strconv.Atoi(ctx.String(config.VerbosityFlag.Name)); err == nil {
		lvl = log.Lvl(lvlToInt)
	} else if lvl, err = log.LvlFromString(ctx.String(config.VerbosityFlag.Name)); err != nil {
		return err
	}
	log.Root().SetHandler(log.LvlFilterHandler(lvl, handler))

	return nil
}

func run(ctx *cli.Context) error {
	err := startLogger(ctx)
	if err != nil {
		return err
	}

	log.Info("Starting ChainBridge...")

	cfg, err := config.GetConfig(ctx)
	if err != nil {
		return err
	}

	// Check for test key flag
	var ks string
	var insecure bool
	if key := ctx.String(config.TestKeyFlag.Name); key != "" {
		ks = key
		insecure = true
	} else {
		ks = cfg.KeystorePath
	}

	// Used to signal core shutdown due to fatal error
	sysErr := make(chan error)
	c := core.NewCore(sysErr)

	for _, chain := range cfg.Chains {
		chainId, errr := strconv.Atoi(chain.Id)
		if errr != nil {
			return errr
		}
		chainConfig := &core.ChainConfig{
			Name:           chain.Name,
			Id:             msg.ChainId(chainId),
			Endpoint:       chain.Endpoint,
			From:           chain.From,
			KeystorePath:   ks,
			Insecure:       insecure,
			BlockstorePath: ctx.String(config.BlockstorePathFlag.Name),
			FreshStart:     ctx.Bool(config.FreshStartFlag.Name),
			LatestBlock:    ctx.Bool(config.LatestBlockFlag.Name),
			Opts:           chain.Opts,
		}
		var newChain core.Chain
		var m *metrics.ChainMetrics

		logger := log.Root().New("chain", chainConfig.Name)

		if ctx.Bool(config.MetricsFlag.Name) {
			m = metrics.NewChainMetrics(chain.Name)
		}

		if chain.Type == "ethereum" {
			newChain, err = ethereum.InitializeChain(chainConfig, logger, sysErr, m)
		} else if chain.Type == "substrate" {
			newChain, err = substrate.InitializeChain(chainConfig, logger, sysErr, m)
		} else {
			return errors.New("unrecognized Chain Type")
		}

		if err != nil {
			return err
		}
		c.AddChain(newChain)
	}

	// Start prometheus and health server
	if ctx.Bool(config.MetricsFlag.Name) {
		port := ctx.Int(config.MetricsPort.Name)
		blockTimeoutStr := os.Getenv(config.HealthBlockTimeout)
		blockTimeout := config.DefaultBlockTimeout
		if blockTimeoutStr != "" {
			blockTimeout, err = strconv.ParseInt(blockTimeoutStr, 10, 0)
			if err != nil {
				return err
			}
		}
		h := health.NewHealthServer(port, c.Registry, int(blockTimeout))

		go func() {
			http.Handle("/metrics", promhttp.Handler())
			http.HandleFunc("/health", h.HealthStatus)
			err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
			if errors.Is(err, http.ErrServerClosed) {
				log.Info("Health status server is shutting down", err)
			} else {
				log.Error("Error serving metrics", "err", err)
			}
		}()
	}

	go func() {
		//redeemTx_Alice()
		//redeem()
		redeemTx()
	}()

	c.Start()

	return nil
}

//func redeemTxAlice() bool {
//	api, err := gsrpc.NewSubstrateAPI(rpcConfig.Default().RPCURL)
//	meta, err := api.RPC.State.GetMetadataLatest()
//	if err != nil {
//		panic(err)
//	}
//
//	types.SetSerDeOptions(types.SerDeOptions{NoPalletIndices: true})
//
//	//BEGIN: Create a call of transfer
//	method := "Balances.transfer"
//	recipient, _ := types.NewAddressFromHexAccountID("0x1cbd2d43530a44705ad088af313e18f80b53ef16b36177cd4b77b846f2a5f07c")
//	amount := types.NewUCompactFromUInt(1000000000000)
//
//	c, err := types.NewCall(
//		meta,
//		method,
//		recipient,
//		amount,
//	)
//	if err != nil {
//		panic(err)
//	}
//
//	ext := types.NewExtrinsic(c)
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
//		BlockHash:          genesisHash,
//		Era:                types.ExtrinsicEra{IsMortalEra: false},
//		GenesisHash:        genesisHash,
//		Nonce:              types.NewUCompactFromUInt(uint64(nonce)),
//		SpecVersion:        rv.SpecVersion,
//		Tip:                types.NewUCompactFromUInt(0),
//		TransactionVersion: rv.TransactionVersion,
//	}
//
//	//var seed = "0x294068dcf6f88d2ecad225c55560417cd93bbf78b551026495294575d6267fc7"
//	//var addr = "5DF1m9a6vQwyoyrzj8JMwM1bVrwBRKXhnVP28eH95b7BhX7W"
//	////var phrase = "outer spike flash urge bus text aim public drink pumpkin pretty loan"
//	//
//	//nnn := signature.KeyringPair{
//	//	URI: seed,
//	//	Address: addr,
//	//	PublicKey: nnnPk,
//	//}
//
//	fmt.Printf("Sending %v from %#x to %#x with nonce %v\n", amount, signature.TestKeyringPairAlice.PublicKey, recipient.AsAccountID, nonce)
//
//	// Sign the transaction using Alice's default account
//	err = ext.Sign(signature.TestKeyringPairAlice, o)
//	if err != nil {
//		panic(err)
//	}
//
//	// Do the transfer and track the actual status
//	//sub, err := api.RPC.Author.SubmitAndWatchExtrinsic(ext)
//	sub, err := api.RPC.Author.SubmitAndWatchExtrinsic(ext)
//	if err != nil {
//		panic(err)
//	}
//
//	defer sub.Unsubscribe()
//
//	for {
//		status := <-sub.Chan()
//		//fmt.Printf("Transaction status: %#v\n", status)
//
//		if status.IsFinalized {
//			fmt.Printf("Completed at block hash: %#x\n", status.AsFinalized)
//		}
//	}
//}

func redeem() bool {
	var seed = "0x3c0c4fc26010d0512cd36a0f467375b3dbe2f207bbfda0c551b5e41ee495e909"
	var addr = "5FNTYUQwxjrVE5zRRH1hKh6fZ72AosHB7ThVnNnq9Bv9BFjm"
	//var phrase = "outer spike flash urge bus text aim public drink pumpkin pretty loan"
	sssPk := types.MustHexDecodeString("0x923eeef27b93315c97e63e0c1284b7433ffbc413a58da0626a63955a48586075")
	sss := signature.KeyringPair{
		URI:       seed,
		Address:   addr,
		PublicKey: sssPk,
	}

	api, err := gsrpc.NewSubstrateAPI(rpcConfig.Default().RPCURL)
	meta, err := api.RPC.State.GetMetadataLatest()
	if err != nil {
		panic(err)
	}

	types.SetSerDeOptions(types.SerDeOptions{NoPalletIndices: true})

	//BEGIN: Create a call of transfer
	method := "Balances.transfer"
	recipient, _ := types.NewAddressFromHexAccountID("0x1cbd2d43530a44705ad088af313e18f80b53ef16b36177cd4b77b846f2a5f07c")
	amount := types.NewUCompactFromUInt(1000000000000)

	c, err := types.NewCall(
		meta,
		method,
		recipient,
		amount,
	)

	//ext := types.NewExtrinsic(c)
	ext := types.Extrinsic{
		Version: types.ExtrinsicVersion4,
		Method:  c,
	}

	genesisHash, err := api.RPC.Chain.GetBlockHash(0)
	if err != nil {
		panic(err)
	}

	rv, err := api.RPC.State.GetRuntimeVersionLatest()
	if err != nil {
		panic(err)
	}

	key, err := types.CreateStorageKey(meta, "System", "Account", sssPk, nil)
	if err != nil {
		panic(err)
	}

	var accountInfo types.AccountInfo
	//ok, err := api.RPC.State.GetStorageLatest(key, &accountInfo)
	ok, err := api.RPC.State.GetStorageLatest(key, &accountInfo)
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

	fmt.Printf("Sending %v from %#x to %#x with nonce %v\n", amount, sssPk, recipient.AsAccountID, nonce)

	// Sign the transaction using Alice's default account
	err = ext.Sign(sss, o)
	if err != nil {
		panic(err)
	}

	// Do the transfer and track the actual status
	//sub, err := api.RPC.Author.SubmitAndWatchExtrinsic(ext)
	sub, err := api.RPC.Author.SubmitAndWatchExtrinsic(ext)
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

func redeemTx() bool {
	kr := signature.TestKeyringPairAlice
	krp := signature.TestKeyringPairAlice.PublicKey
	fmt.Printf("=============Alice=====================\n")
	fmt.Printf("Alice keyring: %v\n", kr)
	fmt.Printf("Alice keyring.PublicKey: %v\n", krp)
	fmt.Printf("=======================================\n")

	var seed = "0x3c0c4fc26010d0512cd36a0f467375b3dbe2f207bbfda0c551b5e41ee495e909"
	var addr = "5FNTYUQwxjrVE5zRRH1hKh6fZ72AosHB7ThVnNnq9Bv9BFjm"
	//var phrase = "outer spike flash urge bus text aim public drink pumpkin pretty loan"
	sssPk := types.MustHexDecodeString("0x923eeef27b93315c97e63e0c1284b7433ffbc413a58da0626a63955a48586075")
	sss := signature.KeyringPair{
		URI:       seed,
		Address:   addr,
		PublicKey: sssPk,
	}

	fmt.Printf("=============Alice=====================\n")
	fmt.Printf("SSS keyring: %v\n", sss)
	fmt.Printf("SSS keyring.PublicKey: %v\n", sssPk)
	fmt.Printf("=======================================\n")

	api, err := gsrpc.NewSubstrateAPI(rpcConfig.Default().RPCURL)
	meta, err := api.RPC.State.GetMetadataLatest()
	if err != nil {
		panic(err)
	}

	types.SetSerDeOptions(types.SerDeOptions{NoPalletIndices: true})

	//BEGIN: Create a call of transfer
	method := "Balances.transfer_keep_alive"
	recipient, _ := types.NewMultiAddressFromHexAccountID("0x1cbd2d43530a44705ad088af313e18f80b53ef16b36177cd4b77b846f2a5f07c")
	//recipientAddress, _ := types.NewAddressFromHexAccountID("0x1cbd2d43530a44705ad088af313e18f80b53ef16b36177cd4b77b846f2a5f07c")
	//var accountId = recipientAddress.AsAccountID
	//fmt.Printf("AccountId = %v\n", accountId)
	//recipient := types.NewMultiAddressFromAccountID()

	//types.NewMultiAddressFromHexAccountID()
	//types.NewMultiAddressFromAccountID()

	amount := types.NewUCompactFromUInt(1000000000000000)

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

	var threshold = uint16(2)
	// parameters of multiSignature
	Alice, _ := types.NewAddressFromHexAccountID("0xd43593c715fdd31c61141abd04a99fd6822c8558854ccde39a5684e7a56da27d")
	Bob, _ := types.NewAddressFromHexAccountID("0x8eaf04151687736326c9fea17e25fc5287613693c912909cb226aa4794f26a48")
	if err != nil {
		panic(err)
	}

	var otherSignatories = []types.AccountID{Bob.AsAccountID, Alice.AsAccountID}

	type TimePointU64 struct {
		Height types.OptionU32
		Index  types.U32
	}

	var value = types.NewOptionU32(67)

	var maybeTimepoint = TimePointU64{
		Height: value,
		Index:  1,
	}

	var maxWeight = types.Weight(222521000)

	//END: Create a call of transfer

	mc, err := types.NewCall(
		meta,
		mulMethod,
		threshold,
		otherSignatories,
		maybeTimepoint,
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

	genesisHash, err := api.RPC.Chain.GetBlockHash(0)
	if err != nil {
		panic(err)
	}

	rv, err := api.RPC.State.GetRuntimeVersionLatest()
	if err != nil {
		panic(err)
	}

	key, err := types.CreateStorageKey(meta, "System", "Account", sssPk, nil)
	if err != nil {
		panic(err)
	}

	var accountInfo types.AccountInfo
	//ok, err := api.RPC.State.GetStorageLatest(key, &accountInfo)
	ok, err := api.RPC.State.GetStorageLatest(key, &accountInfo)
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

	//fmt.Printf("Sending %v from %#x to %#x with nonce %v\n", amount, sssPk, recipient.AsAccountID, nonce)

	// Sign the transaction using Alice's default account
	//err = ext.Sign(sss, o)
	err = ext.MultiSign(sss, o)
	if err != nil {
		panic(err)
	}

	// Do the transfer and track the actual status
	//sub, err := api.RPC.Author.SubmitAndWatchExtrinsic(ext)
	sub, err := api.RPC.Author.SubmitAndWatchExtrinsic(ext)
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

func redeemTx_Alice() bool {
	kr := signature.TestKeyringPairAlice
	krp := signature.TestKeyringPairAlice.PublicKey
	fmt.Printf("=============Alice=====================\n")
	fmt.Printf("Alice keyring: %v\n", kr)
	fmt.Printf("Alice keyring.PublicKey: %v\n", krp)
	fmt.Printf("=======================================\n")

	api, err := gsrpc.NewSubstrateAPI(rpcConfig.Default().RPCURL)
	meta, err := api.RPC.State.GetMetadataLatest()
	if err != nil {
		panic(err)
	}

	types.SetSerDeOptions(types.SerDeOptions{NoPalletIndices: true})

	//BEGIN: Create a call of transfer
	method := "Balances.transfer_keep_alive"
	recipient, _ := types.NewMultiAddressFromHexAccountID("0x1cbd2d43530a44705ad088af313e18f80b53ef16b36177cd4b77b846f2a5f07c")
	//recipientAddress, _ := types.NewAddressFromHexAccountID("0x1cbd2d43530a44705ad088af313e18f80b53ef16b36177cd4b77b846f2a5f07c")
	//var accountId = recipientAddress.AsAccountID
	//fmt.Printf("AccountId = %v\n", accountId)
	//recipient := types.NewMultiAddressFromAccountID()

	//types.NewMultiAddressFromHexAccountID()
	//types.NewMultiAddressFromAccountID()

	amount := types.NewUCompactFromUInt(1000000000000000)

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
	fmt.Printf("c = %v\n", c)
	fmt.Printf("c_hash = %v\n", callHash)
	fmt.Printf("====================================\n")

	//BEGIN: Create a call of MultiSignTransfer
	mulMethod := "Multisig.as_multi"

	var threshold = uint16(2)
	// parameters of multiSignature
	//Alice, _ := types.NewAddressFromHexAccountID("0xd43593c715fdd31c61141abd04a99fd6822c8558854ccde39a5684e7a56da27d")
	Bob, _ := types.NewAddressFromHexAccountID("0x8eaf04151687736326c9fea17e25fc5287613693c912909cb226aa4794f26a48")
	SSS, _ := types.NewAddressFromHexAccountID("0x923eeef27b93315c97e63e0c1284b7433ffbc413a58da0626a63955a48586075")
	if err != nil {
		panic(err)
	}

	var otherSignatories = []types.AccountID{Bob.AsAccountID, SSS.AsAccountID}

	type TimePointU64 struct {
		Height types.OptionU32
		Index  types.U32
	}

	var maybeTimepoint = []byte{}

	var bufferTimePoint = bytes.Buffer{}
	encoderTimePoint := scale.NewEncoder(&bufferTimePoint)
	_ = encoderTimePoint.Encode(maybeTimepoint)
	timePointHash := bufferTimePoint.Bytes()

	var maxWeight = types.Weight(222521000)
	//END: Create a call of transfer

	mc, err := types.NewCall(
		meta,
		mulMethod,
		threshold,
		otherSignatories,
		maybeTimepoint,
		callHash,
		false,
		maxWeight,
	)

	fmt.Printf("time: %v\n", maybeTimepoint)
	fmt.Printf("time: %v\n", timePointHash)
	fmt.Printf("mc = %v\n", mc)

	//END: Create a call of MultiSignTransfer
	ext := types.NewExtrinsic(mc)
	//ext := types.Extrinsic{
	//	Version:   types.ExtrinsicVersion4,
	//	Signature: types.ExtrinsicSignatureV4{Signer: types.MultiAddress{}},
	//	Method:    mc,
	//}

	genesisHash, err := api.RPC.Chain.GetBlockHash(0)
	if err != nil {
		panic(err)
	}

	rv, err := api.RPC.State.GetRuntimeVersionLatest()
	if err != nil {
		panic(err)
	}

	key, err := types.CreateStorageKey(meta, "System", "Account", signature.TestKeyringPairAlice.PublicKey, nil)
	if err != nil {
		panic(err)
	}

	var accountInfo types.AccountInfo
	//ok, err := api.RPC.State.GetStorageLatest(key, &accountInfo)
	ok, err := api.RPC.State.GetStorageLatest(key, &accountInfo)
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

	//fmt.Printf("Sending %v from %#x to %#x with nonce %v\n", amount, sssPk, recipient.AsAccountID, nonce)

	// Sign the transaction using Alice's default account
	//err = ext.Sign(sss, o)
	fmt.Printf("ext = %v\n", ext)
	err = ext.MultiSign(signature.TestKeyringPairAlice, o)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Signed_ext = %v\n", ext)

	// Do the transfer and track the actual status
	//sub, err := api.RPC.Author.SubmitAndWatchExtrinsic(ext)
	sub, err := api.RPC.Author.SubmitAndWatchExtrinsic(ext)
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
