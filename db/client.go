package db

import (
	"context"
	"github.com/go-pg/pg/v10"
	"os"
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

type Record struct {
	BlockNumber     uint64
	Data            []byte
	SubmittedHeight uint64 // celestia height the data submits to
	Commitment      []byte
	Proof           []byte
	SubmitToEth     bool
}

func (c *Client) Insert(record *Record) error {
	_, err := c.Internal.Model(record).Insert()
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) Update(record *Record) error {
	_, err := c.Internal.Model(record).
		Where("block_number = ?", record.BlockNumber).Update()
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) MaxBlockNumber() (uint64, error) {
	record := new(Record)
	err := c.Internal.Model(record).ColumnExpr("Max(block_number)").Select()
	if err != nil {
		return 0, err
	}
	return record.BlockNumber, nil
}

func (c *Client) GetRecord(blockNumber uint64) (*Record, error) {
	record := new(Record)
	err := c.Internal.Model(record).
		Where("block_number = ?", blockNumber).Select()
	if err != nil {
		return nil, err
	}
	return record, nil
}

func (c *Client) GetRecordUnsubmittedToCelestia() ([]Record, error) {
	var records []Record
	err := c.Internal.Model(records).
		Where("submitted_height = 0").Select()
	if err != nil {
		return nil, err
	}
	return records, nil
}

func (c *Client) GetRecordUnsubmittedToEth() ([]Record, error) {
	var records []Record
	err := c.Internal.Model(records).
		Where("submit_to_eth = false").Select()
	if err != nil {
		return nil, err
	}
	return records, nil
}
