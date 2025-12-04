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
	// DiagnosticEventsXDR returns the diagnostic events XDR strings from the error, if available.
	DiagnosticEventsXDR() []string
}

// NetworkClient defines a general interface for interacting with Stellar network services.
// It abstracts the functionality needed for friendbot operations, allowing different
// implementations (Horizon, RPC, etc.) to be used interchangeably.
type NetworkClient interface {
	// SubmitTransaction submits a transaction and blocks until it can return a result.
	SubmitTransaction(txXDR string) error

	// GetAccountDetails retrieves account information for the given account ID.
	GetAccountDetails(accountID string) (*AccountDetails, error)
}

// AccountDetails contains the minimal information needed about an account.
type AccountDetails struct {
	Sequence int64
	Balance  string
}
