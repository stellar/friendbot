package main

import (
	"database/sql"
	"fmt"
	stdhttp "net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/riandyrn/otelchi"
	"github.com/spf13/cobra"

	"github.com/stellar/friendbot/internal"
	"github.com/stellar/go/support/app"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/errors"
	"github.com/stellar/go/support/http"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/problem"
	"github.com/stellar/go/utils/tracer"
)

const (
	serviceName    = "stellar-friendbot"
	serviceVersion = "1.0.0"
)

// ConfigFile represents the configuration loaded from --conf.
type ConfigFile struct {
	Port                   int         `toml:"port" valid:"required"`
	NetworkPassphrase      string      `toml:"network_passphrase" valid:"required"`
	HorizonURL             string      `toml:"horizon_url" valid:"optional"`
	RPCURL                 string      `toml:"rpc_url" valid:"optional"`
	StartingBalance        string      `toml:"starting_balance" valid:"required"`
	TLS                    *config.TLS `valid:"optional"`
	NumMinions             int         `toml:"num_minions" valid:"optional"`
	BaseFee                int64       `toml:"base_fee" valid:"optional"`
	MinionBatchSize        int         `toml:"minion_batch_size" valid:"optional"`
	SubmitTxRetriesAllowed int         `toml:"submit_tx_retries_allowed" valid:"optional"`
	UseCloudflareIP        bool        `toml:"use_cloudflare_ip" valid:"optional"`
	OtelEndpoint           string      `toml:"otel_endpoint" valid:"optional"`
	OtelEnabled            bool        `toml:"otel_enabled" valid:"optional"`

	// FriendbotSecret has been moved to SecretFile, but is still present here as
	// an optional parameter for backwards compatibility with old cfg files.
	FriendbotSecret string `toml:"friendbot_secret" valid:"optional"`
}

// SecretFile represents the secret configuration loaded from --secret.
type SecretFile struct {
	FriendbotSecret string `toml:"friendbot_secret" valid:"required"`
}

// Config is the combined configuration passed to the application.
type Config struct {
	Port                   int
	FriendbotSecret        string
	NetworkPassphrase      string
	HorizonURL             string
	RPCURL                 string
	StartingBalance        string
	TLS                    *config.TLS
	NumMinions             int
	BaseFee                int64
	MinionBatchSize        int
	SubmitTxRetriesAllowed int
	UseCloudflareIP        bool
	OtelEndpoint           string
	OtelEnabled            bool
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "friendbot",
		Short: "friendbot for the Stellar Test Network",
		Long:  "Client-facing API server for the friendbot service on the Stellar Test Network",
		Run:   run,
	}

	rootCmd.PersistentFlags().String("conf", "./friendbot.cfg", "config file path")
	rootCmd.PersistentFlags().String("secret", "", "secret config file path (optional, overrides friendbot_secret from conf)")
	rootCmd.Execute()
}

func run(cmd *cobra.Command, args []string) {
	cfgPath := cmd.PersistentFlags().Lookup("conf").Value.String()
	secretPath := cmd.PersistentFlags().Lookup("secret").Value.String()
	log.SetLevel(log.InfoLevel)

	cfg, err := loadConfig(cfgPath, secretPath)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}

	//Setup and initialize tracer
	tracer, err := tracer.InitializeTracer(cfg.OtelEnabled, cfg.OtelEndpoint, serviceName, serviceVersion)
	if err != nil {
		log.Error("Failed to initialize tracer:", err)
	}
	log.Infof("Tracer initialized")
	defer tracer()

	fb, err := initFriendbot(cfg)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}
	router := initRouter(cfg, fb)
	registerProblems()

	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Port)

	http.Run(http.Config{
		ListenAddr: addr,
		Handler:    router,
		TLS:        cfg.TLS,
		OnStarting: func() {
			log.Infof("starting friendbot server - %s", app.Version())
			log.Infof("listening on %s", addr)
		},
	})
}

// loadConfig loads configuration from the config file and optionally a separate
// secret file. If secretPath is empty, the secret is expected to be in the
// config file (backwards compatible). If secretPath is provided, it overrides
// any secret in the config file.
func loadConfig(cfgPath, secretPath string) (Config, error) {
	var cfgFile ConfigFile
	err := config.Read(cfgPath, &cfgFile)
	if err != nil {
		return Config{}, errors.Wrap(err, "reading config file")
	}

	// Build the combined config from the config file
	cfg := Config{
		Port:                   cfgFile.Port,
		NetworkPassphrase:      cfgFile.NetworkPassphrase,
		HorizonURL:             cfgFile.HorizonURL,
		RPCURL:                 cfgFile.RPCURL,
		StartingBalance:        cfgFile.StartingBalance,
		TLS:                    cfgFile.TLS,
		NumMinions:             cfgFile.NumMinions,
		BaseFee:                cfgFile.BaseFee,
		MinionBatchSize:        cfgFile.MinionBatchSize,
		SubmitTxRetriesAllowed: cfgFile.SubmitTxRetriesAllowed,
		UseCloudflareIP:        cfgFile.UseCloudflareIP,
		OtelEndpoint:           cfgFile.OtelEndpoint,
		OtelEnabled:            cfgFile.OtelEnabled,
		FriendbotSecret:        cfgFile.FriendbotSecret,
	}

	// If --secret is provided, load the secret from the separate file
	if secretPath != "" {
		var secretFile SecretFile
		err = config.Read(secretPath, &secretFile)
		if err != nil {
			return Config{}, errors.Wrap(err, "reading secret file")
		}
		cfg.FriendbotSecret = secretFile.FriendbotSecret
	}

	// Validate that we have a secret
	if cfg.FriendbotSecret == "" {
		return Config{}, errors.New("friendbot_secret is required: provide it in --conf or use --secret")
	}

	return cfg, nil
}

func initRouter(cfg Config, fb *internal.Bot) *chi.Mux {
	mux := newMux(cfg)
	handler := internal.NewFriendbotHandler(fb)
	mux.Get("/", handler.Handle)
	mux.Post("/", handler.Handle)
	mux.NotFound(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		problem.Render(r.Context(), w, problem.NotFound)
	}))

	return mux
}

func newMux(cfg Config) *chi.Mux {
	mux := chi.NewRouter()
	// first apply XFFMiddleware so we can have the real ip in the subsequent
	// middlewares
	mux.Use(http.XFFMiddleware(http.XFFMiddlewareConfig{BehindCloudflare: cfg.UseCloudflareIP}))
	mux.Use(http.NewAPIMux(log.DefaultLogger).Middlewares()...)
	mux.Use(otelchi.Middleware(serviceName, otelchi.WithChiRoutes(mux)))

	return mux
}

func registerProblems() {
	problem.RegisterHost("https://stellar.org/friendbot-errors/")
	problem.RegisterError(sql.ErrNoRows, problem.NotFound)

	accountExistsProblem := problem.BadRequest
	accountExistsProblem.Detail = internal.ErrAccountExists.Error()
	problem.RegisterError(internal.ErrAccountExists, accountExistsProblem)

	accountFundedProblem := problem.BadRequest
	accountFundedProblem.Detail = internal.ErrAccountFunded.Error()
	problem.RegisterError(internal.ErrAccountFunded, accountFundedProblem)
}
