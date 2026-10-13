package transaction

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	Save(ctx context.Context, transaction *Transaction) error

	SaveWithOptimisticLock(ctx context.Context, transaction *Transaction, expectedVersion int64) error

	FindByID(ctx context.Context, id uuid.UUID) (*Transaction, error)

	FindByIdempotencyKey(ctx context.Context, key string) (*Transaction, error)

	FindBySourceAccount(ctx context.Context, accountID uuid.UUID, limit, offset int) ([]*Transaction, error)

	FindByTargetAccount(ctx context.Context, accountID uuid.UUID, limit, offset int) ([]*Transaction, error)

	FindByStatus(ctx context.Context, status TransactionStatus, limit, offset int) ([]*Transaction, error)

	FindAll(ctx context.Context, limit, offset int) ([]*Transaction, error)

	Delete(ctx context.Context, id uuid.UUID) error

	GetPendingTransactions(ctx context.Context, limit int) ([]*Transaction, error)
}
