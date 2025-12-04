package rpcnetworkclient

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/stellar/friendbot/internal"
	"github.com/stellar/go/amount"
	rpcclient "github.com/stellar/go/clients/rpcclient"
	protocol "github.com/stellar/go/protocols/rpc"
	"github.com/stellar/go/xdr"
)

const (
	submitTransactionTimeout = 30 * time.Second
	backoffInitialInterval   = 500 * time.Millisecond
	backoffMaxInterval       = 3500 * time.Millisecond
	statusError              = "ERROR"
)

// NetworkError wraps an RPC error and implements the internal.NetworkError interface.
type NetworkError struct {
	err                 error
	notFound            bool
	timeout             bool
	resultXDR           string
	diagnosticEventsXDR []string
}

// IsNotFound returns true if the error indicates the requested resource was not found.
func (e *NetworkError) IsNotFound() bool {
	return e.notFound
}

// IsBadSequence returns true if the error indicates a bad sequence number.
func (e *NetworkError) IsBadSequence() bool {
	result, err := e.ResultXDR()
	if err != nil {
		return false
	}
	return result.Result.Code == xdr.TransactionResultCodeTxBadSeq
}

// ResultXDR parses and returns the transaction result XDR.
func (e *NetworkError) ResultXDR() (xdr.TransactionResult, error) {
	var result xdr.TransactionResult
	resultXDR, err := e.ResultString()
	if err != nil {
		return result, err
	}
	if err := xdr.SafeUnmarshalBase64(resultXDR, &result); err != nil {
		return result, err
	}
	return result, nil
}

// IsTimeout returns true if the error indicates a timeout occurred.
func (e *NetworkError) IsTimeout() bool {
	return e.timeout
}

// ResultString returns the result string from the error, if available.
func (e *NetworkError) ResultString() (string, error) {
	if e.resultXDR == "" {
		return "", fmt.Errorf("no result XDR available")
	}
	return e.resultXDR, nil
}

// DiagnosticEventStrings returns the diagnostic event XDR strings, if available.
func (e *NetworkError) DiagnosticEventStrings() []string {
	return e.diagnosticEventsXDR
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
// It blocks until the transaction is finalized (SUCCESS or FAILED) or times out after 30 seconds.
func (r *NetworkClient) SubmitTransaction(txXDR string) error {
	// TODO: Pass context down from the request upstream. See https://github.com/stellar/friendbot/issues/22
	ctx, cancel := context.WithTimeout(context.Background(), submitTransactionTimeout)
	defer cancel()

	request := protocol.SendTransactionRequest{
		Transaction: txXDR,
	}

	response, err := r.client.SendTransaction(ctx, request)
	if err != nil {
		return &NetworkError{err: err}
	}

	// If the transaction was rejected immediately, return the error
	if response.Status == statusError {
		return &NetworkError{
			err:                 fmt.Errorf("transaction rejected"),
			resultXDR:           response.ErrorResultXDR,
			diagnosticEventsXDR: response.DiagnosticEventsXDR,
		}
	}

	return r.pollTransactionStatus(ctx, response.Hash)
}

// pollTransactionStatus polls GetTransaction until the transaction is finalized
// (SUCCESS or FAILED) or the context times out.
func (r *NetworkClient) pollTransactionStatus(ctx context.Context, txHash string) error {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = backoffInitialInterval
	b.MaxInterval = backoffMaxInterval
	// Note: MaxElapsedTime is not set here because the context timeout
	// (submitTransactionTimeout) already handles the overall timeout.

	var finalErr error
	err := backoff.Retry(func() error {
		txResponse, err := r.client.GetTransaction(ctx, protocol.GetTransactionRequest{
			Hash: txHash,
		})
		if err != nil {
			finalErr = &NetworkError{err: err}
			return backoff.Permanent(finalErr)
		}

		switch txResponse.Status {
		case protocol.TransactionStatusSuccess:
			return nil
		case protocol.TransactionStatusFailed:
			finalErr = &NetworkError{
				err:                 fmt.Errorf("transaction failed"),
				resultXDR:           txResponse.ResultXDR,
				diagnosticEventsXDR: txResponse.DiagnosticEventsXDR,
			}
			return backoff.Permanent(finalErr)
		default:
			// Transaction not yet processed (including NOT_FOUND status).
			// After sendTransaction returns a hash, the transaction should
			// eventually appear, so we retry until context timeout.
			return fmt.Errorf("transaction not yet finalized")
		}
	}, backoff.WithContext(b, ctx))

	if err != nil {
		if finalErr != nil {
			return finalErr
		}
		return &NetworkError{err: fmt.Errorf("timeout waiting for transaction %s to finalize", txHash), timeout: true}
	}
	return nil
}

// GetAccountDetails retrieves account details using the underlying RPC client.
func (r *NetworkClient) GetAccountDetails(accountID string) (*internal.AccountDetails, error) {
	accountIDObj, err := xdr.AddressToAccountId(accountID)
	if err != nil {
		return nil, &NetworkError{err: err}
	}

	ledgerKey, err := accountIDObj.LedgerKey()
	if err != nil {
		return nil, &NetworkError{err: err}
	}

	ledgerKeyXDR, err := xdr.MarshalBase64(ledgerKey)
	if err != nil {
		return nil, &NetworkError{err: err}
	}

	resp, err := r.client.GetLedgerEntries(context.Background(), protocol.GetLedgerEntriesRequest{
		Keys: []string{ledgerKeyXDR},
	})
	if err != nil {
		return nil, &NetworkError{err: err}
	}

	if len(resp.Entries) != 1 {
		return nil, &NetworkError{
			err:      fmt.Errorf("failed to find ledger entry for account %s", accountID),
			notFound: true,
		}
	}

	var entry xdr.LedgerEntryData
	if err := xdr.SafeUnmarshalBase64(resp.Entries[0].DataXDR, &entry); err != nil {
		return nil, &NetworkError{err: err}
	}
	if entry.Type != xdr.LedgerEntryTypeAccount {
		return nil, &NetworkError{err: fmt.Errorf("unexpected ledger entry type: expected account, got %v", entry.Type)}
	}

	// Convert balance from stroops (int64) to XLM string format
	balance := amount.StringFromInt64(int64(entry.Account.Balance))

	return &internal.AccountDetails{
		Sequence: int64(entry.Account.SeqNum),
		Balance:  balance,
	}, nil
}
