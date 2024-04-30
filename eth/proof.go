package eth

import (
	"context"
	"da-server/eth/contract"
	"fmt"
	"github.com/celestiaorg/celestia-app/pkg/square"
	"github.com/ethereum/go-ethereum"
	ethbind "github.com/ethereum/go-ethereum/accounts/abi/bind"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/rs/zerolog/log"
	bsbindings "github.com/succinctlabs/blobstreamx/bindings"
	"github.com/tendermint/tendermint/crypto/merkle"
	"github.com/tendermint/tendermint/libs/bytes"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	"github.com/tendermint/tendermint/types"
	"math/big"
)

const (
	// Blob index (only the first is supported currently)
	blobIndex = 0
)

func (c *Client) VerifyProofAndRecordWithGasEstimate(celestiaTxHash []byte, transactOpts *ethbind.TransactOpts) (*ethtypes.Transaction, error) {
	transactOpts.NoSend = true
	tx, err := c.VerifyProofAndRecord(celestiaTxHash, transactOpts)
	if err != nil {
		return nil, err
	}
	callMsg := ethereum.CallMsg{
		From:       transactOpts.From,
		To:         &c.ZkLinkContract.Address,
		Gas:        transactOpts.GasLimit,
		GasPrice:   transactOpts.GasPrice,
		GasFeeCap:  transactOpts.GasFeeCap,
		GasTipCap:  transactOpts.GasTipCap,
		Value:      transactOpts.Value,
		Data:       tx.Data(),
		AccessList: nil,
	}
	gas, err := c.Ethereum.EstimateGas(transactOpts.Context, callMsg)
	if err != nil {
		return nil, err
	}
	transactOpts.GasLimit = gas
	transactOpts.NoSend = false
	return c.VerifyProofAndRecord(celestiaTxHash, transactOpts)
}

func (c *Client) VerifyProofAndRecord(celestiaTxHash []byte, transactOpts *ethbind.TransactOpts) (*ethtypes.Transaction, error) {
	ctx := transactOpts.Context
	log.Info().
		Hex("tx_hash", celestiaTxHash).
		Msg("start to process blob")

	// get the nonce corresponding to the block height that contains the PayForBlob transaction
	// since Blobstream X emits events when new batches are submitted, we will query the events
	// and look for the range committing to the blob
	// first, connect to an EVM RPC endpoint

	tx, err := c.Celestia.Tx(ctx, celestiaTxHash, true)
	if err != nil {
		log.Debug().Err(err).Msg("error getting transaction from Celestia RPC server")
		return nil, err
	}
	log.Info().
		Hex("tx_hash", celestiaTxHash).
		Int64("block_height", tx.Height).
		Msg("verifying that the blob was committed to by Blobstream")

	blockRes, err := c.Celestia.Block(ctx, &tx.Height)
	if err != nil {
		log.Debug().Err(err).Msg("error getting block from from Celestia RPC server")
		return nil, err
	}

	event, err := c.getEvent(ctx, tx)
	if err != nil {
		log.Debug().Err(err).Msg("error getting tx event from Celestia RPC server")
		return nil, err
	}

	blobShareRange, err := square.BlobShareRange(blockRes.Block.Txs.ToSliceOfBytes(), int(tx.Index), blobIndex, blockRes.Block.Header.Version.App)
	if err != nil {
		log.Debug().Err(err).
			Uint32("tx_index", tx.Index).
			Int64("blob_index", blobIndex).
			Msg("error getting block share range")
		return nil, err
	}

	log.Info().
		Uint64("shares_start_block", event.StartBlock).
		Uint64("shares_end_block", event.EndBlock).
		Msg("getting root inclusion proof from Celestia RPC server")
	dcProof, err := c.Celestia.DataRootInclusionProof(ctx, uint64(tx.Height), event.StartBlock, event.EndBlock)
	if err != nil {
		log.Debug().Err(err).Msg("error getting root inclusion proof")
		return nil, err
	}

	log.Info().Msg("getting shares proof from Celestia RPC server")
	// get the proof of the shares containing the blob to the data root
	sharesProof, err := c.Celestia.ProveShares(ctx, uint64(tx.Height), uint64(blobShareRange.Start), uint64(blobShareRange.End))
	if err != nil {
		log.Debug().
			Err(err).
			Int64("tx_height", tx.Height).
			Int("start_share", blobShareRange.Start).
			Int("end_share", blobShareRange.End).
			Msg("error getting shares proof")
		return nil, err
	}
	// VerifyProofAndRecord the proof of inclusion
	log.Debug().Msg("verifying shares proofs")
	if !sharesProof.VerifyProof() {
		log.Debug().Err(err).Msg("shares proofs to data root are invalid")
		return nil, err
	}
	log.Info().Msg("shares proofs to data root are valid")

	ethTx, err := c.verifyProofAndRecord(
		sharesProof,
		event.ProofNonce.Uint64(),
		uint64(tx.Height),
		dcProof.Proof,
		blockRes.Block.DataHash,
		transactOpts,
	)
	return ethTx, err
}

