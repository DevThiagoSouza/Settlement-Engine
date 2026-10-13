package tests

import (
	"testing"

	"github.com/google/uuid"
	"github.com/settlement-engine/internal/domain/transaction"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTransaction(t *testing.T) {
	sourceID := uuid.New()
	targetID := uuid.New()

	tx, err := transaction.NewTransaction(
		"IDEMPOTENCY-001",
		sourceID,
		targetID,
		50000,
		transaction.TypeDebit,
		"Test transaction",
	)

	require.NoError(t, err)
	assert.NotNil(t, tx)
	assert.Equal(t, "IDEMPOTENCY-001", tx.IdempotencyKey())
	assert.Equal(t, int64(50000), tx.Amount())
	assert.Equal(t, transaction.TypeDebit, tx.Type())
	assert.Equal(t, transaction.StatusPending, tx.Status())
	assert.Equal(t, int64(1), tx.Version())
}

func TestInvalidTransaction(t *testing.T) {
	tx, err := transaction.NewTransaction("", uuid.New(), uuid.New(), 50000, transaction.TypeDebit, "")
	assert.Error(t, err)
	assert.Nil(t, tx)

	tx, err = transaction.NewTransaction("KEY", uuid.New(), uuid.New(), -1, transaction.TypeDebit, "")
	assert.Error(t, err)
	assert.Nil(t, tx)

	tx, err = transaction.NewTransaction("KEY", uuid.New(), uuid.New(), 50000, transaction.TransactionType("INVALID"), "")
	assert.Error(t, err)
	assert.Nil(t, tx)
}

func TestTransactionStateTransitions(t *testing.T) {
	sourceID := uuid.New()
	targetID := uuid.New()
	tx, _ := transaction.NewTransaction("KEY-001", sourceID, targetID, 50000, transaction.TypeDebit, "Test")

	err := tx.MarkAsProcessed()
	require.NoError(t, err)
	assert.Equal(t, transaction.StatusProcessed, tx.Status())
	assert.NotNil(t, tx.ProcessedAt())

	err = tx.MarkAsProcessed()
	assert.Error(t, err)

	err = tx.Compensate()
	require.NoError(t, err)
	assert.Equal(t, transaction.StatusCompensated, tx.Status())
}

func TestTransactionFailed(t *testing.T) {
	sourceID := uuid.New()
	targetID := uuid.New()
	tx, _ := transaction.NewTransaction("KEY-002", sourceID, targetID, 50000, transaction.TypeDebit, "Test")

	err := tx.MarkAsFailed("Insufficient funds")
	require.NoError(t, err)
	assert.Equal(t, transaction.StatusFailed, tx.Status())
	assert.Equal(t, "Insufficient funds", tx.FailureReason())
	assert.NotNil(t, tx.ProcessedAt())
}

func TestIdempotencyKey(t *testing.T) {
	sourceID := uuid.New()
	targetID := uuid.New()

	tx1, _ := transaction.NewTransaction("UNIQUE-KEY-001", sourceID, targetID, 50000, transaction.TypeDebit, "")
	tx2, _ := transaction.NewTransaction("UNIQUE-KEY-001", sourceID, targetID, 50000, transaction.TypeDebit, "")

	assert.Equal(t, tx1.IdempotencyKey(), tx2.IdempotencyKey())
}

func TestTransactionMetadata(t *testing.T) {
	sourceID := uuid.New()
	targetID := uuid.New()
	tx, _ := transaction.NewTransaction("KEY-003", sourceID, targetID, 50000, transaction.TypeDebit, "")

	tx.SetMetadata("reference", "ORDER-123")
	tx.SetMetadata("correlationId", "CORR-456")

	ref, ok := tx.GetMetadata("reference")
	assert.True(t, ok)
	assert.Equal(t, "ORDER-123", ref)

	_, ok = tx.GetMetadata("nonexistent")
	assert.False(t, ok)
}

func TestTransactionConsistency(t *testing.T) {
	sourceID := uuid.New()
	targetID := uuid.New()
	tx, _ := transaction.NewTransaction("KEY-004", sourceID, targetID, 50000, transaction.TypeDebit, "")

	err := tx.ValidateConsistency()
	assert.NoError(t, err)

}

func TestTransactionReconstruction(t *testing.T) {
	sourceID := uuid.New()
	targetID := uuid.New()
	original, _ := transaction.NewTransaction("KEY-005", sourceID, targetID, 50000, transaction.TypeDebit, "Test description")

	reconstructed, err := transaction.ReconstructTransaction(
		original.ID(),
		original.IdempotencyKey(),
		original.SourceAccountID(),
		original.TargetAccountID(),
		original.Amount(),
		original.Type(),
		original.Status(),
		original.Description(),
		original.Version(),
		original.CreatedAt(),
		original.ProcessedAt(),
		original.FailureReason(),
	)

	require.NoError(t, err)
	assert.Equal(t, original.ID(), reconstructed.ID())
	assert.Equal(t, original.IdempotencyKey(), reconstructed.IdempotencyKey())
	assert.Equal(t, original.Amount(), reconstructed.Amount())
}
