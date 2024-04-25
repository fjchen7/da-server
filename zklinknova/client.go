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
	client   jsonrpc.RPCClient
	DbClient *db.Client
}

func NewClient(ctx context.Context, cfg Config, dbClient *db.Client) (*Client, error) {
	return &Client{
		client:   jsonrpc.NewClient(cfg.NodeRPCEndpoint),
		DbClient: dbClient,
	}, nil
}

func NewClientFromEnv(ctx context.Context, dbClient *db.Client) (*Client, error) {
	cfg := Config{
		NodeRPCEndpoint: os.Getenv("ZKLINK_NOVA_RPC_ENDPOINT"),
	}
	return NewClient(ctx, cfg, dbClient)
}

const RpcMethod = "zks_getL1BatchDA"

func (client *Client) FetchData(blockNumber uint64) (*Batch, error) {
	params := []interface{}{blockNumber}
	response, err := client.client.Call(context.Background(), RpcMethod, &params)
	if err != nil {
		return nil, err
	}
	result := response.Result.(map[string]interface{})
	// TODO: convert to []byte from hex string
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

func (client *Client) FetchAndStoreData() (uint64, error) {
	var batchNumber uint64
	batchNumber, err := client.DbClient.MaxBatchNumber()
	if err != nil {
		return 0, err
	}
	batchNumber += 1
	log.Info().
		Uint64("batch_number", batchNumber).
		Err(err).
		Msg("fetching data from zklink nova")
	batch, err := client.FetchData(batchNumber)
	if err != nil {
		return batchNumber, err
	}
	if batch == nil {
		return 0, nil
	}
	record := &db.Record{
		BatchNumber:     batchNumber,
		Data:            batch.Data,
		CommittedHeight: 0,
		CommittedTxHash: nil,
		Commitment:      nil,
		SubmitToEth:     false,
	}
	err = client.DbClient.Insert(record)
	if err != nil {
		return batchNumber, err
	}

	return batchNumber, nil
}

func (client *Client) Run(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		for {
			select {
			case <-ticker.C:
				batchNumber, err := client.FetchAndStoreData()
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
