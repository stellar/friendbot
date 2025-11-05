package horizon

import (
	"net/http"

	"github.com/stellar/friendbot/internal"
	"github.com/stellar/go/clients/horizonclient"
)

// NetworkError wraps a horizonclient.Error and implements the internal.NetworkError interface.
type NetworkError struct {
	err *horizonclient.Error
}

// NewNetworkError creates a new NetworkError from a horizonclient.Error.
func NewNetworkError(err *horizonclient.Error) *NetworkError {
	return &NetworkError{err: err}
}

// IsNotFound returns true if the error indicates the requested resource was not found.
func (e *NetworkError) IsNotFound() bool {
	return e.err.Response.StatusCode == http.StatusNotFound
}

// IsBadSequence returns true if the error indicates a bad sequence number.
func (e *NetworkError) IsBadSequence() bool {
	resCode, err := e.err.ResultCodes()
	return err == nil && resCode.TransactionCode == "tx_bad_seq"
}

// IsTimeout returns true if the error indicates a timeout occurred.
func (e *NetworkError) IsTimeout() bool {
	return e.err.Problem.Status == http.StatusGatewayTimeout
}

// ResultString returns the result string from the error, if available.
func (e *NetworkError) ResultString() (string, error) {
	return e.err.ResultString()
}

// Error implements the error interface.
func (e *NetworkError) Error() string {
	return e.err.Error()
}

// Unwrap returns the underlying horizonclient.Error.
func (e *NetworkError) Unwrap() error {
	return e.err
}

// NetworkClient wraps a horizon client and implements the internal.NetworkClient interface.
type NetworkClient struct {
	client horizonclient.ClientInterface
}

// NewNetworkClient creates a new NetworkClient wrapping the given horizon client.
func NewNetworkClient(client horizonclient.ClientInterface) *NetworkClient {
	return &NetworkClient{client: client}
}

// SubmitTransaction submits a transaction using the underlying horizon client.
func (h *NetworkClient) SubmitTransaction(txXDR string) (*internal.TransactionResult, error) {
	result, err := h.client.SubmitTransactionXDR(txXDR)
	if err != nil {
		if hErr, ok := err.(*horizonclient.Error); ok {
			return nil, NewNetworkError(hErr)
		}
		return nil, err
	}
	return &internal.TransactionResult{
		Successful: result.Successful,
	}, nil
}

// GetAccountDetails retrieves account details using the underlying horizon client.
func (h *NetworkClient) GetAccountDetails(accountID string) (*internal.AccountDetails, error) {
	request := horizonclient.AccountRequest{AccountID: accountID}
	account, err := h.client.AccountDetail(request)
	if err != nil {
		if hErr, ok := err.(*horizonclient.Error); ok {
			return nil, NewNetworkError(hErr)
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

	return &internal.AccountDetails{
		Sequence: account.Sequence,
		Balance:  nativeBalance,
	}, nil
}
