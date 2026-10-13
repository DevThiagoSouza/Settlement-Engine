package tests

import (
	"testing"

	"github.com/google/uuid"
	"github.com/settlement-engine/internal/domain/account"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAccount(t *testing.T) {
	acc, err := account.NewAccount("CUST-001", 100000)
	require.NoError(t, err)
	assert.NotNil(t, acc)
	assert.Equal(t, "CUST-001", acc.CustomerID())
	assert.Equal(t, int64(100000), acc.Balance())
	assert.Equal(t, int64(1), acc.Version())

	acc, err = account.NewAccount("", 100000)
	assert.Error(t, err)
	assert.Nil(t, acc)

	acc, err = account.NewAccount("CUST-002", -1000)
	assert.Error(t, err)
	assert.Nil(t, acc)
}

func TestAccountDebit(t *testing.T) {
	acc, _ := account.NewAccount("CUST-001", 100000)
	initialVersion := acc.Version()

	err := acc.Debit(50000)
	require.NoError(t, err)
	assert.Equal(t, int64(50000), acc.Balance())
	assert.Equal(t, initialVersion+1, acc.Version())

	err = acc.Debit(100000)
	assert.Error(t, err)
	assert.Equal(t, account.ErrInsufficientFunds, err)

	err = acc.Debit(-1000)
	assert.Error(t, err)
}

func TestAccountCredit(t *testing.T) {
	acc, _ := account.NewAccount("CUST-001", 100000)
	initialVersion := acc.Version()

	err := acc.Credit(50000)
	require.NoError(t, err)
	assert.Equal(t, int64(150000), acc.Balance())
	assert.Equal(t, initialVersion+1, acc.Version())
}

func TestOptimisticLocking(t *testing.T) {
	acc, _ := account.NewAccount("CUST-001", 100000)
	expectedVersion := acc.Version()

	assert.True(t, acc.CanUpdate(expectedVersion))

	acc.Debit(10000)

	assert.False(t, acc.CanUpdate(expectedVersion))
}

func TestAccountConsistency(t *testing.T) {
	acc, _ := account.NewAccount("CUST-001", 100000)
	err := acc.ValidateConsistency()
	assert.NoError(t, err)

	_, err = account.ReconstructAccount(uuid.Nil, "CUST-002", 50000, 1, acc.CreatedAt(), acc.UpdatedAt(), acc.LastModified())
	assert.Error(t, err)
}

func TestAccountReconstruction(t *testing.T) {
	originalAcc, _ := account.NewAccount("CUST-001", 100000)

	reconstructed, err := account.ReconstructAccount(
		originalAcc.ID(),
		originalAcc.CustomerID(),
		originalAcc.Balance(),
		originalAcc.Version(),
		originalAcc.CreatedAt(),
		originalAcc.UpdatedAt(),
		originalAcc.LastModified(),
	)

	require.NoError(t, err)
	assert.Equal(t, originalAcc.ID(), reconstructed.ID())
	assert.Equal(t, originalAcc.Balance(), reconstructed.Balance())
	assert.Equal(t, originalAcc.Version(), reconstructed.Version())
}
