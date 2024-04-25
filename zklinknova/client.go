package zklinknova

import (
	"context"
	"da-server/db"
	"encoding/hex"
	"github.com/ybbus/jsonrpc/v3"
	"log"
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
	var blockNumber uint64
	blockNumber, err := client.DbClient.MaxBlockNumber()
	if err != nil {
		return 0, err
	}
	blockNumber += 1
	log.Printf("Fetching data with block number %d", blockNumber)
	batch, err := client.FetchData(blockNumber)
	if err != nil {
		return blockNumber, err
	}
	if batch == nil {
		return 0, nil
	}
	record := &db.Record{
		BlockNumber:     blockNumber,
		Data:            batch.Data,
		SubmittedHeight: 0,
		SubmittedTxHash: nil,
		Commitment:      nil,
		SubmitToEth:     false,
	}
	err = client.DbClient.Insert(record)
	if err != nil {
		return blockNumber, err
	}

	return blockNumber, nil
}

func (client *Client) Run(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		for {
			select {
			case <-ticker.C:
				blockNumber, err := client.FetchAndStoreData()
				if err != nil {
					log.Printf("[Error] Encounter error when fetching data with block number %d from zklink nova: %s", blockNumber, err)
				} else {
					log.Printf("Fetch data with block number %d and store to DB", blockNumber)
				}
			case <-ctx.Done():
				log.Printf("zklink nova fetching data task is cancelled by user")
				return
			}

		}
	}()
}
