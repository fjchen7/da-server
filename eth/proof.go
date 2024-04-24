package eth

import (
	"context"
	"da-server/eth/client"
	"fmt"
	"github.com/celestiaorg/celestia-app/pkg/square"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	ethcmn "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/rs/zerolog/log"
	bsbindings "github.com/succinctlabs/blobstreamx/bindings"
	"github.com/tendermint/tendermint/crypto/merkle"
	"github.com/tendermint/tendermint/libs/bytes"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	"github.com/tendermint/tendermint/rpc/client/http"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	"github.com/tendermint/tendermint/types"
	"math/big"
)

const (
	// Blob index (only the first is supported currently)
	blobIndex             = 0
	ethereumRpcEndPoint   = "evm_rpc_endpoint"
	celestiaHttpEndPoint  = "tcp://localhost:26657"
	celestiaWebsocketPath = "/websocket"

	zklinkContractAddress = "contract_Address"
	blobStreamContractAddress = "contract_Address"
)

func verify(txHash []byte) error {
	ctx := context.Background()

	log.Info().
		Hex("tx_hash", txHash).
		Msg("start to process blob")

	// get the nonce corresponding to the block height that contains the PayForBlob transaction
	// since Blobstream X emits events when new batches are submitted, we will query the events
	// and look for the range committing to the blob
	// first, connect to an EVM RPC endpoint
	ethClient, err := ethclient.Dial(ethereumRpcEndPoint)
	if err != nil {
		log.Debug().Err(err).Msg("error connecting Ethereum RPC server")
		return err
	}
	defer ethClient.Close()

	// start the Celestia RPC client
	trpc, err := http.New(celestiaHttpEndPoint, celestiaWebsocketPath)
	if err != nil {
		log.Debug().Err(err).Msg("error connecting Celestia RPC server")
		return err
	}
	log.Info().Msg("starting Celestia RPC server")
	err = trpc.Start()
	if err != nil {
		log.Debug().Err(err).Msg("error starting Celestia RPC server")
		return err
	}

	tx, err := trpc.Tx(ctx, txHash, true)
	if err != nil {
		log.Debug().Err(err).Msg("error getting transaction from Celestia RPC server")
		return err
	}
	log.Info().
		Hex("tx_hash", txHash).
		Int64("block_height", tx.Height).
		Msg("verifying that the blob was committed to by Blobstream")

	blockRes, err := trpc.Block(ctx, &tx.Height)
	if err != nil {
		log.Debug().Err(err).Msg("error getting block from from Celestia RPC server")
		return err
	}

	event, err := getEvent(ctx, ethClient, tx)
	if err != nil {
		log.Debug().Err(err).Msg("error getting tx event from Celestia RPC server")
		return err
	}

	blobShareRange, err := square.BlobShareRange(blockRes.Block.Txs.ToSliceOfBytes(), int(tx.Index), blobIndex, blockRes.Block.Header.Version.App)
	if err != nil {
		log.Debug().Err(err).
			Uint32("tx_index", tx.Index).
			Int64("blob_index", blobIndex).
			Msg("error getting block share range")
		return err
		return err
	}

	log.Info().
		Uint64("shares_start_block", event.StartBlock).
		Uint64("shares_end_block", event.EndBlock).
		Msg("getting root inclusion proof from Celestia RPC server")
	dcProof, err := trpc.DataRootInclusionProof(ctx, uint64(tx.Height), event.StartBlock, event.EndBlock)
	if err != nil {
		log.Debug().Err(err).Msg("error getting root inclusion proof")
		return err
	}

	log.Info().Msg("getting shares proof from Celestia RPC server")
	// get the proof of the shares containing the blob to the data root
	sharesProof, err := trpc.ProveShares(ctx, uint64(tx.Height), uint64(blobShareRange.Start), uint64(blobShareRange.End))
	if err != nil {
		log.Debug().
			Err(err).
			Int64("tx_height", tx.Height).
			Int("start_share", blobShareRange.Start).
			Int("end_share", blobShareRange.End).
			Msg("error getting shares proof")
		return err
	}
	// Verify the proof of inclusion
	log.Debug().Msg("verifying shares proofs")
	if !sharesProof.VerifyProof() {
		log.Debug().Err(err).Msg("shares proofs to data root are invalid")
		return err
	}
	log.Info().Msg("shares proofs to data root are valid")

	contractAddress := ethcmn.HexToAddress(zklinkContractAddress)
	contract, err := client.NewWrappers(contractAddress, ethClient)
	if err != nil {
		log.Debug().Err(err).Msg("error creating an Ethereum contract instance")
		return err
	}

	err = verifyProofAndRecord(
		ctx,
		contract,
		sharesProof,
		event.ProofNonce.Uint64(),
		uint64(tx.Height),
		dcProof.Proof,
		blockRes.Block.DataHash,
	)
	if err != nil {
		log.Debug().Err(err).Msg("error executing contract method verify_proof_and_record")
		return err
	}

	return nil
}

