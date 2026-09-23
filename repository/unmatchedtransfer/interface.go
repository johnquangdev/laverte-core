package unmatchedtransfer

import (
	"context"
	"time"

	"github.com/johnquangdev/laverte-core/model"
)

type IRepository interface {
	// RecordIfNew inserts t unless a row with the same provider transaction id
	// exists, reporting whether it inserted. False with a nil error is the normal
	// outcome for a redelivered webhook; the caller alerts the admin only on true.
	RecordIfNew(ctx context.Context, t *model.UnmatchedTransfer) (bool, error)
	GetByID(ctx context.Context, id uint) (*model.UnmatchedTransfer, error)
	// List returns newest first, at most limit rows; openOnly drops resolved ones.
	List(ctx context.Context, openOnly bool, limit int) ([]*model.UnmatchedTransfer, error)
	// ResolveIfOpen stamps the resolution only while the row is still open,
	// reporting whether it won, so two admins cannot both close the same transfer
	// and have one note silently overwrite the other.
	ResolveIfOpen(ctx context.Context, id, adminID uint, note string, at time.Time) (bool, error)
}
