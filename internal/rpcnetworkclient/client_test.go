package rpcnetworkclient

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
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
