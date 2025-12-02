package internal

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

// SimulateTransactionResult contains the result of simulating a transaction.
type SimulateTransactionResult struct {
	// TransactionDataXDR is the SorobanTransactionData XDR in base64.
	TransactionDataXDR string
	// MinResourceFee is the minimum resource fee required for the transaction.
	MinResourceFee int64
	// AuthXDR is the list of SorobanAuthorizationEntry XDRs in base64.
	AuthXDR []string
	// ResultXDR is the ScVal XDR result from simulation in base64 (for read-only calls).
	ResultXDR string
}

// NetworkClient defines a general interface for interacting with Stellar network services.
// It abstracts the functionality needed for friendbot operations, allowing different
// implementations (Horizon, RPC, etc.) to be used interchangeably.
type NetworkClient interface {
	// SubmitTransaction submits a transaction and blocks until it can return a result.
	SubmitTransaction(txXDR string) error

	// GetAccountDetails retrieves account information for the given account ID.
	GetAccountDetails(accountID string) (*AccountDetails, error)

	// SimulateTransaction simulates a transaction and returns the result.
	// This is required for Soroban transactions to get resource fees and auth entries.
	// For network clients that don't support simulation (like Horizon), this returns an error.
	SimulateTransaction(txXDR string) (*SimulateTransactionResult, error)
}

// AccountDetails contains the minimal information needed about an account.
type AccountDetails struct {
	Sequence int64
	Balance  string
}

// ParseSequenceNumber returns the sequence number as int64.
func (a *AccountDetails) ParseSequenceNumber() (int64, error) {
	return a.Sequence, nil
}
