package elements

import (
	"errors"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// ErrClientShuttingDown ...
var ErrClientShuttingDown = errors.New("elements is shutting down")

// OutPoint ...
type OutPoint struct {
	Hash  chainhash.Hash
	Index uint32
}
