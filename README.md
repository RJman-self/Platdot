# Platdot

English | [简体中文](./README_CN.md)

​																	![build](https://img.shields.io/badge/build-passing-{})![test](https://img.shields.io/badge/test-passing-{})![release](https://img.shields.io/badge/release-v1.0.0-E6007A)

![](https://cdn.jsdelivr.net/gh/rjman-self/resources/assets/Platdot.png)

`Platdot` is a cross-chain project based on [chainbridge](https://github.com/chainsafe/chainbridge). In order to complete the double-ended cross chain, chainbridge needs to deploy a smart contract with multis-signed functions on EVM, and Integrate the corresponding multi-sign module based on the substrate development chain, but you can't upload your module on the existing polkadot and kusama network, our team is redesigned and optimized to this part of the function, realizing Alaya / Platon and Kusama / Polkadot's cross chain interoperability. With Platodt, there is no need to complete cross-chain recharge and redemption by the exchange, and the multi-signature design of Platdot has achieved the circulation of the detriments, or it can be applied to Kusama, ChainX and other networks with great value.

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
