package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/settlement-engine/internal/application/dtos"
	"github.com/stretchr/testify/assert"
)

type LoadTestConfig struct {
	NumAccounts         int
	TransactionsPerTest int
	ConcurrentWorkers   int
	ServerURL           string
	Timeout             time.Duration
}

type LoadTestResults struct {
	TotalTransactions   int64
	SuccessfulTxs       int64
	FailedTxs           int64
	IdempotentMatches   int64
	AverageDuration     time.Duration
	MinDuration         time.Duration
	MaxDuration         time.Duration
	ThroughputPerSecond float64
	StartTime           time.Time
	EndTime             time.Time
}

type LoadTestRunner struct {
	config  LoadTestConfig
	client  *http.Client
	results LoadTestResults
	mutex   sync.Mutex
}

func NewLoadTestRunner(config LoadTestConfig) *LoadTestRunner {
	return &LoadTestRunner{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
		results: LoadTestResults{
			MinDuration: time.Hour,
		},
	}
}

func (r *LoadTestRunner) CreateTestAccounts() ([]string, error) {
	accounts := make([]string, r.config.NumAccounts)

	for i := 0; i < r.config.NumAccounts; i++ {
		req := dtos.CreateAccountRequest{
			CustomerID:     fmt.Sprintf("LOAD-TEST-CUST-%d-%s", i, uuid.New().String()[:8]),
			InitialBalance: 10000000,
		}

		jsonData, _ := json.Marshal(req)
		resp, err := r.client.Post(
			fmt.Sprintf("%s/accounts", r.config.ServerURL),
			"application/json",
			bytes.NewReader(jsonData),
		)

		if err != nil {
			return nil, fmt.Errorf("failed to create account %d: %w", i, err)
		}

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()

		accountID := result["data"].(map[string]interface{})["account_id"].(string)
		accounts[i] = accountID
	}

	return accounts, nil
}

func (r *LoadTestRunner) ProcessTransactionWorker(
	accounts []string,
	txQueue <-chan map[string]interface{},
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	for tx := range txQueue {
		startTime := time.Now()

		sourceIdx := tx["source_idx"].(int)
		targetIdx := tx["target_idx"].(int)
		isRetry := tx["is_retry"].(bool)

		req := dtos.ProcessTransactionRequest{
			IdempotencyKey:  tx["idempotency_key"].(string),
			SourceAccountID: accounts[sourceIdx],
			TargetAccountID: accounts[targetIdx],
			Amount:          int64(tx["amount"].(float64)),
			Description:     "Load test transaction",
		}

		jsonData, _ := json.Marshal(req)
		resp, err := r.client.Post(
			fmt.Sprintf("%s/transactions", r.config.ServerURL),
			"application/json",
			bytes.NewReader(jsonData),
		)

		duration := time.Since(startTime)
		r.updateMetrics(duration, err == nil, isRetry)

		if resp != nil {
			resp.Body.Close()
		}
	}
}

func (r *LoadTestRunner) updateMetrics(duration time.Duration, success bool, isRetry bool) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	atomic.AddInt64(&r.results.TotalTransactions, 1)

	if success {
		atomic.AddInt64(&r.results.SuccessfulTxs, 1)
		if isRetry {
			atomic.AddInt64(&r.results.IdempotentMatches, 1)
		}
	} else {
		atomic.AddInt64(&r.results.FailedTxs, 1)
	}

	if duration < r.results.MinDuration {
		r.results.MinDuration = duration
	}
	if duration > r.results.MaxDuration {
		r.results.MaxDuration = duration
	}
}

func (r *LoadTestRunner) RunTransactionLoadTest() (*LoadTestResults, error) {
	fmt.Printf("Creating %d test accounts...\n", r.config.NumAccounts)
	accounts, err := r.CreateTestAccounts()
	if err != nil {
		return nil, err
	}

	fmt.Printf("Created %d accounts. Starting load test with %d workers...\n",
		len(accounts), r.config.ConcurrentWorkers)

	r.results.StartTime = time.Now()

	txQueue := make(chan map[string]interface{}, r.config.ConcurrentWorkers*2)

	var wg sync.WaitGroup
	for i := 0; i < r.config.ConcurrentWorkers; i++ {
		wg.Add(1)
		go r.ProcessTransactionWorker(accounts, txQueue, &wg)
	}

	go func() {
		for i := 0; i < r.config.TransactionsPerTest; i++ {
			sourceIdx := i % r.config.NumAccounts
			targetIdx := (i + 1) % r.config.NumAccounts

			idempotencyKey := fmt.Sprintf("LOAD-TEST-TX-%d", i)

			txQueue <- map[string]interface{}{
				"source_idx":      sourceIdx,
				"target_idx":      targetIdx,
				"amount":          100000.0,
				"idempotency_key": idempotencyKey,
				"is_retry":        false,
			}

			if i%10 == 0 && i > 0 {
				idempotencyKey = fmt.Sprintf("LOAD-TEST-TX-%d", i-1)
				txQueue <- map[string]interface{}{
					"source_idx":      sourceIdx,
					"target_idx":      targetIdx,
					"amount":          100000.0,
					"idempotency_key": idempotencyKey,
					"is_retry":        true,
				}
			}
		}
		close(txQueue)
	}()

	wg.Wait()
	r.results.EndTime = time.Now()

	totalDuration := r.results.EndTime.Sub(r.results.StartTime)
	r.results.ThroughputPerSecond = float64(r.results.SuccessfulTxs) / totalDuration.Seconds()
	r.results.AverageDuration = time.Duration(int64(totalDuration) / r.results.TotalTransactions)

	return &r.results, nil
}

