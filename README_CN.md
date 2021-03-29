# Platdot

[English](./README.md) | 简体中文

​																	![build](https://img.shields.io/badge/build-passing-{})![test](https://img.shields.io/badge/test-passing-{})![release](https://img.shields.io/badge/release-v1.0.0-E6007A)

![](https://cdn.jsdelivr.net/gh/rjman-self/resources/assets/Platdot.png)

`Platdot` 是基于 [ChainBridge](https://github.com/ChainSafe/ChainBridge) 开发的一个跨链项目. 为了完成双端跨链, Chainbridge需要在EVM上部署具有多签功能的智能合约,并在基于substrate开发的链上集成相应的多签模块，但在现有Polkadot和Kusama网络上无法上传自己的模块，我们团队对这一部分功能进行了重新设计和优化，实现了Alaya/PlatON和Kusama/Polkadot的跨链互通。通过Platodt,无需借助交易所即可完成跨链充值和赎，Platdot的多重签名设计实现了去中心化的代币流通，也可以应用于Kusama、ChainX等具有巨大价值的网络。

## 安装

### 依赖项

+ Make sure the Golang environment is installed

+ [Platdot-contract v1.0.0](https://github.com/ChainSafe/chainbridge-solidity): Deploy and configure smart contract in Alaya.

### 构建

`git clone https://github.com/RJman-self/Platdot.git`

`make build`: Builds `platdot` in `./build`.

**or**

`make install`: Uses `go install` to add `platdot` to your `GOBIN`.

## 启动Platdot

查阅 `GitHub Wiki` 文档.

### RunningLocally

[部署Alaya上的智能合约](https://github.com/RJman-self/Platdot/wiki/Deploy-smart-contracts-to-the-alaya-network)

[作为Relayer启动Platdot](https://github.com/RJman-self/Platdot/wiki/Start-A-Relayer)

## License

The project is released under the terms of the `GPLv3`.