package events

import (
	"time"

	"github.com/google/uuid"
)

type DomainEvent interface {
	EventType() string
	AggregateID() uuid.UUID
	OccurredAt() time.Time
}

type BaseDomainEvent struct {
	aggregateID uuid.UUID
	occurredAt  time.Time
	eventType   string
}

func NewBaseDomainEvent(aggregateID uuid.UUID, eventType string) BaseDomainEvent {
	return BaseDomainEvent{
		aggregateID: aggregateID,
		occurredAt:  time.Now().UTC(),
		eventType:   eventType,
	}
}

func (b *BaseDomainEvent) EventType() string {
	return b.eventType
}

func (b *BaseDomainEvent) AggregateID() uuid.UUID {
	return b.aggregateID
}

func (b *BaseDomainEvent) OccurredAt() time.Time {
	return b.occurredAt
}

type AccountCreatedEvent struct {
	BaseDomainEvent
	CustomerID     string
	InitialBalance int64
}

func NewAccountCreatedEvent(accountID uuid.UUID, customerID string, initialBalance int64) *AccountCreatedEvent {
	return &AccountCreatedEvent{
		BaseDomainEvent: NewBaseDomainEvent(accountID, "account.created"),
		CustomerID:      customerID,
		InitialBalance:  initialBalance,
	}
}

type AccountDebitedEvent struct {
	BaseDomainEvent
	Amount        int64
	TransactionID uuid.UUID
	Description   string
}

func NewAccountDebitedEvent(accountID, transactionID uuid.UUID, amount int64, description string) *AccountDebitedEvent {
	return &AccountDebitedEvent{
		BaseDomainEvent: NewBaseDomainEvent(accountID, "account.debited"),
		Amount:          amount,
		TransactionID:   transactionID,
		Description:     description,
	}
}

type AccountCreditedEvent struct {
	BaseDomainEvent
	Amount        int64
	TransactionID uuid.UUID
	Description   string
}

func NewAccountCreditedEvent(accountID, transactionID uuid.UUID, amount int64, description string) *AccountCreditedEvent {
	return &AccountCreditedEvent{
		BaseDomainEvent: NewBaseDomainEvent(accountID, "account.credited"),
		Amount:          amount,
		TransactionID:   transactionID,
		Description:     description,
	}
}

type TransactionInitiatedEvent struct {
	BaseDomainEvent
	IdempotencyKey  string
	SourceAccountID uuid.UUID
	TargetAccountID uuid.UUID
	Amount          int64
	Description     string
}

func NewTransactionInitiatedEvent(
	transactionID uuid.UUID,
	idempotencyKey string,
	sourceAccountID uuid.UUID,
	targetAccountID uuid.UUID,
	amount int64,
	description string,
) *TransactionInitiatedEvent {
	return &TransactionInitiatedEvent{
		BaseDomainEvent: NewBaseDomainEvent(transactionID, "transaction.initiated"),
		IdempotencyKey:  idempotencyKey,
		SourceAccountID: sourceAccountID,
		TargetAccountID: targetAccountID,
		Amount:          amount,
		Description:     description,
	}
}

type TransactionProcessedEvent struct {
	BaseDomainEvent
	Status string
}

func NewTransactionProcessedEvent(transactionID uuid.UUID, status string) *TransactionProcessedEvent {
	return &TransactionProcessedEvent{
		BaseDomainEvent: NewBaseDomainEvent(transactionID, "transaction.processed"),
		Status:          status,
	}
}

type TransactionFailedEvent struct {
	BaseDomainEvent
	Reason string
}

func NewTransactionFailedEvent(transactionID uuid.UUID, reason string) *TransactionFailedEvent {
	return &TransactionFailedEvent{
		BaseDomainEvent: NewBaseDomainEvent(transactionID, "transaction.failed"),
		Reason:          reason,
	}
}

type ReconciliationCompletedEvent struct {
	BaseDomainEvent
	ReconciliationID uuid.UUID
	DiscrepancyFound bool
	Details          string
}

func NewReconciliationCompletedEvent(reconciliationID uuid.UUID, discrepancyFound bool, details string) *ReconciliationCompletedEvent {
	return &ReconciliationCompletedEvent{
		BaseDomainEvent:  NewBaseDomainEvent(reconciliationID, "reconciliation.completed"),
		ReconciliationID: reconciliationID,
		DiscrepancyFound: discrepancyFound,
		Details:          details,
	}
}
