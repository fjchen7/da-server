
package client

import (
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

type Wrappers struct {
	
}

type SharesProof struct {
	Data [][]byte
	ShareProofs []NamespaceMerkleMultiproof
	Namespace Namespace
	RowRoots []NamespaceNode
	RowProofs []BinaryMerkleProof
	AttestationProof AttestationProof
}

func NewWrappers(address common.Address, client *ethclient.Client) (*Wrappers, error) {
	// TODO
	return nil, nil
}

func (w Wrappers) VeriProofAndRecord(txOpts *bind.TransactOpts, sharesProof SharesProof, blockDataRoot [32]byte) (int, error) {
	// TODO
	return 0, nil

}