package horizonnetworkclient

import (
	"net/http"
	"testing"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/support/render/problem"
	"github.com/stretchr/testify/assert"
)

func TestNewNetworkClient(t *testing.T) {
	mockClient := &horizonclient.MockClient{}
	client := NewNetworkClient(mockClient)

	assert.NotNil(t, client)
	assert.Equal(t, mockClient, client.client)
}

func TestNetworkError_IsNotFound(t *testing.T) {
	response := &http.Response{StatusCode: http.StatusNotFound}
	horizonErr := &horizonclient.Error{
		Response: response,
		Problem: problem.P{
			Status: http.StatusNotFound,
		},
	}
	networkErr := NewNetworkError(horizonErr)

	assert.True(t, networkErr.IsNotFound())
	assert.False(t, networkErr.IsBadSequence())
	assert.False(t, networkErr.IsTimeout())
}

func TestNetworkError_IsBadSequence(t *testing.T) {
	// Test case 1: ResultCodes returns bad sequence code
	badSeqErr := &horizonclient.Error{
		Problem: problem.P{
			Extras: map[string]interface{}{
				"result_codes": map[string]interface{}{
					"transaction": "tx_bad_seq",
				},
			},
		},
	}
	networkErr := NewNetworkError(badSeqErr)
	assert.True(t, networkErr.IsBadSequence())

	// Test case 2: ResultCodes returns different code
	goodSeqErr := &horizonclient.Error{
		Problem: problem.P{
			Extras: map[string]interface{}{
				"result_codes": map[string]interface{}{
					"transaction": "tx_success",
				},
			},
		},
	}
	networkErr2 := NewNetworkError(goodSeqErr)
	assert.False(t, networkErr2.IsBadSequence())

	// Test case 3: ResultCodes returns error (no result_codes in extras)
	noResultCodesErr := &horizonclient.Error{
		Problem: problem.P{
			Extras: map[string]interface{}{},
		},
	}
	networkErr3 := NewNetworkError(noResultCodesErr)
	assert.False(t, networkErr3.IsBadSequence())
}

func TestNetworkError_IsTimeout(t *testing.T) {
	response := &http.Response{StatusCode: http.StatusGatewayTimeout}
	horizonErr := &horizonclient.Error{
		Response: response,
		Problem: problem.P{
			Status: http.StatusGatewayTimeout,
		},
	}
	networkErr := NewNetworkError(horizonErr)

	assert.False(t, networkErr.IsNotFound())
	assert.False(t, networkErr.IsBadSequence())
	assert.True(t, networkErr.IsTimeout())
}

func TestNetworkError_ResultString(t *testing.T) {
	horizonErr := &horizonclient.Error{}
	networkErr := NewNetworkError(horizonErr)

	// Test that the method exists and can be called
	result, err := networkErr.ResultString()
	// The actual result depends on the underlying error implementation
	// We just verify it doesn't panic and returns something
	assert.NotNil(t, result)
	_ = err // err might be nil or not depending on the underlying error
}

func TestNetworkError_Error(t *testing.T) {
	horizonErr := &horizonclient.Error{}
	networkErr := NewNetworkError(horizonErr)

	// Test that the method exists and can be called
	result := networkErr.Error()
	assert.NotEmpty(t, result) // Should return some string
}

func TestNetworkError_Unwrap(t *testing.T) {
	horizonErr := &horizonclient.Error{}
	networkErr := NewNetworkError(horizonErr)

	assert.Equal(t, horizonErr, networkErr.Unwrap())
}

func TestNetworkClient_SubmitTransaction_Success(t *testing.T) {
	mockClient := &horizonclient.MockClient{}
	expectedResult := horizon.Transaction{Successful: true}

	mockClient.On("SubmitTransactionXDR", "test-xdr").Return(expectedResult, nil)

	client := NewNetworkClient(mockClient)
	result, err := client.SubmitTransaction("test-xdr")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Successful)
	mockClient.AssertExpectations(t)
}

