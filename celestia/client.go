package celestia

import (
	"DA-server/zklinknova"
	"context"
	"encoding/hex"
	"github.com/celestiaorg/celestia-node/api/rpc/client"
	"github.com/celestiaorg/celestia-node/blob"
	"github.com/celestiaorg/celestia-node/share"
	"log"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	NodeRPCEndpoint string
	JWTToken        string
	Namespace       share.Namespace
	GasPrice        blob.GasPrice
	DBConfig        DBConfig
}

type Client struct {
	Internal  client.Client
	Namespace share.Namespace
	GasPrice  blob.GasPrice
	DBConfig  DBConfig
}

// NewClient creates a new Client with one connection per Namespace with the
// given token as the authorization token.
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	internal, err := client.NewClient(ctx, cfg.NodeRPCEndpoint, cfg.JWTToken)
	if err != nil {
		return nil, err
	}
	return &Client{
			Internal:  *internal,
			Namespace: cfg.Namespace,
			GasPrice:  cfg.GasPrice,
			DBConfig:  cfg.DBConfig,
		},
		nil
}

func NewClientFromEnv(ctx context.Context) (*Client, error) {
	namespaceHexStr := os.Getenv("CELESTIA_NAMESPACE")
	if strings.HasPrefix(namespaceHexStr, "0x") {
		namespaceHexStr = strings.TrimPrefix(namespaceHexStr, "0x")
	}
	namespace, err := hex.DecodeString(namespaceHexStr)
	if err != nil {
		return nil, err
	}

	gasPriceStr := os.Getenv("CELESTIA_GASPRICE")
	gasPrice, err := strconv.ParseFloat(gasPriceStr, 64)
	if err != nil {
		return nil, err
	}

	portStr := os.Getenv("POSTGRESQL_PORT")
	port, err := strconv.ParseUint(portStr, 10, 32)
	if err != nil {
		return nil, err
	}
	dbConfig := DBConfig{
		host:     os.Getenv("POSTGRESQL_HOST"),
		port:     port,
		user:     os.Getenv("POSTGRESQL_USER"),
		password: os.Getenv("POSTGRESQL_PASSWORD"),
		dbname:   os.Getenv("POSTGRESQL_DBNAME"),
	}

	cfg := Config{
		NodeRPCEndpoint: os.Getenv("CELESTIA_NODE_RPC_ENDPOINT"),
		JWTToken:        os.Getenv("CELESTIA_NODE_JWT_TOKEN"),
		Namespace:       namespace,
		GasPrice:        blob.GasPrice(gasPrice),
		DBConfig:        dbConfig,
	}
	return NewClient(ctx, cfg)
}

// SubmitBlob submits a blob data to Celestia node.
func (client *Client) SubmitBlob(payLoad []byte) (uint64, *blob.Commitment, error) {
	namespace := client.Namespace
	gasPrice := client.GasPrice
	submittedBlob, err := blob.NewBlobV0(namespace, payLoad)
	if err != nil {
		return 0, nil, err
	}

	// TODO: calculate commitment
	commitment := blob.Commitment([]byte{})
	height, err := client.Internal.Blob.Submit(context.Background(), []*blob.Blob{submittedBlob}, gasPrice)
	if err != nil {
		return 0, nil, err
	}
	return height, &commitment, nil
}

func (client *Client) GetProof(
	height uint64,
	commitment blob.Commitment) (*blob.Proof, error) {
	namespace := client.Namespace
	return client.Internal.Blob.GetProof(context.Background(), height, namespace, commitment)
}

type DAProof struct {
	Height     uint64
	Commitment blob.Commitment
	Proof      blob.Proof
}

func (client *Client) Subscribe(ctx context.Context, batches <-chan *zklinknova.Batch) (chan *DAProof, error) {
	out := make(chan *DAProof)
	dbConnector, err := NewConnector(client.DBConfig)
	if err != nil {
		return nil, err
	}
	go func() error {
		defer close(out)
		defer dbConnector.Close()
		for {
			select {
			case batch := <-batches:
				height, commitment, err := client.SubmitBlob(batch.Data)
				if err != nil {
					// TODO: re-transmit mechanism
					return err
				}
				log.Printf("Submit batch at %d with commitment %s to Celestia", height, hex.EncodeToString(*commitment))
				proof, err := client.GetProof(height, *commitment)
				if err != nil {
					// TODO: re-transmit mechanism
					return err
				}
				log.Printf("Fetch DA Proof at %d from Celestia", height)
				daProof := DAProof{
					Height:     height,
					Commitment: *commitment,
					Proof:      *proof,
				}
				err = dbConnector.StoreDAProof(daProof)
				if err != nil {
					return err
				}
				log.Printf("Store DA Proof at %d to database", daProof.Height)
				out <- &daProof
			case <-ctx.Done():
				return nil
			}

		}
	}()
	return out, nil
}
