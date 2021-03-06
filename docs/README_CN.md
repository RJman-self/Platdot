<p align="center"><a href=""><img width="400" title="Platdot" src='https://cdn.jsdelivr.net/gh/rjman-self/resources/assets/platdot-logo.png' /></a></p>

# Platdot

[English](../README.md) | 简体中文

​![build](https://img.shields.io/badge/build-passing-{})    ![test](https://img.shields.io/badge/test-passing-{})   ![release](https://img.shields.io/badge/release-v1.0.0-E6007A)

[communicate with us](https://matrix.to/#/#platdot:matrix.org?via=matrix.org)

## A cross-chain asset transfer solution

`Platdot` 是基于 [ChainBridge](https://github.com/ChainSafe/ChainBridge) 开发的一个跨链项目，为Platon提供Polkadot的跨链桥，实现Pdot的发行、赎回、转账功能。目前，Platdot支持在EVM以及支持multisig-pallet的substrate链（如polkadot / kusama）之间进行跨链转移资产。EVM的智能合约作为桥接的一端，合约允许在接收事务时实现自定的处理行为，例如，polkadot网络上锁定DOT资产，在EVM上执行合约能铸造并发行PDOT资产，同样地，在EVM上执行合约能销毁PDOT资产，并从Polkadot的多重签名地址赎回DOT资产。Platodt当前基于受信任的的联盟模型运行，用户能够以很低的手续费，完成抵押发行和赎回操作。

现在处于测试阶段，实现了Kusama网络和Alaya网络的KSM、AKSM流通

![Platdot-overview](Platdot-overview.png)

## 安装

### 依赖项

+ Make sure the Golang environment is installed

### 构建

`git clone https://github.com/RJman-self/Platdot.git`

`make build`: Builds `platdot` in `./build`.

**or**

`make install`: Uses `go install` to add `platdot` to your `GOBIN`.

## 启动Platdot

查阅下方 `GitHub Wiki` 文档.

[作为Relayer启动Platdot](https://github.com/RJman-self/Platdot/wiki/Start-Platdot-as-a-relayer)

## License

The project is released under the terms of the `GPLv3`.
