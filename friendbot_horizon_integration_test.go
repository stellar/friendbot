package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/stellar/friendbot/internal"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var horizonURL = os.Getenv("HORIZON_URL")

// getNetworkPassphrase fetches the network passphrase from the horizon root endpoint
func getNetworkPassphrase(t *testing.T, horizonURL string) string {
	t.Helper()

	resp, err := http.Get(horizonURL)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var root struct {
		NetworkPassphrase string `json:"network_passphrase"`
	}
	err = json.Unmarshal(body, &root)
	require.NoError(t, err)

	return root.NetworkPassphrase
}

// fundAccount uses the friendbot endpoint to fund an account
func fundAccount(t *testing.T, horizonURL, address string) error {
	t.Helper()

	friendbotURL := fmt.Sprintf("%s/friendbot?addr=%s", horizonURL, address)

	resp, err := http.Get(friendbotURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("friendbot returned status %d", resp.StatusCode)
	}

	var result struct {
		Successful bool `json:"successful"`
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	err = json.Unmarshal(body, &result)
	if err != nil {
		return err
	}
	if !result.Successful {
		return fmt.Errorf("account funding failed")
	}
	return nil
}

// Setup creates a test friendbot with real horizon client.
func setupHorizonIntegration(t *testing.T) http.Handler {
	t.Helper()

	if horizonURL == "" {
		t.Skip("HORIZON_URL environment variable not set, skipping horizon integration tests")
	}

	// Get network passphrase from horizon
	networkPassphrase := getNetworkPassphrase(t, horizonURL)
	startingBalance := "1000.00" // Use smaller amount so bot account keeps reserve
	baseFee := int64(txnbuild.MinBaseFee)

	// Generate random keypair for the bot account that holds the funds
	botKeypair, err := keypair.Random()
	require.NoError(t, err)
	botAccount := internal.Account{AccountID: botKeypair.Address()}
	err = fundAccount(t, horizonURL, botKeypair.Address())
	require.NoError(t, err)

	// Generate random keypair for a minion that will be used for sequence numbers when funding
	minionKeypair, err := keypair.Random()
	require.NoError(t, err)
	err = fundAccount(t, horizonURL, minionKeypair.Address())
	require.NoError(t, err)

	// Create horizon client
	hclient := &horizonclient.Client{
		HorizonURL: horizonURL,
		HTTP:       http.DefaultClient,
		AppName:    "friendbot-integration-test",
	}

	// Create minion that will fund accounts
	minion := internal.Minion{
		Account: internal.Account{
			AccountID: minionKeypair.Address(),
		},
		Keypair:              minionKeypair,
		BotAccount:           botAccount,
		BotKeypair:           botKeypair,
		Horizon:              hclient,
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
	return router
}

func TestFriendbotHorizonIntegration_SuccessfulFunding_GET(t *testing.T) {
	router := setupHorizonIntegration(t)

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
		Hash       string `json:"hash"`
		Successful bool   `json:"successful"`
	}
	err = json.Unmarshal([]byte(body), &result)
	require.NoError(t, err)
	assert.Equal(t, true, result.Successful)
	assert.NotEmpty(t, result.Hash)
}

func TestFriendbotHorizonIntegration_SuccessfulFunding_POST(t *testing.T) {
	router := setupHorizonIntegration(t)

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
		Hash       string `json:"hash"`
		Successful bool   `json:"successful"`
	}
	err = json.Unmarshal([]byte(body), &result)
	require.NoError(t, err)
	assert.Equal(t, true, result.Successful)
	assert.NotEmpty(t, result.Hash)
}

func TestFriendbotHorizonIntegration_MissingAddressParameter(t *testing.T) {
	router := setupHorizonIntegration(t)

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

func TestFriendbotHorizonIntegration_InvalidAddress(t *testing.T) {
	router := setupHorizonIntegration(t)

	invalidAddress := "invalid_address"

	req := httptest.NewRequest("GET", "/?addr="+invalidAddress, nil)
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

func TestFriendbotHorizonIntegration_AccountAlreadyFunded(t *testing.T) {
	router := setupHorizonIntegration(t)

	// Generate random recipient address
	recipientKeypair, err := keypair.Random()
	require.NoError(t, err)
	recipientAddress := recipientKeypair.Address()

	// First funding attempt
	req := httptest.NewRequest("GET", "/?addr="+recipientAddress, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Second funding attempt - should fail (either with account already funded or transaction error)
	req2 := httptest.NewRequest("GET", "/?addr="+recipientAddress, nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusBadRequest, w2.Code)

	// Assert the full JSON error response matches expected structure
	body := w2.Body.String()
	expectedJSON := `{
          "type": "https://stellar.org/horizon-errors/bad_request",
          "title": "Bad Request",
          "status": 400,
          "detail": "account already funded to starting balance"
        }`
	assert.JSONEq(t, expectedJSON, body)
}

func TestFriendbotHorizonIntegration_404NotFound(t *testing.T) {
	router := setupHorizonIntegration(t)

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
