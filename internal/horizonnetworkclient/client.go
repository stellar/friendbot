package horizonnetworkclient

import (
	"errors"
	"net/http"

	"github.com/stellar/friendbot/internal"
	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	"github.com/stellar/go-stellar-sdk/strkey"
)

// ErrSimulationNotSupported is returned when SimulateTransaction is called on a Horizon client.
var ErrSimulationNotSupported = errors.New("transaction simulation is not supported by Horizon, configure rpc_url instead of horizon_url to fund contract addresses")

// ErrContractAddressNotSupported is returned when GetAccountDetails is called with a contract address on a Horizon client.
var ErrContractAddressNotSupported = errors.New("contract addresses are not supported by Horizon, configure rpc_url instead of horizon_url to fund contract addresses")

// NetworkError wraps a horizon error and implements the internal.NetworkError interface.
type NetworkError struct {
	err *horizonclient.Error
}

// NewNetworkError creates a new NetworkError from a horizonclient.Error.
func NewNetworkError(err *horizonclient.Error) *NetworkError {
	return &NetworkError{err: err}
}

// IsNotFound returns true if the error indicates the requested resource was not found.
func (e *NetworkError) IsNotFound() bool {
	return e.err.Response != nil && e.err.Response.StatusCode == http.StatusNotFound
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

// DiagnosticEventStrings returns nil as Horizon does not provide diagnostic events.
func (e *NetworkError) DiagnosticEventStrings() []string {
	return nil
}

// Error implements the error interface.
func (e *NetworkError) Error() string {
	return e.err.Error()
}

// Unwrap returns the underlying horizonclient.Error.
func (e *NetworkError) Unwrap() error {
	return e.err
}

// Ensure NetworkError implements the internal.NetworkError interface.
var _ internal.NetworkError = (*NetworkError)(nil)

// NetworkClient wraps a horizon client and implements the internal.NetworkClient interface.
type NetworkClient struct {
	client horizonclient.ClientInterface
}

// Ensure NetworkClient implements the internal.NetworkClient interface.
var _ internal.NetworkClient = (*NetworkClient)(nil)

// NewNetworkClient creates a new NetworkClient wrapping the given horizon client.
func NewNetworkClient(client horizonclient.ClientInterface) *NetworkClient {
	return &NetworkClient{client: client}
}

// URL returns the Horizon URL if the underlying client is a *horizonclient.Client.
// Returns empty string for mock clients or other implementations.
func (h *NetworkClient) URL() string {
	if c, ok := h.client.(*horizonclient.Client); ok {
		return c.HorizonURL
	}
	return ""
}

// SubmitTransaction submits a transaction using the underlying horizon client.
func (h *NetworkClient) SubmitTransaction(txXDR string) error {
	_, err := h.client.SubmitTransactionXDR(txXDR)
	if err != nil {
		if hErr, ok := err.(*horizonclient.Error); ok {
			return NewNetworkError(hErr)
		}
		return err
	}
	return nil
}

// GetAccountDetails retrieves account details using the underlying horizon client.
// For contract addresses (C addresses), this returns an error since Horizon cannot query contract balances.
func (h *NetworkClient) GetAccountDetails(address string) (*internal.AccountDetails, error) {
	// Contract addresses are not supported by Horizon
	if strkey.IsValidContractAddress(address) {
		return nil, ErrContractAddressNotSupported
	}

	request := horizonclient.AccountRequest{AccountID: address}
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

// SimulateTransaction returns an error as Horizon does not support transaction simulation.
// To fund contracts (C addresses), use RPC instead.
func (h *NetworkClient) SimulateTransaction(txXDR string) (*internal.SimulateTransactionResult, error) {
	return nil, ErrSimulationNotSupported
}

// SupportsContractAddresses returns false as Horizon cannot fund contract addresses.
func (h *NetworkClient) SupportsContractAddresses() bool {
	return false
}
