package internal

import (
	"context"
	"log"
	"sync"
)

// Bot represents the friendbot subsystem and primarily delegates work
// to its Minions.
type Bot struct {
	Minions         []Minion
	nextMinionIndex int
	indexMux        sync.Mutex
}

// SubmitResult is the result from the asynchronous tx submission.
type SubmitResult struct {
	maybeTransactionSuccess *TransactionResult
	maybeErr                error
}

// Pay funds the account at `destAddress`.
func (bot *Bot) Pay(ctx context.Context, destAddress string) (*TransactionResult, error) {
	bot.indexMux.Lock()
	log.Printf("Selecting minion at index %d of max length %d", bot.nextMinionIndex, len(bot.Minions))
	minion := bot.Minions[bot.nextMinionIndex]
	bot.nextMinionIndex = (bot.nextMinionIndex + 1) % len(bot.Minions)
	bot.indexMux.Unlock()
	resultChan := make(chan SubmitResult)
	go minion.Run(ctx, destAddress, resultChan)
	maybeSubmitResult := <-resultChan
	close(resultChan)
	return maybeSubmitResult.maybeTransactionSuccess, maybeSubmitResult.maybeErr
}
