package rpcnetworkclient

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/stellar/friendbot/internal"
	rpcclient "github.com/stellar/go/clients/rpcclient"
	"github.com/stellar/go/protocols/rpc"
	"github.com/stellar/go/xdr"
)

// NetworkError wraps an RPC error and implements the internal.NetworkError interface.
type NetworkError struct {
	err error
}

// NewNetworkError creates a new NetworkError from an RPC error.
func NewNetworkError(err error) *NetworkError {
	return &NetworkError{err: err}
}

// IsNotFound returns true if the error indicates the requested resource was not found.
func (e *NetworkError) IsNotFound() bool {
	// Check if the error message contains "not found" or similar
	errMsg := e.err.Error()
	return strings.Contains(strings.ToLower(errMsg), "not found") ||
		strings.Contains(strings.ToLower(errMsg), "account not found")
}

// IsBadSequence returns true if the error indicates a bad sequence number.
func (e *NetworkError) IsBadSequence() bool {
	// Check if the error message contains bad sequence indicators
	errMsg := e.err.Error()
	return strings.Contains(strings.ToLower(errMsg), "bad sequence") ||
		strings.Contains(strings.ToLower(errMsg), "bad_seq")
}

// IsTimeout returns true if the error indicates a timeout occurred.
func (e *NetworkError) IsTimeout() bool {
	// Check if the error message contains timeout indicators
	errMsg := e.err.Error()
	return strings.Contains(strings.ToLower(errMsg), "timeout") ||
		strings.Contains(strings.ToLower(errMsg), "deadline exceeded")
}

// ResultString returns the result string from the error, if available.
func (e *NetworkError) ResultString() (string, error) {
	// RPC errors don't have a direct result string like Horizon
	return "", fmt.Errorf("RPC client does not provide result strings: %w", e.err)
}

// Error implements the error interface.
func (e *NetworkError) Error() string {
	return e.err.Error()
}

// Unwrap returns the underlying error.
func (e *NetworkError) Unwrap() error {
	return e.err
}

// NetworkClient wraps an RPC client and implements the internal.NetworkClient interface.
type NetworkClient struct {
	client *rpcclient.Client
}

// NewNetworkClient creates a new NetworkClient wrapping the given RPC client.
func NewNetworkClient(url string, httpClient *http.Client) *NetworkClient {
	client := rpcclient.NewClient(url, httpClient)
	return &NetworkClient{client: client}
}

// SubmitTransaction submits a transaction using the underlying RPC client.
func (r *NetworkClient) SubmitTransaction(txXDR string) (*internal.TransactionResult, error) {
	request := protocol.SendTransactionRequest{
		Transaction: txXDR,
	}

	response, err := r.client.SendTransaction(context.Background(), request)
	if err != nil {
		return nil, NewNetworkError(err)
	}

	// Check if the transaction was successful
	successful := response.Status == "PENDING"

	return &internal.TransactionResult{
		Successful: successful,
	}, nil
}

// GetAccountDetails retrieves account details using the underlying RPC client.
func (r *NetworkClient) GetAccountDetails(accountID string) (*internal.AccountDetails, error) {
	// We need to get the raw account entry to access balance information
	// Since LoadAccount doesn't expose the balance, we'll implement it manually
	// similar to how LoadAccount works internally

	accountIDObj, err := xdr.AddressToAccountId(accountID)
	if err != nil {
		return nil, NewNetworkError(err)
	}

	ledgerKey, err := accountIDObj.LedgerKey()
	if err != nil {
		return nil, NewNetworkError(err)
	}

	accountKey, err := xdr.MarshalBase64(ledgerKey)
	if err != nil {
		return nil, NewNetworkError(err)
	}

	resp, err := r.client.GetLedgerEntries(context.Background(), protocol.GetLedgerEntriesRequest{
		Keys: []string{accountKey},
	})
	if err != nil {
		return nil, NewNetworkError(err)
	}

	if len(resp.Entries) != 1 {
		return nil, NewNetworkError(fmt.Errorf("failed to find ledger entry for account %s", accountID))
	}

	var entry xdr.LedgerEntryData
	if err := xdr.SafeUnmarshalBase64(resp.Entries[0].DataXDR, &entry); err != nil {
		return nil, NewNetworkError(err)
	}

	// Convert balance from stroops (int64) to string
	balance := fmt.Sprintf("%d", entry.Account.Balance)

	return &internal.AccountDetails{
		Sequence: int64(entry.Account.SeqNum),
		Balance:  balance,
	}, nil
}