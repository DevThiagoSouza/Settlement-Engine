package reconciliation

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/settlement-engine/internal/domain/account"
	"github.com/settlement-engine/internal/domain/transaction"
)

type ReconciliationResult struct {
	ReconciliationID      uuid.UUID
	ExecutedAt            time.Time
	ProcessingTimeMs      int64
	TotalAccounts         int64
	TotalTransactions     int64
	ProcessedTransactions int64
	FailedTransactions    int64
	PendingTransactions   int64
	DiscrepanciesFound    bool
	Discrepancies         []Discrepancy
	SummaryReport         SummaryReport
}

type Discrepancy struct {
	Type          string
	AggregateID   uuid.UUID
	Details       string
	ExpectedValue int64
	ActualValue   int64
}

type SummaryReport struct {
	TotalDebitAmount        int64
	TotalCreditAmount       int64
	NetFlow                 int64
	TotalBalanceAllAccounts int64
}

type ReconciliationService struct {
	accountRepo     account.Repository
	transactionRepo transaction.Repository
}

func NewReconciliationService(
	accountRepo account.Repository,
	transactionRepo transaction.Repository,
) *ReconciliationService {
	return &ReconciliationService{
		accountRepo:     accountRepo,
		transactionRepo: transactionRepo,
	}
}

func (rs *ReconciliationService) ReconcileAll(ctx context.Context) (*ReconciliationResult, error) {
	startTime := time.Now()
	reconciliationID := uuid.New()

	result := &ReconciliationResult{
		ReconciliationID: reconciliationID,
		ExecutedAt:       startTime,
		Discrepancies:    make([]Discrepancy, 0),
		SummaryReport:    SummaryReport{},
	}

	accounts, err := rs.accountRepo.FindAll(ctx, 10000, 0)
	if err != nil {
		return nil, err
	}

	result.TotalAccounts = int64(len(accounts))

	for _, acc := range accounts {
		result.SummaryReport.TotalBalanceAllAccounts += acc.Balance()
	}

	transactions, err := rs.transactionRepo.FindAll(ctx, 100000, 0)
	if err != nil {
		return nil, err
	}

	result.TotalTransactions = int64(len(transactions))

	for _, tx := range transactions {
		switch tx.Status() {
		case transaction.StatusProcessed:
			result.ProcessedTransactions++
			if tx.Type() == transaction.TypeDebit {
				result.SummaryReport.TotalDebitAmount += tx.Amount()
			} else {
				result.SummaryReport.TotalCreditAmount += tx.Amount()
			}
		case transaction.StatusFailed:
			result.FailedTransactions++
		case transaction.StatusPending:
			result.PendingTransactions++
		}
	}

	result.SummaryReport.NetFlow = result.SummaryReport.TotalCreditAmount - result.SummaryReport.TotalDebitAmount

	rs.validateTransactionConsistency(transactions, result)
	rs.validateAccountConsistency(ctx, accounts, result)

	if len(result.Discrepancies) > 0 {
		result.DiscrepanciesFound = true
	}

	result.ProcessingTimeMs = time.Since(startTime).Milliseconds()
	return result, nil
}

func (rs *ReconciliationService) ReconcileAccount(ctx context.Context, accountID uuid.UUID) (*ReconciliationResult, error) {
	startTime := time.Now()
	reconciliationID := uuid.New()

	result := &ReconciliationResult{
		ReconciliationID: reconciliationID,
		ExecutedAt:       startTime,
		Discrepancies:    make([]Discrepancy, 0),
		SummaryReport:    SummaryReport{},
	}

	acc, err := rs.accountRepo.FindByID(ctx, accountID)
	if err != nil {
		return nil, err
	}

	result.TotalAccounts = 1
	result.SummaryReport.TotalBalanceAllAccounts = acc.Balance()

	sourceTransactions, err := rs.transactionRepo.FindBySourceAccount(ctx, accountID, 100000, 0)
	if err != nil {
		return nil, err
	}

	targetTransactions, err := rs.transactionRepo.FindByTargetAccount(ctx, accountID, 100000, 0)
	if err != nil {
		return nil, err
	}

	allTransactions := append(sourceTransactions, targetTransactions...)
	result.TotalTransactions = int64(len(allTransactions))

	calculatedBalance := int64(0)
	for _, tx := range allTransactions {
		if tx.Status() == transaction.StatusProcessed {
			result.ProcessedTransactions++
			if tx.SourceAccountID() == accountID && tx.Type() == transaction.TypeDebit {
				calculatedBalance -= tx.Amount()
				result.SummaryReport.TotalDebitAmount += tx.Amount()
			} else if tx.TargetAccountID() == accountID {
				calculatedBalance += tx.Amount()
				result.SummaryReport.TotalCreditAmount += tx.Amount()
			}
		}
	}

	result.SummaryReport.NetFlow = result.SummaryReport.TotalCreditAmount - result.SummaryReport.TotalDebitAmount

	if calculatedBalance != acc.Balance() {
		result.DiscrepanciesFound = true
		result.Discrepancies = append(result.Discrepancies, Discrepancy{
			Type:          "BALANCE_MISMATCH",
			AggregateID:   accountID,
			Details:       fmt.Sprintf("Calculated balance doesn't match actual balance"),
			ExpectedValue: calculatedBalance,
			ActualValue:   acc.Balance(),
		})
	}

	result.ProcessingTimeMs = time.Since(startTime).Milliseconds()
	return result, nil
}