func verifyProofAndRecord(
	ctx context.Context,
	contract *client.Wrappers,
	sharesProof types.ShareProof,
	nonce uint64,
	height uint64,
	dataRootInclusionProof merkle.Proof,
	dataRoot []byte,
) error {
	var blockDataRoot [32]byte
	copy(blockDataRoot[:], dataRoot)
	tx, err := contract.VeriProofAndRecord(
		&bind.TransactOpts{
			Context: ctx,
		},
		client.SharesProof{
			Data:             sharesProof.Data,
			ShareProofs:      toNamespaceMerkleMultiProofs(sharesProof.ShareProofs),
			Namespace:        *namespace(sharesProof.NamespaceID),
			RowRoots:         toRowRoots(sharesProof.RowProof.RowRoots),
			RowProofs:        toRowProofs(sharesProof.RowProof.Proofs),
			AttestationProof: toAttestationProof(nonce, height, blockDataRoot, dataRootInclusionProof),
		},
		blockDataRoot,
	)
	_ = tx
	if err != nil {
		return err
	}
	// TODO: wait for transaction
	return nil
}

func getEvent(ctx context.Context, ethClient *ethclient.Client, tx *ctypes.ResultTx) (*bsbindings.BlobstreamXDataCommitmentStored, error) {
	blobStreamX, err := bsbindings.NewBlobstreamX(ethcmn.HexToAddress(blobStreamContractAddress), ethClient)
	if err != nil {
		return nil, err
	}

	LatestBlockNumber, err := ethClient.BlockNumber(ctx)
	if err != nil {
		return nil, err
	}

	eventsIterator, err := blobStreamX.FilterDataCommitmentStored(
		&bind.FilterOpts{
			Context: ctx,
			Start:   LatestBlockNumber - 90000, // 90000 can be replaced with the range of EVM blocks to look for the events in
			End:     &LatestBlockNumber,
		},
		nil,
		nil,
		nil,
	)
	if err != nil {
		return nil, err
	}

	var event *bsbindings.BlobstreamXDataCommitmentStored
	for eventsIterator.Next() {
		e := eventsIterator.Event
		if int64(e.StartBlock) <= tx.Height && tx.Height < int64(e.EndBlock) {
			event = &bsbindings.BlobstreamXDataCommitmentStored{
				ProofNonce:     e.ProofNonce,
				StartBlock:     e.StartBlock,
				EndBlock:       e.EndBlock,
				DataCommitment: e.DataCommitment,
			}
			break
		}
	}
	if err := eventsIterator.Error(); err != nil {
		return nil, err
	}
	err = eventsIterator.Close()
	if err != nil {
		return nil, err
	}
	if event == nil {
		return nil, fmt.Errorf("couldn't find range containing the block height")
	}
	return event, nil
}

