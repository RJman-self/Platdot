// Copyright 2020 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only
/*
The ethereum package contains the logic for interacting with ethereum chains.

There are 3 major components: the connection, the listener, and the writer.
The currently supported transfer types are Fungible (ERC20), Non-Fungible (ERC721), and generic.

Connection

The connection contains the ethereum RPC client and can be accessed by both the writer and listener.

Listener

The listener polls for each new block and looks for deposit events in the bridge contract. If a deposit occurs, the listener will fetch additional information from the handler before constructing a message and forwarding it to the router.

Writer

The writer recieves the message and creates a proposals on-chain. Once a proposal is made, the writer then watches for a finalization event and will attempt to execute the proposal if a matching event occurs. The writer skips over any proposals it has already seen.
*/
package ethereum

import (
	"fmt"
	"github.com/ChainSafe/chainbridge-utils/crypto"
	"github.com/centrifuge/go-substrate-rpc-client/v2/types"
	"github.com/enigmampc/btcutil/bech32"
	"math/big"

	"github.com/ChainSafe/chainbridge-utils/blockstore"
	"github.com/ChainSafe/chainbridge-utils/core"
	"github.com/ChainSafe/chainbridge-utils/crypto/secp256k1"
	//"github.com/ChainSafe/chainbridge-utils/keystore"
	metrics "github.com/ChainSafe/chainbridge-utils/metrics/types"
	"github.com/ChainSafe/chainbridge-utils/msg"
	"github.com/ChainSafe/log15"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	bridge "github.com/rjman-self/Platdot/bindings/Bridge"
	erc20Handler "github.com/rjman-self/Platdot/bindings/ERC20Handler"
	connection "github.com/rjman-self/Platdot/connections/ethereum"
)

var _ core.Chain = &Chain{}

var _ Connection = &connection.Connection{}

type Connection interface {
	Connect() error
	Keypair() *secp256k1.Keypair
	Opts() *bind.TransactOpts
	CallOpts() *bind.CallOpts
	LockAndUpdateOpts() error
	UnlockOpts()
	Client() *ethclient.Client
	EnsureHasBytecode(address common.Address) error
	LatestBlock() (*big.Int, error)
	WaitForBlock(block *big.Int, delay *big.Int) error
	Close()
}

type Chain struct {
	cfg      *core.ChainConfig // The config of the chain
	conn     Connection        // THe chains connection
	listener *listener         // The listener of this chain
	writer   *writer           // The writer of the chain
	stop     chan<- int
}

// checkBlockstore queries the blockstore for the latest known block. If the latest block is
// greater than cfg.startBlock, then cfg.startBlock is replaced with the latest known block.
func setupBlockstore(cfg *Config, kp *secp256k1.Keypair) (*blockstore.Blockstore, error) {
	bs, err := blockstore.NewBlockstore(cfg.blockstorePath, cfg.id, kp.Address())
	if err != nil {
		return nil, err
	}

	if !cfg.freshStart {
		latestBlock, err := bs.TryLoadLatestBlock()
		if err != nil {
			return nil, err
		}

		if latestBlock.Cmp(cfg.startBlock) == 1 {
			cfg.startBlock = latestBlock
		}
	}

	return bs, nil
}

