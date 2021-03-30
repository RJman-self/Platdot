# Platdot-Overview

## 跨链流程

### Listener和Writer



![ChainBridge](https://cdn.jsdelivr.net/gh/rjman-self/resources/assets/20210330101928.png)

ChainA的代币流通到ChainB：
`ChainA Tx` -> `Deposit` -> `Event` -> `Message` -> `parseMessage` -> `ChainB Tx`

+ 用户在ChainA上发起`交易`进行抵押(Deposit)，与合约交互产生Deposit`Event`
+ ChainA的`Listener`持续监听，从存储中检索到Deposit`Event`，构造`Message`转发给对应ChainB的`Writer`
+ Writer解析Message参数，构造多签交易，