# Platdot测试

## 说明

1. `Alaya`是PlatON的先行网
2. `Westend`是Polkadot和Kusama的测试网

## 测试网络配置

### Alaya本地测试网的智能合约地址

#### Bridge

```json
atx1762m2ryuvnnrk3d9q6gfy6whk29n59xu34typ5
```

#### ERC20Handle

```json
atx1t3zvgf73mmhzax24epgv02vqznzw24a5m78cnz
```

#### ERC20

```json
atx1lfhrcc6xectcfe850kf83rcntlw0ha7wck9qjz
```

### Alaya主网的智能合约地址

#### Bridge

```json
atp1ugd2pewannp76f36fk3nt3gt886e4u9d5qrz5x
```

#### ERC20Handle

```json
atp1rd7pjyygepf3r8a8zk8y25n3d3hy249whnuayy
```

#### ERC20

```json
atp1ktl7tjphkm5j48y2qjkvpqtjgyt25qrws4lsd0
```

### Westend测试网络不需要配置

## 测试账户配置

### 本地Polkadot测试网Relayer账户

共同创建一个多签账户SHQWY

#### Hhh

```json
var RelayerSeedOrSecret = "0x68341ec5d0c60361873c98043c1bd7ff840b14d66c518164ac9a95e5fa067443"
var RelayerPublicKey = "0x0a19674301c56a1721feb98dbe93cfab911a8c1bed127f598ef93b374bcc6e71"
var RelayerAddress = "5CHwt8bFyDLC3MyzPQugmmxZTGjShBW2kFMWiC2kSL5TuJxd"
```

#### Qqq

```json
var RelayerSeedOrSecret = "0xf1ecd5c7ae22623fe28a61d909b8bf5eb800af393b164f4586b7831d396f1ff9"
var RelayerPublicKey = "0x50a80eb26a7fb43ff4f84ead705fc61c1d4074112e53f781a6b03c0c7504f663"
var RelayerAddress = "5DtTcR48x8yWkSNwn6G1hjVnY61kCVT2SaGfKfU7WAHRvLKq"
```

#### Sss

```json
var RelayerSeedOrSecret = "0x3c0c4fc26010d0512cd36a0f467375b3dbe2f207bbfda0c551b5e41ee495e909"
var RelayerPublicKey = "0x923eeef27b93315c97e63e0c1284b7433ffbc413a58da0626a63955a48586075"
var RelayerAddress = "5FNTYUQwxjrVE5zRRH1hKh6fZ72AosHB7ThVnNnq9Bv9BFjm"
```

#### Www

```json
var RelayerSeedOrSecret = "0x5e0edc4213f8f443bc88b9ddf94c0e172e2b4b6781857053455fe104b1739479"
var RelayerPublicKey = "0xa45a0ddd81da79f65cbcfeefc8e62382b1f56ccbbdd9533f77cdc49172cca33d"
var RelayerAddress = "5FnCTkAtgLinh6apZJwTX7n72H1A37MHE6xAXChZDbtUWMSg"
```

#### Yyy

```json
var RelayerSeedOrSecret = "0xb98c02ed51d1461d1e8b67eedb58a9ea4531973aede056bc6a477d51e6b31626"
var RelayerPublicKey = "0xe6c2b6c4a5d3a770814f3ebe99893d1bb66e8f0d086a2badfcbb481b043ada1a"
var RelayerAddress = "5HHGiHzTrdbAUFihSC1pgCER41qdrQSxiEBvDsJ2titfAof2"
```

### 本地Alaya测试网Relayer账户

#### RJman

```json
"mainnet":"atp1sy2tvmghdv47hwz89yu9wz2y29nd0frr9jzd2m"
"testnet":"atx1sy2tvmghdv47hwz89yu9wz2y29nd0frr0578e3"
"seed": "8f3980925aa12f9e5e555f641138049571b71e179cf084a007c1e9a671353519"
"password": "chainbridge"
```

#### Hacpy

```json
"mainnet":"atp18hqda4eajphkfarxaa2rutc5dwdwx9z5vy2nmh"
"testnet":"atx18hqda4eajphkfarxaa2rutc5dwdwx9z5xzkega"
"seed": "e5425865ee39b8f995553ee3135c9060b6296c120d4063f45511e3d2a1654266"
"password": "/5f7&=NA)jfr"
```

#### Qqq

