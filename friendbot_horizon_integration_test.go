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

// This horizon instance must be configured with a working friendbot
// endpoint as these tests depend on the horizon friendbot to setup
// accounts for the friendbot in these tests to use.
var horizonURL = os.Getenv("HORIZON_URL")

// getNetworkPassphrase fetches the network passphrase from the horizon root endpoint
func getNetworkPassphrase(t *testing.T, horizonURL string) string {
	t.Helper()

	// #nosec G107 - the url is from a trusted source configured in CI or local
	//nolint:noctx
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

	// #nosec G107 - the url is from a trusted source configured in CI or local
	//nolint:noctx
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

// Setup creates running instance of friendbot from current code and requires an external instance of horizon that has been configured with its own separate instance of friendbot to support funding accounts. These tests utilize that to fund new minion and bot accounts on target network used by this local friendbot instance being tested.
func setupHorizonIntegration(t *testing.T) (http.Handler, horizonclient.ClientInterface) {
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
	return router, hclient
}

func TestFriendbotHorizonIntegration_SuccessfulFunding_GET(t *testing.T) {
	router, hclient := setupHorizonIntegration(t)

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

	// Check that the recipient account has the expected balance
	accountRequest := horizonclient.AccountRequest{AccountID: recipientAddress}
	accountDetails, err := hclient.AccountDetail(accountRequest)
	require.NoError(t, err)

	var balance string
	for _, b := range accountDetails.Balances {
		if b.Type == "native" {
			balance = b.Balance
			break
		}
	}
	assert.Equal(t, "1000.0000000", balance)
}

func TestFriendbotHorizonIntegration_SuccessfulFunding_POST(t *testing.T) {
	router, hclient := setupHorizonIntegration(t)

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

	// Check that the recipient account has the expected balance
	accountRequest := horizonclient.AccountRequest{AccountID: recipientAddress}
	accountDetails, err := hclient.AccountDetail(accountRequest)
	require.NoError(t, err)

	var balance string
	for _, b := range accountDetails.Balances {
		if b.Type == "native" {
			balance = b.Balance
			break
		}
	}
	assert.Equal(t, "1000.0000000", balance)
}

func TestFriendbotHorizonIntegration_MissingAddressParameter(t *testing.T) {
	router, _ := setupHorizonIntegration(t)

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
	router, _ := setupHorizonIntegration(t)

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
	router, hclient := setupHorizonIntegration(t)

	// Generate random recipient address
	recipientKeypair, err := keypair.Random()
	require.NoError(t, err)
	recipientAddress := recipientKeypair.Address()

	// First funding attempt
	req := httptest.NewRequest("GET", "/?addr="+recipientAddress, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Check that the recipient account has the expected balance after first funding
	accountRequest := horizonclient.AccountRequest{AccountID: recipientAddress}
	accountDetails, err := hclient.AccountDetail(accountRequest)
	require.NoError(t, err)

	var balance string
	for _, b := range accountDetails.Balances {
		if b.Type == "native" {
			balance = b.Balance
			break
		}
	}
	assert.Equal(t, "1000.0000000", balance)

	// Second funding attempt - should fail (either with account already funded or transaction error)
	req2 := httptest.NewRequest("GET", "/?addr="+recipientAddress, nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusBadRequest, w2.Code)

	// Check that the balance hasn't changed after the failed funding attempt
	accountDetails2, err := hclient.AccountDetail(accountRequest)
	require.NoError(t, err)

	var balance2 string
	for _, b := range accountDetails2.Balances {
		if b.Type == "native" {
			balance2 = b.Balance
			break
		}
	}
	assert.Equal(t, balance, balance2)

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
	router, _ := setupHorizonIntegration(t)

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
