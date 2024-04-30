package contract

import (
	ethbind "github.com/ethereum/go-ethereum/accounts/abi/bind"
	ethcmn "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type Wrappers struct {
	Address ethcmn.Address
}

type SharesProof struct {
	Data             [][]byte
	ShareProofs      []NamespaceMerkleMultiproof
	Namespace        Namespace
	RowRoots         []NamespaceNode
	RowProofs        []BinaryMerkleProof
	AttestationProof AttestationProof
}

func NewWrappers(address string, client *ethclient.Client) (*Wrappers, error) {
	w := &Wrappers{
		Address: ethcmn.HexToAddress(address),
	}
	return w, nil
}

func (w *Wrappers) VeriProofAndRecord(txOpts *ethbind.TransactOpts, sharesProof SharesProof, blockDataRoot [32]byte) (*types.Transaction, error) {
	// TODO
	return nil, nil
}