func (c *Client) verifyProofAndRecord(
	sharesProof types.ShareProof,
	nonce uint64,
	height uint64,
	dataRootInclusionProof merkle.Proof,
	dataRoot []byte,
	transactOpts *ethbind.TransactOpts,
) (*ethtypes.Transaction, error) {
	var blockDataRoot [32]byte
	copy(blockDataRoot[:], dataRoot)
	tx, err := c.ZkLinkContract.VeriProofAndRecord(
		transactOpts,
		contract.SharesProof{
			Data:             sharesProof.Data,
			ShareProofs:      toNamespaceMerkleMultiProofs(sharesProof.ShareProofs),
			Namespace:        *namespace(sharesProof.NamespaceID),
			RowRoots:         toRowRoots(sharesProof.RowProof.RowRoots),
			RowProofs:        toRowProofs(sharesProof.RowProof.Proofs),
			AttestationProof: toAttestationProof(nonce, height, blockDataRoot, dataRootInclusionProof),
		},
		blockDataRoot,
	)
	return tx, err
}

func (c *Client) getEvent(ctx context.Context, tx *ctypes.ResultTx) (*bsbindings.BlobstreamXDataCommitmentStored, error) {
	LatestBlockNumber, err := c.Ethereum.BlockNumber(ctx)
	if err != nil {
		return nil, err
	}

	eventsIterator, err := c.BlobStreamX.FilterDataCommitmentStored(
		&ethbind.FilterOpts{
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
) contract.AttestationProof {
	sideNodes := make([][32]byte, len(dataRootInclusionProof.Aunts))
	for i, sideNode := range dataRootInclusionProof.Aunts {
		var bzSideNode [32]byte
		for k, b := range sideNode {
			bzSideNode[k] = b
		}
		sideNodes[i] = bzSideNode
	}

	return contract.AttestationProof{
		TupleRootNonce: big.NewInt(int64(nonce)),
		Tuple: contract.DataRootTuple{
			Height:   big.NewInt(int64(height)),
			DataRoot: blockDataRoot,
		},
		Proof: contract.BinaryMerkleProof{
			SideNodes: sideNodes,
			Key:       big.NewInt(dataRootInclusionProof.Index),
			NumLeaves: big.NewInt(dataRootInclusionProof.Total),
		},
	}
}

func toRowRoots(roots []bytes.HexBytes) []contract.NamespaceNode {
	rowRoots := make([]contract.NamespaceNode, len(roots))
	for i, root := range roots {
		rowRoots[i] = *toNamespaceNode(root.Bytes())
	}
	return rowRoots
}

func toRowProofs(proofs []*merkle.Proof) []contract.BinaryMerkleProof {
	rowProofs := make([]contract.BinaryMerkleProof, len(proofs))
	for i, proof := range proofs {
		sideNodes := make([][32]byte, len(proof.Aunts))
		for j, sideNode := range proof.Aunts {
			var bzSideNode [32]byte
			for k, b := range sideNode {
				bzSideNode[k] = b
			}
			sideNodes[j] = bzSideNode
		}
		rowProofs[i] = contract.BinaryMerkleProof{
			SideNodes: sideNodes,
			Key:       big.NewInt(proof.Index),
			NumLeaves: big.NewInt(proof.Total),
		}
	}
	return rowProofs
}

func toNamespaceMerkleMultiProofs(proofs []*tmproto.NMTProof) []contract.NamespaceMerkleMultiproof {
	shareProofs := make([]contract.NamespaceMerkleMultiproof, len(proofs))
	for i, proof := range proofs {
		sideNodes := make([]contract.NamespaceNode, len(proof.Nodes))
		for j, node := range proof.Nodes {
			sideNodes[j] = *toNamespaceNode(node)
		}
		shareProofs[i] = contract.NamespaceMerkleMultiproof{
			BeginKey:  big.NewInt(int64(proof.Start)),
			EndKey:    big.NewInt(int64(proof.End)),
			SideNodes: sideNodes,
		}
	}
	return shareProofs
}

func minNamespace(innerNode []byte) *contract.Namespace {
	version := innerNode[0]
	var id [28]byte
	for i, b := range innerNode[1:28] {
		id[i] = b
	}
	return &contract.Namespace{
		Version: [1]byte{version},
		Id:      id,
	}
}

func maxNamespace(innerNode []byte) *contract.Namespace {
	version := innerNode[29]
	var id [28]byte
	for i, b := range innerNode[30:57] {
		id[i] = b
	}
	return &contract.Namespace{
		Version: [1]byte{version},
		Id:      id,
	}
}

func toNamespaceNode(node []byte) *contract.NamespaceNode {
	minNs := minNamespace(node)
	maxNs := maxNamespace(node)
	var digest [32]byte
	for i, b := range node[58:] {
		digest[i] = b
	}
	return &contract.NamespaceNode{
		Min:    *minNs,
		Max:    *maxNs,
		Digest: digest,
	}
}

func namespace(namespaceID []byte) *contract.Namespace {
	version := namespaceID[0]
	var id [28]byte
	for i, b := range namespaceID[1:] {
		id[i] = b
	}
	return &contract.Namespace{
		Version: [1]byte{version},
		Id:      id,
	}
}
