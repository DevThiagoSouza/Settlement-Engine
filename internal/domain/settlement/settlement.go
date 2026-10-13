package settlement

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/settlement-engine/internal/domain/account"
	"github.com/settlement-engine/internal/domain/events"
	"github.com/settlement-engine/internal/domain/transaction"
)

var (
	ErrInvalidSettlement      = errors.New("invalid settlement")
	ErrInvalidAggregateState  = errors.New("invalid aggregate state")
	ErrSettlementNotPending   = errors.New("settlement is not in pending state")
	ErrInsufficientFundsError = errors.New("insufficient funds for settlement")
)

type SettlementStatus string

const (
	StatusPending    SettlementStatus = "PENDING"
	StatusSettled    SettlementStatus = "SETTLED"
	StatusFailed     SettlementStatus = "FAILED"
	StatusRolledback SettlementStatus = "ROLLEDBACK"
)

type Settlement struct {
	id                uuid.UUID
	transactionID     uuid.UUID
	sourceAccount     *account.Account
	targetAccount     *account.Account
	sourceTransaction *transaction.Transaction
	targetTransaction *transaction.Transaction
	status            SettlementStatus
	amount            int64
	domainEvents      []events.DomainEvent
	createdAt         time.Time
	settledAt         *time.Time
	failureReason     string
}

func NewSettlement(
	transactionID uuid.UUID,
	sourceAccount *account.Account,
	targetAccount *account.Account,
	sourceTransaction *transaction.Transaction,
	targetTransaction *transaction.Transaction,
	amount int64,
) (*Settlement, error) {
	if transactionID == uuid.Nil {
		return nil, ErrInvalidSettlement
	}
	if sourceAccount == nil {
		return nil, errors.New("source account cannot be nil")
	}
	if targetAccount == nil {
		return nil, errors.New("target account cannot be nil")
	}
	if sourceTransaction == nil {
		return nil, errors.New("source transaction cannot be nil")
	}
	if targetTransaction == nil {
		return nil, errors.New("target transaction cannot be nil")
	}
	if amount <= 0 {
		return nil, errors.New("amount must be positive")
	}

	return &Settlement{
		id:                uuid.New(),
		transactionID:     transactionID,
		sourceAccount:     sourceAccount,
		targetAccount:     targetAccount,
		sourceTransaction: sourceTransaction,
		targetTransaction: targetTransaction,
		status:            StatusPending,
		amount:            amount,
		domainEvents:      make([]events.DomainEvent, 0),
		createdAt:         time.Now().UTC(),
	}, nil
}

func (s *Settlement) ID() uuid.UUID {
	return s.id
}

func (s *Settlement) TransactionID() uuid.UUID {
	return s.transactionID
}

func (s *Settlement) Status() SettlementStatus {
	return s.status
}

func (s *Settlement) Amount() int64 {
	return s.amount
}

func (s *Settlement) DomainEvents() []events.DomainEvent {
	return s.domainEvents
}

func (s *Settlement) ClearDomainEvents() {
	s.domainEvents = make([]events.DomainEvent, 0)
}

func (s *Settlement) ExecuteSettlement() error {
	if s.status != StatusPending {
		return ErrSettlementNotPending
	}

	if s.sourceAccount.Balance() < s.amount {
		s.status = StatusFailed
		s.failureReason = ErrInsufficientFundsError.Error()
		s.recordFailureEvent()
		return ErrInsufficientFundsError
	}

	if err := s.sourceAccount.Debit(s.amount); err != nil {
		s.status = StatusFailed
		s.failureReason = err.Error()
		s.recordFailureEvent()
		return err
	}

	if err := s.targetAccount.Credit(s.amount); err != nil {
		s.sourceAccount.Credit(s.amount)
		s.status = StatusFailed
		s.failureReason = err.Error()
		s.recordFailureEvent()
		return err
	}

	if err := s.sourceTransaction.MarkAsProcessed(); err != nil {
		return err
	}

	if err := s.targetTransaction.MarkAsProcessed(); err != nil {
		return err
	}

	s.status = StatusSettled
	now := time.Now().UTC()
	s.settledAt = &now

	s.recordSuccessEvents()

	return nil
}

func (s *Settlement) recordSuccessEvents() {
	s.domainEvents = append(s.domainEvents,
		events.NewAccountDebitedEvent(
			s.sourceAccount.ID(),
			s.transactionID,
			s.amount,
			"Settlement debit",
		),
	)

	s.domainEvents = append(s.domainEvents,
		events.NewAccountCreditedEvent(
			s.targetAccount.ID(),
			s.transactionID,
			s.amount,
			"Settlement credit",
		),
	)

	s.domainEvents = append(s.domainEvents,
		events.NewTransactionProcessedEvent(
			s.transactionID,
			"SETTLED",
		),
	)
}

func (s *Settlement) recordFailureEvent() {
	s.domainEvents = append(s.domainEvents,
		events.NewTransactionFailedEvent(
			s.transactionID,
			s.failureReason,
		),
	)
}

func (s *Settlement) Rollback() error {
	if s.status != StatusSettled {
		return errors.New("only settled settlements can be rolled back")
	}

	if err := s.sourceAccount.Credit(s.amount); err != nil {
		return err
	}

	if err := s.targetAccount.Debit(s.amount); err != nil {
		s.sourceAccount.Debit(s.amount)
		return err
	}

	s.sourceTransaction.Compensate()
	s.targetTransaction.Compensate()

	s.status = StatusRolledback
	return nil
}

func (s *Settlement) ValidateConsistency() error {
	if s.id == uuid.Nil {
		return ErrInvalidSettlement
	}
	if s.sourceAccount == nil || s.targetAccount == nil {
		return ErrInvalidAggregateState
	}
	if err := s.sourceAccount.ValidateConsistency(); err != nil {
		return err
	}
	if err := s.targetAccount.ValidateConsistency(); err != nil {
		return err
	}
	if err := s.sourceTransaction.ValidateConsistency(); err != nil {
		return err
	}
	if err := s.targetTransaction.ValidateConsistency(); err != nil {
		return err
	}
	return nil
}

type SettlementReport struct {
	SettlementID    uuid.UUID
	TransactionID   uuid.UUID
	Status          SettlementStatus
	Amount          int64
	SourceAccountID uuid.UUID
	TargetAccountID uuid.UUID
	SourceBalance   int64
	TargetBalance   int64
	CreatedAt       time.Time
	SettledAt       *time.Time
	FailureReason   string
}

func (s *Settlement) GetSettlementReport() SettlementReport {
	return SettlementReport{
		SettlementID:    s.id,
		TransactionID:   s.transactionID,
		Status:          s.status,
		Amount:          s.amount,
		SourceAccountID: s.sourceAccount.ID(),
		TargetAccountID: s.targetAccount.ID(),
		SourceBalance:   s.sourceAccount.Balance(),
		TargetBalance:   s.targetAccount.Balance(),
		CreatedAt:       s.createdAt,
		SettledAt:       s.settledAt,
		FailureReason:   s.failureReason,
	}
}
