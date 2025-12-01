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
	"github.com/stellar/friendbot/testutil"
	"github.com/stellar/go/amount"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/txnbuild"
	"github.com/stellar/go/xdr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This RPC instance must be configured with a working friendbot
// endpoint as these tests depend on the RPC friendbot to setup
// accounts for the friendbot in these tests to use.
var rpcURL = os.Getenv("RPC_URL")

// rpcIntegrationSetup holds all the components needed for RPC integration tests
type rpcIntegrationSetup struct {
	Router            http.Handler
	NetworkClient     internal.NetworkClient
	NetworkPassphrase string
	StartingBalance   string
	MinionKeypair     *keypair.Full
}

// Setup creates running instance of friendbot from current code and requires an external instance of RPC that has been configured with its own separate instance of friendbot to support funding accounts. These tests utilize that to fund new minion and bot accounts on target network used by this local friendbot instance being tested.
func setupRPCIntegration(t *testing.T) *rpcIntegrationSetup {
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
	return &rpcIntegrationSetup{
		Router:            router,
		NetworkClient:     rpcClient,
		NetworkPassphrase: networkPassphrase,
		StartingBalance:   startingBalance,
		MinionKeypair:     minionKeypair,
	}
}

// getContractBalance queries the native SAC contract's balance function to get the XLM balance
// of a contract address. Returns the balance as a string in stroops.
func getContractBalance(t *testing.T, setup *rpcIntegrationSetup, contractAddress string) int64 {
	t.Helper()

	// Decode the contract address to get the contract ID
	contractIDBytes, err := strkey.Decode(strkey.VersionByteContract, contractAddress)
	require.NoError(t, err)

	// Convert to xdr.ContractId (which is a typedef for Hash, which is [32]byte)
	var contractID xdr.ContractId
	copy(contractID[:], contractIDBytes)

	// Get the native asset contract ID
	nativeSACIDBytes, err := xdr.MustNewNativeAsset().ContractID(setup.NetworkPassphrase)
	require.NoError(t, err)

	// Convert to xdr.ContractId
	nativeSACID := xdr.ContractId(nativeSACIDBytes)

	// Build the balance function call
	// balance(id: Address) -> i128
	contractIDScAddress := xdr.ScAddress{
		Type:       xdr.ScAddressTypeScAddressTypeContract,
		ContractId: &contractID,
	}

	invokeOp := txnbuild.InvokeHostFunction{
		HostFunction: xdr.HostFunction{
			Type: xdr.HostFunctionTypeHostFunctionTypeInvokeContract,
			InvokeContract: &xdr.InvokeContractArgs{
				ContractAddress: xdr.ScAddress{
					Type:       xdr.ScAddressTypeScAddressTypeContract,
					ContractId: &nativeSACID,
				},
				FunctionName: xdr.ScSymbol("balance"),
				Args: xdr.ScVec{
					xdr.ScVal{
						Type:    xdr.ScValTypeScvAddress,
						Address: &contractIDScAddress,
					},
				},
			},
		},
		SourceAccount: setup.MinionKeypair.Address(),
	}

	// Get account details for sequence number
	accountDetails, err := setup.NetworkClient.GetAccountDetails(setup.MinionKeypair.Address())
	require.NoError(t, err)

	seqNum, err := accountDetails.ParseSequenceNumber()
	require.NoError(t, err)

	// Build the transaction
	tx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount: &txnbuild.SimpleAccount{
				AccountID: setup.MinionKeypair.Address(),
				Sequence:  seqNum,
			},
			IncrementSequenceNum: true,
			Operations:           []txnbuild.Operation{&invokeOp},
			BaseFee:              txnbuild.MinBaseFee,
			Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewInfiniteTimeout()},
		},
	)
	require.NoError(t, err)

	// Serialize for simulation
	txXDR, err := tx.Base64()
	require.NoError(t, err)

	// Simulate the transaction
	simResult, err := setup.NetworkClient.SimulateTransaction(txXDR)
	require.NoError(t, err)

	// Parse the result - it should contain an i128 value
	// The result is in the simulation response's results array
	require.NotEmpty(t, simResult.ResultXDR, "simulation should return a result")

	var resultVal xdr.ScVal
	err = xdr.SafeUnmarshalBase64(simResult.ResultXDR, &resultVal)
	require.NoError(t, err)

	// The balance is returned as an i128
	require.Equal(t, xdr.ScValTypeScvI128, resultVal.Type, "expected i128 result type")

	i128 := resultVal.MustI128()
	// For balances that fit in int64, the high part should be 0
	require.Equal(t, int64(0), int64(i128.Hi), "balance too large for int64")

	return int64(i128.Lo)
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
	setup := setupRPCIntegration(t)

	// Generate random recipient address
	recipientKeypair, err := keypair.Random()
	require.NoError(t, err)
	recipientAddress := recipientKeypair.Address()

	req := httptest.NewRequest("GET", "/?addr="+recipientAddress, nil)
	w := httptest.NewRecorder()

	setup.Router.ServeHTTP(w, req)

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
	accountDetails, err := setup.NetworkClient.GetAccountDetails(recipientAddress)
	require.NoError(t, err)

	// Balance is returned as XLM string format
	balance := accountDetails.Balance
	expectedBalance := "1000.0000000"
	assert.Equal(t, expectedBalance, balance)
}

