# Platdot

[English]() | 简体中文

​																	![sad](https://img.shields.io/badge/build-passing-{右半部分颜色})![sad](https://img.shields.io/badge/test-passing-{右半部分颜色})![sad](https://img.shields.io/badge/release-v1.0.0-E6007A)

![](https://cdn.jsdelivr.net/gh/rjman-self/resources/assets/Platdot.png)

`Platdot` is a cross-chain project based on [ChainBridge](https://github.com/ChainSafe/ChainBridge). In order to achieve two-way cross-chain, platdot needs to deploy a pallet on the Substrate chain that is equivalent to the smart contract in the EVM, but it cannot be deployed on polkadot. Our team has improved this. Through Platdot, it can be passed without pallet. The multi-signature module realizes a completely decentralized token transfer across Polkadot, transferring Dot on Polkadot to PlatON, and it can also be applied to kusama, chainX and other networks that have huge value but cannot deploy pallets on their own.

## Installation

### Dependencies

+ Make sure the Golang environment is installed

+ [Platdot-contract v1.0.0](https://github.com/ChainSafe/chainbridge-solidity): Deploy and configure smart contract in Alaya.

### Building

`git clone https://github.com/RJman-self/Platdot.git`

`make build`: Builds `platdot` in `./build`.

**or**

`make install`: Uses `go install` to add `platdot` to your `GOBIN`.

## Getting Start

Documentations are now moved to `GitHub Wiki`.

### RunningLocally

[Deploy Smart Contracts](https://github.com/RJman-self/Platdot/wiki/Deploy-smart-contracts-to-the-alaya-network)

[Start A Relayer](https://github.com/RJman-self/Platdot/wiki/Start-A-Relayer)

## License

The project is released under the terms of the `GPLv3`.