package contract

import "math/big"

type BinaryMerkleProof struct {
	SideNodes [][32]byte
	Key       *big.Int
	NumLeaves *big.Int
}
type DataRootTuple struct {
	Height   *big.Int
	DataRoot [32]byte
}

type AttestationProof struct {
	TupleRootNonce *big.Int
	Tuple          DataRootTuple
	Proof          BinaryMerkleProof
}

type Namespace struct {
	Version [1]byte
	Id      [28]byte
}

type NamespaceNode struct {
	Min    Namespace
	Max    Namespace
	Digest [32]byte
}

type NamespaceMerkleMultiproof struct {
	BeginKey  *big.Int
	EndKey    *big.Int
	SideNodes []NamespaceNode
}
