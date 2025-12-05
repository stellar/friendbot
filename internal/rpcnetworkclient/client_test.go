package rpcnetworkclient

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/xdr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewNetworkClient(t *testing.T) {
	client := NewNetworkClient("http://localhost:8080", nil)
	assert.NotNil(t, client)
	assert.NotNil(t, client.client)
}

func TestNetworkError_IsNotFound(t *testing.T) {
	// Test not found error
	networkErr := &NetworkError{err: errors.New("account not found"), notFound: true}
	assert.True(t, networkErr.IsNotFound())

	// Test regular error
	networkErr2 := &NetworkError{err: errors.New("some other error")}
	assert.False(t, networkErr2.IsNotFound())
}

func TestNetworkError_IsBadSequence(t *testing.T) {
	// Test bad sequence error with XDR result
	// This is a base64-encoded TransactionResult with txBAD_SEQ code
	badSeqResultXDR := "AAAAAAAAAGT////7AAAAAA=="
	networkErr := &NetworkError{err: errors.New("transaction failed"), resultXDR: badSeqResultXDR}
	assert.True(t, networkErr.IsBadSequence())

	// Test non-bad-sequence error with XDR result (txFAILED)
	failedResultXDR := "AAAAAAAAAGT////9AAAAAA=="
	networkErr2 := &NetworkError{err: errors.New("transaction failed"), resultXDR: failedResultXDR}
	assert.False(t, networkErr2.IsBadSequence())

	// Test error without resultXDR
	networkErr3 := &NetworkError{err: errors.New("some other error")}
	assert.False(t, networkErr3.IsBadSequence())

	// Test error with invalid XDR
	networkErr4 := &NetworkError{err: errors.New("transaction failed"), resultXDR: "invalid-xdr"}
	assert.False(t, networkErr4.IsBadSequence())
}

func TestNetworkError_IsTimeout(t *testing.T) {
	// Test timeout error
	networkErr := &NetworkError{err: errors.New("timeout occurred"), timeout: true}
	assert.True(t, networkErr.IsTimeout())

	// Test regular error
	networkErr2 := &NetworkError{err: errors.New("some other error")}
	assert.False(t, networkErr2.IsTimeout())
}

func TestNetworkError_ResultString(t *testing.T) {
	// Test error without resultXDR
	networkErr := &NetworkError{err: errors.New("test error")}
	result, err := networkErr.ResultString()
	assert.Empty(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no result XDR available")

	// Test error with resultXDR
	networkErr2 := &NetworkError{err: errors.New("test error"), resultXDR: "AAAAAAAAAGT////7AAAAAA=="}
	result2, err2 := networkErr2.ResultString()
	assert.Equal(t, "AAAAAAAAAGT////7AAAAAA==", result2)
	assert.NoError(t, err2)

	// Test error with invalid resultXDR (ResultString returns raw string, doesn't validate)
	networkErr3 := &NetworkError{err: errors.New("test error"), resultXDR: "invalid-xdr"}
	result3, err3 := networkErr3.ResultString()
	assert.Equal(t, "invalid-xdr", result3)
	assert.NoError(t, err3)
}

func TestNetworkError_Error(t *testing.T) {
	originalErr := errors.New("original error")
	networkErr := &NetworkError{err: originalErr}

	result := networkErr.Error()
	assert.Equal(t, "original error", result)
}

func TestNetworkError_Unwrap(t *testing.T) {
	originalErr := errors.New("original error")
	networkErr := &NetworkError{err: originalErr}

	assert.Equal(t, originalErr, networkErr.Unwrap())
}

func TestNetworkClient_GetAccountDetails_Success(t *testing.T) {
	testAccountKP := keypair.MustRandom()
	testAccountID := testAccountKP.Address()
	testSequence := int64(12345)
	testBalance := int64(100_0000000) // 100 XLM in stroops

	server := newMockRPCServer(t, func(method string, params json.RawMessage) (any, error) {
		require.Equal(t, "getLedgerEntries", method)

		return map[string]any{
			"entries": []map[string]any{
				{
					"xdr":                   createTestAccountEntryXDR(t, testAccountID, testSequence, testBalance),
					"lastModifiedLedgerSeq": 1000,
				},
			},
			"latestLedger": 1001,
		}, nil
	})
	defer server.Close()

	client := NewNetworkClient(server.URL, nil)
	details, err := client.GetAccountDetails(testAccountID)

	require.NoError(t, err)
	assert.Equal(t, testSequence, details.Sequence)
	assert.Equal(t, "100.0000000", details.Balance)
}

func TestNetworkClient_GetAccountDetails_NotFound(t *testing.T) {
	testAccountKP := keypair.MustRandom()
	testAccountID := testAccountKP.Address()

	server := newMockRPCServer(t, func(method string, params json.RawMessage) (any, error) {
		require.Equal(t, "getLedgerEntries", method)

		return map[string]any{
			"entries":      []map[string]any{},
			"latestLedger": 1001,
		}, nil
	})
	defer server.Close()

	client := NewNetworkClient(server.URL, nil)
	details, err := client.GetAccountDetails(testAccountID)

	assert.Nil(t, details)
	require.Error(t, err)

	var networkErr *NetworkError
	require.ErrorAs(t, err, &networkErr)
	assert.True(t, networkErr.IsNotFound())
}

func TestNetworkClient_GetAccountDetails_InvalidAccountID(t *testing.T) {
	server := newMockRPCServer(t, func(method string, params json.RawMessage) (any, error) {
		t.Fatal("server should not be called for invalid account ID")
		return nil, nil
	})
	defer server.Close()

	client := NewNetworkClient(server.URL, nil)
	details, err := client.GetAccountDetails("invalid-account-id")

	assert.Nil(t, details)
	require.Error(t, err)
}

func TestNetworkClient_SubmitTransaction_Success(t *testing.T) {
	testTxXDR := "AAAAAgAAAA..."
	testTxHash := "abc123def456789012345678901234567890123456789012345678901234abcd"

	server := newMockRPCServer(t, func(method string, params json.RawMessage) (any, error) {
		switch method {
		case "sendTransaction":
			return map[string]any{
				"status":       "PENDING",
				"hash":         testTxHash,
				"latestLedger": 1000,
			}, nil
		case "getTransaction":
			return map[string]any{
				"status":       "SUCCESS",
				"latestLedger": 1001,
			}, nil
		default:
			t.Fatalf("unexpected method: %s", method)
			return nil, nil
		}
	})
	defer server.Close()

	client := NewNetworkClient(server.URL, nil)
	err := client.SubmitTransaction(testTxXDR)

	assert.NoError(t, err)
}

func TestNetworkClient_SubmitTransaction_Rejected(t *testing.T) {
	testTxXDR := "AAAAAgAAAA..."
	// This is a base64-encoded TransactionResult with an error
	testErrorResultXDR := "AAAAAAAAAGT////9AAAAAA=="

	server := newMockRPCServer(t, func(method string, params json.RawMessage) (any, error) {
		require.Equal(t, "sendTransaction", method)

		return map[string]any{
			"status":         "ERROR",
			"errorResultXdr": testErrorResultXDR,
			"latestLedger":   1000,
		}, nil
	})
	defer server.Close()

	client := NewNetworkClient(server.URL, nil)
	err := client.SubmitTransaction(testTxXDR)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "transaction rejected")

	var networkErr *NetworkError
	require.ErrorAs(t, err, &networkErr)
	resultXDR, resultErr := networkErr.ResultString()
	assert.NoError(t, resultErr)
	assert.Equal(t, testErrorResultXDR, resultXDR)
}

