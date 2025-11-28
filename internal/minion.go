package internal

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/stellar/go/amount"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/support/errors"
	"github.com/stellar/go/txnbuild"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const createAccountAlreadyExistXDR = "AAAAAAAAAGT/////AAAAAQAAAAAAAAAA/////AAAAAA="

var ErrAccountExists error = errors.New(fmt.Sprintf("createAccountAlreadyExist (%s)", createAccountAlreadyExistXDR))

var ErrAccountFunded error = errors.New("account already funded to starting balance")

var botTracer = otel.Tracer("stellar_friendbot_minion")

// Minion contains a Stellar channel account and Go channels to communicate with friendbot.
type Minion struct {
	Account         Account
	Keypair         *keypair.Full
	BotAccount      txnbuild.Account
	BotKeypair      *keypair.Full
	NetworkClient   NetworkClient
	Network         string
	StartingBalance string
	BaseFee         int64

	// Mockable functions
	SubmitTransaction    func(ctx context.Context, minion *Minion, networkClient NetworkClient, txHash [32]byte, tx string) (*TransactionResult, error)
	CheckSequenceRefresh func(minion *Minion, networkClient NetworkClient) error
	CheckAccountExists   func(minion *Minion, networkClient NetworkClient, destAddress string) (bool, string, error)

	// Uninitialized.
	forceRefreshSequence bool
}

// Run reads a payment destination address and an output channel. It attempts
// to pay that address and submits the result to the channel.
func (minion *Minion) Run(ctx context.Context, destAddress string, resultChan chan SubmitResult) {
	ctx, span := botTracer.Start(ctx, "minion.run.pay_minion")
	defer span.End()
	span.SetAttributes(attribute.String("minion.account_id", minion.Account.AccountID))
	err := minion.CheckSequenceRefresh(minion, minion.NetworkClient)
	if err != nil {
		resultChan <- SubmitResult{
			maybeTransactionSuccess: nil,
			maybeErr:                errors.Wrap(err, "checking minion seq"),
		}
		return
	}
	exists, balance, err := minion.CheckAccountExists(minion, minion.NetworkClient, destAddress)
	if err != nil {
		resultChan <- SubmitResult{
			maybeTransactionSuccess: nil,
			maybeErr:                errors.Wrap(err, "checking account exists"),
		}
		return
	}
	if exists {
		span.AddEvent("Destination account exists")
		span.SetAttributes(attribute.String("destination.account_address", destAddress),
			attribute.String("destination.account_balance", balance))
	}
	err = minion.checkBalance(balance)
	if err != nil {
		resultChan <- SubmitResult{
			maybeTransactionSuccess: nil,
			maybeErr:                errors.Wrap(err, "account already funded"),
		}
		return
	}
	txHash, txStr, err := minion.makeTx(destAddress, exists)
	if err != nil {
		resultChan <- SubmitResult{
			maybeTransactionSuccess: nil,
			maybeErr:                errors.Wrap(err, "making payment tx"),
		}
		return
	}
	_, err = minion.Account.IncrementSequenceNumber()
	if err != nil {
		resultChan <- SubmitResult{
			maybeTransactionSuccess: nil,
			maybeErr:                errors.Wrap(err, "incrementing submitters sequence number"),
		}
		return
	}
	succ, err := minion.SubmitTransaction(ctx, minion, minion.NetworkClient, txHash, txStr)
	resultChan <- SubmitResult{
		maybeTransactionSuccess: succ,
		maybeErr:                errors.Wrapf(err, "submitting tx to minion %x", txHash),
	}
	if succ != nil {
		span.SetAttributes(attribute.Bool("minion.tx_success_status", succ.Successful))
		span.SetStatus(codes.Ok, codes.Ok.String())
	}
}

// SubmitTransaction should be passed to the Minion.
func SubmitTransaction(ctx context.Context, minion *Minion, networkClient NetworkClient, txHash [32]byte, tx string) (*TransactionResult, error) {
	_, span := botTracer.Start(ctx, "minion.submit_transaction")
	defer span.End()

	err := networkClient.SubmitTransaction(tx)
	if err != nil {
		errStr := "submitting tx to horizon"
		switch e := err.(type) {
		case NetworkError:
			minion.checkHandleBadSequence(e)
			resStr, resErr := e.ResultString()
			if resErr != nil {
				errStr += ": error getting network error code: " + resErr.Error()
			} else if resStr == createAccountAlreadyExistXDR {
				span.SetStatus(codes.Error, errStr)
				span.AddEvent("transaction submission failed")
				return nil, errors.Wrap(ErrAccountExists, errStr)
			} else {
				errStr += ": network error string: " + resStr
			}
			span.SetStatus(codes.Error, errStr)
			span.AddEvent("transaction submission failed")
			return nil, errors.New(errStr)
		}
		span.SetStatus(codes.Error, err.Error())
		span.AddEvent("transaction submission failed")
		return nil, errors.Wrap(err, errStr)
	}
	// Construct the final transaction result with hash and envelope XDR
	result := &TransactionResult{
		Successful:  true,
		Hash:        hex.EncodeToString(txHash[:]),
		EnvelopeXdr: tx,
	}
	span.SetAttributes(attribute.String("minion.tx_hash", result.Hash))
	span.AddEvent("transaction submission success")
	span.SetStatus(codes.Ok, codes.Ok.String())
	return result, nil
}

