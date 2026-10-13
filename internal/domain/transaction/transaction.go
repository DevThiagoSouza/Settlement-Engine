package transaction

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidTransactionID = errors.New("invalid transaction ID")
	ErrInvalidAmount        = errors.New("invalid amount")
	ErrDuplicateTransaction = errors.New("duplicate transaction")
	ErrTransactionNotFound  = errors.New("transaction not found")
)

type TransactionType string

const (
	TypeDebit  TransactionType = "DEBIT"
	TypeCredit TransactionType = "CREDIT"
)

type TransactionStatus string

const (
	StatusPending     TransactionStatus = "PENDING"
	StatusProcessed   TransactionStatus = "PROCESSED"
	StatusFailed      TransactionStatus = "FAILED"
	StatusCompensated TransactionStatus = "COMPENSATED"
)

type Transaction struct {
	id              uuid.UUID
	idempotencyKey  string
	sourceAccountID uuid.UUID
	targetAccountID uuid.UUID
	amount          int64
	transactionType TransactionType
	status          TransactionStatus
	description     string
	metadata        map[string]interface{}
	version         int64
	createdAt       time.Time
	processedAt     *time.Time
	failureReason   string
}

func NewTransaction(
	idempotencyKey string,
	sourceAccountID uuid.UUID,
	targetAccountID uuid.UUID,
	amount int64,
	transactionType TransactionType,
	description string,
) (*Transaction, error) {
	if idempotencyKey == "" {
		return nil, errors.New("idempotency key cannot be empty")
	}
	if sourceAccountID == uuid.Nil && transactionType != TypeCredit {
		return nil, ErrInvalidTransactionID
	}
	if targetAccountID == uuid.Nil {
		return nil, ErrInvalidTransactionID
	}
	if amount <= 0 {
		return nil, ErrInvalidAmount
	}
	if transactionType != TypeDebit && transactionType != TypeCredit {
		return nil, errors.New("invalid transaction type")
	}

	return &Transaction{
		id:              uuid.New(),
		idempotencyKey:  idempotencyKey,
		sourceAccountID: sourceAccountID,
		targetAccountID: targetAccountID,
		amount:          amount,
		transactionType: transactionType,
		status:          StatusPending,
		description:     description,
		metadata:        make(map[string]interface{}),
		version:         1,
		createdAt:       time.Now().UTC(),
	}, nil
}

func ReconstructTransaction(
	id uuid.UUID,
	idempotencyKey string,
	sourceAccountID uuid.UUID,
	targetAccountID uuid.UUID,
	amount int64,
	transactionType TransactionType,
	status TransactionStatus,
	description string,
	version int64,
	createdAt time.Time,
	processedAt *time.Time,
	failureReason string,
) (*Transaction, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidTransactionID
	}

	tx := &Transaction{
		id:              id,
		idempotencyKey:  idempotencyKey,
		sourceAccountID: sourceAccountID,
		targetAccountID: targetAccountID,
		amount:          amount,
		transactionType: transactionType,
		status:          status,
		description:     description,
		metadata:        make(map[string]interface{}),
		version:         version,
		createdAt:       createdAt,
		processedAt:     processedAt,
		failureReason:   failureReason,
	}

	return tx, nil
}

func (t *Transaction) ID() uuid.UUID {
	return t.id
}

func (t *Transaction) IdempotencyKey() string {
	return t.idempotencyKey
}

func (t *Transaction) SourceAccountID() uuid.UUID {
	return t.sourceAccountID
}

func (t *Transaction) TargetAccountID() uuid.UUID {
	return t.targetAccountID
}

func (t *Transaction) Amount() int64 {
	return t.amount
}

func (t *Transaction) Type() TransactionType {
	return t.transactionType
}

func (t *Transaction) Status() TransactionStatus {
	return t.status
}

func (t *Transaction) Description() string {
	return t.description
}

func (t *Transaction) Version() int64 {
	return t.version
}

func (t *Transaction) CreatedAt() time.Time {
	return t.createdAt
}

func (t *Transaction) ProcessedAt() *time.Time {
	return t.processedAt
}

func (t *Transaction) FailureReason() string {
	return t.failureReason
}

func (t *Transaction) MarkAsProcessed() error {
	if t.status != StatusPending {
		return errors.New("only pending transactions can be marked as processed")
	}

	t.status = StatusProcessed
	now := time.Now().UTC()
	t.processedAt = &now
	t.version++
	return nil
}

func (t *Transaction) MarkAsFailed(reason string) error {
	if t.status != StatusPending {
		return errors.New("only pending transactions can be marked as failed")
	}

	t.status = StatusFailed
	now := time.Now().UTC()
	t.processedAt = &now
	t.failureReason = reason
	t.version++
	return nil
}

func (t *Transaction) Compensate() error {
	if t.status != StatusProcessed {
		return errors.New("only processed transactions can be compensated")
	}

	t.status = StatusCompensated
	t.version++
	return nil
}

func (t *Transaction) SetMetadata(key string, value interface{}) {
	t.metadata[key] = value
}

func (t *Transaction) GetMetadata(key string) (interface{}, bool) {
	val, ok := t.metadata[key]
	return val, ok
}

func (t *Transaction) CanUpdate(expectedVersion int64) bool {
	return t.version == expectedVersion
}

func (t *Transaction) ValidateConsistency() error {
	if t.id == uuid.Nil {
		return ErrInvalidTransactionID
	}
	if t.amount <= 0 {
		return ErrInvalidAmount
	}
	if t.status == "" {
		return errors.New("invalid status")
	}
	return nil
}