```json
"mainnet":"atp1hfhgzgluf9qneuguyw36jrnnsexn388d2lu6xr"
"testnet":"atx1hfhgzgluf9qneuguyw36jrnnsexn388dqeqs4f"
"seed": "8c354461be702d358816a64bff331f37f3e0f1b1949424f9f9a9a88f28e92f53"
"password": "111111"
```

#### Www

```json
"mainnet":"atp1kaptzrfz7v8w4xn7xw88nuxmt5e5uyvhysxqth"
"testnet":"atx1kaptzrfz7v8w4xn7xw88nuxmt5e5uyvhwk62ca"
"seed": "924702bf0810dc5ffa091768381b7197622c98697fbb7660e0c9d5e702ca9d0b"
"password": "111111"
```

#### Yyy

```json
"mainnet":"atp1tdk4rtlu3pv440r96hqn8s33et7kn25730tpjr"
"testnet":"atx1tdk4rtlu3pv440r96hqn8s33et7kn257mfhtpf"
"seed": "2ee254bd1c8d85860170ee09dd66e9f925d2ac74b3d7676bdc9e3d37b7b9bd7a"
"password": "111111"
```

## Platdot运行所需的Config

### Sss

```json
{
  "chains": [
    {
      "name": "alaya",
      "type": "ethereum",
      "id": "222",
      "endpoint": "http://localhost:6789",
      "from": "atx1sy2tvmghdv47hwz89yu9wz2y29nd0frr0578e3",
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
      "endpoint": "ws://127.0.0.1:9944",
      "from": "5FNTYUQwxjrVE5zRRH1hKh6fZ72AosHB7ThVnNnq9Bv9BFjm",
      "latestBlock": "true",
      "opts": {
        "MultiSignAddress": "0x4acbb630c9f7e011af0783bdd8b2a22ab834b8e5bcc1dbf35bd2a8f1ca8ebd8e",
        "TotalRelayer": "5",
        "CurrentRelayerNumber": "1",
        "MultiSignThreshold": "3",
        "OtherRelayer1": "0x0a19674301c56a1721feb98dbe93cfab911a8c1bed127f598ef93b374bcc6e71",
        "OtherRelayer2": "0x50a80eb26a7fb43ff4f84ead705fc61c1d4074112e53f781a6b03c0c7504f663",
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

### Hhh

```json
{
  "chains": [
    {
      "name": "polkadot",
      "type": "substrate",
      "id": "1",
      "endpoint": "ws://127.0.0.1:9944",
      "from": "1EF2TrKpzbfUtzWM3xguvniJtj6PV4Apk5zsV26zR6z5iSq",
      "latestBlock": "true",
      "opts": {
        "MultiSignAddress": "0x4acbb630c9f7e011af0783bdd8b2a22ab834b8e5bcc1dbf35bd2a8f1ca8ebd8e",
        "TotalRelayer": "5",
        "CurrentRelayerNumber": "1",
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

### Qqq

```json
{
  "chains": [
    {
      "name": "alaya",
      "type": "ethereum",
      "id": "222",
      "endpoint": "http://localhost:6789",
      "from": "atx1hfhgzgluf9qneuguyw36jrnnsexn388dqeqs4f",
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
      "endpoint": "ws://127.0.0.1:9944",
      "from": "12pkkkKCovEzByPTjjK1qtKwPi1Pto1AX519UxTU4FJx6iGJ",
      "latestBlock": "true",
      "opts": {
        "MultiSignAddress": "0x4acbb630c9f7e011af0783bdd8b2a22ab834b8e5bcc1dbf35bd2a8f1ca8ebd8e",
        "TotalRelayer": "5",
        "CurrentRelayerNumber": "3",
        "MultiSignThreshold": "3",
        "OtherRelayer1": "0x0a19674301c56a1721feb98dbe93cfab911a8c1bed127f598ef93b374bcc6e71",
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

### Www

```json
{
  "chains": [
    {
      "name": "alaya",
      "type": "ethereum",
      "id": "222",
      "endpoint": "http://localhost:6789",
      "from": "atx1kaptzrfz7v8w4xn7xw88nuxmt5e5uyvhwk62ca",
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
      "endpoint": "ws://127.0.0.1:9944",
      "from": "14iVc5RxY7zG8dbLWwzTfGcFstzojQuRJbgegVgumguzguWQ",
      "latestBlock": "true",
      "opts": {
        "MultiSignAddress": "0x4acbb630c9f7e011af0783bdd8b2a22ab834b8e5bcc1dbf35bd2a8f1ca8ebd8e",
        "TotalRelayer": "5",
        "CurrentRelayerNumber": "4",
        "MultiSignThreshold": "3",
        "OtherRelayer1": "0x0a19674301c56a1721feb98dbe93cfab911a8c1bed127f598ef93b374bcc6e71",
        "OtherRelayer2": "0x50a80eb26a7fb43ff4f84ead705fc61c1d4074112e53f781a6b03c0c7504f663",
        "OtherRelayer3": "0x923eeef27b93315c97e63e0c1284b7433ffbc413a58da0626a63955a48586075",
        "OtherRelayer4": "0xe6c2b6c4a5d3a770814f3ebe99893d1bb66e8f0d086a2badfcbb481b043ada1a",
        "ResourceId": "0x0000000000000000000000000000000000000000000000000000000000000000",
        "MaxWeight": "22698000000",
        "DestId": "222"
      }
    }
  ]
}
```

### Yyy

```json
{
  "chains": [
    {
      "name": "alaya",
      "type": "ethereum",
      "id": "222",
      "endpoint": "http://localhost:6789",
      "from": "atx1tdk4rtlu3pv440r96hqn8s33et7kn257mfhtpf",
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
      "endpoint": "ws://127.0.0.1:9944",
      "from": "16DZrdFXiQrdunjDPq4ppM4ZudqHYi16nivQPAHPSovBM7vd",
      "latestBlock": "true",
      "opts": {
        "MultiSignAddress": "0x4acbb630c9f7e011af0783bdd8b2a22ab834b8e5bcc1dbf35bd2a8f1ca8ebd8e",
        "TotalRelayer": "5",
        "CurrentRelayerNumber": "5",
        "MultiSignThreshold": "3",
        "OtherRelayer1": "0x0a19674301c56a1721feb98dbe93cfab911a8c1bed127f598ef93b374bcc6e71",
        "OtherRelayer2": "0x50a80eb26a7fb43ff4f84ead705fc61c1d4074112e53f781a6b03c0c7504f663",
        "OtherRelayer3": "0x923eeef27b93315c97e63e0c1284b7433ffbc413a58da0626a63955a48586075",
        "OtherRelayer4": "0xa45a0ddd81da79f65cbcfeefc8e62382b1f56ccbbdd9533f77cdc49172cca33d",
        "ResourceId": "0x0000000000000000000000000000000000000000000000000000000000000000",
        "MaxWeight": "22698000000",
        "DestId": "222"
      }
    }
  ]
}
```

## 创建启动密钥

#### Sss

```json
chainbridge accounts import --privateKey 0x3c0c4fc26010d0512cd36a0f467375b3dbe2f207bbfda0c551b5e41ee495e909 --sr25519 &&
chainbridge accounts import --privateKey 8f3980925aa12f9e5e555f641138049571b71e179cf084a007c1e9a671353519 --secp256k1
```

#### Hhh

```json
chainbridge accounts import --privateKey 0x68341ec5d0c60361873c98043c1bd7ff840b14d66c518164ac9a95e5fa067443 --sr25519 &&
chainbridge accounts import --privateKey e5425865ee39b8f995553ee3135c9060b6296c120d4063f45511e3d2a1654266 --secp256k1
```

#### Qqq

```json
chainbridge accounts import --privateKey 0xf1ecd5c7ae22623fe28a61d909b8bf5eb800af393b164f4586b7831d396f1ff9 --sr25519 &&
chainbridge accounts import --privateKey 8c354461be702d358816a64bff331f37f3e0f1b1949424f9f9a9a88f28e92f53 --secp256k1
```

#### Www

```json
chainbridge accounts import --privateKey 0x5e0edc4213f8f443bc88b9ddf94c0e172e2b4b6781857053455fe104b1739479 --sr25519 &&
chainbridge accounts import --privateKey 924702bf0810dc5ffa091768381b7197622c98697fbb7660e0c9d5e702ca9d0b --secp256k1
```

#### Yyy

```json
chainbridge accounts import --privateKey 0xb98c02ed51d1461d1e8b67eedb58a9ea4531973aede056bc6a477d51e6b31626 --sr25519 &&
chainbridge accounts import --privateKey 2ee254bd1c8d85860170ee09dd66e9f925d2ac74b3d7676bdc9e3d37b7b9bd7a --secp256k1
```

## 测试