func TestFriendbotRPCIntegration_SuccessfulFunding_POST(t *testing.T) {
	setup := setupRPCIntegration(t)

	// Generate random recipient address
	recipientKeypair, err := keypair.Random()
	require.NoError(t, err)
	recipientAddress := recipientKeypair.Address()

	formData := url.Values{}
	formData.Set("addr", recipientAddress)

	req := httptest.NewRequest("POST", "/", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	setup.Router.ServeHTTP(w, req)

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
	accountDetails, err := setup.NetworkClient.GetAccountDetails(recipientAddress)
	require.NoError(t, err)

	// Balance is returned as XLM string format
	balance := accountDetails.Balance
	expectedBalance := "1000.0000000"
	assert.Equal(t, expectedBalance, balance)
}

func TestFriendbotRPCIntegration_MissingAddressParameter(t *testing.T) {
	setup := setupRPCIntegration(t)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	setup.Router.ServeHTTP(w, req)

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
	setup := setupRPCIntegration(t)

	invalidAddress := "invalid_address"

	req := httptest.NewRequest("GET", "/?addr="+invalidAddress, nil)
	w := httptest.NewRecorder()

	setup.Router.ServeHTTP(w, req)

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
	setup := setupRPCIntegration(t)

	// Generate random recipient address
	recipientKeypair, err := keypair.Random()
	require.NoError(t, err)
	recipientAddress := recipientKeypair.Address()

	// First funding attempt
	req := httptest.NewRequest("GET", "/?addr="+recipientAddress, nil)
	w := httptest.NewRecorder()
	setup.Router.ServeHTTP(w, req)

	// Check that the recipient account has the expected balance after first funding
	accountDetails, err := setup.NetworkClient.GetAccountDetails(recipientAddress)
	require.NoError(t, err)

	balance := accountDetails.Balance
	expectedBalance := "1000.0000000"
	assert.Equal(t, expectedBalance, balance)

	// Second funding attempt - should fail (either with account already funded or transaction error)
	req2 := httptest.NewRequest("GET", "/?addr="+recipientAddress, nil)
	w2 := httptest.NewRecorder()
	setup.Router.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusBadRequest, w2.Code)

	// Check that the balance hasn't changed after the failed funding attempt
	accountDetails2, err := setup.NetworkClient.GetAccountDetails(recipientAddress)
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
	setup := setupRPCIntegration(t)

	// Generate random recipient address
	recipientKeypair, err := keypair.Random()
	require.NoError(t, err)
	recipientAddress := recipientKeypair.Address()

	// First funding attempt
	req := httptest.NewRequest("GET", "/?addr="+recipientAddress, nil)
	w := httptest.NewRecorder()
	setup.Router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	// Check that the recipient account has the expected balance after first funding
	accountDetails, err := setup.NetworkClient.GetAccountDetails(recipientAddress)
	require.NoError(t, err)

	balance := accountDetails.Balance
	expectedBalance := "1000.0000000"
	assert.Equal(t, expectedBalance, balance)

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
	bumpSeqTx, err = bumpSeqTx.Sign(setup.NetworkPassphrase, recipientKeypair)
	require.NoError(t, err)
	bumpSeqTxXDR, err := bumpSeqTx.Base64()
	require.NoError(t, err)

	err = setup.NetworkClient.SubmitTransaction(bumpSeqTxXDR)
	require.NoError(t, err)

	// Check balance after bump seq tx - should be slightly lower due to fees
	accountDetails2, err := setup.NetworkClient.GetAccountDetails(recipientAddress)
	require.NoError(t, err)

	balance2 := accountDetails2.Balance
	assert.NotEqual(t, balance, balance2, "balance should have decreased due to transaction fees")

	// Second funding attempt - should succeed since balance is now below starting balance
	req2 := httptest.NewRequest("GET", "/?addr="+recipientAddress, nil)
	w2 := httptest.NewRecorder()
	setup.Router.ServeHTTP(w2, req2)

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
	accountDetails3, err := setup.NetworkClient.GetAccountDetails(recipientAddress)
	require.NoError(t, err)

	balance3 := accountDetails3.Balance
	// Balance should be: original (1000) - bump seq fee (0.00001) + starting balance (1000) = 1999.9999900
	expectedBalance3 := "1999.9999900"
	assert.Equal(t, expectedBalance3, balance3)
}

