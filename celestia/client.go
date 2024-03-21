package celestia

import (
	"DA-server/zklinknova"
	"context"
	"encoding/hex"
	"github.com/celestiaorg/celestia-node/api/rpc/client"
	"github.com/celestiaorg/celestia-node/blob"
	"github.com/celestiaorg/celestia-node/share"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	NodeRPCEndpoint string
	JWTToken    string
	Namespace   share.Namespace
	GasPrice    blob.GasPrice
}

type Client struct {
	Internal  client.Client
	Namespace share.Namespace
	GasPrice  blob.GasPrice
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
			GasPrice:  cfg.GasPrice},
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
	cfg := Config{
		NodeRPCEndpoint: os.Getenv("CELESTIA_NODE_RPC_ENDPOINT"),
		JWTToken:    os.Getenv("CELESTIA_NODE_JWT_TOKEN"),
		Namespace:   namespace,
		GasPrice:    blob.GasPrice(gasPrice),
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
	commitment blob.Commitment,
) (*blob.Proof, error) {
	namespace := client.Namespace
	return client.Internal.Blob.GetProof(context.Background(), height, namespace, commitment)
}

type DAProof struct {
	Commitment blob.Commitment
	Proof      blob.Proof
}

func (client *Client) Subscribe(ctx context.Context, batches <-chan *zklinknova.Batch) (chan<- *DAProof, error) {
	out := make(chan *DAProof)
	go func() error {
		defer close(out)
		for {
			select {
			case batch := <-batches:
				height, commitment, err := client.SubmitBlob(batch.Data)
				if err != nil {
					// TODO: re-transmit mechanism
					return err
				}
				proof, err := client.GetProof(height, *commitment)
				if err != nil {
					// TODO: re-transmit mechanism
					return err
				}
				out <- &DAProof{
					Commitment: *commitment,
					Proof:      *proof,
				}
			case <-ctx.Done():
				return nil
			}

		}
	}()
	return out, nil
}
