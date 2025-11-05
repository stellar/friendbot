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
	notFoundErr := errors.New("account not found")
	networkErr := NewNetworkError(notFoundErr)
	assert.True(t, networkErr.IsNotFound())

	// Test regular error
	regularErr := errors.New("some other error")
	networkErr2 := NewNetworkError(regularErr)
	assert.False(t, networkErr2.IsNotFound())
}

func TestNetworkError_IsBadSequence(t *testing.T) {
	// Test bad sequence error
	badSeqErr := errors.New("bad sequence number")
	networkErr := NewNetworkError(badSeqErr)
	assert.True(t, networkErr.IsBadSequence())

	// Test bad_seq error
	badSeqErr2 := errors.New("tx_bad_seq occurred")
	networkErr2 := NewNetworkError(badSeqErr2)
	assert.True(t, networkErr2.IsBadSequence())

	// Test regular error
	regularErr := errors.New("some other error")
	networkErr3 := NewNetworkError(regularErr)
	assert.False(t, networkErr3.IsBadSequence())
}

func TestNetworkError_IsTimeout(t *testing.T) {
	// Test timeout error
	timeoutErr := errors.New("timeout occurred")
	networkErr := NewNetworkError(timeoutErr)
	assert.True(t, networkErr.IsTimeout())

	// Test deadline exceeded error
	deadlineErr := errors.New("deadline exceeded")
	networkErr2 := NewNetworkError(deadlineErr)
	assert.True(t, networkErr2.IsTimeout())

	// Test regular error
	regularErr := errors.New("some other error")
	networkErr3 := NewNetworkError(regularErr)
	assert.False(t, networkErr3.IsTimeout())
}

func TestNetworkError_ResultString(t *testing.T) {
	networkErr := NewNetworkError(errors.New("test error"))

	result, err := networkErr.ResultString()
	assert.Empty(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "RPC client does not provide result strings")
}

func TestNetworkError_Error(t *testing.T) {
	originalErr := errors.New("original error")
	networkErr := NewNetworkError(originalErr)

	result := networkErr.Error()
	assert.Equal(t, "original error", result)
}

func TestNetworkError_Unwrap(t *testing.T) {
	originalErr := errors.New("original error")
	networkErr := NewNetworkError(originalErr)

	assert.Equal(t, originalErr, networkErr.Unwrap())
}

// Note: Testing SubmitTransaction and GetAccountDetails would require
// setting up a real RPC server or complex mocking of the internal RPC client.
// For now, we focus on testing the error handling and structure.
// Integration tests can be added later when a test RPC server is available.