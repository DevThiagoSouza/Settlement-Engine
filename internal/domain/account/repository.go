package account

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	Save(ctx context.Context, account *Account) error

	SaveWithOptimisticLock(ctx context.Context, account *Account, expectedVersion int64) error

	FindByID(ctx context.Context, id uuid.UUID) (*Account, error)

	FindByCustomerID(ctx context.Context, customerID string) (*Account, error)

	FindAll(ctx context.Context, limit, offset int) ([]*Account, error)

	Delete(ctx context.Context, id uuid.UUID) error

	GetBalance(ctx context.Context, id uuid.UUID) (int64, error)
}