func (r *LoadTestRunner) RunConcurrencyTest(numConcurrentOps int) (*LoadTestResults, error) {
	sourceReq := dtos.CreateAccountRequest{
		CustomerID:     fmt.Sprintf("CONCURRENCY-TEST-SOURCE-%s", uuid.New().String()[:8]),
		InitialBalance: 100000000,
	}
	jsonData, _ := json.Marshal(sourceReq)
	resp, _ := r.client.Post(
		fmt.Sprintf("%s/accounts", r.config.ServerURL),
		"application/json",
		bytes.NewReader(jsonData),
	)
	var sourceResult map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&sourceResult)
	resp.Body.Close()
	sourceID := sourceResult["data"].(map[string]interface{})["account_id"].(string)

	targetReq := dtos.CreateAccountRequest{
		CustomerID:     fmt.Sprintf("CONCURRENCY-TEST-TARGET-%s", uuid.New().String()[:8]),
		InitialBalance: 50000000,
	}
	jsonData, _ = json.Marshal(targetReq)
	resp, _ = r.client.Post(
		fmt.Sprintf("%s/accounts", r.config.ServerURL),
		"application/json",
		bytes.NewReader(jsonData),
	)
	var targetResult map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&targetResult)
	resp.Body.Close()
	targetID := targetResult["data"].(map[string]interface{})["account_id"].(string)

	r.results.StartTime = time.Now()

	var wg sync.WaitGroup
	wg.Add(numConcurrentOps)

	for i := 0; i < numConcurrentOps; i++ {
		go func(idx int) {
			defer wg.Done()

			startTime := time.Now()

			req := dtos.ProcessTransactionRequest{
				IdempotencyKey:  fmt.Sprintf("CONCURRENCY-TX-%d-%s", idx, uuid.New().String()[:8]),
				SourceAccountID: sourceID,
				TargetAccountID: targetID,
				Amount:          10000,
				Description:     "Concurrent test",
			}

			jsonData, _ := json.Marshal(req)
			resp, err := r.client.Post(
				fmt.Sprintf("%s/transactions", r.config.ServerURL),
				"application/json",
				bytes.NewReader(jsonData),
			)

			duration := time.Since(startTime)
			r.updateMetrics(duration, err == nil, false)

			if resp != nil {
				resp.Body.Close()
			}
		}(i)
	}

	wg.Wait()
	r.results.EndTime = time.Now()

	totalDuration := r.results.EndTime.Sub(r.results.StartTime)
	r.results.ThroughputPerSecond = float64(r.results.SuccessfulTxs) / totalDuration.Seconds()

	return &r.results, nil
}

func TestLoadTransactionProcessing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}
	if !serverAvailable("http://localhost:8080/health") {
		t.Skip("load test server is not available at localhost:8080")
	}

	config := LoadTestConfig{
		NumAccounts:         100,
		TransactionsPerTest: 10000,
		ConcurrentWorkers:   50,
		ServerURL:           "http://localhost:8080/api/v1",
		Timeout:             30 * time.Second,
	}

	runner := NewLoadTestRunner(config)
	results, err := runner.RunTransactionLoadTest()

	assert.NoError(t, err)
	assert.NotNil(t, results)
	assert.Greater(t, results.SuccessfulTxs, int64(0))
	assert.Greater(t, results.ThroughputPerSecond, 0.0)

	fmt.Printf("\n=== Load Test Results ===\n")
	fmt.Printf("Total Transactions: %d\n", results.TotalTransactions)
	fmt.Printf("Successful: %d\n", results.SuccessfulTxs)
	fmt.Printf("Failed: %d\n", results.FailedTxs)
	fmt.Printf("Idempotent Matches: %d\n", results.IdempotentMatches)
	fmt.Printf("Average Duration: %v\n", results.AverageDuration)
	fmt.Printf("Min Duration: %v\n", results.MinDuration)
	fmt.Printf("Max Duration: %v\n", results.MaxDuration)
	fmt.Printf("Throughput: %.2f tx/sec\n", results.ThroughputPerSecond)
	fmt.Printf("Total Time: %v\n", results.EndTime.Sub(results.StartTime))
}

func TestConcurrencySameAccount(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrency test in short mode")
	}
	if !serverAvailable("http://localhost:8080/health") {
		t.Skip("concurrency test server is not available at localhost:8080")
	}

	config := LoadTestConfig{
		ServerURL: "http://localhost:8080/api/v1",
		Timeout:   10 * time.Second,
	}

	runner := NewLoadTestRunner(config)
	results, err := runner.RunConcurrencyTest(100)

	assert.NoError(t, err)
	assert.NotNil(t, results)
	assert.Equal(t, int64(100), results.TotalTransactions)
	assert.Equal(t, int64(100), results.SuccessfulTxs)

	fmt.Printf("\n=== Concurrency Test Results ===\n")
	fmt.Printf("Total Transactions: %d\n", results.TotalTransactions)
	fmt.Printf("Successful: %d\n", results.SuccessfulTxs)
	fmt.Printf("Failed: %d\n", results.FailedTxs)
	fmt.Printf("Throughput: %.2f tx/sec\n", results.ThroughputPerSecond)
	fmt.Printf("Total Time: %v\n", results.EndTime.Sub(results.StartTime))
}

func serverAvailable(url string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
