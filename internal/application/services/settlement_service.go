package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/settlement-engine/internal/application/ports"
	"github.com/settlement-engine/internal/domain/account"
	"github.com/settlement-engine/internal/domain/events"
	"github.com/settlement-engine/internal/domain/settlement"
	"github.com/settlement-engine/internal/domain/transaction"
	"github.com/settlement-engine/internal/infrastructure/outbox"
)

var (
	ErrAccountNotFound             = errors.New("account not found")
	ErrTransactionNotFound         = errors.New("transaction not found")
	ErrIdempotentTransactionExists = errors.New("transaction with same idempotency key already exists")
	ErrForbidden                   = errors.New("operation forbidden")
)

type SettlementService struct {
	accountRepo     account.Repository
	transactionRepo transaction.Repository
	outboxRepo      outbox.OutboxRepository
	unitOfWork      ports.UnitOfWork
}

func NewSettlementService(
	accountRepo account.Repository,
	transactionRepo transaction.Repository,
	outboxRepo outbox.OutboxRepository,
	unitOfWorks ...ports.UnitOfWork,
) *SettlementService {
	var unitOfWork ports.UnitOfWork
	if len(unitOfWorks) > 0 {
		unitOfWork = unitOfWorks[0]
	}
	return &SettlementService{
		accountRepo:     accountRepo,
		transactionRepo: transactionRepo,
		outboxRepo:      outboxRepo,
		unitOfWork:      unitOfWork,
	}
}

type CreateAccountRequest struct {
	CustomerID     string
	InitialBalance int64
}

type CreateAccountResponse struct {
	AccountID  uuid.UUID
	CustomerID string
	Balance    int64
	Version    int64
}

