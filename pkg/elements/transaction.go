package elements

import (
	"time"

	"github.com/vulpemventures/go-elements/transaction"
)

// TransactionExtended ...
type TransactionExtended struct {
	Tx       *transaction.Transaction
	Received time.Time
}
