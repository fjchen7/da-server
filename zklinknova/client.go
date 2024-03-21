package zklinknova

import (
	"context"
	"os"
	"time"
)

type Config struct {
	NodeRPCHost string
}

type Client struct {
}

func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	return &Client{}, nil
}

func NewClientFromEnv(ctx context.Context) (*Client, error) {
	cfg := Config{
		NodeRPCHost: os.Getenv("ZKLINK_NOVA_RPC_HOST"),
	}
	return NewClient(ctx, cfg)
}

func (client *Client) fetchBatchData() (Batch, error) {
	// TODO: implement
	batch := Batch{
		Number: 0,
		data:   nil,
	}
	return batch, nil
}

type Batch struct {
	Number uint64
	data []byte
}

func (client *Client) Subscribe(ctx context.Context, interval time.Duration) (chan<- *Batch, error) {
	out := make(chan *Batch)
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
				out <- &batch
			case <-ctx.Done():
				return nil
			}

		}
	}()
	return out, nil
}
