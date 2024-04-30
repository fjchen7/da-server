package db

import (
	"github.com/ethereum/go-ethereum/common"
	"time"
)

type Record struct {
	tableName       struct{} `pg:"data"`
	BatchNumber     uint64   `pg:"batch_number,pk"` // Primary key
	Data            []byte
	CommittedHeight uint64 // celestia height the data submits to
	CommittedTxHash []byte // celestia txHash the data submits to
	Commitment      []byte
	ConfirmedToEth  bool      `pg:"default:false"`
	CreatedAt       time.Time `pg:"default:now()"`
	UpdatedAt       time.Time
}

type ConfirmStatus string

const (
	ConfirmStatusConfirmed ConfirmStatus = "confirmed"
	ConfirmStatusFailed    ConfirmStatus = "failed"
)

const HashLength = common.HashLength

type EthTransaction struct {
	tableName     struct{}         `pg:"tx"`
	BatchNumber   uint64           `pg:"batch_number,pk"`
	Hash          [HashLength]byte `pg:"tx_hash,pk"`
	Nonce         uint64
	SentTime      time.Time
	ConfirmStatus ConfirmStatus
	CreatedAt     time.Time `pg:"default:now()"`
	UpdatedAt     time.Time
}