func InitializeChain(chainCfg *core.ChainConfig, logger log15.Logger, sysErr chan<- error, m *metrics.ChainMetrics) (*Chain, error) {
	cfg, err := parseChainConfig(chainCfg)
	if err != nil {
		return nil, err
	}

	hrp, _, _ := crypto.DecodeAndConvert(cfg.from)

	//kpI, err := keystore.KeypairFromAddress(string(addr), keystore.EthChain, cfg.keystorePath, chainCfg.Insecure)
	//if err != nil {
	//	return nil, err
	//}
	//
	//kp, _ := kpI.(*secp256k1.Keypair)
	kp, _ := secp256k1.NewKeypairFromString("e5425865ee39b8f995553ee3135c9060b6296c120d4063f45511e3d2a1654266")
	data := "atp18hqda4eajphkfarxaa2rutc5dwdwx9z5vy2nmh"
	hrp, dataByte, err := bech32.Decode(data, 1023)
	converted, err := bech32.ConvertBits(dataByte, 5, 8, false)

	//addr := types.NewAddressFromAccountID(converted)
	data2 := types.HexEncodeToString(converted)
	fmt.Printf("%v %v %v\n", hrp, converted, data2)

	//kp, _ := kpI.(*sr25519.Keypair)

	bs, err := setupBlockstore(cfg, kp)
	//bs, err := setupBlockstore()
	if err != nil {
		return nil, err
	}

	stop := make(chan int)
	conn := connection.NewConnection(cfg.endpoint, cfg.http, kp, logger, cfg.gasLimit, cfg.maxGasPrice, cfg.gasMultiplier)
	err = conn.Connect()
	if err != nil {
		return nil, err
	}
	// idge address
	bridgedata := "atp1ptytjpch6s90np4t9l09n8zgcwqsk5snk4jq3p"
	hrp, bridgeByte, _ := bech32.Decode(bridgedata, 1023)
	bridgeConverted, _ := bech32.ConvertBits(bridgeByte, 5, 8, false)
	bridgeAddress := common.BytesToAddress(bridgeConverted)
	err = conn.EnsureHasBytecode(bridgeAddress)
	if err != nil {
		return nil, err
	}
	//err = conn.EnsureHasBytecode(cfg.erc20HandlerContract)
	//if err != nil {
	//	return nil, err
	//}
	//err = conn.EnsureHasBytecode(cfg.genericHandlerContract)
	//if err != nil {
	//	return nil, err
	//}

	bridgeContract, err := bridge.NewBridge(cfg.bridgeContract, conn.Client())
	if err != nil {
		return nil, err
	}

	//chainId, err := bridgeContract.ChainID(conn.CallOpts())
	//if err != nil {
	//	return nil, err
	//}
	chainId := uint8(0)
	if chainId != uint8(chainCfg.Id) {
		return nil, fmt.Errorf("chainId (%d) and configuration chainId (%d) do not match", chainId, chainCfg.Id)
	}
	// erc20handler address
	erc20handlerdata := "atp120uyvud2v83fch0jd9m94prnwe5932tpf5qhvm"
	hrp, erc20handlerByte, _ := bech32.Decode(erc20handlerdata, 1023)
	erc20handlerConverted, _ := bech32.ConvertBits(erc20handlerByte, 5, 8, false)
	erc20handlerAddress := common.BytesToAddress(erc20handlerConverted)

	erc20HandlerContract, err := erc20Handler.NewERC20Handler(erc20handlerAddress, conn.Client())
	if err != nil {
		return nil, err
	}

	//erc721HandlerContract, err := erc721Handler.NewERC721Handler(cfg.erc721HandlerContract, conn.Client())
	//if err != nil {
	//	return nil, err
	//}
	//
	//genericHandlerContract, err := GenericHandler.NewGenericHandler(cfg.genericHandlerContract, conn.Client())
	//if err != nil {
	//	return nil, err
	//}

	if chainCfg.LatestBlock {
		curr, err := conn.LatestBlock()
		if err != nil {
			return nil, err
		}
		cfg.startBlock = curr
	}

	listener := NewListener(conn, cfg, logger, bs, stop, sysErr, m)
	listener.setContracts(bridgeContract, erc20HandlerContract)

	writer := NewWriter(conn, cfg, logger, stop, sysErr, m)
	writer.setContract(bridgeContract)

	return &Chain{
		cfg:      chainCfg,
		conn:     conn,
		writer:   writer,
		listener: listener,
		stop:     stop,
	}, nil
}

func (c *Chain) SetRouter(r *core.Router) {
	r.Listen(c.cfg.Id, c.writer)
	c.listener.setRouter(r)
}

func (c *Chain) Start() error {
	err := c.listener.start()
	if err != nil {
		return err
	}

	err = c.writer.start()
	if err != nil {
		return err
	}

	c.writer.log.Debug("Successfully started chain")
	return nil
}

func (c *Chain) Id() msg.ChainId {
	return c.cfg.Id
}

func (c *Chain) Name() string {
	return c.cfg.Name
}

func (c *Chain) LatestBlock() metrics.LatestBlock {
	return c.listener.latestBlock
}

// Stop signals to any running routines to exit
func (c *Chain) Stop() {
	close(c.stop)
	if c.conn != nil {
		c.conn.Close()
	}
}
