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
			Successful:  true,
			Hash:        hex.EncodeToString(txHash[:]),
			EnvelopeXdr: tx,
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

	// Assert the JSON response matches expected structure
	body := w.Body.String()
	expectedJSON := `{
          "successful": true,
          "hash": "test_hash",
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

	// Assert the JSON response matches expected structure
	body := w.Body.String()
	expectedJSON := `{
          "successful": true,
          "hash": "test_hash",
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
          "type": "https://stellar.org/horizon-errors/bad_request",
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
          "type": "https://stellar.org/horizon-errors/bad_request",
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
          "type": "https://stellar.org/horizon-errors/bad_request",
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
          "type": "https://stellar.org/horizon-errors/not_found",
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
