package dtos

type CreateAccountRequest struct {
	CustomerID     string `json:"customer_id"`
	InitialBalance int64  `json:"initial_balance"`
}

type ProcessTransactionRequest struct {
	IdempotencyKey  string `json:"idempotency_key"`
	SourceAccountID string `json:"source_account_id"`
	TargetAccountID string `json:"target_account_id"`
	Amount          int64  `json:"amount"`
	Description     string `json:"description"`
}
