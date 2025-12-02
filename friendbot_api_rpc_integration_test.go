package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/stellar/friendbot/internal"
	"github.com/stellar/friendbot/internal/rpcnetworkclient"
	"github.com/stellar/friendbot/internal/testutil"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This RPC instance must be configured with a working friendbot
// endpoint as these tests depend on the RPC friendbot to setup
// accounts for the friendbot in these tests to use.
var rpcURL = os.Getenv("RPC_URL")

// Setup creates running instance of friendbot from current code and requires an external instance of RPC that has been configured with its own separate instance of friendbot to support funding accounts. These tests utilize that to fund new minion and bot accounts on target network used by this local friendbot instance being tested.
func setupRPCIntegration(t *testing.T) (http.Handler, internal.NetworkClient) {
	t.Helper()

	if rpcURL == "" {
		t.Skip("RPC_URL environment variable not set, skipping RPC integration tests")
	}

	// Get network passphrase from RPC
	networkPassphrase := getNetworkPassphraseFromRPC(t, rpcURL)
	startingBalance := "1000.00" // Use smaller amount so bot account keeps reserve
	baseFee := int64(txnbuild.MinBaseFee)

	// Generate random keypair for the bot account that holds the funds
	botKeypair, err := keypair.Random()
	require.NoError(t, err)
	botAccount := internal.Account{AccountID: botKeypair.Address()}
	err = testutil.FundAccount(t, botKeypair.Address())
	require.NoError(t, err)

	// Generate random keypair for a minion that will be used for sequence numbers when funding
	minionKeypair, err := keypair.Random()
	require.NoError(t, err)
	err = testutil.FundAccount(t, minionKeypair.Address())
	require.NoError(t, err)

	// Create RPC client
	rpcClient := rpcnetworkclient.NewNetworkClient(rpcURL, http.DefaultClient)

	// Create minion that will fund accounts
	minion := internal.Minion{
		Account: internal.Account{
			AccountID: minionKeypair.Address(),
		},
		Keypair:              minionKeypair,
		BotAccount:           botAccount,
		BotKeypair:           botKeypair,
		NetworkClient:        rpcClient,
		Network:              networkPassphrase,
		StartingBalance:      startingBalance,
		SubmitTransaction:    internal.SubmitTransaction,
		CheckSequenceRefresh: internal.CheckSequenceRefresh,
		CheckAccountExists:   internal.CheckAccountExists,
		BaseFee:              baseFee,
	}

	fb := &internal.Bot{Minions: []internal.Minion{minion}}
	registerProblems()
	cfg := Config{}
	router := initRouter(cfg, fb)
	return router, rpcClient
}

// getNetworkPassphraseFromRPC fetches the network passphrase from the RPC getNetwork method
func getNetworkPassphraseFromRPC(t *testing.T, rpcURL string) string {
	t.Helper()

	payload := `{"jsonrpc": "2.0", "id": 8675309, "method": "getNetwork"}`
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, rpcURL, strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("RPC getNetwork returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	var response struct {
		Jsonrpc string `json:"jsonrpc"`
		Id      int    `json:"id"`
		Result  struct {
			Passphrase string `json:"passphrase"`
		} `json:"result"`
	}
	err = json.Unmarshal(body, &response)
	if err != nil {
		t.Fatal(err)
	}

	return response.Result.Passphrase
}

