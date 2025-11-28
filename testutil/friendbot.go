package testutil

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
)

// friendbotURL is the URL of the friendbot endpoint used for testing.
var friendbotURL = os.Getenv("FRIENDBOT_URL")

// FundAccount uses the friendbot endpoint to fund an account
func FundAccount(t *testing.T, address string) error {
	t.Helper()

	if friendbotURL == "" {
		t.Skip("FRIENDBOT_URL environment variable not set, skipping test")
	}

	url := fmt.Sprintf("%s?addr=%s", friendbotURL, address)

	// #nosec G107 - the url is from a trusted source configured in CI or local
	//nolint:noctx
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("friendbot returned status %d", resp.StatusCode)
	}

	var result struct {
		Successful bool `json:"successful"`
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	err = json.Unmarshal(body, &result)
	if err != nil {
		return err
	}
	if !result.Successful {
		return fmt.Errorf("account funding failed")
	}
	return nil
}
