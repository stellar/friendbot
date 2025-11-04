package internal

import (
	"github.com/stellar/go/clients/horizonclient"
)

// NetworkError represents a network operation error with abstracted checking methods.
type NetworkError interface {
	error
	// IsNotFound returns true if the error indicates the requested resource was not found.
	IsNotFound() bool
	// IsBadSequence returns true if the error indicates a bad sequence number.
	IsBadSequence() bool
	// IsTimeout returns true if the error indicates a timeout occurred.
	IsTimeout() bool
	// ResultString returns the result string from the error, if available.
	ResultString() (string, error)
}

// NetworkClient defines a general interface for interacting with Stellar network services.
// It abstracts the functionality needed for friendbot operations, allowing different
// implementations (Horizon, RPC, etc.) to be used interchangeably.
type NetworkClient interface {
	// SubmitTransaction submits a transaction in XDR format and returns the result.
	SubmitTransaction(txXDR string) (*TransactionResult, error)

	// GetAccountDetails retrieves account information for the given account ID.
	GetAccountDetails(accountID string) (*AccountDetails, error)
}

// TransactionResult contains the minimal information needed about a transaction result.
type TransactionResult struct {
	Successful bool
}

// AccountDetails contains the minimal information needed about an account.
type AccountDetails struct {
	Sequence int64
	Balance  string
}

// HorizonNetworkError wraps a horizonclient.Error and implements the NetworkError interface.
type HorizonNetworkError struct {
	err *horizonclient.Error
}

// NewHorizonNetworkError creates a new HorizonNetworkError from a horizonclient.Error.
func NewHorizonNetworkError(err *horizonclient.Error) *HorizonNetworkError {
	return &HorizonNetworkError{err: err}
}

// IsNotFound returns true if the error indicates the requested resource was not found.
func (e *HorizonNetworkError) IsNotFound() bool {
	return e.err.Response.StatusCode == 404
}

// IsBadSequence returns true if the error indicates a bad sequence number.
func (e *HorizonNetworkError) IsBadSequence() bool {
	resCode, err := e.err.ResultCodes()
	return err == nil && resCode.TransactionCode == "tx_bad_seq"
}

// IsTimeout returns true if the error indicates a timeout occurred.
func (e *HorizonNetworkError) IsTimeout() bool {
	return e.err.Problem.Status == 504 // Gateway Timeout
}

// ResultString returns the result string from the error, if available.
func (e *HorizonNetworkError) ResultString() (string, error) {
	return e.err.ResultString()
}

// Error implements the error interface.
func (e *HorizonNetworkError) Error() string {
	return e.err.Error()
}

// Unwrap returns the underlying horizonclient.Error.
func (e *HorizonNetworkError) Unwrap() error {
	return e.err
}

// HorizonNetworkClient wraps a horizon client and implements the NetworkClient interface.
type HorizonNetworkClient struct {
	client horizonclient.ClientInterface
}

// NewHorizonNetworkClient creates a new HorizonNetworkClient wrapping the given horizon client.
func NewHorizonNetworkClient(client horizonclient.ClientInterface) *HorizonNetworkClient {
	return &HorizonNetworkClient{client: client}
}

// SubmitTransaction submits a transaction using the underlying horizon client.
func (h *HorizonNetworkClient) SubmitTransaction(txXDR string) (*TransactionResult, error) {
	result, err := h.client.SubmitTransactionXDR(txXDR)
	if err != nil {
		if hErr, ok := err.(*horizonclient.Error); ok {
			return nil, NewHorizonNetworkError(hErr)
		}
		return nil, err
	}
	return &TransactionResult{
		Successful: result.Successful,
	}, nil
}

// GetAccountDetails retrieves account details using the underlying horizon client.
func (h *HorizonNetworkClient) GetAccountDetails(accountID string) (*AccountDetails, error) {
	request := horizonclient.AccountRequest{AccountID: accountID}
	account, err := h.client.AccountDetail(request)
	if err != nil {
		if hErr, ok := err.(*horizonclient.Error); ok {
			return nil, NewHorizonNetworkError(hErr)
		}
		return nil, err
	}

	nativeBalance := "0"
	for _, balance := range account.Balances {
		if balance.Type == "native" {
			nativeBalance = balance.Balance
			break
		}
	}

	return &AccountDetails{
		Sequence: account.Sequence,
		Balance:  nativeBalance,
	}, nil
}