func (s *SettlementService) CreateAccount(ctx context.Context, req CreateAccountRequest) (*CreateAccountResponse, error) {
	if s.unitOfWork == nil {
		return s.createAccount(ctx, req)
	}

	var response *CreateAccountResponse
	err := s.unitOfWork.Execute(ctx, func(txCtx context.Context) error {
		var err error
		response, err = s.createAccount(txCtx, req)
		return err
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

func (s *SettlementService) createAccount(ctx context.Context, req CreateAccountRequest) (*CreateAccountResponse, error) {
	if principal, ok := ports.Principal(ctx); ok && principal != req.CustomerID {
		return nil, ErrForbidden
	}
	acc, err := account.NewAccount(req.CustomerID, req.InitialBalance)
	if err != nil {
		return nil, err
	}

	if err := s.accountRepo.Save(ctx, acc); err != nil {
		return nil, err
	}

	domainEvent := events.NewAccountCreatedEvent(acc.ID(), acc.CustomerID(), acc.Balance())

	outboxEvent, err := outbox.NewOutboxEvent(domainEvent)
	if err != nil {
		return nil, err
	}

	if err := s.outboxRepo.Save(ctx, outboxEvent); err != nil {
		return nil, err
	}

	return &CreateAccountResponse{
		AccountID:  acc.ID(),
		CustomerID: acc.CustomerID(),
		Balance:    acc.Balance(),
		Version:    acc.Version(),
	}, nil
}

type ProcessTransactionRequest struct {
	IdempotencyKey  string
	SourceAccountID uuid.UUID
	TargetAccountID uuid.UUID
	Amount          int64
	Description     string
}

type ProcessTransactionResponse struct {
	TransactionID uuid.UUID
	Status        string
	SourceBalance int64
	TargetBalance int64
	ErrorMessage  string
}

func (s *SettlementService) ProcessTransaction(ctx context.Context, req ProcessTransactionRequest) (*ProcessTransactionResponse, error) {
	if s.unitOfWork == nil {
		return s.processTransaction(ctx, req)
	}

	var response *ProcessTransactionResponse
	err := s.unitOfWork.Execute(ctx, func(txCtx context.Context) error {
		var err error
		response, err = s.processTransaction(txCtx, req)
		return err
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

func (s *SettlementService) processTransaction(ctx context.Context, req ProcessTransactionRequest) (*ProcessTransactionResponse, error) {
	existingTx, err := s.transactionRepo.FindByIdempotencyKey(ctx, req.IdempotencyKey)
	if err == nil && existingTx != nil {
		if existingTx.SourceAccountID() != req.SourceAccountID ||
			existingTx.TargetAccountID() != req.TargetAccountID ||
			existingTx.Amount() != req.Amount ||
			existingTx.Description() != req.Description {
			return nil, ErrIdempotentTransactionExists
		}
		sourceAcc, sourceErr := s.accountRepo.FindByID(ctx, req.SourceAccountID)
		targetAcc, targetErr := s.accountRepo.FindByID(ctx, req.TargetAccountID)
		if sourceErr != nil || targetErr != nil {
			return nil, ErrAccountNotFound
		}

		return &ProcessTransactionResponse{
			TransactionID: existingTx.ID(),
			Status:        string(existingTx.Status()),
			SourceBalance: sourceAcc.Balance(),
			TargetBalance: targetAcc.Balance(),
		}, nil
	}
	if err != nil && !errors.Is(err, transaction.ErrTransactionNotFound) {
		return nil, err
	}
	if req.SourceAccountID == req.TargetAccountID {
		return nil, errors.New("source and target accounts must be different")
	}

	sourceAcc, err := s.accountRepo.FindByID(ctx, req.SourceAccountID)
	if err != nil {
		return nil, ErrAccountNotFound
	}
	if principal, ok := ports.Principal(ctx); ok && sourceAcc.CustomerID() != principal {
		return nil, ErrForbidden
	}

	targetAcc, err := s.accountRepo.FindByID(ctx, req.TargetAccountID)
	if err != nil {
		return nil, ErrAccountNotFound
	}

	newTransaction, err := transaction.NewTransaction(
		req.IdempotencyKey,
		req.SourceAccountID,
		req.TargetAccountID,
		req.Amount,
		transaction.TypeDebit,
		req.Description,
	)
	if err != nil {
		return nil, err
	}

	if err := s.transactionRepo.Save(ctx, newTransaction); err != nil {
		return nil, err
	}

	creditTransaction, err := transaction.NewTransaction(
		req.IdempotencyKey+"-credit",
		uuid.Nil,
		req.TargetAccountID,
		req.Amount,
		transaction.TypeCredit,
		req.Description,
	)
	if err != nil {
		return nil, err
	}

	if err := s.transactionRepo.Save(ctx, creditTransaction); err != nil {
		return nil, err
	}

	settl, err := settlement.NewSettlement(
		newTransaction.ID(),
		sourceAcc,
		targetAcc,
		newTransaction,
		creditTransaction,
		req.Amount,
	)
	if err != nil {
		return nil, err
	}

	if err := settl.ExecuteSettlement(); err != nil {
		if markErr := newTransaction.MarkAsFailed(err.Error()); markErr == nil {
			_ = s.transactionRepo.SaveWithOptimisticLock(ctx, newTransaction, newTransaction.Version()-1)
		}
		if markErr := creditTransaction.MarkAsFailed(err.Error()); markErr == nil {
			_ = s.transactionRepo.SaveWithOptimisticLock(ctx, creditTransaction, creditTransaction.Version()-1)
		}
		return &ProcessTransactionResponse{
			TransactionID: newTransaction.ID(),
			Status:        string(settlement.StatusFailed),
			SourceBalance: sourceAcc.Balance(),
			TargetBalance: targetAcc.Balance(),
			ErrorMessage:  err.Error(),
		}, nil
	}

	if err := s.accountRepo.SaveWithOptimisticLock(ctx, sourceAcc, sourceAcc.Version()-1); err != nil {
		return nil, fmt.Errorf("concurrency conflict on source account: %w", err)
	}

	if err := s.accountRepo.SaveWithOptimisticLock(ctx, targetAcc, targetAcc.Version()-1); err != nil {
		return nil, fmt.Errorf("concurrency conflict on target account: %w", err)
	}

	if err := s.transactionRepo.SaveWithOptimisticLock(ctx, newTransaction, newTransaction.Version()-1); err != nil {
		return nil, err
	}

	if err := s.transactionRepo.SaveWithOptimisticLock(ctx, creditTransaction, creditTransaction.Version()-1); err != nil {
		return nil, err
	}

	for _, domainEvent := range settl.DomainEvents() {
		outboxEvent, err := outbox.NewOutboxEvent(domainEvent)
		if err != nil {
			return nil, err
		}

		if err := s.outboxRepo.Save(ctx, outboxEvent); err != nil {
			return nil, err
		}
	}

	return &ProcessTransactionResponse{
		TransactionID: newTransaction.ID(),
		Status:        string(settlement.StatusSettled),
		SourceBalance: sourceAcc.Balance(),
		TargetBalance: targetAcc.Balance(),
	}, nil
}

type GetAccountRequest struct {
	AccountID uuid.UUID
}

type GetAccountResponse struct {
	AccountID  uuid.UUID
	CustomerID string
	Balance    int64
	Version    int64
}

func (s *SettlementService) GetAccount(ctx context.Context, accountID uuid.UUID) (*GetAccountResponse, error) {
	acc, err := s.accountRepo.FindByID(ctx, accountID)
	if err != nil {
		return nil, ErrAccountNotFound
	}
	if principal, ok := ports.Principal(ctx); ok && acc.CustomerID() != principal {
		return nil, ErrForbidden
	}

	return &GetAccountResponse{
		AccountID:  acc.ID(),
		CustomerID: acc.CustomerID(),
		Balance:    acc.Balance(),
		Version:    acc.Version(),
	}, nil
}

type GetTransactionRequest struct {
	TransactionID uuid.UUID
}

type GetTransactionResponse struct {
	TransactionID   uuid.UUID
	IdempotencyKey  string
	SourceAccountID uuid.UUID
	TargetAccountID uuid.UUID
	Amount          int64
	Status          string
	Description     string
}

func (s *SettlementService) GetTransaction(ctx context.Context, txID uuid.UUID) (*GetTransactionResponse, error) {
	tx, err := s.transactionRepo.FindByID(ctx, txID)
	if err != nil {
		return nil, ErrTransactionNotFound
	}

	return &GetTransactionResponse{
		TransactionID:   tx.ID(),
		IdempotencyKey:  tx.IdempotencyKey(),
		SourceAccountID: tx.SourceAccountID(),
		TargetAccountID: tx.TargetAccountID(),
		Amount:          tx.Amount(),
		Status:          string(tx.Status()),
		Description:     tx.Description(),
	}, nil
}
