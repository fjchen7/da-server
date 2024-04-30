package eth

import (
	"context"
	"da-server/db"
	"da-server/eth/contract"
	"fmt"
	"github.com/ethereum/go-ethereum"
	ethbind "github.com/ethereum/go-ethereum/accounts/abi/bind"
	ethcmn "github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/rs/zerolog/log"
	bsbindings "github.com/succinctlabs/blobstreamx/bindings"
	"github.com/tendermint/tendermint/rpc/client/http"
	"math/big"
	"os"
	"time"
)

const (
	ethereumRpcEndPoint   = "evm_rpc_endpoint"
	celestiaHttpEndPoint  = "tcp://localhost:26657"
	celestiaWebsocketPath = "/websocket"

	zklinkContractAddress     = "contract_Address"
	blobStreamContractAddress = "contract_Address"
)

type Config struct {
	EthereumNodeRPCEndpoint   string
	CelestiaHttpEndPoint      string
	CelestiaWebsocketPath     string
	ZkLinkContractAddress     string
	BlobStreamContractAddress string
}

type Client struct {
	Db             *db.Client
	Ethereum       *ethclient.Client
	BlobStreamX    *bsbindings.BlobstreamX
	Celestia       *http.HTTP
	ZkLinkContract *contract.Wrappers
}

func NewClient(cfg Config, db *db.Client, eth *ethclient.Client) (*Client, error) {
	// TODO: set from environment
	blobStreamX, err := bsbindings.NewBlobstreamX(ethcmn.HexToAddress(cfg.BlobStreamContractAddress), eth)
	if err != nil {
		return nil, err
	}
	// start the Celestia RPC
	celestia, err := http.New(cfg.CelestiaHttpEndPoint, cfg.CelestiaWebsocketPath)
	if err != nil {
		log.Debug().Err(err).Msg("error connecting Celestia RPC server")
		return nil, err
	}
	log.Info().Msg("starting Celestia RPC server")
	err = celestia.Start()
	// Init zkLink contract instance
	zkLinkContract, err := contract.NewWrappers(cfg.ZkLinkContractAddress, eth)
	if err != nil {
		log.Debug().Err(err).Msg("error creating an Ethereum contractInstance instance")
		return nil, err
	}
	return &Client{
		Db:             db,
		Ethereum:       eth,
		BlobStreamX:    blobStreamX,
		Celestia:       celestia,
		ZkLinkContract: zkLinkContract,
	}, nil
}

func NewClientFromEnv(dbClient *db.Client, ethClient *ethclient.Client) (*Client, error) {
	cfg := Config{
		EthereumNodeRPCEndpoint: os.Getenv("ETHEREUM_RPC_ENDPOINT"),
	}
	return NewClient(cfg, dbClient, ethClient)
}

func (c *Client) FetchBlobStreamXLatestNumber() (uint64, error) {
	return c.BlobStreamX.LatestBlock(
		&ethbind.CallOpts{
			Pending: false,
		})
}

func (c *Client) GetLastUsedNonce(ctx context.Context) (uint64, error) {
	_blockNumber, err := c.Ethereum.BlockNumber(ctx)
	if err != nil {
		return 0, nil
	}
	blockNumber := big.NewInt(0)
	blockNumber.SetUint64(_blockNumber)
	return c.Ethereum.NonceAt(ctx, c.ZkLinkContract.Address, blockNumber)
}

