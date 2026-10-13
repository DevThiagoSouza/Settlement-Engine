package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/settlement-engine/internal/application/dtos"
	"github.com/stretchr/testify/suite"
)

type IntegrationTestSuite struct {
	suite.Suite
	client     *http.Client
	baseURL    string
	serverPort string
}

func (suite *IntegrationTestSuite) SetupSuite() {
	suite.client = &http.Client{
		Timeout: 10 * time.Second,
	}
	suite.serverPort = "8080"
	suite.baseURL = fmt.Sprintf("http://localhost:%s/api/v1", suite.serverPort)

	timeout := time.Now().Add(2 * time.Second)
	for time.Now().Before(timeout) {
		resp, err := suite.client.Get(fmt.Sprintf("http://localhost:%s/health", suite.serverPort))
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	suite.T().Skip("integration server is not available at localhost:8080")
}

func (suite *IntegrationTestSuite) request(method, endpoint string, body interface{}) (*http.Response, []byte) {
	var bodyReader io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		suite.NoError(err)
		bodyReader = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequest(method, suite.baseURL+endpoint, bodyReader)
	suite.NoError(err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := suite.client.Do(req)
	suite.NoError(err)
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	suite.NoError(err)

	return resp, data
}

func (suite *IntegrationTestSuite) TestHealthCheck() {
	resp, _ := suite.request("GET", "/health", nil)
	suite.Equal(http.StatusOK, resp.StatusCode)
}

func (suite *IntegrationTestSuite) TestCreateAccount() {
	req := dtos.CreateAccountRequest{
		CustomerID:     fmt.Sprintf("CUST-%s", uuid.New().String()[:8]),
		InitialBalance: 100000,
	}

	resp, data := suite.request("POST", "/accounts", req)
	suite.Equal(http.StatusCreated, resp.StatusCode)

	var result map[string]interface{}
	err := json.Unmarshal(data, &result)
	suite.NoError(err)

	accountData := result["data"].(map[string]interface{})
	suite.NotNil(accountData["account_id"])
	suite.Equal(req.CustomerID, accountData["customer_id"])
	suite.Equal(float64(100000), accountData["balance"])
}

func (suite *IntegrationTestSuite) TestGetAccount() {
	createReq := dtos.CreateAccountRequest{
		CustomerID:     fmt.Sprintf("CUST-%s", uuid.New().String()[:8]),
		InitialBalance: 50000,
	}
	resp, data := suite.request("POST", "/accounts", createReq)
	suite.Equal(http.StatusCreated, resp.StatusCode)

	var createResult map[string]interface{}
	json.Unmarshal(data, &createResult)
	accountID := createResult["data"].(map[string]interface{})["account_id"].(string)

	resp, data = suite.request("GET", fmt.Sprintf("/accounts/get?id=%s", accountID), nil)
	suite.Equal(http.StatusOK, resp.StatusCode)

	var getResult map[string]interface{}
	json.Unmarshal(data, &getResult)
	accountData := getResult["data"].(map[string]interface{})
	suite.Equal(accountID, accountData["account_id"])
	suite.Equal(float64(50000), accountData["balance"])
}

func (suite *IntegrationTestSuite) TestProcessTransactionHappyPath() {
	acc1Req := dtos.CreateAccountRequest{
		CustomerID:     fmt.Sprintf("CUST-%s", uuid.New().String()[:8]),
		InitialBalance: 100000,
	}
	resp1, data1 := suite.request("POST", "/accounts", acc1Req)
	suite.Equal(http.StatusCreated, resp1.StatusCode)
	var result1 map[string]interface{}
	json.Unmarshal(data1, &result1)
	acc1ID := result1["data"].(map[string]interface{})["account_id"].(string)

	acc2Req := dtos.CreateAccountRequest{
		CustomerID:     fmt.Sprintf("CUST-%s", uuid.New().String()[:8]),
		InitialBalance: 50000,
	}
	resp2, data2 := suite.request("POST", "/accounts", acc2Req)
	suite.Equal(http.StatusCreated, resp2.StatusCode)
	var result2 map[string]interface{}
	json.Unmarshal(data2, &result2)
	acc2ID := result2["data"].(map[string]interface{})["account_id"].(string)

	txReq := dtos.ProcessTransactionRequest{
		IdempotencyKey:  fmt.Sprintf("TX-%s", uuid.New().String()[:8]),
		SourceAccountID: acc1ID,
		TargetAccountID: acc2ID,
		Amount:          25000,
		Description:     "Test transfer",
	}

	resp, data := suite.request("POST", "/transactions", txReq)
	suite.Equal(http.StatusOK, resp.StatusCode)

	var txResult map[string]interface{}
	json.Unmarshal(data, &txResult)
	txData := txResult["data"].(map[string]interface{})
	suite.Equal("SETTLED", txData["status"])
	suite.Equal(float64(75000), txData["source_balance"])
	suite.Equal(float64(75000), txData["target_balance"])
}

func (suite *IntegrationTestSuite) TestIdempotency() {
	acc1Req := dtos.CreateAccountRequest{
		CustomerID:     fmt.Sprintf("CUST-%s", uuid.New().String()[:8]),
		InitialBalance: 100000,
	}
	resp1, data1 := suite.request("POST", "/accounts", acc1Req)
	var result1 map[string]interface{}
	json.Unmarshal(data1, &result1)
	acc1ID := result1["data"].(map[string]interface{})["account_id"].(string)

	acc2Req := dtos.CreateAccountRequest{
		CustomerID:     fmt.Sprintf("CUST-%s", uuid.New().String()[:8]),
		InitialBalance: 50000,
	}
	resp2, data2 := suite.request("POST", "/accounts", acc2Req)
	var result2 map[string]interface{}
	json.Unmarshal(data2, &result2)
	acc2ID := result2["data"].(map[string]interface{})["account_id"].(string)

	idempotencyKey := fmt.Sprintf("TX-%s", uuid.New().String()[:8])
	txReq := dtos.ProcessTransactionRequest{
		IdempotencyKey:  idempotencyKey,
		SourceAccountID: acc1ID,
		TargetAccountID: acc2ID,
		Amount:          25000,
		Description:     "Test transfer",
	}

	resp1, data1 = suite.request("POST", "/transactions", txReq)
	suite.Equal(http.StatusOK, resp1.StatusCode)
	var txResult1 map[string]interface{}
	json.Unmarshal(data1, &txResult1)
	txID1 := txResult1["data"].(map[string]interface{})["transaction_id"]

	resp2, data2 = suite.request("POST", "/transactions", txReq)
	suite.Equal(http.StatusOK, resp2.StatusCode)
	var txResult2 map[string]interface{}
	json.Unmarshal(data2, &txResult2)
	txID2 := txResult2["data"].(map[string]interface{})["transaction_id"]

	suite.Equal(txID1, txID2)
}

func (suite *IntegrationTestSuite) TestInsufficientFunds() {
	acc1Req := dtos.CreateAccountRequest{
		CustomerID:     fmt.Sprintf("CUST-%s", uuid.New().String()[:8]),
		InitialBalance: 10000,
	}
	_, data1 := suite.request("POST", "/accounts", acc1Req)
	var result1 map[string]interface{}
	json.Unmarshal(data1, &result1)
	acc1ID := result1["data"].(map[string]interface{})["account_id"].(string)

	acc2Req := dtos.CreateAccountRequest{
		CustomerID:     fmt.Sprintf("CUST-%s", uuid.New().String()[:8]),
		InitialBalance: 50000,
	}
	_, data2 := suite.request("POST", "/accounts", acc2Req)
	var result2 map[string]interface{}
	json.Unmarshal(data2, &result2)
	acc2ID := result2["data"].(map[string]interface{})["account_id"].(string)

	txReq := dtos.ProcessTransactionRequest{
		IdempotencyKey:  fmt.Sprintf("TX-%s", uuid.New().String()[:8]),
		SourceAccountID: acc1ID,
		TargetAccountID: acc2ID,
		Amount:          50000,
		Description:     "Large transfer",
	}

	resp, data := suite.request("POST", "/transactions", txReq)
	suite.Equal(http.StatusOK, resp.StatusCode)

	var txResult map[string]interface{}
	json.Unmarshal(data, &txResult)
	txData := txResult["data"].(map[string]interface{})
	suite.Equal("FAILED", txData["status"])
}

func (suite *IntegrationTestSuite) TestInvalidUUID() {
	resp, _ := suite.request("GET", "/accounts/get?id=invalid-uuid", nil)
	suite.Equal(http.StatusBadRequest, resp.StatusCode)
}

func (suite *IntegrationTestSuite) TestNonexistentAccount() {
	randomUUID := uuid.New().String()
	resp, _ := suite.request("GET", fmt.Sprintf("/accounts/get?id=%s", randomUUID), nil)
	suite.Equal(http.StatusNotFound, resp.StatusCode)
}

func (suite *IntegrationTestSuite) TestConcurrentTransactions() {
	acc1Req := dtos.CreateAccountRequest{
		CustomerID:     fmt.Sprintf("CUST-%s", uuid.New().String()[:8]),
		InitialBalance: 1000000,
	}
	_, data1 := suite.request("POST", "/accounts", acc1Req)
	var result1 map[string]interface{}
	json.Unmarshal(data1, &result1)
	acc1ID := result1["data"].(map[string]interface{})["account_id"].(string)

	acc2Req := dtos.CreateAccountRequest{
		CustomerID:     fmt.Sprintf("CUST-%s", uuid.New().String()[:8]),
		InitialBalance: 500000,
	}
	_, data2 := suite.request("POST", "/accounts", acc2Req)
	var result2 map[string]interface{}
	json.Unmarshal(data2, &result2)
	acc2ID := result2["data"].(map[string]interface{})["account_id"].(string)

	for i := 0; i < 5; i++ {
		txReq := dtos.ProcessTransactionRequest{
			IdempotencyKey:  fmt.Sprintf("TX-CONCURRENT-%d-%s", i, uuid.New().String()[:8]),
			SourceAccountID: acc1ID,
			TargetAccountID: acc2ID,
			Amount:          10000,
			Description:     fmt.Sprintf("Concurrent transfer %d", i),
		}

		resp, data := suite.request("POST", "/transactions", txReq)
		suite.Equal(http.StatusOK, resp.StatusCode)

		var txResult map[string]interface{}
		json.Unmarshal(data, &txResult)
		txData := txResult["data"].(map[string]interface{})
		suite.Equal("SETTLED", txData["status"], fmt.Sprintf("Transaction %d failed", i))
	}

	resp, data := suite.request("GET", fmt.Sprintf("/accounts/get?id=%s", acc1ID), nil)
	suite.Equal(http.StatusOK, resp.StatusCode)
	var finalResult map[string]interface{}
	json.Unmarshal(data, &finalResult)
	finalBalance := finalResult["data"].(map[string]interface{})["balance"]
	expectedBalance := float64(1000000 - 50000)
	suite.Equal(expectedBalance, finalBalance)
}

func (suite *IntegrationTestSuite) TestGetTransaction() {
	acc1Req := dtos.CreateAccountRequest{
		CustomerID:     fmt.Sprintf("CUST-%s", uuid.New().String()[:8]),
		InitialBalance: 100000,
	}
	_, data1 := suite.request("POST", "/accounts", acc1Req)
	var result1 map[string]interface{}
	json.Unmarshal(data1, &result1)
	acc1ID := result1["data"].(map[string]interface{})["account_id"].(string)

	acc2Req := dtos.CreateAccountRequest{
		CustomerID:     fmt.Sprintf("CUST-%s", uuid.New().String()[:8]),
		InitialBalance: 50000,
	}
	_, data2 := suite.request("POST", "/accounts", acc2Req)
	var result2 map[string]interface{}
	json.Unmarshal(data2, &result2)
	acc2ID := result2["data"].(map[string]interface{})["account_id"].(string)

	txReq := dtos.ProcessTransactionRequest{
		IdempotencyKey:  fmt.Sprintf("TX-%s", uuid.New().String()[:8]),
		SourceAccountID: acc1ID,
		TargetAccountID: acc2ID,
		Amount:          25000,
		Description:     "Test transfer",
	}

	resp, data := suite.request("POST", "/transactions", txReq)
	var txResult map[string]interface{}
	json.Unmarshal(data, &txResult)
	txID := txResult["data"].(map[string]interface{})["transaction_id"].(string)

	resp, data = suite.request("GET", fmt.Sprintf("/transactions/get?id=%s", txID), nil)
	suite.Equal(http.StatusOK, resp.StatusCode)

	var getResult map[string]interface{}
	json.Unmarshal(data, &getResult)
	txData := getResult["data"].(map[string]interface{})
	suite.Equal(txID, txData["transaction_id"])
	suite.Equal(float64(25000), txData["amount"])
}

func TestIntegrationSuite(t *testing.T) {
	suite.Run(t, new(IntegrationTestSuite))
}
