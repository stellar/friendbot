package main

import (
	"net/http"
	"testing"

	"github.com/stellar/friendbot/internal"
	"github.com/stellar/friendbot/internal/horizonnetworkclient"
	"github.com/stellar/friendbot/internal/rpcnetworkclient"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/support/render/problem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestInitFriendbot_createMinionAccounts_success(t *testing.T) {

	randSecretKey := "SDLNA2YUQSFIWVEB57M6D3OOCJHFVCVQZJ33LPA656KJESVRK5DQUZOH"
	botKP, err := keypair.Parse(randSecretKey)
	assert.NoError(t, err)

	botKeypair := botKP.(*keypair.Full)
	botAccountID := botKeypair.Address()
	botAccountMock := horizon.Account{
		AccountID: botAccountID,
		Sequence:  1,
	}
	botAccount := internal.Account{AccountID: botAccountID, Sequence: 1}

	horizonClientMock := horizonclient.MockClient{}
	horizonClientMock.
		On("AccountDetail", horizonclient.AccountRequest{
			AccountID: botAccountID,
		}).
		Return(botAccountMock, nil)
	horizonClientMock.
		On("SubmitTransactionXDR", mock.Anything).
		Return(horizon.Transaction{}, nil)

	numMinion := 1000
	minionBatchSize := 50
	submitTxRetriesAllowed := 5
	networkClient := horizonnetworkclient.NewNetworkClient(&horizonClientMock)
	createdMinions, err := createMinionAccounts(botAccount, botKeypair, "Test SDF Network ; September 2015", "10000", "101", numMinion, minionBatchSize, submitTxRetriesAllowed, 1000, networkClient)
	assert.NoError(t, err)

	assert.Equal(t, 1000, len(createdMinions))
}

func TestInitFriendbot_createMinionAccounts_timeoutError(t *testing.T) {
	randSecretKey := "SDLNA2YUQSFIWVEB57M6D3OOCJHFVCVQZJ33LPA656KJESVRK5DQUZOH"
	botKP, err := keypair.Parse(randSecretKey)
	assert.NoError(t, err)

	botKeypair := botKP.(*keypair.Full)
	botAccountID := botKeypair.Address()
	botAccountMock := horizon.Account{
		AccountID: botAccountID,
		Sequence:  1,
	}
	botAccount := internal.Account{AccountID: botAccountID, Sequence: 1}

	horizonClientMock := horizonclient.MockClient{}
	horizonClientMock.
		On("AccountDetail", horizonclient.AccountRequest{
			AccountID: botAccountID,
		}).
		Return(botAccountMock, nil)

	// Successful on first 3 calls only, and then a timeout error occurs
	horizonClientMock.
		On("SubmitTransactionXDR", mock.Anything).
		Return(horizon.Transaction{}, nil).Times(3)
	hError := &horizonclient.Error{
		Problem: problem.P{
			Type:   "timeout",
			Title:  "Timeout",
			Status: http.StatusGatewayTimeout,
		},
	}
	horizonClientMock.
		On("SubmitTransactionXDR", mock.Anything).
		Return(horizon.Transaction{}, hError)

	numMinion := 1000
	minionBatchSize := 50
	submitTxRetriesAllowed := 5
	networkClient := horizonnetworkclient.NewNetworkClient(&horizonClientMock)
	createdMinions, err := createMinionAccounts(botAccount, botKeypair, "Test SDF Network ; September 2015", "10000", "101", numMinion, minionBatchSize, submitTxRetriesAllowed, 1000, networkClient)
	assert.Equal(t, 150, len(createdMinions))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "after retrying 5 times: submitting create accounts tx:")
}

func TestNewNetworkClient(t *testing.T) {
	t.Run("error when both URLs are set", func(t *testing.T) {
		cfg := Config{
			HorizonURL: "https://horizon.stellar.org",
			RPCURL:     "https://rpc.stellar.org",
		}
		client, err := newNetworkClient(cfg)
		assert.Nil(t, client)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "only one of horizon_url or rpc_url should be provided")
	})

	t.Run("error when neither URL is set", func(t *testing.T) {
		cfg := Config{}
		client, err := newNetworkClient(cfg)
		assert.Nil(t, client)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "either horizon_url or rpc_url must be provided")
	})

	t.Run("returns horizon client when only HorizonURL is set", func(t *testing.T) {
		cfg := Config{
			HorizonURL: "https://horizon.stellar.org",
		}
		client, err := newNetworkClient(cfg)
		assert.NoError(t, err)
		assert.NotNil(t, client)
		horizonClient, ok := client.(*horizonnetworkclient.NetworkClient)
		assert.True(t, ok, "expected horizon network client")
		assert.Equal(t, "https://horizon.stellar.org", horizonClient.URL())
	})

	t.Run("returns RPC client when only RPCURL is set", func(t *testing.T) {
		cfg := Config{
			RPCURL: "https://rpc.stellar.org",
		}
		client, err := newNetworkClient(cfg)
		assert.NoError(t, err)
		assert.NotNil(t, client)
		rpcClient, ok := client.(*rpcnetworkclient.NetworkClient)
		assert.True(t, ok, "expected RPC network client")
		assert.Equal(t, "https://rpc.stellar.org", rpcClient.URL())
	})
}
