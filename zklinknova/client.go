package zklinknova

import (
	"context"
	"da-server/db"
	"encoding/hex"
	"github.com/rs/zerolog/log"
	"github.com/ybbus/jsonrpc/v3"
	"os"
	"time"
)

type Config struct {
	NodeRPCEndpoint string
}

type Client struct {
	client jsonrpc.RPCClient
	Db     *db.Client
}

func NewClient(ctx context.Context, cfg Config, dbClient *db.Client) (*Client, error) {
	return &Client{
		client: jsonrpc.NewClient(cfg.NodeRPCEndpoint),
		Db:     dbClient,
	}, nil

}

func NewClientFromEnv(ctx context.Context, dbClient *db.Client) (*Client, error) {
	cfg := Config{
		NodeRPCEndpoint: os.Getenv("ZKLINK_NOVA_RPC_ENDPOINT"),
	}
	return NewClient(ctx, cfg, dbClient)
}

const RpcMethod = "zks_getL1BatchDA"

func (c *Client) FetchData(blockNumber uint64) (*Batch, error) {
	params := []interface{}{blockNumber}
	response, err := c.client.Call(context.Background(), RpcMethod, &params)
	if err != nil {
		return nil, err
	}
	result := response.Result.(map[string]interface{})
	dataHex := result["data"].(string)[2:]
	data, err := hex.DecodeString(dataHex)
	if err != nil {
		return nil, err
	}
	batch := Batch{
		Number: blockNumber,
		Data:   data,
	}
	return &batch, nil
}

type Batch struct {
	Number uint64
	Data   []byte
}

func (c *Client) FetchAndStoreData() (uint64, error) {
	var batchNumber uint64
	batchNumber, err := c.Db.MaxBatchNumber()
	if err != nil {
		return 0, err
	}
	batchNumber += 1
	log.Info().
		Uint64("batch_number", batchNumber).
		Err(err).
		Msg("fetching data from zklink nova")
	batch, err := c.FetchData(batchNumber)
	if err != nil {
		return batchNumber, err
	}
	if batch == nil {
		return 0, nil
	}
	record := &db.Record{
		BatchNumber:             batchNumber,
		Data:                    batch.Data,
		CelestiaCommittedHeight: 0,
		CelestiaCommittedTxHash: nil,
		CelestiaCommitment:      nil,
		ConfirmedInEth:          false,
	}
	err = c.Db.InsertRecord(record)
	if err != nil {
		return batchNumber, err
	}

	return batchNumber, nil
}

func (c *Client) Run(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		for {
			select {
			case <-ticker.C:
				batchNumber, err := c.FetchAndStoreData()
				if err != nil {
					log.Debug().
						Uint64("batch_number", batchNumber).
						Err(err).
						Msg("error store data from ZkLink Nova to DB")
				} else {
					log.Debug().
						Uint64("batch_number", batchNumber).
						Msg("fetch data from ZkLink Nova and store to DB")
				}
			case <-ctx.Done():
				log.Info().Msg("ZkLink Nova fetching data task is cancelled by user")
				return
			}

		}
	}()
}
