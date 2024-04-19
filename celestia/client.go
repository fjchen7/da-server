package celestia

import (
	"context"
	"da-server/db"
	"da-server/zklinknova"
	"encoding/hex"
	"encoding/json"
	"github.com/celestiaorg/celestia-node/api/rpc/client"
	"github.com/celestiaorg/celestia-node/blob"
	"github.com/celestiaorg/celestia-node/share"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

const L2ToL1MessageByteLength = 88
const NamespaceIdBytesLen = 28
const NamespaceVersionBytesLen = 1
const NamespaceBytesLen = NamespaceIdBytesLen + NamespaceVersionBytesLen
const DataArrayCount = 774
const DataBytesLen = DataArrayCount * L2ToL1MessageByteLength

type Config struct {
	NodeRPCEndpoint string
	JWTToken        string
	Namespace       share.Namespace
	GasPrice        blob.GasPrice
}

type Client struct {
	Internal  client.Client
	Namespace share.Namespace
	GasPrice  blob.GasPrice
	DbClient  *db.Client
}

// NewClient creates a new Client with one connection per Namespace with the
// given token as the authorization token.
func NewClient(ctx context.Context, cfg Config, dbClient *db.Client) (*Client, error) {
	internal, err := client.NewClient(ctx, cfg.NodeRPCEndpoint, cfg.JWTToken)
	if err != nil {
		return nil, err
	}
	return &Client{
			Internal:  *internal,
			Namespace: cfg.Namespace,
			GasPrice:  cfg.GasPrice,
			DbClient:  dbClient,
		},
		nil
}

func NewClientFromEnv(ctx context.Context, dbClient *db.Client) (*Client, error) {
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
		JWTToken:        os.Getenv("CELESTIA_NODE_JWT_TOKEN"),
		Namespace:       namespace,
		GasPrice:        blob.GasPrice(gasPrice),
	}
	return NewClient(ctx, cfg, dbClient)
}

// SubmitBlob submits a blob data to Celestia node.
func (client *Client) SubmitBlob(payLoad []byte) (uint64, *blob.Commitment, error) {
	namespace := client.Namespace
	gasPrice := client.GasPrice
	// Resize to fixed length
	payLoad = append(payLoad, make([]byte, DataBytesLen-len(payLoad))...)
	submittedBlob, err := blob.NewBlobV0(namespace, payLoad)
	if err != nil {
		return 0, nil, err
	}
	commitment := submittedBlob.Commitment
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
	SubmitHeight uint64
	Commitment   blob.Commitment
	Proof        blob.Proof
}

func (client *Client) Subscribe(ctx context.Context, batches <-chan *zklinknova.Batch) (chan *DAProof, error) {
	return nil, nil
}

func (client *Client) Submit() ([]uint64, error) {
	records, err := client.DbClient.GetRecordUnsubmittedToCelestia()
	if err != nil {
		return nil, err
	}
	var submitted []uint64
	for _, record := range records {
		log.Printf("Find uncommitted data with block number %d\n", record.BlockNumber)
		submittedHeight, commitment, err := client.SubmitBlob(record.Data)
		if err != nil {
			// TODO: re-transmit mechanism
			return nil, err
		}
		log.Printf("Commit data with block number %d to Celestia at height %d\n", record.BlockNumber, submittedHeight)
		proof, err := client.GetProof(submittedHeight, *commitment)
		if err != nil {
			// TODO: re-transmit mechanism
			return nil, err
		}
		log.Printf("Get proof at celestia height %d\n", submittedHeight)

		record.SubmittedHeight = submittedHeight
		record.Commitment = *commitment
		record.Proof, err = json.Marshal(proof)
		if err != nil {
			return nil, err
		}

		err = client.DbClient.Update(&record)
		if err != nil {
			return nil, err
		}
		submitted = append(submitted, record.BlockNumber)
		log.Printf("Save data commitment and proof submitted at Celestia height %d to database\n", record.SubmittedHeight)
	}

	return submitted, nil
}

func (client *Client) Run(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		for {
			select {
			case <-ticker.C:
				submitted, err := client.Submit()
				log.Printf("Submit data with block number %v to Celestia\n", submitted)
				if err != nil {
					log.Printf("[Error] Encounter error when submitting data to Celestia: %+v\n", err)
				}
			case <-ctx.Done():
				log.Printf("Celestia submitting data task is cancelled by user\n")
				return
			}

		}
	}()
}