// CheckSequenceRefresh establishes the minion's initial sequence number, if needed.
// This should also be passed to the minion.
func CheckSequenceRefresh(minion *Minion, networkClient NetworkClient) error {
	if minion.Account.Sequence != 0 && !minion.forceRefreshSequence {
		return nil
	}
	err := minion.Account.RefreshSequenceNumber(networkClient)
	if err != nil {
		return errors.Wrap(err, "refreshing minion seqnum")
	}
	minion.forceRefreshSequence = false
	return nil
}

// CheckAccountExists checks if the specified address exists as a Stellar account.
// And returns the current native balance of the account also.
// This should also be passed to the minion.
func CheckAccountExists(minion *Minion, networkClient NetworkClient, address string) (bool, string, error) {
	accountDetails, err := networkClient.GetAccountDetails(address)
	switch e := err.(type) {
	case nil:
		return true, accountDetails.Balance, nil
	case NetworkError:
		if e.IsNotFound() {
			return false, "0", nil
		}
	}
	return false, "0", err
}

func (minion *Minion) checkHandleBadSequence(err NetworkError) {
	isTxBadSeqCode := err.IsBadSequence()
	if !isTxBadSeqCode {
		return
	}
	minion.forceRefreshSequence = true
}

func (minion *Minion) checkBalance(balance string) error {
	bal, err := amount.ParseInt64(balance)
	if err != nil {
		return errors.Wrap(err, "cannot parse account balance")
	}
	starting, err := amount.ParseInt64(minion.StartingBalance)
	if err != nil {
		return errors.Wrap(err, "cannot parse starting balance")
	}
	if bal >= starting {
		return ErrAccountFunded
	}
	return nil
}

func (minion *Minion) makeTx(destAddress string, exists bool) ([32]byte, string, error) {
	if exists {
		return minion.makePaymentTx(destAddress)
	} else {
		return minion.makeCreateTx(destAddress)
	}
}

func (minion *Minion) makeCreateTx(destAddress string) ([32]byte, string, error) {
	createAccountOp := txnbuild.CreateAccount{
		Destination:   destAddress,
		SourceAccount: minion.BotAccount.GetAccountID(),
		Amount:        minion.StartingBalance,
	}
	tx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount:        minion.Account,
			IncrementSequenceNum: true,
			Operations:           []txnbuild.Operation{&createAccountOp},
			BaseFee:              minion.BaseFee,
			Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewInfiniteTimeout()},
		},
	)
	if err != nil {
		return [32]byte{}, "", errors.Wrap(err, "unable to build tx")
	}

	tx, err = tx.Sign(minion.Network, minion.Keypair, minion.BotKeypair)
	if err != nil {
		return [32]byte{}, "", errors.Wrap(err, "unable to sign tx")
	}

	txe, err := tx.Base64()
	if err != nil {
		return [32]byte{}, "", errors.Wrap(err, "unable to serialize")
	}

	txh, err := tx.Hash(minion.Network)
	if err != nil {
		return [32]byte{}, "", errors.Wrap(err, "unable to hash")
	}
	return txh, txe, err
}

func (minion *Minion) makePaymentTx(destAddress string) ([32]byte, string, error) {
	paymentOp := txnbuild.Payment{
		SourceAccount: minion.BotAccount.GetAccountID(),
		Destination:   destAddress,
		Asset:         txnbuild.NativeAsset{},
		Amount:        minion.StartingBalance,
	}
	tx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount:        minion.Account,
			IncrementSequenceNum: true,
			Operations:           []txnbuild.Operation{&paymentOp},
			BaseFee:              minion.BaseFee,
			Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewInfiniteTimeout()},
		},
	)
	if err != nil {
		return [32]byte{}, "", errors.Wrap(err, "unable to build tx")
	}

	tx, err = tx.Sign(minion.Network, minion.Keypair, minion.BotKeypair)
	if err != nil {
		return [32]byte{}, "", errors.Wrap(err, "unable to sign tx")
	}

	txe, err := tx.Base64()
	if err != nil {
		return [32]byte{}, "", errors.Wrap(err, "unable to serialize")
	}

	txh, err := tx.Hash(minion.Network)
	if err != nil {
		return [32]byte{}, "", errors.Wrap(err, "unable to hash")
	}

	return txh, txe, err
}
