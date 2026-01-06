package rpcnetworkclient

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/stellar/friendbot/internal"
	"github.com/stellar/go-stellar-sdk/amount"
	rpcclient "github.com/stellar/go-stellar-sdk/clients/rpcclient"
	protocol "github.com/stellar/go-stellar-sdk/protocols/rpc"
	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/txnbuild"
	"github.com/stellar/go-stellar-sdk/xdr"
)

const (
	submitTransactionTimeout = 30 * time.Second
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
	return fmt.Sprintf("%s, result_xdr: %s, diagnostic_events_xdr: %v", e.err.Error(), e.resultXDR, e.diagnosticEventsXDR)
}

// Unwrap returns the underlying error.
func (e *NetworkError) Unwrap() error {
	return e.err
}

// Ensure NetworkError implements the internal.NetworkError interface.
var _ internal.NetworkError = (*NetworkError)(nil)

// zeroAddress is the zero G address used as source account for simulations.
const zeroAddress = "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF"

// NetworkClient wraps an RPC client and implements the internal.NetworkClient interface.
type NetworkClient struct {
	client      *rpcclient.Client
	nativeSACID xdr.Hash
	// TODO: Remove this field once rpcclient.Client exposes URL().
	// See https://github.com/stellar/go-stellar-sdk/issues/5885
	url string
}

// Ensure NetworkClient implements the internal.NetworkClient interface.
var _ internal.NetworkClient = (*NetworkClient)(nil)

// NewNetworkClient creates a new NetworkClient wrapping the given RPC client.
// The networkPassphrase is used to derive the native SAC contract address for balance queries.
func NewNetworkClient(url string, httpClient *http.Client, networkPassphrase string) *NetworkClient {
	client := rpcclient.NewClient(url, httpClient)
	nativeSACID, err := xdr.MustNewNativeAsset().ContractID(networkPassphrase)
	if err != nil {
		panic(fmt.Sprintf("failed to derive native SAC ID: %v", err))
	}
	return &NetworkClient{
		client:      client,
		nativeSACID: nativeSACID,
		url:         url,
	}
}

// URL returns the RPC URL used by this client.
func (r *NetworkClient) URL() string {
	return r.url
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

	txResponse, err := r.client.PollTransaction(ctx, response.Hash, rpcclient.PollTransactionOptions{})
	if err != nil {
		// Context timeout/cancellation
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return &NetworkError{
				err:     fmt.Errorf("timeout waiting for transaction %s to finalize", response.Hash),
				timeout: true,
			}
		}
		return &NetworkError{err: err}
	}

	// Check for failed transaction (SDK returns response without error for FAILED)
	if txResponse.Status == protocol.TransactionStatusFailed {
		return &NetworkError{
			err:                 fmt.Errorf("transaction failed"),
			resultXDR:           txResponse.ResultXDR,
			diagnosticEventsXDR: txResponse.DiagnosticEventsXDR,
		}
	}

	return nil
}

// SimulateTransaction simulates a transaction using the underlying RPC client.
// This is required for Soroban transactions to get resource fees.
func (r *NetworkClient) SimulateTransaction(txXDR string) (*internal.SimulateTransactionResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), submitTransactionTimeout)
	defer cancel()

	request := protocol.SimulateTransactionRequest{
		Transaction: txXDR,
	}

	response, err := r.client.SimulateTransaction(ctx, request)
	if err != nil {
		return nil, &NetworkError{err: err}
	}

	if response.Error != "" {
		return nil, &NetworkError{err: fmt.Errorf("simulation error: %s", response.Error)}
	}

	// Extract result XDR if present
	var resultXDR string
	for _, result := range response.Results {
		if result.ReturnValueXDR != nil && resultXDR == "" {
			resultXDR = *result.ReturnValueXDR
		}
	}

	return &internal.SimulateTransactionResult{
		TransactionDataXDR: response.TransactionDataXDR,
		ResultXDR:          resultXDR,
	}, nil
}

// GetAccountDetails retrieves account details using the underlying RPC client.
// For regular accounts (G addresses), it queries the ledger directly.
// For contract addresses (C addresses), it queries the native SAC balance via simulation.
// Contracts are treated as always existing, with sequence 0.
func (r *NetworkClient) GetAccountDetails(address string) (*internal.AccountDetails, error) {
	// Check if this is a contract address (C address)
	if strkey.IsValidContractAddress(address) {
		return r.getContractDetails(address)
	}

	// Regular account (G address)
	return r.getAccountDetails(address)
}