func TestFriendbotRPCIntegration_404NotFound(t *testing.T) {
	setup := setupRPCIntegration(t)

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()

	setup.Router.ServeHTTP(w, req)

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

// TestFriendbotRPCIntegration_ContractFunding_GET tests funding a contract address (C address)
// using the native Stellar Asset Contract (SAC) transfer function.
func TestFriendbotRPCIntegration_ContractFunding_GET(t *testing.T) {
	setup := setupRPCIntegration(t)

	// Use a well-known contract address (all zeros with valid checksum)
	contractAddress := "CAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABSC4"

	// Get balance before funding
	balanceBefore := getContractBalance(t, setup, contractAddress)
	t.Logf("Contract balance before funding: %d stroops", balanceBefore)

	req := httptest.NewRequest("GET", "/?addr="+url.QueryEscape(contractAddress), nil)
	w := httptest.NewRecorder()

	setup.Router.ServeHTTP(w, req)

	// The request should succeed (200 OK)
	assert.Equal(t, http.StatusOK, w.Code)

	body := w.Body.String()
	var result struct {
		Hash        string `json:"hash"`
		Successful  bool   `json:"successful"`
		EnvelopeXdr string `json:"envelope_xdr"`
	}
	err := json.Unmarshal([]byte(body), &result)
	require.NoError(t, err)
	assert.True(t, result.Successful)
	assert.NotEmpty(t, result.Hash)
	assert.NotEmpty(t, result.EnvelopeXdr)

	// Get balance after funding and verify it increased by the starting balance
	balanceAfter := getContractBalance(t, setup, contractAddress)
	t.Logf("Contract balance after funding: %d stroops", balanceAfter)

	// Convert starting balance to stroops
	expectedIncreaseStroops, err := amount.ParseInt64(setup.StartingBalance)
	require.NoError(t, err)

	actualIncrease := balanceAfter - balanceBefore
	assert.Equal(t, expectedIncreaseStroops, actualIncrease,
		"Contract balance should have increased by %s XLM (%d stroops)",
		setup.StartingBalance, expectedIncreaseStroops)

	t.Logf("Successfully funded contract %s with transaction hash %s", contractAddress, result.Hash)
}

// TestFriendbotRPCIntegration_ContractFunding_POST tests funding a contract address using POST method.
func TestFriendbotRPCIntegration_ContractFunding_POST(t *testing.T) {
	setup := setupRPCIntegration(t)

	// Use a well-known contract address (all zeros with valid checksum)
	contractAddress := "CAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABSC4"

	// Get balance before funding
	balanceBefore := getContractBalance(t, setup, contractAddress)
	t.Logf("Contract balance before funding: %d stroops", balanceBefore)

	formData := url.Values{}
	formData.Set("addr", contractAddress)

	req := httptest.NewRequest("POST", "/", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	setup.Router.ServeHTTP(w, req)

	// The request should succeed (200 OK)
	assert.Equal(t, http.StatusOK, w.Code)

	body := w.Body.String()
	var result struct {
		Hash        string `json:"hash"`
		Successful  bool   `json:"successful"`
		EnvelopeXdr string `json:"envelope_xdr"`
	}
	err := json.Unmarshal([]byte(body), &result)
	require.NoError(t, err)
	assert.True(t, result.Successful)
	assert.NotEmpty(t, result.Hash)
	assert.NotEmpty(t, result.EnvelopeXdr)

	// Get balance after funding and verify it increased by the starting balance
	balanceAfter := getContractBalance(t, setup, contractAddress)
	t.Logf("Contract balance after funding: %d stroops", balanceAfter)

	// Convert starting balance to stroops
	expectedIncreaseStroops, err := amount.ParseInt64(setup.StartingBalance)
	require.NoError(t, err)

	actualIncrease := balanceAfter - balanceBefore
	assert.Equal(t, expectedIncreaseStroops, actualIncrease,
		"Contract balance should have increased by %s XLM (%d stroops)",
		setup.StartingBalance, expectedIncreaseStroops)

	t.Logf("Successfully funded contract %s with transaction hash %s", contractAddress, result.Hash)
}

// TestFriendbotRPCIntegration_InvalidContractAddress tests that an invalid contract address
// (that looks like a C address but is malformed) returns an error.
func TestFriendbotRPCIntegration_InvalidContractAddress(t *testing.T) {
	setup := setupRPCIntegration(t)

	// An invalid C address (wrong checksum)
	invalidContractAddress := "CINVALIDCONTRACTADDRESS12345"

	req := httptest.NewRequest("GET", "/?addr="+url.QueryEscape(invalidContractAddress), nil)
	w := httptest.NewRecorder()

	setup.Router.ServeHTTP(w, req)

	// Should return 400 Bad Request with address validation error
	assert.Equal(t, http.StatusBadRequest, w.Code)

	body := w.Body.String()
	assert.Contains(t, body, `"invalid_field"`)
	assert.Contains(t, body, `"addr"`)
}
