package zklinknova

import (
	"context"
	"da-server/db"
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

func NewClientFromEnv(ctx context.Context) (*Client, error) {
	return nil, nil
}

func NewClientFromEnv1(ctx context.Context, dbClient *db.Client) (*Client, error) {
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
	data := result["data"].([]byte)
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
	batch, err := client.FetchData(blockNumber)
	if err != nil {
		return blockNumber, err
	}
	record := &db.Record{
		BlockNumber:     blockNumber,
		Data:            batch.Data,
		SubmittedHeight: 0,
		Commitment:      nil,
		Proof:           nil,
		SubmitToEth:     false,
	}
	err = client.DbClient.Insert(record)
	if err != nil {
		return blockNumber, err
	}

	return blockNumber, nil
}

func (client *Client) Poll(ctx context.Context, intervalInMillisecond int64) (chan *Batch, error) {
	return nil, nil
}

func (client *Client) Run(ctx context.Context, intervalInMillisecond int64) {
	interval := time.Duration(intervalInMillisecond * 1000)
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
