// Copyright 2020 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only
/*
Provides the command-line interface for the chainbridge application.

For configuration and CLI commands see the README: https://github.com/ChainSafe/ChainBridge.
*/
package main

import (
	"errors"
	"fmt"
	"github.com/ChainSafe/ChainBridge/chains/ethereum"
	gsrpc "github.com/centrifuge/go-substrate-rpc-client/v2"
	rpcConfig "github.com/centrifuge/go-substrate-rpc-client/v2/config"
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
		//redeemTxAlice()
		redeemTx()
		//redeemMultiSignTx()
	}()

	c.Start()

	return nil
}
func redeemTxAlice() bool {
	//nnnPk := types.MustHexDecodeString("0x3418f5e3f3e90db1e870bee7a2909d3ecb27623ed07b220aaf205f053c660c1e")

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
	if err != nil {
		panic(err)
	}

	ext := types.NewExtrinsic(c)
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

	//var seed = "0x294068dcf6f88d2ecad225c55560417cd93bbf78b551026495294575d6267fc7"
	//var addr = "5DF1m9a6vQwyoyrzj8JMwM1bVrwBRKXhnVP28eH95b7BhX7W"
	////var phrase = "outer spike flash urge bus text aim public drink pumpkin pretty loan"
	//
	//nnn := signature.KeyringPair{
	//	URI: seed,
	//	Address: addr,
	//	PublicKey: nnnPk,
	//}

	fmt.Printf("Sending %v from %#x to %#x with nonce %v\n", amount, signature.TestKeyringPairAlice.PublicKey, recipient.AsAccountID, nonce)

	// Sign the transaction using Alice's default account
	err = ext.Sign(signature.TestKeyringPairAlice, o)
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

	var seed = "0xc797fbfb4a2f8dea4ef00b19d25c263ce835852e5e4b7ff2345b05215338c9b4"
	var addr = "5DyhbKavDtMijvnsTxnRVySqfyeXi9eUejhE8sANTK33h6UT"
	//var phrase = "outer spike flash urge bus text aim public drink pumpkin pretty loan"
	nnnPk := types.MustHexDecodeString("0x54a7595feeefd067568b31f85f052fbbe0a5a0812979466bab9243e2ce80e26f")

	nnn := signature.KeyringPair{
		URI:       seed,
		Address:   addr,
		PublicKey: nnnPk,
	}

	fmt.Printf("=============Alice=====================\n")
	fmt.Printf("NNN keyring: %v\n", nnn)
	fmt.Printf("NNN keyring.PublicKey: %v\n", nnnPk)
	fmt.Printf("=======================================\n")

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
	if err != nil {
		panic(err)
	}

	ext := types.NewExtrinsic(c)
	genesisHash, err := api.RPC.Chain.GetBlockHash(0)
	if err != nil {
		panic(err)
	}
	rv, err := api.RPC.State.GetRuntimeVersionLatest()
	if err != nil {
		panic(err)
	}

	key, err := types.CreateStorageKey(meta, "System", "Account", nnnPk, nil)
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

	fmt.Printf("Sending %v from %#x to %#x with nonce %v\n", amount, nnnPk, recipient.AsAccountID, nonce)

	// Sign the transaction using Alice's default account
	err = ext.Sign(nnn, o)
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

func redeemMultiSignTx() bool {
	nnnPk := types.MustHexDecodeString("0x3418f5e3f3e90db1e870bee7a2909d3ecb27623ed07b220aaf205f053c660c1e")

	api, err := gsrpc.NewSubstrateAPI(rpcConfig.Default().RPCURL)
	if err != nil {
		panic(err)
	}

	meta, err := api.RPC.State.GetMetadataLatest()
	if err != nil {
		panic(err)
	}

	//serialize signature data
	types.SetSerDeOptions(types.SerDeOptions{NoPalletIndices: true})

	//BEGIN: Create a call of transfer
	method := "Balances.transfer"
	recipient := types.NewAddressFromAccountID([]byte("5HGjWAeFDfFCWPsjFQdVV2Msvz2XtMktvgocEZcCj68kUMaw"))
	amount := types.NewUCompactFromUInt(1000)

	c, err := types.NewCall(
		meta,
		method,
		recipient,
		amount,
	)
	if err != nil {
		panic(err)
	}

	ext := types.NewExtrinsic(c)

	genesisHash, err := api.RPC.Chain.GetBlockHash(0)
	if err != nil {
		panic(err)
	}
	rv, err := api.RPC.State.GetRuntimeVersionLatest()
	if err != nil {
		panic(err)
	}

	//key, err := types.CreateStorageKey(meta, "System", "Account", signature.TestKeyringPairAlice.PublicKey, nil)
	key, err := types.CreateStorageKey(meta, "System", "Account", nnnPk, nil)
	if err != nil {
		panic(err)
	}

	var accountInfo types.AccountInfo
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

	fmt.Printf("===================================================================================")
	fmt.Printf("Multisign: Sending %v from %#x to %#x with nonce %v", amount, nnnPk, recipient.AsAccountID, nonce)
	fmt.Printf("===================================================================================")

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

	sub, err := api.RPC.Author.SubmitAndWatchExtrinsic(ext)
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
