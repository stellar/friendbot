package main

import (
	"context"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stellar/friendbot/internal"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Setup creates a test friendbot with mocked horizon.
func setup(t *testing.T) http.Handler {
	mockSubmitTransaction := func(ctx context.Context, minion *internal.Minion, networkClient internal.NetworkClient, txHash [32]byte, tx string) (*internal.TransactionResult, error) {
		// Emulate a successful transaction
		txSuccess := internal.TransactionResult{
			EnvelopeXdr: tx,
			Successful:  true,
			Hash:        hex.EncodeToString(txHash[:]),
		}
		return &txSuccess, nil
	}

	mockCheckAccountExists := func(minion *internal.Minion, networkClient internal.NetworkClient, destAddress string) (bool, string, error) {
		// Return account doesn't exist for all test cases (will be overridden in specific tests)
		return false, "0", nil
	}

	botSeed := "SCWNLYELENPBXN46FHYXETT5LJCYBZD5VUQQVW4KZPHFO2YTQJUWT4D5"
	botKeypair, err := keypair.Parse(botSeed)
	require.NoError(t, err)
	botAccount := internal.Account{AccountID: botKeypair.Address()}

	minionSeed := "SDTNSEERJPJFUE2LSDNYBFHYGVTPIWY7TU2IOJZQQGLWO2THTGB7NU5A"
	minionKeypair, err := keypair.Parse(minionSeed)
	require.NoError(t, err)

	minion := internal.Minion{
		Account: internal.Account{
			AccountID: minionKeypair.Address(),
			Sequence:  1,
		},
		Keypair:              minionKeypair.(*keypair.Full),
		BotAccount:           botAccount,
		BotKeypair:           botKeypair.(*keypair.Full),
		NetworkClient:        nil, // Not used in mocks
		Network:              "Test SDF Network ; September 2015",
		StartingBalance:      "10000.00",
		SubmitTransaction:    mockSubmitTransaction,
		CheckSequenceRefresh: internal.CheckSequenceRefresh,
		CheckAccountExists:   mockCheckAccountExists,
		BaseFee:              txnbuild.MinBaseFee,
	}

	fb := &internal.Bot{Minions: []internal.Minion{minion}}

	// Register problem handlers (normally done in main)
	registerProblems()

	// Create router with test config
	cfg := Config{}
	router := initRouter(cfg, fb)

	return router
}

func TestFriendbotAPI_SuccessfulFunding_GET(t *testing.T) {
	router := setup(t)

	recipientAddress := "GDJIN6W6PLTPKLLM57UW65ZH4BITUXUMYQHIMAZFYXF45PZVAWDBI77Z"

	req := httptest.NewRequest("GET", "/?addr="+url.QueryEscape(recipientAddress), nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Assert the full JSON response matches expected structure
	body := w.Body.String()
	expectedJSON := `{
          "successful": true,
          "hash": "a6f2f2459152559f4a5b3cd3c8652ed3491dee7d4c7729659362408db25f731b",
          "envelope_xdr": "AAAAAgAAAAD4Az3jKU6lbzq/L5HG9/GzBT+FYusOz71oyYMbZkP+GAAAAGQAAAAAAAAAAgAAAAEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAPXQ8gjyrVHa47a6JDPkVHwPPDKxNRE2QBcamA4JvlOGAAAAAAAAAADShvreeub1LWzv6W93J+BROl6MxA6GAyXFy86/NQWGFAAAABdIdugAAAAAAAAAAAJmQ/4YAAAAQDRLEljDVYALnTk9mDceQEd5PrjQyE3LUAjstIyTWH5t/TP909F66TgEfBFKMxSKF6fka7ZuPcSs40ix4AomEgoJvlOGAAAAQPSGs88OwXubz7UT6nFhvhF47EQfaOsmiIsOkjgzUrmBoypJQTmMMbgeix0kdbfHqS75+iefJpdXLNFDreGnxgE="
        }`
	assert.JSONEq(t, expectedJSON, body)
}

func TestFriendbotAPI_SuccessfulFunding_POST(t *testing.T) {
	router := setup(t)

	recipientAddress := "GDJIN6W6PLTPKLLM57UW65ZH4BITUXUMYQHIMAZFYXF45PZVAWDBI77Z"

	formData := url.Values{}
	formData.Set("addr", recipientAddress)

	req := httptest.NewRequest("POST", "/", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Assert the full JSON response matches expected structure
	body := w.Body.String()
	expectedJSON := `{
          "successful": true,
          "hash": "a6f2f2459152559f4a5b3cd3c8652ed3491dee7d4c7729659362408db25f731b",
          "envelope_xdr": "AAAAAgAAAAD4Az3jKU6lbzq/L5HG9/GzBT+FYusOz71oyYMbZkP+GAAAAGQAAAAAAAAAAgAAAAEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAPXQ8gjyrVHa47a6JDPkVHwPPDKxNRE2QBcamA4JvlOGAAAAAAAAAADShvreeub1LWzv6W93J+BROl6MxA6GAyXFy86/NQWGFAAAABdIdugAAAAAAAAAAAJmQ/4YAAAAQDRLEljDVYALnTk9mDceQEd5PrjQyE3LUAjstIyTWH5t/TP909F66TgEfBFKMxSKF6fka7ZuPcSs40ix4AomEgoJvlOGAAAAQPSGs88OwXubz7UT6nFhvhF47EQfaOsmiIsOkjgzUrmBoypJQTmMMbgeix0kdbfHqS75+iefJpdXLNFDreGnxgE="
        }`
	assert.JSONEq(t, expectedJSON, body)
}

func TestFriendbotAPI_MissingAddressParameter(t *testing.T) {
	router := setup(t)

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

func TestFriendbotAPI_InvalidAddress(t *testing.T) {
	router := setup(t)

	invalidAddress := "invalid_address"

	req := httptest.NewRequest("GET", "/?addr="+url.QueryEscape(invalidAddress), nil)
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

func TestFriendbotAPI_AccountAlreadyFunded(t *testing.T) {
	// Create friendbot with mock that returns account already exists and funded
	mockSubmitTransaction := func(ctx context.Context, minion *internal.Minion, networkClient internal.NetworkClient, txHash [32]byte, tx string) (*internal.TransactionResult, error) {
		txSuccess := internal.TransactionResult{EnvelopeXdr: tx, Successful: true}
		return &txSuccess, nil
	}

	mockCheckAccountExists := func(minion *internal.Minion, networkClient internal.NetworkClient, destAddress string) (bool, string, error) {
		return true, "10000.00", nil // Account exists and has balance
	}

	botSeed := "SCWNLYELENPBXN46FHYXETT5LJCYBZD5VUQQVW4KZPHFO2YTQJUWT4D5"
	botKeypair, err := keypair.Parse(botSeed)
	require.NoError(t, err)
	botAccount := internal.Account{AccountID: botKeypair.Address()}

	minionSeed := "SDTNSEERJPJFUE2LSDNYBFHYGVTPIWY7TU2IOJZQQGLWO2THTGB7NU5A"
	minionKeypair, err := keypair.Parse(minionSeed)
	require.NoError(t, err)

	minion := internal.Minion{
		Account: internal.Account{
			AccountID: minionKeypair.Address(),
			Sequence:  1,
		},
		Keypair:              minionKeypair.(*keypair.Full),
		BotAccount:           botAccount,
		BotKeypair:           botKeypair.(*keypair.Full),
		NetworkClient:        nil, // Not used in mocks
		Network:              "Test SDF Network ; September 2015",
		StartingBalance:      "10000.00",
		SubmitTransaction:    mockSubmitTransaction,
		CheckSequenceRefresh: internal.CheckSequenceRefresh,
		CheckAccountExists:   mockCheckAccountExists,
		BaseFee:              txnbuild.MinBaseFee,
	}

	fb := &internal.Bot{Minions: []internal.Minion{minion}}

	cfg := Config{UseCloudflareIP: false}
	router := initRouter(cfg, fb)

	recipientAddress := "GDJIN6W6PLTPKLLM57UW65ZH4BITUXUMYQHIMAZFYXF45PZVAWDBI77Z"

	req := httptest.NewRequest("GET", "/?addr="+url.QueryEscape(recipientAddress), nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// The error mapping is working correctly - returns 400 Bad Request
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Assert the full JSON error response matches expected structure
	body := w.Body.String()
	expectedJSON := `{
          "type": "https://stellar.org/friendbot-errors/bad_request",
          "title": "Bad Request",
          "status": 400,
          "detail": "account already funded to starting balance"
        }`
	assert.JSONEq(t, expectedJSON, body)
}

func TestFriendbotAPI_404NotFound(t *testing.T) {
	router := setup(t)

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

func TestFriendbotAPI_MethodNotAllowed(t *testing.T) {
	router := setup(t)

	// Test PUT method which should not be allowed
	req := httptest.NewRequest("PUT", "/", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Chi router returns 405 Method Not Allowed for undefined methods on existing routes
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	// Assert the response body - should be empty or contain method not allowed message
	body := w.Body.String()
	if body != "" {
		// If there's a response body, it should contain error details
		assert.Contains(t, body, "Method Not Allowed")
	}
}

func TestFriendbotAPI_ValidContractAddress(t *testing.T) {
	// Test that a valid C address (contract) is accepted by the handler
	// This tests address validation only, not the actual contract funding flow
	// since that requires RPC simulation

	// Create a mock that simulates a contract funding attempt
	mockSubmitTransaction := func(ctx context.Context, minion *internal.Minion, networkClient internal.NetworkClient, txHash [32]byte, tx string) (*internal.TransactionResult, error) {
		txSuccess := internal.TransactionResult{
			EnvelopeXdr: tx,
			Successful:  true,
			Hash:        hex.EncodeToString(txHash[:]),
		}
		return &txSuccess, nil
	}

	mockCheckAccountExists := func(minion *internal.Minion, networkClient internal.NetworkClient, destAddress string) (bool, string, error) {
		return false, "0", nil
	}

	botSeed := "SCWNLYELENPBXN46FHYXETT5LJCYBZD5VUQQVW4KZPHFO2YTQJUWT4D5"
	botKeypair, err := keypair.Parse(botSeed)
	require.NoError(t, err)
	botAccount := internal.Account{AccountID: botKeypair.Address()}

	minionSeed := "SDTNSEERJPJFUE2LSDNYBFHYGVTPIWY7TU2IOJZQQGLWO2THTGB7NU5A"
	minionKeypair, err := keypair.Parse(minionSeed)
	require.NoError(t, err)

	// Create a mock network client that returns an error for simulation
	// (this simulates the case where RPC simulation is not available)
	mockNetworkClient := &mockNetworkClientWithSimulation{
		simulateErr: internal.ErrContractFundingRequiresRPC,
	}

	minion := internal.Minion{
		Account: internal.Account{
			AccountID: minionKeypair.Address(),
			Sequence:  1,
		},
		Keypair:              minionKeypair.(*keypair.Full),
		BotAccount:           botAccount,
		BotKeypair:           botKeypair.(*keypair.Full),
		NetworkClient:        mockNetworkClient,
		Network:              "Test SDF Network ; September 2015",
		StartingBalance:      "10000.00",
		SubmitTransaction:    mockSubmitTransaction,
		CheckSequenceRefresh: internal.CheckSequenceRefresh,
		CheckAccountExists:   mockCheckAccountExists,
		BaseFee:              txnbuild.MinBaseFee,
	}

	fb := &internal.Bot{Minions: []internal.Minion{minion}}
	registerProblems()
	cfg := Config{}
	router := initRouter(cfg, fb)

	// Use a valid C address (contract address)
	// This is a sample contract address for testing
	contractAddress := "CAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABSC4"

	req := httptest.NewRequest("GET", "/?addr="+url.QueryEscape(contractAddress), nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// The request should fail because simulation is not supported,
	// but it should NOT fail due to address validation
	// The error should be related to the transaction building, not address validation
	// It returns 500 because it's a server-side issue (lack of RPC support)
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	body := w.Body.String()
	// Should NOT contain "invalid_field" error for "addr" (address validation passed)
	assert.NotContains(t, body, `"invalid_field"`)
	// Should be a server error, not a bad request
	assert.Contains(t, body, "Internal Server Error")
}

func TestFriendbotAPI_InvalidContractAddress(t *testing.T) {
	router := setup(t)

	// Test an invalid address that looks like a C address but isn't valid
	invalidContractAddress := "CINVALIDADDRESS"

	req := httptest.NewRequest("GET", "/?addr="+url.QueryEscape(invalidContractAddress), nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	body := w.Body.String()
	// Should contain address validation error
	assert.Contains(t, body, `"invalid_field"`)
	assert.Contains(t, body, `"addr"`)
}

// mockNetworkClientWithSimulation implements internal.NetworkClient for testing
type mockNetworkClientWithSimulation struct {
	simulateResult *internal.SimulateTransactionResult
	simulateErr    error
}

func (m *mockNetworkClientWithSimulation) SubmitTransaction(txXDR string) error {
	return nil
}

func (m *mockNetworkClientWithSimulation) GetAccountDetails(accountID string) (*internal.AccountDetails, error) {
	return nil, nil
}

func (m *mockNetworkClientWithSimulation) SimulateTransaction(txXDR string) (*internal.SimulateTransactionResult, error) {
	if m.simulateErr != nil {
		return nil, m.simulateErr
	}
	return m.simulateResult, nil
}

func TestValidateAddress(t *testing.T) {
	// Test valid G address (account)
	validGAddress := "GDJIN6W6PLTPKLLM57UW65ZH4BITUXUMYQHIMAZFYXF45PZVAWDBI77Z"
	err := internal.ValidateAddress(validGAddress)
	assert.NoError(t, err)

	// Test valid C address (contract)
	validCAddress := "CAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABSC4"
	err = internal.ValidateAddress(validCAddress)
	assert.NoError(t, err)

	// Test invalid address
	invalidAddress := "invalid_address"
	err = internal.ValidateAddress(invalidAddress)
	assert.Error(t, err)

	// Test empty address
	err = internal.ValidateAddress("")
	assert.Error(t, err)
}

func TestIsContractAddress(t *testing.T) {
	// Test G address (not a contract)
	gAddress := "GDJIN6W6PLTPKLLM57UW65ZH4BITUXUMYQHIMAZFYXF45PZVAWDBI77Z"
	assert.False(t, internal.IsContractAddress(gAddress))

	// Test C address (contract)
	cAddress := "CAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABSC4"
	assert.True(t, internal.IsContractAddress(cAddress))

	// Test invalid address
	assert.False(t, internal.IsContractAddress("invalid"))

	// Test empty address
	assert.False(t, internal.IsContractAddress(""))
}

// TestFriendbotAPI_ContractFunding_SuccessfulWithMockedSimulation tests that contract funding
// succeeds when simulation returns valid data
func TestFriendbotAPI_ContractFunding_SuccessfulWithMockedSimulation(t *testing.T) {
	mockSubmitTransaction := func(ctx context.Context, minion *internal.Minion, networkClient internal.NetworkClient, txHash [32]byte, tx string) (*internal.TransactionResult, error) {
		txSuccess := internal.TransactionResult{
			EnvelopeXdr: tx,
			Successful:  true,
			Hash:        hex.EncodeToString(txHash[:]),
		}
		return &txSuccess, nil
	}

	mockCheckAccountExists := func(minion *internal.Minion, networkClient internal.NetworkClient, destAddress string) (bool, string, error) {
		return false, "0", nil
	}

	botSeed := "SCWNLYELENPBXN46FHYXETT5LJCYBZD5VUQQVW4KZPHFO2YTQJUWT4D5"
	botKeypair, err := keypair.Parse(botSeed)
	require.NoError(t, err)
	botAccount := internal.Account{AccountID: botKeypair.Address()}

	minionSeed := "SDTNSEERJPJFUE2LSDNYBFHYGVTPIWY7TU2IOJZQQGLWO2THTGB7NU5A"
	minionKeypair, err := keypair.Parse(minionSeed)
	require.NoError(t, err)

	// Create a mock network client that returns valid simulation data
	// This is a minimal valid SorobanTransactionData XDR (generated from xdr.SorobanTransactionData)
	mockNetworkClient := &mockNetworkClientWithSimulation{
		simulateResult: &internal.SimulateTransactionResult{
			// Valid SorobanTransactionData XDR with empty footprint and 100000 instructions/resource fee
			TransactionDataXDR: "AAAAAAAAAAAAAAAAAAGGoAAAAAAAAAAAAAAAAAABhqA=",
			MinResourceFee:     100000,
			AuthXDR:            []string{},
		},
	}

	minion := internal.Minion{
		Account: internal.Account{
			AccountID: minionKeypair.Address(),
			Sequence:  1,
		},
		Keypair:              minionKeypair.(*keypair.Full),
		BotAccount:           botAccount,
		BotKeypair:           botKeypair.(*keypair.Full),
		NetworkClient:        mockNetworkClient,
		Network:              "Test SDF Network ; September 2015",
		StartingBalance:      "10000.00",
		SubmitTransaction:    mockSubmitTransaction,
		CheckSequenceRefresh: internal.CheckSequenceRefresh,
		CheckAccountExists:   mockCheckAccountExists,
		BaseFee:              txnbuild.MinBaseFee,
	}

	fb := &internal.Bot{Minions: []internal.Minion{minion}}
	registerProblems()
	cfg := Config{}
	router := initRouter(cfg, fb)

	// Use a valid C address (contract address)
	contractAddress := "CAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABSC4"

	req := httptest.NewRequest("GET", "/?addr="+url.QueryEscape(contractAddress), nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	body := w.Body.String()
	assert.Contains(t, body, `"successful": true`)
	assert.Contains(t, body, `"hash"`)
	assert.Contains(t, body, `"envelope_xdr"`)
}

// TestFriendbotAPI_ContractFunding_POST tests contract funding via POST method
func TestFriendbotAPI_ContractFunding_POST(t *testing.T) {
	mockSubmitTransaction := func(ctx context.Context, minion *internal.Minion, networkClient internal.NetworkClient, txHash [32]byte, tx string) (*internal.TransactionResult, error) {
		txSuccess := internal.TransactionResult{
			EnvelopeXdr: tx,
			Successful:  true,
			Hash:        hex.EncodeToString(txHash[:]),
		}
		return &txSuccess, nil
	}

	mockCheckAccountExists := func(minion *internal.Minion, networkClient internal.NetworkClient, destAddress string) (bool, string, error) {
		return false, "0", nil
	}

	botSeed := "SCWNLYELENPBXN46FHYXETT5LJCYBZD5VUQQVW4KZPHFO2YTQJUWT4D5"
	botKeypair, err := keypair.Parse(botSeed)
	require.NoError(t, err)
	botAccount := internal.Account{AccountID: botKeypair.Address()}

	minionSeed := "SDTNSEERJPJFUE2LSDNYBFHYGVTPIWY7TU2IOJZQQGLWO2THTGB7NU5A"
	minionKeypair, err := keypair.Parse(minionSeed)
	require.NoError(t, err)

	mockNetworkClient := &mockNetworkClientWithSimulation{
		simulateResult: &internal.SimulateTransactionResult{
			TransactionDataXDR: "AAAAAAAAAAAAAAAAAAGGoAAAAAAAAAAAAAAAAAABhqA=",
			MinResourceFee:     100000,
			AuthXDR:            []string{},
		},
	}

	minion := internal.Minion{
		Account: internal.Account{
			AccountID: minionKeypair.Address(),
			Sequence:  1,
		},
		Keypair:              minionKeypair.(*keypair.Full),
		BotAccount:           botAccount,
		BotKeypair:           botKeypair.(*keypair.Full),
		NetworkClient:        mockNetworkClient,
		Network:              "Test SDF Network ; September 2015",
		StartingBalance:      "10000.00",
		SubmitTransaction:    mockSubmitTransaction,
		CheckSequenceRefresh: internal.CheckSequenceRefresh,
		CheckAccountExists:   mockCheckAccountExists,
		BaseFee:              txnbuild.MinBaseFee,
	}

	fb := &internal.Bot{Minions: []internal.Minion{minion}}
	registerProblems()
	cfg := Config{}
	router := initRouter(cfg, fb)

	contractAddress := "CAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABSC4"

	formData := url.Values{}
	formData.Set("addr", contractAddress)

	req := httptest.NewRequest("POST", "/", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	body := w.Body.String()
	assert.Contains(t, body, `"successful": true`)
}

// TestFriendbotAPI_ContractFunding_SimulationError tests that simulation errors are handled properly
func TestFriendbotAPI_ContractFunding_SimulationError(t *testing.T) {
	mockSubmitTransaction := func(ctx context.Context, minion *internal.Minion, networkClient internal.NetworkClient, txHash [32]byte, tx string) (*internal.TransactionResult, error) {
		txSuccess := internal.TransactionResult{
			EnvelopeXdr: tx,
			Successful:  true,
			Hash:        hex.EncodeToString(txHash[:]),
		}
		return &txSuccess, nil
	}

	mockCheckAccountExists := func(minion *internal.Minion, networkClient internal.NetworkClient, destAddress string) (bool, string, error) {
		return false, "0", nil
	}

	botSeed := "SCWNLYELENPBXN46FHYXETT5LJCYBZD5VUQQVW4KZPHFO2YTQJUWT4D5"
	botKeypair, err := keypair.Parse(botSeed)
	require.NoError(t, err)
	botAccount := internal.Account{AccountID: botKeypair.Address()}

	minionSeed := "SDTNSEERJPJFUE2LSDNYBFHYGVTPIWY7TU2IOJZQQGLWO2THTGB7NU5A"
	minionKeypair, err := keypair.Parse(minionSeed)
	require.NoError(t, err)

	// Create a mock network client that returns simulation error
	mockNetworkClient := &mockNetworkClientWithSimulation{
		simulateErr: assert.AnError,
	}

	minion := internal.Minion{
		Account: internal.Account{
			AccountID: minionKeypair.Address(),
			Sequence:  1,
		},
		Keypair:              minionKeypair.(*keypair.Full),
		BotAccount:           botAccount,
		BotKeypair:           botKeypair.(*keypair.Full),
		NetworkClient:        mockNetworkClient,
		Network:              "Test SDF Network ; September 2015",
		StartingBalance:      "10000.00",
		SubmitTransaction:    mockSubmitTransaction,
		CheckSequenceRefresh: internal.CheckSequenceRefresh,
		CheckAccountExists:   mockCheckAccountExists,
		BaseFee:              txnbuild.MinBaseFee,
	}

	fb := &internal.Bot{Minions: []internal.Minion{minion}}
	registerProblems()
	cfg := Config{}
	router := initRouter(cfg, fb)

	contractAddress := "CAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABSC4"

	req := httptest.NewRequest("GET", "/?addr="+url.QueryEscape(contractAddress), nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should return 500 Internal Server Error when simulation fails
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	body := w.Body.String()
	// Should NOT contain "invalid_field" - the error is in simulation, not address validation
	assert.NotContains(t, body, `"invalid_field"`)
}

// TestFriendbotAPI_ContractDoesNotCheckBalance tests that contracts skip balance check
// (unlike accounts, we don't check if a contract is "already funded")
func TestFriendbotAPI_ContractDoesNotCheckBalance(t *testing.T) {
	mockSubmitTransaction := func(ctx context.Context, minion *internal.Minion, networkClient internal.NetworkClient, txHash [32]byte, tx string) (*internal.TransactionResult, error) {
		txSuccess := internal.TransactionResult{
			EnvelopeXdr: tx,
			Successful:  true,
			Hash:        hex.EncodeToString(txHash[:]),
		}
		return &txSuccess, nil
	}

	// This mock should never be called for contract addresses
	mockCheckAccountExistsCalled := false
	mockCheckAccountExists := func(minion *internal.Minion, networkClient internal.NetworkClient, destAddress string) (bool, string, error) {
		mockCheckAccountExistsCalled = true
		return true, "10000.00", nil // Would normally block funding for accounts
	}

	botSeed := "SCWNLYELENPBXN46FHYXETT5LJCYBZD5VUQQVW4KZPHFO2YTQJUWT4D5"
	botKeypair, err := keypair.Parse(botSeed)
	require.NoError(t, err)
	botAccount := internal.Account{AccountID: botKeypair.Address()}

	minionSeed := "SDTNSEERJPJFUE2LSDNYBFHYGVTPIWY7TU2IOJZQQGLWO2THTGB7NU5A"
	minionKeypair, err := keypair.Parse(minionSeed)
	require.NoError(t, err)

	mockNetworkClient := &mockNetworkClientWithSimulation{
		simulateResult: &internal.SimulateTransactionResult{
			TransactionDataXDR: "AAAAAAAAAAAAAAAAAAGGoAAAAAAAAAAAAAAAAAABhqA=",
			MinResourceFee:     100000,
			AuthXDR:            []string{},
		},
	}

	minion := internal.Minion{
		Account: internal.Account{
			AccountID: minionKeypair.Address(),
			Sequence:  1,
		},
		Keypair:              minionKeypair.(*keypair.Full),
		BotAccount:           botAccount,
		BotKeypair:           botKeypair.(*keypair.Full),
		NetworkClient:        mockNetworkClient,
		Network:              "Test SDF Network ; September 2015",
		StartingBalance:      "10000.00",
		SubmitTransaction:    mockSubmitTransaction,
		CheckSequenceRefresh: internal.CheckSequenceRefresh,
		CheckAccountExists:   mockCheckAccountExists,
		BaseFee:              txnbuild.MinBaseFee,
	}

	fb := &internal.Bot{Minions: []internal.Minion{minion}}
	registerProblems()
	cfg := Config{}
	router := initRouter(cfg, fb)

	contractAddress := "CAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABSC4"

	req := httptest.NewRequest("GET", "/?addr="+url.QueryEscape(contractAddress), nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should succeed because contracts don't have balance checks
	assert.Equal(t, http.StatusOK, w.Code)

	// CheckAccountExists should NOT have been called for contract addresses
	assert.False(t, mockCheckAccountExistsCalled, "CheckAccountExists should not be called for contract addresses")
}

// TestFriendbotAPI_GAddressStillWorksWithSimulationClient tests that G addresses
// continue to work normally even when using a network client with simulation support
func TestFriendbotAPI_GAddressStillWorksWithSimulationClient(t *testing.T) {
	mockSubmitTransaction := func(ctx context.Context, minion *internal.Minion, networkClient internal.NetworkClient, txHash [32]byte, tx string) (*internal.TransactionResult, error) {
		txSuccess := internal.TransactionResult{
			EnvelopeXdr: tx,
			Successful:  true,
			Hash:        hex.EncodeToString(txHash[:]),
		}
		return &txSuccess, nil
	}

	mockCheckAccountExists := func(minion *internal.Minion, networkClient internal.NetworkClient, destAddress string) (bool, string, error) {
		return false, "0", nil
	}

	botSeed := "SCWNLYELENPBXN46FHYXETT5LJCYBZD5VUQQVW4KZPHFO2YTQJUWT4D5"
	botKeypair, err := keypair.Parse(botSeed)
	require.NoError(t, err)
	botAccount := internal.Account{AccountID: botKeypair.Address()}

	minionSeed := "SDTNSEERJPJFUE2LSDNYBFHYGVTPIWY7TU2IOJZQQGLWO2THTGB7NU5A"
	minionKeypair, err := keypair.Parse(minionSeed)
	require.NoError(t, err)

	// SimulateTransaction should never be called for G addresses
	simulateCalled := false
	mockNetworkClient := &mockNetworkClientWithSimulation{
		simulateResult: &internal.SimulateTransactionResult{
			TransactionDataXDR: "AAAAAAAAAAAAAAAAAAGGoAAAAAAAAAAAAAAAAAABhqA=",
			MinResourceFee:     100000,
			AuthXDR:            []string{},
		},
	}
	// Wrap to track if simulate is called
	originalSimulate := mockNetworkClient.SimulateTransaction
	mockNetworkClient.simulateResult = nil
	mockNetworkClient.simulateErr = nil

	minion := internal.Minion{
		Account: internal.Account{
			AccountID: minionKeypair.Address(),
			Sequence:  1,
		},
		Keypair:              minionKeypair.(*keypair.Full),
		BotAccount:           botAccount,
		BotKeypair:           botKeypair.(*keypair.Full),
		NetworkClient:        &trackingNetworkClient{mockNetworkClient, &simulateCalled},
		Network:              "Test SDF Network ; September 2015",
		StartingBalance:      "10000.00",
		SubmitTransaction:    mockSubmitTransaction,
		CheckSequenceRefresh: internal.CheckSequenceRefresh,
		CheckAccountExists:   mockCheckAccountExists,
		BaseFee:              txnbuild.MinBaseFee,
	}
	_ = originalSimulate // suppress unused warning

	fb := &internal.Bot{Minions: []internal.Minion{minion}}
	registerProblems()
	cfg := Config{}
	router := initRouter(cfg, fb)

	// Use a G address (account)
	accountAddress := "GDJIN6W6PLTPKLLM57UW65ZH4BITUXUMYQHIMAZFYXF45PZVAWDBI77Z"

	req := httptest.NewRequest("GET", "/?addr="+url.QueryEscape(accountAddress), nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	body := w.Body.String()
	assert.Contains(t, body, `"successful": true`)

	// SimulateTransaction should NOT have been called for G addresses
	assert.False(t, simulateCalled, "SimulateTransaction should not be called for G addresses")
}

// trackingNetworkClient wraps a network client to track if SimulateTransaction is called
type trackingNetworkClient struct {
	*mockNetworkClientWithSimulation
	simulateCalled *bool
}

func (t *trackingNetworkClient) SimulateTransaction(txXDR string) (*internal.SimulateTransactionResult, error) {
	*t.simulateCalled = true
	return t.mockNetworkClientWithSimulation.SimulateTransaction(txXDR)
}