// getAccountDetails retrieves details for a regular Stellar account (G address).
func (r *NetworkClient) getAccountDetails(accountID string) (*internal.AccountDetails, error) {
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

// getContractDetails retrieves the native token balance for a contract address.
// Contracts are treated as always existing (no not-found error), with sequence 0.
func (r *NetworkClient) getContractDetails(contractAddress string) (*internal.AccountDetails, error) {
	// Decode the contract address to get the contract ID
	contractIDBytes, err := strkey.Decode(strkey.VersionByteContract, contractAddress)
	if err != nil {
		return nil, &NetworkError{err: fmt.Errorf("invalid contract address: %w", err)}
	}

	// Convert to xdr.ContractId (which is a typedef for Hash, which is [32]byte)
	var contractID xdr.ContractId
	copy(contractID[:], contractIDBytes)

	// Use the pre-computed native SAC ID
	nativeSACID := xdr.ContractId(r.nativeSACID)

	// Build the balance function call: balance(id: Address) -> i128
	contractIDScAddress := xdr.ScAddress{
		Type:       xdr.ScAddressTypeScAddressTypeContract,
		ContractId: &contractID,
	}

	invokeOp := txnbuild.InvokeHostFunction{
		HostFunction: xdr.HostFunction{
			Type: xdr.HostFunctionTypeHostFunctionTypeInvokeContract,
			InvokeContract: &xdr.InvokeContractArgs{
				ContractAddress: xdr.ScAddress{
					Type:       xdr.ScAddressTypeScAddressTypeContract,
					ContractId: &nativeSACID,
				},
				FunctionName: xdr.ScSymbol("balance"),
				Args: xdr.ScVec{
					xdr.ScVal{
						Type:    xdr.ScValTypeScvAddress,
						Address: &contractIDScAddress,
					},
				},
			},
		},
		SourceAccount: zeroAddress,
	}

	// Build the transaction for simulation using the zero address.
	// The zero address doesn't need to exist for simulation.
	tx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount: &txnbuild.SimpleAccount{
				AccountID: zeroAddress,
				Sequence:  0,
			},
			IncrementSequenceNum: true,
			Operations:           []txnbuild.Operation{&invokeOp},
			BaseFee:              txnbuild.MinBaseFee,
			Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewInfiniteTimeout()},
		},
	)
	if err != nil {
		return nil, &NetworkError{err: fmt.Errorf("failed to build balance query tx: %w", err)}
	}

	// Serialize for simulation
	txXDR, err := tx.Base64()
	if err != nil {
		return nil, &NetworkError{err: fmt.Errorf("failed to serialize balance query tx: %w", err)}
	}

	// Simulate the transaction
	simResult, err := r.SimulateTransaction(txXDR)
	if err != nil {
		return nil, err
	}

	// Parse the result - it should contain an i128 value
	if simResult.ResultXDR == "" {
		// No balance entry means 0 balance
		return &internal.AccountDetails{
			Sequence: 0,
			Balance:  "0",
		}, nil
	}

	var resultVal xdr.ScVal
	if err := xdr.SafeUnmarshalBase64(simResult.ResultXDR, &resultVal); err != nil {
		return nil, &NetworkError{err: fmt.Errorf("failed to parse balance result: %w", err)}
	}

	if resultVal.Type != xdr.ScValTypeScvI128 {
		return nil, &NetworkError{err: fmt.Errorf("unexpected balance result type: %v", resultVal.Type)}
	}

	i128 := resultVal.MustI128()
	// For balances that fit in int64, convert to string format
	// The high part should be 0 and low part must fit in int64 for reasonable balances
	if i128.Hi != 0 || i128.Lo > math.MaxInt64 {
		return nil, &NetworkError{err: fmt.Errorf("balance too large")}
	}

	// Convert stroops to XLM string format
	balance := amount.StringFromInt64(int64(i128.Lo)) //nolint:gosec // overflow checked above

	return &internal.AccountDetails{
		Sequence: 0, // Contracts don't have sequence numbers
		Balance:  balance,
	}, nil
}

// SupportsContractAddresses returns true as RPC can fund contract addresses.
func (r *NetworkClient) SupportsContractAddresses() bool {
	return true
}
