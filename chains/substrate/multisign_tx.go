package substrate

import (
	"github.com/ChainSafe/chainbridge-utils/msg"
	"github.com/rjman-self/go-polkadot-rpc-client/expand"
)

type MultiSignTxId uint64
type BlockNumber int64

type MultiSignTx struct {
	BlockNumber   BlockNumber
	MultiSignTxId MultiSignTxId
}

type MultiSigAsMulti struct {
	OriginMsTx       MultiSignTx
	Executed         bool
	Threshold        uint16
	OtherSignatories []string
	MaybeTimePoint   expand.TimePointSafe32
	DestAddress      string
	DestAmount       string
	StoreCall        bool
	MaxWeight        uint64
}

type DepositTarget struct {
	DestAddress string
	DestAmount  string
}

type DepositNonce struct {
	Nonce  msg.Nonce
	Status bool
}

type MultiSignTxStatistics struct {
	TotalCount    MultiSignTxId
	CurrentTx     MultiSignTx
	DeleteTxCount MultiSignTxId
	DeleteTxId    MultiSignTxId
}
