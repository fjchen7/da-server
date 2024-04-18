package eth

import (
	"context"
	"da-server/celestia"
	"log"
	"os"
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
		NodeRPCEndpoint: os.Getenv("ETHEREUM_RPC_ENDPOINT"),
	}
	return NewClient(ctx, cfg)
}

func (client *Client) submitDAProof(daProof *celestia.DAProof) error {
	// TODO: submit da proof to ETH contract
	return nil
}

func (client *Client) Subscribe(ctx context.Context, daProofs <-chan *celestia.DAProof) error {
	go func() error {
		for {
			select {
			case daProof := <-daProofs:
				err := client.submitDAProof(daProof)
				if err != nil {
					return err
				}
				log.Printf("Submit DA Proof at %d to Ethereum Contract", daProof.SubmitHeight)
			case <-ctx.Done():
				return nil
			}

		}
	}()
	return nil
}