func (c *Client) Submit(ctx context.Context, celestiaLatestHeight uint64) ([]uint64, error) {
	records, err := c.Db.GetRecordUnsubmittedToEth(celestiaLatestHeight)
	if err != nil {
		return nil, err
	}
	log.Info().
		Msgf("%d ETH uncommitted data found in DB", len(records))
	var submitted []uint64
	nonce, err := c.GetLastUsedNonce(ctx)
	if err != nil {
		return nil, err
	}
	// TODO: Set Transaction Opts
	txOpts := &ethbind.TransactOpts{
		From:      ethcmn.Address{},
		Nonce:     big.NewInt(0),
		Signer:    nil,
		Value:     nil,
		GasPrice:  nil,
		GasFeeCap: nil,
		GasTipCap: nil,
		GasLimit:  0,
		Context:   ctx,
		NoSend:    false,
	}
	for _, record := range records {
		nonce++
		txOpts.Nonce.SetUint64(nonce)
		tx, err := c.VerifyProofAndRecordWithGasEstimate(record.CelestiaCommittedTxHash, txOpts)
		if err != nil {
			log.Debug().
				Uint64("batch_number", record.BatchNumber).
				Hex("celestia_tx_hash", record.CelestiaCommittedTxHash).
				Err(err).
				Msg("error verify and record data in Ethereum")
			continue
		}
		dbTx := &db.EthTransaction{
			BatchNumber: record.BatchNumber,
			Hash:        tx.Hash(),
			Nonce:       tx.Nonce(),
			SentTime:    tx.Time(),
		}
		err = c.Db.InsertTransaction(dbTx)
		if err != nil {
			log.Debug().
				Uint64("batch_number", record.BatchNumber).
				Hex("celestia_tx_hash", record.CelestiaCommittedTxHash).
				Hex("eth_tx_hash", dbTx.Hash[:]).
				Err(err).
				Msg("error verify and record data in Ethereum")
			return submitted, err
		}
		submitted = append(submitted, record.BatchNumber)
	}
	return submitted, nil
}

func (c *Client) RunSubmit(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		for {
			select {
			case <-ticker.C:
				latestHeight, err := c.FetchBlobStreamXLatestNumber()
				if err != nil {
					log.Debug().
						Err(err).
						Msg("error fetch latest celestia synced block number in Ethereum")
					continue
				}
				_, err = c.Submit(ctx, latestHeight)
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

func (c *Client) RunMonitorTx(ctx context.Context, interval time.Duration) {
	// Logic
	// 1. Find all unfanalized batch numbers.
	// 2. Find all sent txs for each batch number
	// 3. Getting tx receipt and check tx status.
	// 4. Update txs to DB. If tx is confirmed, update the corresponding line in table `data`.
	go func() {
		ticker := time.NewTicker(interval)
		for {
			select {
			case <-ticker.C:
				// Get all unconfirmed batch
				unconfirmedBatchNumbers, err := c.Db.GetUnconfirmedBatchNumbers()
				if err != nil {
					log.Debug().
						Err(err).
						Msg("error getting unconfirmed batch numbers from DB")
					continue
				}
				for _, batchNumber := range unconfirmedBatchNumbers {
					log.Info().
						Uint64("batch_number", batchNumber).
						Msg("confirming data submitting transaction status in Ethereum")
					err = c.checkAndUpdateUnconfirmedBatchNumberStatus(ctx, batchNumber)
					if err != nil {
						log.Debug().
							Err(err).
							Uint64("batch_number", batchNumber).
							Msg("error checking unconfirmed txs from Ethereum")
					}
				}
			case <-ctx.Done():
				log.Info().Msg("Ethereum submitting data task is cancelled by user")
				return
			}
		}
		ticker.Stop()
	}()
}

func (c *Client) checkAndUpdateUnconfirmedBatchNumberStatus(ctx context.Context, batchNumber uint64) error {
	txs, err := c.Db.GetUnfanalizedEthTransactions(batchNumber)
	if err != nil {
		return nil
	}
	for _, tx := range txs {
		receipt, err := c.Ethereum.TransactionReceipt(ctx, tx.Hash)
		if err == ethereum.NotFound {
			continue
		}
		if err != nil {
			return err
		}
		if receipt.Status == ethtypes.ReceiptStatusSuccessful {
			tx.ConfirmStatus = db.ConfirmStatusConfirmed
		} else if receipt.Status == ethtypes.ReceiptStatusFailed {
			tx.ConfirmStatus = db.ConfirmStatusFailed
		} else {
			return fmt.Errorf("unknow Ethereum tx status: %d", receipt.Status)
		}
		err = c.Db.UpdateTransactionStatus(tx.Hash, tx.ConfirmStatus)
		if err != nil {
			return err
		}
	}
	return nil
}
