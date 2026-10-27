package ports

import (
	"context"
	"errors"
	"time"

	"github.com/jbspeakr/urbanartcore-bot/internal/domain"
)

// ErrLedgerConflict means another run changed the ledger while this one
// was writing it.
var ErrLedgerConflict = errors.New("ledger changed during write")

// Ledger remembers what was published where, so a rerun of the same day
// posts nothing and a post is not featured again within a year.
type Ledger interface {
	// Records returns every record whose slot date is on or after since.
	Records(ctx context.Context, since time.Time) ([]domain.PublishRecord, error)

	// Append adds a record. It never drops a record another run wrote.
	Append(ctx context.Context, record domain.PublishRecord) error
}