func TestFriendbotRPCIntegration_SuccessfulFunding_GET(t *testing.T) {
	router, rpcClient := setupRPCIntegration(t)

	// Generate random recipient address
	recipientKeypair, err := keypair.Random()
	require.NoError(t, err)
	recipientAddress := recipientKeypair.Address()

	req := httptest.NewRequest("GET", "/?addr="+recipientAddress, nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	var result struct {
		Hash        string `json:"hash"`
		Successful  bool   `json:"successful"`
		EnvelopeXdr string `json:"envelope_xdr"`
	}
	err = json.Unmarshal([]byte(body), &result)
	require.NoError(t, err)
	assert.Equal(t, true, result.Successful)
	assert.NotEmpty(t, result.Hash)
	assert.NotEmpty(t, result.EnvelopeXdr)

	// Check that the recipient account has the expected balance
	accountDetails, err := rpcClient.GetAccountDetails(recipientAddress)
	require.NoError(t, err)

	// Balance is returned as XLM string format
	balance := accountDetails.Balance
	expectedBalance := "1000.0000000"
	assert.Equal(t, expectedBalance, balance)
}

func TestFriendbotRPCIntegration_SuccessfulFunding_POST(t *testing.T) {
	router, rpcClient := setupRPCIntegration(t)

	// Generate random recipient address
	recipientKeypair, err := keypair.Random()
	require.NoError(t, err)
	recipientAddress := recipientKeypair.Address()

	formData := url.Values{}
	formData.Set("addr", recipientAddress)

	req := httptest.NewRequest("POST", "/", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	var result struct {
		Hash        string `json:"hash"`
		Successful  bool   `json:"successful"`
		EnvelopeXdr string `json:"envelope_xdr"`
	}
	err = json.Unmarshal([]byte(body), &result)
	require.NoError(t, err)
	assert.Equal(t, true, result.Successful)
	assert.NotEmpty(t, result.Hash)
	assert.NotEmpty(t, result.EnvelopeXdr)

	// Check that the recipient account has the expected balance
	accountDetails, err := rpcClient.GetAccountDetails(recipientAddress)
	require.NoError(t, err)

	// Balance is returned as XLM string format
	balance := accountDetails.Balance
	expectedBalance := "1000.0000000"
	assert.Equal(t, expectedBalance, balance)
}

func TestFriendbotRPCIntegration_MissingAddressParameter(t *testing.T) {
	router, _ := setupRPCIntegration(t)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Assert the full JSON error response matches expected structure
	body := w.Body.String()
	expectedJSON := `{
          "type": "https://stellar.org/friendbot-errors/bad_request",
          "title": "Bad Request",
          "status": 400,
          "detail": "The request you sent was invalid in some way.",
          "extras": {
            "invalid_field": "addr",
            "reason": "strkey is 0 bytes long; minimum valid length is 5"
          }
        }`
	assert.JSONEq(t, expectedJSON, body)
}

func TestFriendbotRPCIntegration_InvalidAddress(t *testing.T) {
	router, _ := setupRPCIntegration(t)

	invalidAddress := "invalid_address"

	req := httptest.NewRequest("GET", "/?addr="+invalidAddress, nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Assert the full JSON error response matches expected structure
	body := w.Body.String()
	expectedJSON := `{
          "type": "https://stellar.org/friendbot-errors/bad_request",
          "title": "Bad Request",
          "status": 400,
          "detail": "The request you sent was invalid in some way.",
          "extras": {
            "invalid_field": "addr",
            "reason": "base32 decode failed: illegal base32 data at input byte 15"
          }
        }`
	assert.JSONEq(t, expectedJSON, body)
}

func TestFriendbotRPCIntegration_AccountAlreadyFunded(t *testing.T) {
	router, rpcClient := setupRPCIntegration(t)

	// Generate random recipient address
	recipientKeypair, err := keypair.Random()
	require.NoError(t, err)
	recipientAddress := recipientKeypair.Address()

	// First funding attempt
	req := httptest.NewRequest("GET", "/?addr="+recipientAddress, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Check that the recipient account has the expected balance after first funding
	accountDetails, err := rpcClient.GetAccountDetails(recipientAddress)
	require.NoError(t, err)

	balance := accountDetails.Balance
	expectedBalance := "1000.0000000"
	assert.Equal(t, expectedBalance, balance)

	// Second funding attempt - should fail (either with account already funded or transaction error)
	req2 := httptest.NewRequest("GET", "/?addr="+recipientAddress, nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusBadRequest, w2.Code)

	// Check that the balance hasn't changed after the failed funding attempt
	accountDetails2, err := rpcClient.GetAccountDetails(recipientAddress)
	require.NoError(t, err)

	balance2 := accountDetails2.Balance
	assert.Equal(t, balance, balance2)

	// Assert the full JSON error response matches expected structure
	body := w2.Body.String()
	expectedJSON := `{
          "type": "https://stellar.org/friendbot-errors/bad_request",
          "title": "Bad Request",
          "status": 400,
          "detail": "account already funded to starting balance"
        }`
	assert.JSONEq(t, expectedJSON, body)
}

func TestFriendbotRPCIntegration_AccountRefundedAfterSpending(t *testing.T) {
	router, rpcClient := setupRPCIntegration(t)

	// Generate random recipient address
	recipientKeypair, err := keypair.Random()
	require.NoError(t, err)
	recipientAddress := recipientKeypair.Address()

	// First funding attempt
	req := httptest.NewRequest("GET", "/?addr="+recipientAddress, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	// Check that the recipient account has the expected balance after first funding
	accountDetails, err := rpcClient.GetAccountDetails(recipientAddress)
	require.NoError(t, err)

	balance := accountDetails.Balance
	expectedBalance := "1000.0000000"
	assert.Equal(t, expectedBalance, balance)

	// Get network passphrase for transaction building
	networkPassphrase := getNetworkPassphraseFromRPC(t, rpcURL)

	// Submit a bump sequence transaction to spend some XLM on fees
	// This will lower the account balance slightly
	bumpSeqTx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount: &txnbuild.SimpleAccount{
				AccountID: recipientAddress,
				Sequence:  accountDetails.Sequence,
			},
			IncrementSequenceNum: true,
			Operations: []txnbuild.Operation{
				&txnbuild.BumpSequence{},
			},
			BaseFee:       txnbuild.MinBaseFee,
			Preconditions: txnbuild.Preconditions{TimeBounds: txnbuild.NewInfiniteTimeout()},
		},
	)
	require.NoError(t, err)
	bumpSeqTx, err = bumpSeqTx.Sign(networkPassphrase, recipientKeypair)
	require.NoError(t, err)
	bumpSeqTxXDR, err := bumpSeqTx.Base64()
	require.NoError(t, err)

	err = rpcClient.SubmitTransaction(bumpSeqTxXDR)
	require.NoError(t, err)

	// Check balance after bump seq tx - should be slightly lower due to fees
	accountDetails2, err := rpcClient.GetAccountDetails(recipientAddress)
	require.NoError(t, err)

	balance2 := accountDetails2.Balance
	assert.NotEqual(t, balance, balance2, "balance should have decreased due to transaction fees")

	// Second funding attempt - should succeed since balance is now below starting balance
	req2 := httptest.NewRequest("GET", "/?addr="+recipientAddress, nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
	body := w2.Body.String()
	var result struct {
		Hash        string `json:"hash"`
		Successful  bool   `json:"successful"`
		EnvelopeXdr string `json:"envelope_xdr"`
	}
	err = json.Unmarshal([]byte(body), &result)
	require.NoError(t, err)
	assert.Equal(t, true, result.Successful)
	assert.NotEmpty(t, result.Hash)
	assert.NotEmpty(t, result.EnvelopeXdr)

	// Check that the recipient account received another starting balance payment
	// (friendbot sends the full starting balance, not just the difference)
	accountDetails3, err := rpcClient.GetAccountDetails(recipientAddress)
	require.NoError(t, err)

	balance3 := accountDetails3.Balance
	// Balance should be: original (1000) - bump seq fee (0.00001) + starting balance (1000) = 1999.9999900
	expectedBalance3 := "1999.9999900"
	assert.Equal(t, expectedBalance3, balance3)
}

func TestFriendbotRPCIntegration_404NotFound(t *testing.T) {
	router, _ := setupRPCIntegration(t)

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	// Assert the full JSON error response matches expected structure
	body := w.Body.String()
	expectedJSON := `{
          "type": "https://stellar.org/friendbot-errors/not_found",
          "title": "Resource Missing",
          "status": 404,
          "detail": "The resource at the url requested was not found.  This usually occurs for one of two reasons:  The url requested is not valid, or no data in our database could be found with the parameters provided."
        }`
	assert.JSONEq(t, expectedJSON, body)
}