func toAttestationProof(
	nonce uint64,
	height uint64,
	blockDataRoot [32]byte,
	dataRootInclusionProof merkle.Proof,
) client.AttestationProof {
	sideNodes := make([][32]byte, len(dataRootInclusionProof.Aunts))
	for i, sideNode := range dataRootInclusionProof.Aunts {
		var bzSideNode [32]byte
		for k, b := range sideNode {
			bzSideNode[k] = b
		}
		sideNodes[i] = bzSideNode
	}

	return client.AttestationProof{
		TupleRootNonce: big.NewInt(int64(nonce)),
		Tuple: client.DataRootTuple{
			Height:   big.NewInt(int64(height)),
			DataRoot: blockDataRoot,
		},
		Proof: client.BinaryMerkleProof{
			SideNodes: sideNodes,
			Key:       big.NewInt(dataRootInclusionProof.Index),
			NumLeaves: big.NewInt(dataRootInclusionProof.Total),
		},
	}
}

func toRowRoots(roots []bytes.HexBytes) []client.NamespaceNode {
	rowRoots := make([]client.NamespaceNode, len(roots))
	for i, root := range roots {
		rowRoots[i] = *toNamespaceNode(root.Bytes())
	}
	return rowRoots
}

func toRowProofs(proofs []*merkle.Proof) []client.BinaryMerkleProof {
	rowProofs := make([]client.BinaryMerkleProof, len(proofs))
	for i, proof := range proofs {
		sideNodes := make([][32]byte, len(proof.Aunts))
		for j, sideNode := range proof.Aunts {
			var bzSideNode [32]byte
			for k, b := range sideNode {
				bzSideNode[k] = b
			}
			sideNodes[j] = bzSideNode
		}
		rowProofs[i] = client.BinaryMerkleProof{
			SideNodes: sideNodes,
			Key:       big.NewInt(proof.Index),
			NumLeaves: big.NewInt(proof.Total),
		}
	}
	return rowProofs
}

func toNamespaceMerkleMultiProofs(proofs []*tmproto.NMTProof) []client.NamespaceMerkleMultiproof {
	shareProofs := make([]client.NamespaceMerkleMultiproof, len(proofs))
	for i, proof := range proofs {
		sideNodes := make([]client.NamespaceNode, len(proof.Nodes))
		for j, node := range proof.Nodes {
			sideNodes[j] = *toNamespaceNode(node)
		}
		shareProofs[i] = client.NamespaceMerkleMultiproof{
			BeginKey:  big.NewInt(int64(proof.Start)),
			EndKey:    big.NewInt(int64(proof.End)),
			SideNodes: sideNodes,
		}
	}
	return shareProofs
}

func minNamespace(innerNode []byte) *client.Namespace {
	version := innerNode[0]
	var id [28]byte
	for i, b := range innerNode[1:28] {
		id[i] = b
	}
	return &client.Namespace{
		Version: [1]byte{version},
		Id:      id,
	}
}

func maxNamespace(innerNode []byte) *client.Namespace {
	version := innerNode[29]
	var id [28]byte
	for i, b := range innerNode[30:57] {
		id[i] = b
	}
	return &client.Namespace{
		Version: [1]byte{version},
		Id:      id,
	}
}

func toNamespaceNode(node []byte) *client.NamespaceNode {
	minNs := minNamespace(node)
	maxNs := maxNamespace(node)
	var digest [32]byte
	for i, b := range node[58:] {
		digest[i] = b
	}
	return &client.NamespaceNode{
		Min:    *minNs,
		Max:    *maxNs,
		Digest: digest,
	}
}

func namespace(namespaceID []byte) *client.Namespace {
	version := namespaceID[0]
	var id [28]byte
	for i, b := range namespaceID[1:] {
		id[i] = b
	}
	return &client.Namespace{
		Version: [1]byte{version},
		Id:      id,
	}
}
