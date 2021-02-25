package elements

import (
	"io"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/vulpemventures/go-elements/transaction"
)

// MaxBlockPayload is the maximum bytes a block message can be in bytes.
// After Segregated Witness, the max block payload has been raised to 4MB.
const MaxBlockPayload = 4000000

// minTxPayload is the minimum payload size for a transaction.  Note
// that any realistically usable transaction must have at least one
// input or output, but that is a rule enforced at a higher layer, so
// it is intentionally not included here.
// Version 4 bytes + Varint number of transaction inputs 1 byte + Varint
// number of transaction outputs 1 byte + LockTime 4 bytes + min input
// payload + min output payload.
const minTxPayload = 10

// minTxInPayload is the minimum payload size for a transaction input.
// PreviousOutPoint.Hash + PreviousOutPoint.Index 4 bytes + Varint for
// SignatureScript length 1 byte + Sequence 4 bytes.
const minTxInPayload = 9 + chainhash.HashSize

// maxTxPerBlock is the maximum number of transactions that could
// possibly fit into a block.
const maxTxPerBlock = (MaxBlockPayload / minTxPayload) + 1

// MessageEncoding represents the wire message encoding format to be used.
type MessageEncoding uint32

const (
	// BaseEncoding encodes all messages in the default format specified
	// for the Bitcoin wire protocol.
	BaseEncoding MessageEncoding = 1 << iota

	// WitnessEncoding encodes all messages other than transaction messages
	// using the default Bitcoin wire protocol specification. For transaction
	// messages, the new encoding format detailed in BIP0144 will be used.
	WitnessEncoding
)

// LatestEncoding is the most recently specified encoding for the Bitcoin wire
// protocol.
var LatestEncoding = WitnessEncoding

// Block ...
type Block struct {
	Header       wire.BlockHeader
	Transactions []*transaction.Transaction
}

// BlockID ...
type BlockID struct {
	Height    int32
	Hash      chainhash.Hash
	Timestamp time.Time
}

// Deserialize decodes a block from r into the receiver using a format that is
// suitable for long-term storage such as a database while respecting the
// Version field in the block.  This function differs from BtcDecode in that
// BtcDecode decodes from the bitcoin wire protocol as it was sent across the
// network.  The wire encoding can technically differ depending on the protocol
// version and doesn't even really need to match the format of a stored block at
// all.  As of the time this comment was written, the encoded block is the same
// in both instances, but there is a distinct difference and separating the two
// allows the API to be flexible enough to deal with changes.
func (b *Block) Deserialize(r io.Reader) error {
	return nil
}
