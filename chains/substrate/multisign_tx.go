package substrate

import (
	"github.com/centrifuge/go-substrate-rpc-client/v2/types"
	"github.com/rjman-self/go-polkadot-rpc-client/expand"
	"github.com/rjman-self/platdot-utils/msg"
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
	DepositNonce     msg.Nonce
	YesVote          []types.AccountID
}

type MultiSignTxStatistics struct {
	TotalCount    MultiSignTxId
	CurrentTx     MultiSignTx
}
