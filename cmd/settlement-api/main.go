package main

import (
	"context"
	"fmt"
	"log"

	"github.com/settlement-engine/internal/application/services"
	"github.com/settlement-engine/internal/infrastructure/database"
	"github.com/settlement-engine/internal/infrastructure/persistence"
	"github.com/settlement-engine/internal/infrastructure/reconciliation"
)

func runExample() {
	cfg := database.DefaultConfig()
	db, err := database.NewConnection(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	accountRepo := persistence.NewPostgresAccountRepository(db)
	transactionRepo := persistence.NewPostgresTransactionRepository(db)
	outboxRepo := persistence.NewPostgresOutboxRepository(db)

	settlementService := services.NewSettlementService(accountRepo, transactionRepo, outboxRepo)
	reconciliationService := reconciliation.NewReconciliationService(accountRepo, transactionRepo)

	ctx := context.Background()

	fmt.Println("=== Creating Accounts ===")

	createAcctReq1 := services.CreateAccountRequest{
		CustomerID:     "CUST-001",
		InitialBalance: 100000,
	}

	accResp1, err := settlementService.CreateAccount(ctx, createAcctReq1)
	if err != nil {
		log.Fatalf("Failed to create account 1: %v", err)
	}
	fmt.Printf("Account 1 Created: ID=%s, Balance=%.2f\n", accResp1.AccountID, float64(accResp1.Balance)/100)

	createAcctReq2 := services.CreateAccountRequest{
		CustomerID:     "CUST-002",
		InitialBalance: 50000,
	}

	accResp2, err := settlementService.CreateAccount(ctx, createAcctReq2)
	if err != nil {
		log.Fatalf("Failed to create account 2: %v", err)
	}
	fmt.Printf("Account 2 Created: ID=%s, Balance=%.2f\n", accResp2.AccountID, float64(accResp2.Balance)/100)

	fmt.Println("\n=== Processing Transactions ===")

	txReq1 := services.ProcessTransactionRequest{
		IdempotencyKey:  "TX-2026-05-07-001",
		SourceAccountID: accResp1.AccountID,
		TargetAccountID: accResp2.AccountID,
		Amount:          10000,
		Description:     "Pagamento de fatura",
	}

	txResp1, err := settlementService.ProcessTransaction(ctx, txReq1)
	if err != nil {
		log.Fatalf("Failed to process transaction 1: %v", err)
	}
	fmt.Printf("Transaction 1: Status=%s, Source Balance=%.2f, Target Balance=%.2f\n",
		txResp1.Status, float64(txResp1.SourceBalance)/100, float64(txResp1.TargetBalance)/100)

	fmt.Println("\n--- Reprocessing same transaction (should return cached result) ---")
	txResp1Dup, err := settlementService.ProcessTransaction(ctx, txReq1)
	if err != nil {
		log.Fatalf("Failed to reprocess transaction 1: %v", err)
	}
	fmt.Printf("Transaction 1 (Reprocessed): Status=%s, ID=%s\n", txResp1Dup.Status, txResp1Dup.TransactionID)

	txReq2 := services.ProcessTransactionRequest{
		IdempotencyKey:  "TX-2026-05-07-002",
		SourceAccountID: accResp2.AccountID,
		TargetAccountID: accResp1.AccountID,
		Amount:          5000,
		Description:     "Pagamento retorno",
	}

	txResp2, err := settlementService.ProcessTransaction(ctx, txReq2)
	if err != nil {
		log.Fatalf("Failed to process transaction 2: %v", err)
	}
	fmt.Printf("Transaction 2: Status=%s, Source Balance=%.2f, Target Balance=%.2f\n",
		txResp2.Status, float64(txResp2.SourceBalance)/100, float64(txResp2.TargetBalance)/100)

	fmt.Println("\n=== Attempting Transaction with Insufficient Funds ===")
	txReqFail := services.ProcessTransactionRequest{
		IdempotencyKey:  "TX-2026-05-07-003",
		SourceAccountID: accResp2.AccountID,
		TargetAccountID: accResp1.AccountID,
		Amount:          1000000,
		Description:     "Transferência que vai falhar",
	}

	txRespFail, err := settlementService.ProcessTransaction(ctx, txReqFail)
	if err != nil {
		log.Fatalf("Failed to process failing transaction: %v", err)
	}
	fmt.Printf("Transaction (Expected to Fail): Status=%s, Error=%s\n", txRespFail.Status, txRespFail.ErrorMessage)

	fmt.Println("\n=== Fetching Account Information ===")
	getAccResp, err := settlementService.GetAccount(ctx, accResp1.AccountID)
	if err != nil {
		log.Fatalf("Failed to get account: %v", err)
	}
	fmt.Printf("Account 1 Final State: ID=%s, Balance=%.2f, Version=%d\n",
		getAccResp.AccountID, float64(getAccResp.Balance)/100, getAccResp.Version)

	getTxResp, err := settlementService.GetTransaction(ctx, txResp1.TransactionID)
	if err != nil {
		log.Fatalf("Failed to get transaction: %v", err)
	}
	fmt.Printf("Transaction 1 Details: ID=%s, Status=%s, Amount=%.2f\n",
		getTxResp.TransactionID, getTxResp.Status, float64(getTxResp.Amount)/100)

	fmt.Println("\n=== Running Reconciliation ===")

	fullReconciliation, err := reconciliationService.ReconcileAll(ctx)
	if err != nil {
		log.Fatalf("Failed to run reconciliation: %v", err)
	}

	fmt.Printf("Reconciliation Results:\n")
	fmt.Printf("  Total Accounts: %d\n", fullReconciliation.TotalAccounts)
	fmt.Printf("  Total Transactions: %d\n", fullReconciliation.TotalTransactions)
	fmt.Printf("  Processed: %d, Failed: %d, Pending: %d\n",
		fullReconciliation.ProcessedTransactions,
		fullReconciliation.FailedTransactions,
		fullReconciliation.PendingTransactions)
	fmt.Printf("  Total Debit: %.2f\n", float64(fullReconciliation.SummaryReport.TotalDebitAmount)/100)
	fmt.Printf("  Total Credit: %.2f\n", float64(fullReconciliation.SummaryReport.TotalCreditAmount)/100)
	fmt.Printf("  Net Flow: %.2f\n", float64(fullReconciliation.SummaryReport.NetFlow)/100)
	fmt.Printf("  Total Balance: %.2f\n", float64(fullReconciliation.SummaryReport.TotalBalanceAllAccounts)/100)
	fmt.Printf("  Discrepancies Found: %v\n", fullReconciliation.DiscrepanciesFound)

	report := reconciliationService.GenerateReconciliationReport(ctx, fullReconciliation)
	fmt.Println(report)

	fmt.Println("\n=== Reconciling Account 1 ===")
	acctReconciliation, err := reconciliationService.ReconcileAccount(ctx, accResp1.AccountID)
	if err != nil {
		log.Fatalf("Failed to reconcile account: %v", err)
	}

	fmt.Printf("Account Reconciliation:\n")
	fmt.Printf("  Transactions: %d\n", acctReconciliation.TotalTransactions)
	fmt.Printf("  Processed: %d\n", acctReconciliation.ProcessedTransactions)
	fmt.Printf("  Discrepancies: %v (count: %d)\n",
		acctReconciliation.DiscrepanciesFound, len(acctReconciliation.Discrepancies))

	fmt.Println("\n=== Transaction Metrics ===")
	txMetrics, err := transactionRepo.GetMetrics(ctx)
	if err != nil {
		log.Fatalf("Failed to get metrics: %v", err)
	}

	fmt.Printf("Transaction Metrics:\n")
	fmt.Printf("  Total Count: %d\n", txMetrics.TotalCount)
	fmt.Printf("  Processed: %d\n", txMetrics.ProcessedCount)
	fmt.Printf("  Failed: %d\n", txMetrics.FailedCount)
	fmt.Printf("  Pending: %d\n", txMetrics.PendingCount)
	fmt.Printf("  Total Amount: %.2f\n", float64(txMetrics.TotalAmount)/100)
	fmt.Printf("  Average Amount: %.2f\n", float64(txMetrics.AverageAmount)/100)

	fmt.Println("\n=== Outbox Events Statistics ===")
	outboxStats, err := outboxRepo.GetStatistics(ctx)
	if err != nil {
		log.Fatalf("Failed to get outbox stats: %v", err)
	}

	fmt.Printf("Outbox Events:\n")
	fmt.Printf("  Total: %d\n", outboxStats.TotalEvents)
	fmt.Printf("  Pending: %d\n", outboxStats.PendingEvents)
	fmt.Printf("  Published: %d\n", outboxStats.PublishedEvents)
	fmt.Printf("  Acknowledged: %d\n", outboxStats.AcknowledgedEvents)
	fmt.Printf("  Failed: %d\n", outboxStats.FailedEvents)

	fmt.Println("\n=== Exemplo completado com sucesso! ===")
}
