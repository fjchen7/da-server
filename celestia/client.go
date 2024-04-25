package celestia

import (
	"context"
	"cosmossdk.io/math"
	"da-server/db"
	"da-server/zklinknova"
	"encoding/hex"
	"github.com/celestiaorg/celestia-node/api/rpc/client"
	"github.com/celestiaorg/celestia-node/blob"
	"github.com/celestiaorg/celestia-node/share"
	"github.com/celestiaorg/celestia-node/state"
	"log"
	"math/big"
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
	Fee             state.Int
	GasLimit        uint64
}

type Client struct {
	Internal  client.Client
	Namespace share.Namespace
	Fee       state.Int
	GasLimit  uint64
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
			Fee:       cfg.Fee,
			GasLimit:  cfg.GasLimit,
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

	gasLimitStr := os.Getenv("CELESTIA_GAS_LIMIT")
	gasLimit, err := strconv.ParseUint(gasLimitStr, 10, 64)
	if err != nil {
		return nil, err
	}

	feeStr := os.Getenv("CELESTIA_FEE")
	fee, _ := new(big.Int).SetString(feeStr, 10)

	cfg := Config{
		NodeRPCEndpoint: os.Getenv("CELESTIA_NODE_RPC_ENDPOINT"),
		JWTToken:        os.Getenv("CELESTIA_NODE_JWT_TOKEN"),
		Namespace:       namespace,
		Fee:             math.NewIntFromBigInt(fee),
		GasLimit:        gasLimit,
	}
	return NewClient(ctx, cfg, dbClient)
}

type BlobSubmitResponse struct {
	Data       []byte
	TxHash     []byte
	Height     int64
	Commitment blob.Commitment
}

// SubmitBlob submits a blob data to Celestia node.
func (client *Client) SubmitBlob(payLoad []byte) (*BlobSubmitResponse, error) {
	namespace := client.Namespace
	fee := client.Fee
	gasLimit := client.GasLimit
	// Resize to fixed length
	payLoad = append(payLoad, make([]byte, DataBytesLen-len(payLoad))...)
	submittedBlob, err := blob.NewBlobV0(namespace, payLoad)
	if err != nil {
		return nil, err
	}
	commitment := submittedBlob.Commitment
	txRes, err := client.Internal.State.SubmitPayForBlob(context.Background(), fee, gasLimit, []*blob.Blob{submittedBlob})
	if err != nil {
		return nil, err
	}
	txHashStr := txRes.TxHash
	if strings.HasPrefix(txHashStr, "0x") {
		txHashStr = strings.TrimPrefix(txHashStr, "0x")
	}
	txHash, err := hex.DecodeString(txHashStr)
	if err != nil {
		return nil, err
	}

	res := BlobSubmitResponse{
		Data:       payLoad,
		TxHash:     txHash,
		Height:     txRes.Height,
		Commitment: commitment,
	}

	return &res, nil
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
	records, err := client.DbClient.GetRecordUncommittedToCelestia()
	if err != nil {
		return nil, err
	}
	var submitted []uint64
	for _, record := range records {
		log.Printf("Find uncommitted data with block number %d\n", record.BatchNumber)
		res, err := client.SubmitBlob(record.Data)
		if err != nil {
			// TODO: re-transmit mechanism
			return nil, err
		}
		record.CommittedTxHash = res.TxHash
		record.CommittedHeight = uint64(res.Height)
		record.Commitment = res.Commitment
		if err != nil {
			return nil, err
		}

		err = client.DbClient.Update(&record)
		if err != nil {
			return nil, err
		}
		submitted = append(submitted, record.BatchNumber)
		log.Printf("Save data commitment and proof submitted at Celestia height %d to database\n", record.CommittedHeight)
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