func TestNetworkClient_SubmitTransaction_Failed(t *testing.T) {
	testTxXDR := "AAAAAgAAAA..."
	testTxHash := "abc123def456789012345678901234567890123456789012345678901234abcd"
	// This is a base64-encoded TransactionResult with txFAILED code
	testFailedResultXDR := "AAAAAAAAAGT////9AAAAAA=="

	server := newMockRPCServer(t, func(method string, params json.RawMessage) (any, error) {
		switch method {
		case "sendTransaction":
			return map[string]any{
				"status":       "PENDING",
				"hash":         testTxHash,
				"latestLedger": 1000,
			}, nil
		case "getTransaction":
			return map[string]any{
				"status":       "FAILED",
				"resultXdr":    testFailedResultXDR,
				"latestLedger": 1001,
			}, nil
		default:
			t.Fatalf("unexpected method: %s", method)
			return nil, nil
		}
	})
	defer server.Close()

	client := NewNetworkClient(server.URL, nil)
	err := client.SubmitTransaction(testTxXDR)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "transaction failed")

	var networkErr *NetworkError
	require.ErrorAs(t, err, &networkErr)
	resultXDR, resultErr := networkErr.ResultString()
	assert.NoError(t, resultErr)
	assert.Equal(t, testFailedResultXDR, resultXDR)
}

// newMockRPCServer creates a test HTTP server that handles JSON-RPC 2.0 requests.
// The handler function receives the method name and params and returns the result or error.
func newMockRPCServer(t *testing.T, handler func(method string, params json.RawMessage) (any, error)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			JSONRPC string          `json:"jsonrpc"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
			ID      any             `json:"id"`
		}
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		result, err := handler(req.Method, req.Params)

		resp := map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
		}
		if err != nil {
			resp["error"] = map[string]any{"code": -1, "message": err.Error()}
		} else {
			resp["result"] = result
		}

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
}

// createTestAccountEntryXDR creates a base64-encoded LedgerEntryData for an account.
func createTestAccountEntryXDR(t *testing.T, accountID string, sequence int64, balance int64) string {
	accountIDObj, err := xdr.AddressToAccountId(accountID)
	require.NoError(t, err)

	entry := xdr.LedgerEntryData{
		Type: xdr.LedgerEntryTypeAccount,
		Account: &xdr.AccountEntry{
			AccountId: accountIDObj,
			Balance:   xdr.Int64(balance),
			SeqNum:    xdr.SequenceNumber(sequence),
		},
	}

	xdrBytes, err := xdr.MarshalBase64(entry)
	require.NoError(t, err)
	return xdrBytes
}
