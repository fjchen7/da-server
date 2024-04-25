package eth

import (
	"context"
	"da-server/celestia"
	"da-server/db"
	"fmt"
	"github.com/rs/zerolog/log"
	"os"
	"time"
)

type Config struct {
	NodeRPCEndpoint string
}

type Client struct {
	DbClient *db.Client
}

func NewClient(ctx context.Context, cfg Config, dbClient *db.Client) (*Client, error) {
	return &Client{
		DbClient: dbClient,
	}, nil
}

func NewClientFromEnv(ctx context.Context, dbClient *db.Client) (*Client, error) {
	cfg := Config{
		NodeRPCEndpoint: os.Getenv("ETHEREUM_RPC_ENDPOINT"),
	}
	return NewClient(ctx, cfg, dbClient)
}

func (client *Client) submitDAProof(daProof *celestia.DAProof) error {
	// TODO: submit da proof to ETH contract
	return nil
}

func (client *Client) Submit() ([]uint64, error) {
	records, err := client.DbClient.GetRecordUnsubmittedToEth()
	if err != nil {
		return nil, err
	}
	log.Info().
		Msgf("%d Celestia-uncommitted data found in DB", len(records))
	var submitted []uint64
	for _, record := range records {
		// TODO: submit record to Ethereum
		fmt.Printf("Submit data with block number %d to Ethereum\n", record.BatchNumber)
		record.SubmitToEth = true
		err = client.DbClient.Update(&record)
		if err != nil {
			return submitted, err
		}
		submitted = append(submitted, record.BatchNumber)
	}

	return submitted, nil
}

func (client *Client) Run(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		for {
			select {
			case <-ticker.C:
				_, err := client.Submit()
				if err != nil {
					log.Debug().
						Err(err).
						Msg("error submitting data to Ethereum")
				}
			case <-ctx.Done():
				log.Info().Msg("Ethereum submitting data task is cancelled by user")
				return
			}
		}
		ticker.Stop()
	}()
}
