package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("loads secret from conf file (backwards compatible)", func(t *testing.T) {
		confFile := filepath.Join(tmpDir, "friendbot.cfg")
		err := os.WriteFile(confFile, []byte(`
port = 8000
friendbot_secret = "SCZANGBA5YHTNYVVV3C7CAZMTQDBJHJG6C34CIRY52VDRRW3DPQUTZY2"
network_passphrase = "Test SDF Network ; September 2015"
starting_balance = "10000.00"
`), 0644)
		require.NoError(t, err)

		cfg, secrets, err := loadConfig(confFile, "")
		require.NoError(t, err)

		assert.Equal(t, 8000, cfg.Port)
		assert.Equal(t, "SCZANGBA5YHTNYVVV3C7CAZMTQDBJHJG6C34CIRY52VDRRW3DPQUTZY2", secrets.FriendbotSecret)
		assert.Equal(t, "Test SDF Network ; September 2015", cfg.NetworkPassphrase)
		assert.Equal(t, "10000.00", cfg.StartingBalance)
	})

	t.Run("loads secret from separate secret file", func(t *testing.T) {
		confFile := filepath.Join(tmpDir, "config.cfg")
		err := os.WriteFile(confFile, []byte(`
port = 8001
network_passphrase = "Test SDF Network ; September 2015"
starting_balance = "5000.00"
`), 0644)
		require.NoError(t, err)

		secretFile := filepath.Join(tmpDir, "secret.cfg")
		err = os.WriteFile(secretFile, []byte(`
friendbot_secret = "SBKGCMBY56GZQ4ZTQ4BXDPXG3MFAMZ6FMZQHGC3APMZ6AXRY5VZL7FRA"
`), 0644)
		require.NoError(t, err)

		cfg, secrets, err := loadConfig(confFile, secretFile)
		require.NoError(t, err)

		assert.Equal(t, 8001, cfg.Port)
		assert.Equal(t, "SBKGCMBY56GZQ4ZTQ4BXDPXG3MFAMZ6FMZQHGC3APMZ6AXRY5VZL7FRA", secrets.FriendbotSecret)
		assert.Equal(t, "Test SDF Network ; September 2015", cfg.NetworkPassphrase)
		assert.Equal(t, "5000.00", cfg.StartingBalance)
	})

	t.Run("secret file overrides secret in conf file", func(t *testing.T) {
		confFile := filepath.Join(tmpDir, "config_with_secret.cfg")
		err := os.WriteFile(confFile, []byte(`
port = 8002
friendbot_secret = "SCZANGBA5YHTNYVVV3C7CAZMTQDBJHJG6C34CIRY52VDRRW3DPQUTZY2"
network_passphrase = "Test SDF Network ; September 2015"
starting_balance = "10000.00"
`), 0644)
		require.NoError(t, err)

		secretFile := filepath.Join(tmpDir, "override_secret.cfg")
		err = os.WriteFile(secretFile, []byte(`
friendbot_secret = "SBKGCMBY56GZQ4ZTQ4BXDPXG3MFAMZ6FMZQHGC3APMZ6AXRY5VZL7FRA"
`), 0644)
		require.NoError(t, err)

		_, secrets, err := loadConfig(confFile, secretFile)
		require.NoError(t, err)

		// Secret should come from the secret file, not the conf file
		assert.Equal(t, "SBKGCMBY56GZQ4ZTQ4BXDPXG3MFAMZ6FMZQHGC3APMZ6AXRY5VZL7FRA", secrets.FriendbotSecret)
	})

	t.Run("error when no secret provided", func(t *testing.T) {
		confFile := filepath.Join(tmpDir, "no_secret.cfg")
		err := os.WriteFile(confFile, []byte(`
port = 8003
network_passphrase = "Test SDF Network ; September 2015"
starting_balance = "10000.00"
`), 0644)
		require.NoError(t, err)

		_, _, err = loadConfig(confFile, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "friendbot_secret is required")
	})

	t.Run("error when config file not found", func(t *testing.T) {
		_, _, err := loadConfig("/nonexistent/path.cfg", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reading config file")
	})

	t.Run("error when secret file not found", func(t *testing.T) {
		confFile := filepath.Join(tmpDir, "valid.cfg")
		err := os.WriteFile(confFile, []byte(`
port = 8004
network_passphrase = "Test SDF Network ; September 2015"
starting_balance = "10000.00"
`), 0644)
		require.NoError(t, err)

		_, _, err = loadConfig(confFile, "/nonexistent/secret.cfg")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reading secret file")
	})
}