func TestNetworkClient_SubmitTransaction_HorizonError(t *testing.T) {
	mockClient := &horizonclient.MockClient{}
	response := &http.Response{StatusCode: http.StatusNotFound}
	horizonErr := &horizonclient.Error{
		Response: response,
		Problem: problem.P{
			Status: http.StatusNotFound,
		},
	}

	mockClient.On("SubmitTransactionXDR", "test-xdr").Return(horizon.Transaction{}, horizonErr)

	client := NewNetworkClient(mockClient)
	result, err := client.SubmitTransaction("test-xdr")

	assert.Error(t, err)
	assert.Nil(t, result)

	var networkErr *NetworkError
	assert.ErrorAs(t, err, &networkErr)
	assert.True(t, networkErr.IsNotFound())

	mockClient.AssertExpectations(t)
}

func TestNetworkClient_SubmitTransaction_GenericError(t *testing.T) {
	mockClient := &horizonclient.MockClient{}
	genericErr := assert.AnError

	mockClient.On("SubmitTransactionXDR", "test-xdr").Return(horizon.Transaction{}, genericErr)

	client := NewNetworkClient(mockClient)
	result, err := client.SubmitTransaction("test-xdr")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, genericErr, err)

	mockClient.AssertExpectations(t)
}

func TestNetworkClient_GetAccountDetails_Success(t *testing.T) {
	mockClient := &horizonclient.MockClient{}
	accountID := "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"
	expectedSequence := int64(12345)

	balances := []horizon.Balance{
		{Balance: "100.0000000"},
		{Balance: "50.0000000"},
	}
	// Set the Type field on the embedded Asset
	balances[0].Type = "native"
	balances[1].Type = "credit_alphanum4"

	account := horizon.Account{
		AccountID: accountID,
		Sequence:  expectedSequence,
		Balances:  balances,
	}

	mockClient.On("AccountDetail", horizonclient.AccountRequest{AccountID: accountID}).Return(account, nil)

	client := NewNetworkClient(mockClient)
	result, err := client.GetAccountDetails(accountID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedSequence, result.Sequence)
	assert.Equal(t, "100.0000000", result.Balance)

	mockClient.AssertExpectations(t)
}

func TestNetworkClient_GetAccountDetails_NoNativeBalance(t *testing.T) {
	mockClient := &horizonclient.MockClient{}
	accountID := "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"

	balances := []horizon.Balance{
		{Balance: "50.0000000"},
	}
	// Set the Type field on the embedded Asset
	balances[0].Type = "credit_alphanum4"

	account := horizon.Account{
		AccountID: accountID,
		Sequence:  12345,
		Balances:  balances,
	}

	mockClient.On("AccountDetail", horizonclient.AccountRequest{AccountID: accountID}).Return(account, nil)

	client := NewNetworkClient(mockClient)
	result, err := client.GetAccountDetails(accountID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(12345), result.Sequence)
	assert.Equal(t, "0", result.Balance) // Should default to "0" when no native balance found

	mockClient.AssertExpectations(t)
}

func TestNetworkClient_GetAccountDetails_HorizonError(t *testing.T) {
	mockClient := &horizonclient.MockClient{}
	accountID := "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"
	response := &http.Response{StatusCode: http.StatusNotFound}
	horizonErr := &horizonclient.Error{
		Response: response,
		Problem: problem.P{
			Status: http.StatusNotFound,
		},
	}

	mockClient.On("AccountDetail", horizonclient.AccountRequest{AccountID: accountID}).Return(horizon.Account{}, horizonErr)

	client := NewNetworkClient(mockClient)
	result, err := client.GetAccountDetails(accountID)

	assert.Error(t, err)
	assert.Nil(t, result)

	networkErr, ok := err.(*NetworkError)
	assert.True(t, ok)
	assert.True(t, networkErr.IsNotFound())

	mockClient.AssertExpectations(t)
}

func TestNetworkClient_GetAccountDetails_GenericError(t *testing.T) {
	mockClient := &horizonclient.MockClient{}
	accountID := "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"
	genericErr := assert.AnError

	mockClient.On("AccountDetail", horizonclient.AccountRequest{AccountID: accountID}).Return(horizon.Account{}, genericErr)

	client := NewNetworkClient(mockClient)
	result, err := client.GetAccountDetails(accountID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, genericErr, err)

	mockClient.AssertExpectations(t)
}
