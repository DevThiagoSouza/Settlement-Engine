CREATE TABLE IF NOT EXISTS accounts (
    id UUID PRIMARY KEY,
    customer_id VARCHAR(255) NOT NULL UNIQUE,
    balance BIGINT NOT NULL CHECK (balance >= 0),
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
    last_modified TIMESTAMP WITH TIME ZONE NOT NULL,
    created_index TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_accounts_customer_id ON accounts(customer_id);
CREATE INDEX idx_accounts_created_at ON accounts(created_at DESC);
CREATE INDEX idx_accounts_updated_at ON accounts(updated_at DESC);
CREATE INDEX idx_accounts_last_modified ON accounts(last_modified DESC);

CREATE TABLE IF NOT EXISTS transactions (
    id UUID PRIMARY KEY,
    idempotency_key VARCHAR(255) NOT NULL UNIQUE,
    source_account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
    target_account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    amount BIGINT NOT NULL CHECK (amount > 0),
    transaction_type VARCHAR(50) NOT NULL,
    status VARCHAR(50) NOT NULL,
    description VARCHAR(500),
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    processed_at TIMESTAMP WITH TIME ZONE,
    failure_reason VARCHAR(500),
    created_index TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_transactions_idempotency_key ON transactions(idempotency_key);
CREATE INDEX idx_transactions_source_account_id ON transactions(source_account_id);
CREATE INDEX idx_transactions_target_account_id ON transactions(target_account_id);
CREATE INDEX idx_transactions_status ON transactions(status);
CREATE INDEX idx_transactions_created_at ON transactions(created_at DESC);
CREATE INDEX idx_transactions_processed_at ON transactions(processed_at DESC) WHERE processed_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS outbox_events (
    id UUID PRIMARY KEY,
    aggregate_id UUID NOT NULL,
    event_type VARCHAR(255) NOT NULL,
    event_data BYTEA NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'PENDING',
    published_at TIMESTAMP WITH TIME ZONE,
    acknowledged_at TIMESTAMP WITH TIME ZONE,
    retry_count INTEGER NOT NULL DEFAULT 0,
    max_retries INTEGER NOT NULL DEFAULT 3,
    failure_reason VARCHAR(500),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_index TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_outbox_events_status ON outbox_events(status);
CREATE INDEX idx_outbox_events_aggregate_id ON outbox_events(aggregate_id);
CREATE INDEX idx_outbox_events_created_at ON outbox_events(created_at DESC);
CREATE INDEX idx_outbox_events_updated_at ON outbox_events(updated_at DESC);
CREATE INDEX idx_outbox_events_status_created ON outbox_events(status, created_at DESC);

CREATE TABLE IF NOT EXISTS audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_type VARCHAR(100) NOT NULL,
    entity_id UUID NOT NULL,
    operation VARCHAR(50) NOT NULL,
    old_values JSONB,
    new_values JSONB,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by VARCHAR(255)
);

CREATE INDEX idx_audit_log_entity ON audit_log(entity_type, entity_id);
CREATE INDEX idx_audit_log_created_at ON audit_log(created_at DESC);

CREATE TABLE IF NOT EXISTS settlement_ledger (
    id UUID PRIMARY KEY,
    transaction_id UUID NOT NULL REFERENCES transactions(id),
    source_account_id UUID NOT NULL REFERENCES accounts(id),
    target_account_id UUID NOT NULL REFERENCES accounts(id),
    amount BIGINT NOT NULL,
    status VARCHAR(50) NOT NULL,
    settled_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE INDEX idx_settlement_ledger_transaction_id ON settlement_ledger(transaction_id);
CREATE INDEX idx_settlement_ledger_source_account ON settlement_ledger(source_account_id);
CREATE INDEX idx_settlement_ledger_target_account ON settlement_ledger(target_account_id);
CREATE INDEX idx_settlement_ledger_status ON settlement_ledger(status);
CREATE INDEX idx_settlement_ledger_created_at ON settlement_ledger(created_at DESC);

CREATE TABLE IF NOT EXISTS reconciliation_reports (
    id UUID PRIMARY KEY,
    executed_at TIMESTAMP WITH TIME ZONE NOT NULL,
    processing_time_ms BIGINT NOT NULL,
    total_accounts BIGINT NOT NULL,
    total_transactions BIGINT NOT NULL,
    processed_transactions BIGINT NOT NULL,
    failed_transactions BIGINT NOT NULL,
    pending_transactions BIGINT NOT NULL,
    discrepancies_found BOOLEAN NOT NULL,
    discrepancies_count INTEGER NOT NULL,
    total_debit_amount BIGINT NOT NULL,
    total_credit_amount BIGINT NOT NULL,
    net_flow BIGINT NOT NULL,
    total_balance_all_accounts BIGINT NOT NULL,
    report_data JSONB,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_reconciliation_reports_executed_at ON reconciliation_reports(executed_at DESC);
CREATE INDEX idx_reconciliation_reports_discrepancies ON reconciliation_reports(discrepancies_found);
CREATE INDEX idx_reconciliation_reports_created_at ON reconciliation_reports(created_at DESC);
