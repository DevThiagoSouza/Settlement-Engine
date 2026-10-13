package account

import (
	"errors"
	"math"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidAccountID    = errors.New("invalid account ID")
	ErrInvalidBalance      = errors.New("invalid balance")
	ErrInsufficientFunds   = errors.New("insufficient funds")
	ErrConcurrencyConflict = errors.New("concurrency conflict: account was modified")
	ErrInvalidVersion      = errors.New("invalid version")
	ErrBalanceOverflow     = errors.New("balance overflow")
)

type Account struct {
	id           uuid.UUID
	customerID   string
	balance      int64
	version      int64
	createdAt    time.Time
	updatedAt    time.Time
	lastModified time.Time
}

func NewAccount(customerID string, initialBalance int64) (*Account, error) {
	if customerID == "" {
		return nil, ErrInvalidAccountID
	}
	if initialBalance < 0 {
		return nil, ErrInvalidBalance
	}

	now := time.Now().UTC()
	return &Account{
		id:           uuid.New(),
		customerID:   customerID,
		balance:      initialBalance,
		version:      1,
		createdAt:    now,
		updatedAt:    now,
		lastModified: now,
	}, nil
}

func ReconstructAccount(id uuid.UUID, customerID string, balance, version int64, createdAt, updatedAt, lastModified time.Time) (*Account, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidAccountID
	}
	if customerID == "" {
		return nil, ErrInvalidAccountID
	}
	if balance < 0 {
		return nil, ErrInvalidBalance
	}
	if version <= 0 {
		return nil, ErrInvalidVersion
	}

	return &Account{
		id:           id,
		customerID:   customerID,
		balance:      balance,
		version:      version,
		createdAt:    createdAt,
		updatedAt:    updatedAt,
		lastModified: lastModified,
	}, nil
}

func (a *Account) ID() uuid.UUID {
	return a.id
}

func (a *Account) CustomerID() string {
	return a.customerID
}

func (a *Account) Balance() int64 {
	return a.balance
}

func (a *Account) Version() int64 {
	return a.version
}

func (a *Account) CreatedAt() time.Time {
	return a.createdAt
}

func (a *Account) UpdatedAt() time.Time {
	return a.updatedAt
}

func (a *Account) LastModified() time.Time {
	return a.lastModified
}

func (a *Account) Debit(amount int64) error {
	if amount <= 0 {
		return errors.New("debit amount must be positive")
	}

	if a.balance < amount {
		return ErrInsufficientFunds
	}

	a.balance -= amount
	a.incrementVersion()
	return nil
}

func (a *Account) Credit(amount int64) error {
	if amount <= 0 {
		return errors.New("credit amount must be positive")
	}
	if amount > math.MaxInt64-a.balance {
		return ErrBalanceOverflow
	}

	a.balance += amount
	a.incrementVersion()
	return nil
}

func (a *Account) Transfer(amount int64, direction string) error {
	if amount <= 0 {
		return errors.New("transfer amount must be positive")
	}

	if direction == "in" {
		if amount > math.MaxInt64-a.balance {
			return ErrBalanceOverflow
		}
		a.balance += amount
	} else if direction == "out" {
		if a.balance < amount {
			return ErrInsufficientFunds
		}
		a.balance -= amount
	} else {
		return errors.New("invalid transfer direction")
	}

	a.incrementVersion()
	return nil
}

func (a *Account) incrementVersion() {
	a.version++
	a.lastModified = time.Now().UTC()
	a.updatedAt = a.lastModified
}

func (a *Account) CanUpdate(expectedVersion int64) bool {
	return a.version == expectedVersion
}

func (a *Account) ValidateConsistency() error {
	if a.id == uuid.Nil {
		return ErrInvalidAccountID
	}
	if a.customerID == "" {
		return ErrInvalidAccountID
	}
	if a.balance < 0 {
		return ErrInvalidBalance
	}
	if a.version <= 0 {
		return ErrInvalidVersion
	}
	return nil
}