func (rs *ReconciliationService) validateTransactionConsistency(txs []*transaction.Transaction, result *ReconciliationResult) {
	seenIDs := make(map[uuid.UUID]bool)

	for _, tx := range txs {
		if tx.SourceAccountID() != uuid.Nil {
			seenIDs[tx.SourceAccountID()] = true
		}
		if tx.TargetAccountID() != uuid.Nil {
			seenIDs[tx.TargetAccountID()] = true
		}

		if err := tx.ValidateConsistency(); err != nil {
			result.DiscrepanciesFound = true
			result.Discrepancies = append(result.Discrepancies, Discrepancy{
				Type:        "INVALID_TRANSACTION",
				AggregateID: tx.ID(),
				Details:     err.Error(),
			})
		}
	}
}

func (rs *ReconciliationService) validateAccountConsistency(ctx context.Context, accounts []*account.Account, result *ReconciliationResult) {
	for _, acc := range accounts {
		if err := acc.ValidateConsistency(); err != nil {
			result.DiscrepanciesFound = true
			result.Discrepancies = append(result.Discrepancies, Discrepancy{
				Type:        "INVALID_ACCOUNT",
				AggregateID: acc.ID(),
				Details:     err.Error(),
			})
		}

		if acc.Balance() < 0 {
			result.DiscrepanciesFound = true
			result.Discrepancies = append(result.Discrepancies, Discrepancy{
				Type:        "NEGATIVE_BALANCE",
				AggregateID: acc.ID(),
				Details:     "Account has negative balance",
				ActualValue: acc.Balance(),
			})
		}
	}
}

type TransactionSnapshot struct {
	Timestamp        time.Time
	Transactions     []*transaction.Transaction
	TotalAmount      int64
	TransactionCount int
}

func (rs *ReconciliationService) GetTransactionSnapshot(ctx context.Context, status transaction.TransactionStatus) (*TransactionSnapshot, error) {
	txs, err := rs.transactionRepo.FindByStatus(ctx, status, 100000, 0)
	if err != nil {
		return nil, err
	}

	var totalAmount int64
	for _, tx := range txs {
		totalAmount += tx.Amount()
	}

	return &TransactionSnapshot{
		Timestamp:        time.Now(),
		Transactions:     txs,
		TotalAmount:      totalAmount,
		TransactionCount: len(txs),
	}, nil
}

func (rs *ReconciliationService) GenerateReconciliationReport(ctx context.Context, result *ReconciliationResult) string {
	report := fmt.Sprintf(`
=== SETTLEMENT ENGINE RECONCILIATION REPORT ===
Reconciliation ID: %s
Executed At: %s
Processing Time: %dms

=== SUMMARY ===
Total Accounts: %d
Total Transactions: %d
Processed Transactions: %d
Failed Transactions: %d
Pending Transactions: %d

=== FINANCIAL SUMMARY ===
Total Debit Amount: %.2f
Total Credit Amount: %.2f
Net Flow: %.2f
Total Balance (All Accounts): %.2f

=== DISCREPANCIES ===
Discrepancies Found: %v
Number of Discrepancies: %d

`,
		result.ReconciliationID,
		result.ExecutedAt.Format(time.RFC3339),
		result.ProcessingTimeMs,
		result.TotalAccounts,
		result.TotalTransactions,
		result.ProcessedTransactions,
		result.FailedTransactions,
		result.PendingTransactions,
		float64(result.SummaryReport.TotalDebitAmount)/100.0,
		float64(result.SummaryReport.TotalCreditAmount)/100.0,
		float64(result.SummaryReport.NetFlow)/100.0,
		float64(result.SummaryReport.TotalBalanceAllAccounts)/100.0,
		result.DiscrepanciesFound,
		len(result.Discrepancies),
	)

	if len(result.Discrepancies) > 0 {
		report += "\nDETAILS:\n"
		for i, disc := range result.Discrepancies {
			report += fmt.Sprintf("%d. Type: %s | ID: %s | Details: %s\n",
				i+1, disc.Type, disc.AggregateID, disc.Details)
		}
	}

	return report
}
