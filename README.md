# Platdot
![img](https://github.com/RJman-self/Platdot/blob/master/Platdot.svg)

## Introduction

​		Bscdot is a cross-chain project based on ChainBridge developed by the ChainX team. In order to achieve two-way cross-chain, chainbridge needs to deploy a pallet on the Substrate chain that is equivalent to the smart contract in the EVM, so it cannot be deployed on polkadot. Our team has improved this. Through Bscdot, it can be passed without pallet. The multi-signature module realizes a completely decentralized token transfer across Polkadot, transferring Dot on Polkadot to Binance Smart Chain, and it can also be applied to kusama, chainX and other networks that have huge value but cannot deploy pallets on their own.

## Demo Video

https://www.youtube.com/watch?v=vTMIlM2oaJc&feature=youtu.be

## UI

https://cdn.jsdelivr.net/gh/rjman-self/resources@master/images/bscdot-1.png

![b-1](https://cdn.jsdelivr.net/gh/rjman-self/resources@master/images/bscdot-1.png)

![2](https://cdn.jsdelivr.net/gh/rjman-self/resources@master/images/bscdot-2.png)

## Contract address on testnet

```json
"opts": {
        "bridge": "atx1762m2ryuvnnrk3d9q6gfy6whk29n59xu34typ5",
        "erc20Handler": "atx1t3zvgf73mmhzax24epgv02vqznzw24a5m78cnz",
        "http": "true",
        "prefix": "atx"
      }
```

## Running Locally

### Prerequisites

- platdot binary
- solidity contract
- Polkadot JS Portal

### Deploy Contracts

To deploy the contracts on to the platon chain

```
alaya-truffle migrate
```

After running, the expected output looks like this:

```
Deploying 'Bridge'
------------------

  transaction hash:    0x3546086f29317fda406b158f987ceb29267cdb049a206fa60c1794165767e77e
  Blocks: 3            Seconds: 4
  contract address:    atx1762m2ryuvnnrk3d9q6gfy6whk29n59xu34typ5
  block number:        3112
  block timestamp:     1615776691128
  account:             atx18hqda4eajphkfarxaa2rutc5dwdwx9z5xzkega
  balance:             999999.9179162
  gas used:            4104190
  gas price:           20 gvon
  value sent:          0 ATP
  total cost:          0.0820838 ATP

Deploying 'ERC20Handler'
------------------------

  transaction hash:    0x0194801955a7ec9a4a18017b6ae29943c6b806804277f1be566192d20aff4474
  Blocks: 0            Seconds: 0
  contract address:    atx1t3zvgf73mmhzax24epgv02vqznzw24a5m78cnz
  block number:        3115
  block timestamp:     1615776694431
  account:             atx18hqda4eajphkfarxaa2rutc5dwdwx9z5xzkega
  balance:             999999.88379392
  gas used:            1706114
  gas price:           20 gvon
  value sent:          0 ATP
  total cost:          0.03412228 ATP


Deploying 'ERC20PresetMinterPauser'
-----------------------------------

  transaction hash:    0x64001ffa15d70285c64b862cd60b5a24adf7c90f27d37a6771258b984a05bd24
  Blocks: 0            Seconds: 0
  contract address:    atx1lfhrcc6xectcfe850kf83rcntlw0ha7wck9qjz
  block number:        3118
  block timestamp:     1615776697733
  account:             atx18hqda4eajphkfarxaa2rutc5dwdwx9z5xzkega
  balance:             999999.83473576
  gas used:            2452908
  gas price:           20 gvon
  value sent:          0 ATP
  total cost:          0.04905816 ATP

  Saving artifacts
  -------------------------------------
  Total cost:          0.16526424 ATP


Summary
=======

   Total deployments:   3
   Final cost:          0.16526424 ATP
```

### Initial Contract


### Running A Relayer

Before running a relayer, you should import your keys in polka and platon. Just:
```bigquery
cd Platdot &&
./build/chainbridge accounts import --privateKey your-polka-privatekey --sr25519 &&
./build/chainbridge accounts import --privateKey your-platon-privatekey --secp256k1
```


Also there is an example config file for a single relayer ("Alice") using the contracts we've deployed.

```
{
  "chains": [
    {
      "name": "alaya",
      "type": "ethereum",
      "id": "222",
      "endpoint": "http://localhost:6790",
      "from": "atx18hqda4eajphkfarxaa2rutc5dwdwx9z5xzkega",
      "opts": {
        "bridge": "atx1762m2ryuvnnrk3d9q6gfy6whk29n59xu34typ5",
        "erc20Handler": "atx1t3zvgf73mmhzax24epgv02vqznzw24a5m78cnz",
        "http": "true",
        "prefix": "atx"
      }
    },
    {
      "name": "polkadot",
      "type": "substrate",
      "id": "1",
      "endpoint": "ws://127.0.0.1:9945",
      "from": "1EF2TrKpzbfUtzWM3xguvniJtj6PV4Apk5zsV26zR6z5iSq",
      "latestBlock": "true",
      "opts": {
        "MultiSignAddress": "0x4acbb630c9f7e011af0783bdd8b2a22ab834b8e5bcc1dbf35bd2a8f1ca8ebd8e",
        "TotalRelayer": "5",
        "CurrentRelayerNumber": "2",
        "MultiSignThreshold": "3",
        "OtherRelayer1": "0x50a80eb26a7fb43ff4f84ead705fc61c1d4074112e53f781a6b03c0c7504f663",
        "OtherRelayer2": "0x923eeef27b93315c97e63e0c1284b7433ffbc413a58da0626a63955a48586075",
        "OtherRelayer3": "0xa45a0ddd81da79f65cbcfeefc8e62382b1f56ccbbdd9533f77cdc49172cca33d",
        "OtherRelayer4": "0xe6c2b6c4a5d3a770814f3ebe99893d1bb66e8f0d086a2badfcbb481b043ada1a",
        "ResourceId": "0x0000000000000000000000000000000000000000000000000000000000000000",
        "MaxWeight": "22698000000",
        "DestId": "222"
      }
    }
  ]
}
```

Run `make build` in bscdot directory to build bscdot. You can then start a relayer as a binary using the default "Alice" key.

```bash
./build/chainbridge --config config.json --testkey alice --latest  --verbosity trace
```

## Transfer in Polkadot.js.org

