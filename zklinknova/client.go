package zklinknova

import (
	"context"
	"log"
	"os"
	"time"
)

type Config struct {
	NodeRPCEndpoint string
}

type Client struct {
}

func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	return &Client{}, nil
}

func NewClientFromEnv(ctx context.Context) (*Client, error) {
	cfg := Config{
		NodeRPCEndpoint: os.Getenv("ZKLINK_NOVA_RPC_ENDPOINT"),
	}
	return NewClient(ctx, cfg)
}

func (client *Client) fetchBatchData() (Batch, error) {
	// TODO: implement
	batch := Batch{
		Number: 0,
		Data:   nil,
	}
	return batch, nil
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
				batch, err := client.fetchBatchData()
				if err != nil {
					// TODO: re-transmit mechanism
					return err
				}
				log.Printf("Fetch batch data at %d from zklink nova", batch.Number)
				out <- &batch
			case <-ctx.Done():
				return nil
			}

		}
	}()
	return out, nil
}
