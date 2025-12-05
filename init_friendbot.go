package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/stellar/friendbot/internal"
	"github.com/stellar/friendbot/internal/horizonnetworkclient"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/support/errors"
	"github.com/stellar/go/txnbuild"
)

func initFriendbot(cfg Config) (*internal.Bot, error) {
	if cfg.FriendbotSecret == "" || cfg.NetworkPassphrase == "" || cfg.HorizonURL == "" || cfg.StartingBalance == "" || cfg.NumMinions < 0 {
		return nil, errors.New("invalid input param(s)")
	}

	// Guarantee that friendbotSecret is a seed, if not blank.
	strkey.MustDecode(strkey.VersionByteSeed, cfg.FriendbotSecret)

	hclient := &horizonclient.Client{
		HorizonURL: cfg.HorizonURL,
		HTTP:       http.DefaultClient,
		AppName:    "friendbot",
	}
	networkClient := horizonnetworkclient.NewNetworkClient(hclient)

	botKP, err := keypair.Parse(cfg.FriendbotSecret)
	if err != nil {
		return nil, errors.Wrap(err, "parsing bot keypair")
	}

	// Casting from the interface type will work, since we
	// already confirmed that friendbotSecret is a seed.
	botKeypair := botKP.(*keypair.Full)
	botAccount := internal.Account{AccountID: botKeypair.Address()}
	// set default values
	minionBalance := "101.00"
	numMinions := cfg.NumMinions
	if numMinions == 0 {
		numMinions = 1000
	}
	minionBatchSize := cfg.MinionBatchSize
	if minionBatchSize == 0 {
		minionBatchSize = 50
	}
	submitTxRetriesAllowed := cfg.SubmitTxRetriesAllowed
	if submitTxRetriesAllowed == 0 {
		submitTxRetriesAllowed = 5
	}
	log.Printf("Found all valid params, now creating %d minions", numMinions)
	minions, err := createMinionAccounts(botAccount, botKeypair, cfg.NetworkPassphrase, cfg.StartingBalance, minionBalance, numMinions, minionBatchSize, submitTxRetriesAllowed, cfg.BaseFee, networkClient)
	if err != nil && len(minions) == 0 {
		return nil, errors.Wrap(err, "creating minion accounts")
	}
	log.Printf("Adding %d minions to friendbot", len(minions))
	return &internal.Bot{Minions: minions}, nil
}

func createMinionAccounts(botAccount internal.Account, botKeypair *keypair.Full, networkPassphrase, newAccountBalance, minionBalance string,
	numMinions, minionBatchSize, submitTxRetriesAllowed int, baseFee int64, networkClient internal.NetworkClient) ([]internal.Minion, error) {

	var minions []internal.Minion
	numRemainingMinions := numMinions
	// Allow retries to account for testnet congestion
	currentSubmitTxRetry := 0

	for numRemainingMinions > 0 {
		var (
			newMinions []internal.Minion
			ops        []txnbuild.Operation
		)
		// Refresh the sequence number before submitting a new transaction.
		rerr := botAccount.RefreshSequenceNumber(networkClient)
		if rerr != nil {
			return minions, errors.Wrap(rerr, "refreshing bot seqnum")
		}
		// The tx will create min(numRemainingMinions, minionBatchSize) Minion accounts.
		numCreateMinions := minionBatchSize
		if numRemainingMinions < minionBatchSize {
			numCreateMinions = numRemainingMinions
		}
		log.Printf("Creating %d new minion accounts", numCreateMinions)
		for i := 0; i < numCreateMinions; i++ {
			minionKeypair, err := keypair.Random()
			if err != nil {
				return minions, errors.Wrap(err, "making keypair")
			}
			newMinions = append(newMinions, internal.Minion{
				Account:              internal.Account{AccountID: minionKeypair.Address()},
				Keypair:              minionKeypair,
				BotAccount:           botAccount,
				BotKeypair:           botKeypair,
				NetworkClient:        networkClient,
				Network:              networkPassphrase,
				StartingBalance:      newAccountBalance,
				SubmitTransaction:    internal.SubmitTransaction,
				CheckSequenceRefresh: internal.CheckSequenceRefresh,
				CheckAccountExists:   internal.CheckAccountExists,
				BaseFee:              baseFee,
			})

			ops = append(ops, &txnbuild.CreateAccount{
				Destination: minionKeypair.Address(),
				Amount:      minionBalance,
			})
		}

		// Build and submit batched account creation tx.
		tx, err := txnbuild.NewTransaction(
			txnbuild.TransactionParams{
				SourceAccount:        botAccount,
				IncrementSequenceNum: true,
				Operations:           ops,
				BaseFee:              txnbuild.MinBaseFee,
				Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(300)},
			},
		)
		if err != nil {
			return minions, errors.Wrap(err, "unable to build tx")
		}

		tx, err = tx.Sign(networkPassphrase, botKeypair)
		if err != nil {
			return minions, errors.Wrap(err, "unable to sign tx")
		}

		txe, err := tx.Base64()
		if err != nil {
			return minions, errors.Wrap(err, "unable to serialize tx")
		}

		err = networkClient.SubmitTransaction(txe)
		if err != nil {
			switch e := err.(type) {
			case internal.NetworkError:
				// If we hit an error here due to network congestion, try again until we hit max # of retries allowed
				if e.IsTimeout() {
					err = errors.Wrap(err, "submitting create accounts tx")
					if currentSubmitTxRetry >= submitTxRetriesAllowed {
						return minions, errors.Wrap(err, fmt.Sprintf("after retrying %d times", currentSubmitTxRetry))
					}
					log.Println(err)
					log.Println("trying again to submit create accounts tx")
					currentSubmitTxRetry += 1
					continue
				}
				return minions, errors.Wrap(err, "submitting create accounts tx")
			}
			return minions, errors.Wrap(err, "submitting create accounts tx")
		}
		currentSubmitTxRetry = 0

		// Process successful create accounts tx.
		numRemainingMinions -= numCreateMinions
		minions = append(minions, newMinions...)
		log.Printf("Submitted create accounts tx for %d minions successfully", numCreateMinions)
	}
	return minions, nil
}
