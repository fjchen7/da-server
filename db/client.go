package db

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/go-pg/pg/v10"
	"os"
	"time"
)

type Config struct {
	Addr     string
	User     string
	Password string
	DbName   string
}

type Client struct {
	Internal *pg.DB
}

func NewConnector(cfg Config) (*Client, error) {
	db := pg.Connect(&pg.Options{
		Addr:     cfg.Addr,
		User:     cfg.User,
		Password: cfg.Password,
		Database: cfg.DbName,
	})

	ctx := context.Background()
	if err := db.Ping(ctx); err != nil {
		return nil, err
	}

	connector := &Client{
		Internal: db,
	}

	return connector, nil
}

func NewConnectorFromEnv() (*Client, error) {
	cfg := Config{
		Addr:     os.Getenv("POSTGRESQL_ADDR"),
		User:     os.Getenv("POSTGRESQL_USER"),
		Password: os.Getenv("POSTGRESQL_PASSWORD"),
		DbName:   os.Getenv("POSTGRESQL_DB_NAME"),
	}
	return NewConnector(cfg)
}

func (c *Client) Close() {
	c.Internal.Close()
}

func (c *Client) InsertRecord(record *Record) error {
	_, err := c.Internal.Model(record).Insert()
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) UpdateRecord(record *Record) error {
	_, err := c.Internal.Model(record).
		Where("batch_number = ?", record.BatchNumber).Update()
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) InsertTransaction(tx *EthTransaction) error {
	_, err := c.Internal.Model(tx).Insert()
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) UpdateTransactionStatus(TxHash [HashLength]byte, status ConfirmStatus) error {
	tx, err := c.Internal.Begin()
	if err != nil {
		return err
	}
	defer tx.Close()
	ctx := context.Background()
	if err := c.Internal.RunInTransaction(ctx, func(dbTx *pg.Tx) error {
		now := time.Now()
		if status == ConfirmStatusConfirmed {
			var batchNumber uint64
			err := dbTx.Model((*EthTransaction)(nil)).
				Column("batch_number").
				Where("hash = ?", TxHash).
				Select(&batchNumber)
			if err != nil {
				return err
			}
			_, err = dbTx.Model((*Record)(nil)).
				Set("confirmed_in_eth = ?", true).
				Set("updated_at = ?", now).
				Where("batch_number = ?", batchNumber).
				Update()
			if err != nil {
				return err
			}
		}
		_, err = dbTx.Model((*EthTransaction)(nil)).
			Set("confirm_status = ?", status).
			Set("updated_at = ?", now).
			Where("hash", TxHash).
			Update()
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (c *Client) MaxBatchNumber() (uint64, error) {
	var blockNumber uint64
	err := c.Internal.Model((*Record)(nil)).ColumnExpr("MAX(record.batch_number)").Select(&blockNumber)
	if err != nil {
		return 0, err
	}
	return blockNumber, nil
}

func (c *Client) GetRecord(batchNumber uint64) (*Record, error) {
	record := new(Record)
	err := c.Internal.Model(record).
		Where("batch_number = ?", batchNumber).Select()
	if err != nil {
		return nil, err
	}
	return record, nil
}

func (c *Client) GetRecordUncommittedToCelestia() ([]Record, error) {
	var records []Record
	err := c.Internal.Model(&records).
		Where("committed_height IS NULL").
		Limit(100).
		Select()
	if err != nil {
		return nil, err
	}
	return records, nil
}

func (c *Client) GetEthTransactions(batchNumber uint64) ([]EthTransaction, error) {
	var txs []EthTransaction
	err := c.Internal.Model(&txs).
		Where("batch_number = ?", batchNumber).
		Select()
	if err != nil {
		return nil, err
	}
	return txs, nil
}
func (c *Client) GetUnfanalizedEthTransactions(batchNumber uint64) ([]EthTransaction, error) {
	var txs []EthTransaction
	err := c.Internal.Model(&txs).
		Where("batch_number = ?", batchNumber).
		Where("confirm_status IS NULL").
		Select()
	if err != nil {
		return nil, err
	}
	return txs, nil
}

func (c *Client) GetUnconfirmedBatchNumbers() ([]uint64, error) {
	var batchNumbers []uint64
	err := c.Internal.Model((*Record)(nil)).
		Column("array_agg(batch_number)").
		Where("confirmed_in_eth = ?", false).
		Select(pg.Array(&batchNumbers))
	if err != nil {
		return nil, err
	}
	return batchNumbers, nil
}

func (c *Client) UpdateConfirmStatusForSingleBatch(txs []EthTransaction) error {
	if len(txs) == 0 {
		return nil
	}
	batchNumber := txs[0].BatchNumber
	batchConfirmed := false
	batchConfirmedTxHash := [HashLength]byte{}
	for _, tx := range txs {
		if batchNumber != tx.BatchNumber {
			return fmt.Errorf("batch number is inconsistent, find %d and %d", batchNumber, tx.BatchNumber)
		}
		if tx.ConfirmStatus == ConfirmStatusConfirmed {
			if !batchConfirmed {
				batchConfirmed = true
				batchConfirmedTxHash = tx.Hash
			} else {
				return fmt.Errorf("find two successful transactions: 0x%s and 0x%s",
					hex.EncodeToString(batchConfirmedTxHash[:]),
					hex.EncodeToString(tx.Hash[:]))
			}
		}
	}

	tx, err := c.Internal.Begin()
	if err != nil {
		return err
	}
	defer tx.Close()
	tx.Model()
	ctx := context.Background()
	if err := c.Internal.RunInTransaction(ctx, func(dbTx *pg.Tx) error {
		now := time.Now()
		_, err := dbTx.Model((*Record)(nil)).
			Set("confirmed_in_eth = ?", batchConfirmed).
			Set("updated_at = ?", now).
			Where("batch_number = ?", batchNumber).
			Update()
		if err != nil {
			return err
		}
		for _, tx := range txs {
			tx.UpdatedAt = now
			_, err = dbTx.
				Model(&tx).
				Column("confirm_status", "updated_at").
				WherePK().
				Update()
			if err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

var NotFound = errors.New("not found")

func (c *Client) GetMaxConfirmedNonce() (uint64, error) {
	return 0, nil
}

// GetRecordUnsubmittedToEth Get all unsubmitted data with block number less than given number
func (c *Client) GetRecordUnsubmittedToEth(maxCelestiaHeight uint64) ([]Record, error) {
	var records []Record
	err := c.Internal.Model(&records).
		Where("confirmed_in_eth = ?", false).
		Where("committed_height < ?", maxCelestiaHeight).
		Select()
	// Should I check if sent time of latest sent tx for this batch number > a given time?
	if err != nil {
		return nil, err
	}
	return records, nil
}
