package btcutil

import (
	"fmt"
	"io"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// UBlock is a regular block, with Udata stuck on
type UBlock struct {
	msgUBlock *wire.MsgUBlock

	serializedUBlock []byte // Serialized bytes for the block

	// TODO get to segwit
	serializedUBlockNoWitness []byte          // Serialized bytes for block w/o witness data
	blockHash                 *chainhash.Hash // Cached block hash
	blockHeight               int32           // Height in the main block chain
	transactions              []*Tx           // Transactions
	txnsGenerated             bool            // ALL wrapped transactions generated
}

// ProofSanity checks the consistency of a UBlock.  Does the proof prove
// all the inputs in the block?
func (ub *UBlock) ProofSanity(inputSkipList []uint32, nl uint64, h uint8) error {
	// get the outpoints that need proof
	proveOPs := BlockToDelOPs(ub.msgUBlock.MsgBlock, inputSkipList)

	// ensure that all outpoints are provided in the extradata
	if len(proveOPs) != len(ub.msgUBlock.UtreexoData.Stxos) {
		err := fmt.Errorf("height %d %d outpoints need proofs but only %d proven\n",
			ub.msgUBlock.UtreexoData.Height, len(proveOPs), len(ub.msgUBlock.UtreexoData.Stxos))
		return err
	}
	for i, _ := range ub.msgUBlock.UtreexoData.Stxos {
		if chainhash.Hash(proveOPs[i].Hash) != chainhash.Hash(ub.msgUBlock.UtreexoData.Stxos[i].TxHash) ||
			proveOPs[i].Index != ub.msgUBlock.UtreexoData.Stxos[i].Index {
			err := fmt.Errorf("block/utxoData mismatch %s v %s\n",
				proveOPs[i].String(), ub.msgUBlock.UtreexoData.Stxos[i].OPString())
			return err
		}
	}
	// derive leafHashes from leafData
	if !ub.msgUBlock.UtreexoData.ProofSanity(nl, h) {
		return fmt.Errorf("height %d LeafData / Proof mismatch", ub.msgUBlock.UtreexoData.Height)
	}

	return nil
}

// blockToDelOPs gives all the UTXOs in a block that need proofs in order to be
// deleted.  All txinputs except for the coinbase input and utxos created
// within the same block (on the skiplist)
func BlockToDelOPs(
	blk *wire.MsgBlock, skiplist []uint32) (delOPs []wire.OutPoint) {

	var blockInIdx uint32
	for txinblock, tx := range blk.Transactions {
		if txinblock == 0 {
			blockInIdx++ // coinbase tx always has 1 input
			continue
		}

		// loop through inputs
		for _, txin := range tx.TxIn {
			// check if on skiplist.  If so, don't make leaf
			if len(skiplist) > 0 && skiplist[0] == blockInIdx {
				// fmt.Printf("skip %s\n", txin.PreviousOutPoint.String())
				skiplist = skiplist[1:]
				blockInIdx++
				continue
			}

			delOPs = append(delOPs, txin.PreviousOutPoint)
			blockInIdx++
		}
	}
	return
}

// Block builds a block from the UBlock. For compatibility with some functions
// that want a block
func (ub *UBlock) Block() *Block {
	block := Block{
		msgBlock: ub.msgUBlock.MsgBlock,
		//serializedBlock:,
		//serializedBlockNoWitness:,
		blockHash:     ub.blockHash,
		blockHeight:   ub.blockHeight,
		transactions:  ub.transactions,
		txnsGenerated: ub.txnsGenerated,
	}
	return &block
}

func (ub *UBlock) MsgUBlock() *wire.MsgUBlock {
	return ub.msgUBlock
}

func (b *UBlock) Height() int32 {
	return b.blockHeight
}

func (b *UBlock) SetHeight(height int32) {
	b.blockHeight = height
}

func (ub *UBlock) Transactions() []*Tx {
	// Return transactions if they have ALL already been generated.  This
	// flag is necessary because the wrapped transactions are lazily
	// generated in a sparse fashion.
	if ub.txnsGenerated {
		return ub.transactions
	}

	// Generate slice to hold all of the wrapped transactions if needed.
	if len(ub.transactions) == 0 {
		ub.transactions = make([]*Tx, len(ub.msgUBlock.MsgBlock.Transactions))
	}

	// Generate and cache the wrapped transactions for all that haven't
	// already ubeen done.
	for i, tx := range ub.transactions {
		if tx == nil {
			newTx := NewTx(ub.msgUBlock.MsgBlock.Transactions[i])
			newTx.SetIndex(i)
			ub.transactions[i] = newTx
		}
	}

	ub.txnsGenerated = true

	return ub.transactions
}

// Hash returns the block identifier hash for the Block.  This is equivalent to
// calling BlockHash on the underlying wire.MsgBlock, however it caches the
// result so subsequent calls are more efficient.
func (ub *UBlock) Hash() *chainhash.Hash {
	// Return the cached block hash if it has already been generated.
	if ub.blockHash != nil {
		return ub.blockHash
	}

	// Cache the block hash and return it.
	hash := ub.msgUBlock.BlockHash()
	ub.blockHash = &hash
	return &hash
}

// Deserialize a UBlock.  It's just a block then udata.
func (ub *UBlock) Deserialize(r io.Reader) (err error) {
	err = ub.Deserialize(r)
	if err != nil {
		return err
	}
	err = ub.msgUBlock.UtreexoData.Deserialize(r)
	return
}

// We don't actually call serialize since from the server side we don't
// serialize, we just glom stuff together from the disk and send it over.
func (ub *UBlock) Serialize(w io.Writer) (err error) {
	err = ub.Serialize(w)
	if err != nil {
		return
	}
	err = ub.msgUBlock.UtreexoData.Serialize(w)
	return
}

// SerializeSize: how big is it, in bytes.
func (ub *UBlock) SerializeSize() int {
	return ub.SerializeSize() + ub.msgUBlock.UtreexoData.SerializeSize()
}

// NewBlockFromBlockAndBytes returns a new instance of a bitcoin block given
// an underlying wire.MsgBlock and the serialized bytes for it.  See Block.
func NewUBlockFromBlockAndBytes(msgUBlock *wire.MsgUBlock, serializedUBlock []byte) *UBlock {
	return &UBlock{
		msgUBlock:        msgUBlock,
		serializedUBlock: serializedUBlock,
		blockHeight:      BlockHeightUnknown,
	}
}
