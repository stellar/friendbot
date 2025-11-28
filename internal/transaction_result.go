package internal

// TransactionResult contains the final transaction result returned to callers.
type TransactionResult struct {
	Successful  bool   `json:"successful"`
	Hash        string `json:"hash"`
	EnvelopeXdr string `json:"envelope_xdr"`
}
