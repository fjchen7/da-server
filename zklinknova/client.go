package zklinknova

import (
	"context"
	"github.com/ybbus/jsonrpc/v3"
	"log"
	"os"
	"time"
)

type Config struct {
	NodeRPCEndpoint string
}

type Client struct {
	client jsonrpc.RPCClient
}

func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	return &Client{
		client: jsonrpc.NewClient(cfg.NodeRPCEndpoint),
	}, nil
}

func NewClientFromEnv(ctx context.Context) (*Client, error) {
	cfg := Config{
		NodeRPCEndpoint: os.Getenv("ZKLINK_NOVA_RPC_ENDPOINT"),
	}
	return NewClient(ctx, cfg)
}

const RpcMethod = "zks_getL1BatchDA"

func (client *Client) fetchBatchData(blockNumber uint64) (*Batch, error) {
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

func (client *Client) Poll(ctx context.Context, intervalInMillisecond int64) (chan *Batch, error) {
	out := make(chan *Batch)
	interval := time.Duration(intervalInMillisecond * 1000)
	go func() error {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		defer close(out)
		for {
			select {
			case <-ticker.C:
				// TODO: get block number from DB
				batch, err := client.fetchBatchData(0)
				if err != nil {
					// TODO: re-transmit mechanism
					return err
				}
				log.Printf("Fetch batch data at %d from zklink nova", batch.Number)
				out <- batch
			case <-ctx.Done():
				return nil
			}

		}
	}()
	return out, nil
}
