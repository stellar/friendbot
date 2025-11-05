package horizon

import (
	"net/http"

	"github.com/stellar/go/clients/horizonclient"
)

// NewHorizonClient creates a new horizon client configured for friendbot.
func NewHorizonClient(horizonURL string) *horizonclient.Client {
	return &horizonclient.Client{
		HorizonURL: horizonURL,
		HTTP:       http.DefaultClient,
		AppName:    "friendbot",
	}
}
