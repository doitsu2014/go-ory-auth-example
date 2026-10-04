// Command identity-service is the go-ory-auth-example domain service around
// identity: serve | migrate up | admin bootstrap | clients create |
// keys rewrap | pii reapply-erasures | pii migrate-kratos-names | healthcheck.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/httpapi"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/hydra"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/keto"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/kratos"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/localkms"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/mailer"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/openbao"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/postgres"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/sms"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/login"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/machine"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/platform"
)

func main() {
	if err := newRoot(os.Stdout, os.Stderr).Execute(); err != nil {
		os.Exit(1)
	}
}

func newRoot(stdout, stderr io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:           "identity-service",
		Short:         "Domain service around Ory Kratos/Keto identities",
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.AddCommand(serveCmd(stderr), migrateCmd(stderr), adminCmd(stdout, stderr), clientsCmd(stdout, stderr),
		keysCmd(stdout, stderr), piiCmd(stdout, stderr), healthcheckCmd())
	return root
}

func loadConfig(stderr io.Writer) (platform.Config, *slog.Logger, error) {
	cfg, err := platform.LoadConfig()
	if err != nil {
		return cfg, nil, err
	}
	return cfg, platform.NewLogger(stderr, cfg.LogLevel), nil
}

func serveCmd(stderr io.Writer) *cobra.Command {
	var migrate bool
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve :8080 public API, :8081 Kratos webhooks, :9090 ops",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, log, err := loadConfig(stderr)
			if err != nil {
				return err
			}
			if migrate {
				cfg.MigrateOnStart = true
			}
			if err := cfg.ValidateServe(); err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return serve(ctx, cfg, log)
		},
	}
	cmd.Flags().BoolVar(&migrate, "migrate", false, "apply migrations before serving (also MIGRATE_ON_START=true)")
	return cmd
}

func oryHTTP(cfg platform.Config) *http.Client {
	t := cfg.OryTimeout
	if t <= 0 {
		t = kratos.DefaultTimeout
	}
	return &http.Client{Timeout: t}
}

// keyManager is the PII key manager that also computes login pseudonyms and
// seals login identifiers (ADR-0013).
type keyManager interface {
	app.KeyManager
	app.LoginKeys
}

// newKeyManager builds the PII key manager (DD-1). renew is the OpenBao
// token renewal loop, nil for the local provider.
func newKeyManager(cfg platform.Config, log *slog.Logger) (keys keyManager, renew func(context.Context), err error) {
	if err := cfg.ValidatePII(); err != nil {
		return nil, nil, err
	}
	switch cfg.PIIKMSProvider {
	case platform.KMSProviderLocal:
		if !cfg.DevEnv() { // also enforced by ValidatePII; fail closed twice
			return nil, nil, errors.New("PII_KMS_PROVIDER=local is only allowed when APP_ENV is local or test")
		}
		log.Warn("using the local key manager (development only)", "app_env", cfg.AppEnv)
		k, err := localkms.NewFromBase64(cfg.PIILocalKEK, cfg.PIILocalBidxKey)
		return k, nil, err
	default:
		c, err := openbao.New(openbao.Config{
			Addr: cfg.PIIOpenBaoAddr, TokenFile: cfg.PIIOpenBaoTokenFile, KEKName: cfg.PIIOpenBaoKEKName,
			BidxKeyName: cfg.PIIOpenBaoBidxKeyName, Timeout: cfg.PIIOpenBaoTimeout, CAFile: cfg.PIIOpenBaoCAFile, Log: log,
			LoginHMACKeyName: cfg.PIIOpenBaoLoginHMACKeyName, LoginKEKName: cfg.PIIOpenBaoLoginKEKName,
		})
		if err != nil {
			return nil, nil, err
		}
		return c, c.RenewLoop, nil
	}
}

// newLoginService wires the login identifier use cases (serve and CLIs).
// Rate limiters are attached by serve only.
func newLoginService(cfg platform.Config, log *slog.Logger, keys app.LoginKeys, store *postgres.Store, kadmin *kratos.Admin) (*app.LoginIdentifierService, error) {
	phase, ok := app.ParseMigrationPhase(cfg.LoginMigrationPhase)
	if !ok {
		return nil, errors.New("LOGIN_MIGRATION_PHASE must be transition or complete")
	}
	return &app.LoginIdentifierService{
		Keys: keys, Logins: store.Repos().Logins, Tx: store, Identities: kadmin,
		Phone:        login.PhonePolicy{DefaultCountry: cfg.LoginPhoneDefaultCountry, AllowedCountries: cfg.LoginPhoneAllowedCountries},
		PhoneEnabled: cfg.SMSProvider != platform.SMSProviderDisabled,
		Phase:        phase, Clock: app.SystemClock{}, Log: log,
	}, nil
}

// newSMS builds the SMS channel (nil when disabled).
func newSMS(cfg platform.Config, mail app.Mailer) (app.SMSSender, error) {
	switch cfg.SMSProvider {
	case platform.SMSProviderSink:
		return &sms.Sink{Mailer: mail}, nil
	case platform.SMSProviderHTTP:
		return sms.NewHTTP(cfg.SMSHTTPURL, cfg.SMSHTTPToken, cfg.DevEnv(), nil)
	}
	return nil, nil
}

func serve(ctx context.Context, cfg platform.Config, log *slog.Logger) error {
	shutdownTracing, err := platform.SetupTracing(ctx, cfg.OTLPEndpoint)
	if err != nil {
		return err
	}
	defer func() { _ = shutdownTracing(context.Background()) }()

	if cfg.MigrateOnStart {
		if err := postgres.MigrateUp(ctx, cfg.MigrateDatabaseURL, log); err != nil {
			return err
		}
	}
	pool, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	store := postgres.NewStore(pool)
	repos := store.Repos()

	hc := oryHTTP(cfg)
	cache := kratos.NewSessionCache(cfg.SessionCacheSize, nil)
	verifier := kratos.NewSessionVerifier(cfg.KratosPublicURL, hc, cache)
	kadmin := kratos.NewAdmin(cfg.KratosAdminURL, hc)
	ketoClient := keto.New(cfg.KetoReadURL, cfg.KetoWriteURL, hc)
	authz := app.MemoAuthorizer{Next: ketoClient}
	mail, err := mailer.New(cfg.SMTPURL, cfg.SMTPFrom)
	if err != nil {
		return err
	}
	clock := app.SystemClock{}
	keys, renewKeys, err := newKeyManager(cfg, log)
	if err != nil {
		return err
	}
	dekCache := app.NewDEKCache(cfg.PIIDEKCacheSize, cfg.PIIDEKCacheTTL, nil)

	// Pseudonymous login identifiers and courier delivery (ADR-0013).
	logins, err := newLoginService(cfg, log, keys, store, kadmin)
	if err != nil {
		return err
	}
	rates, err := cfg.ParseLoginRates()
	if err != nil {
		return err
	}
	logins.ResolveLimiter = app.NewKeyedLimiter[string](rates.Resolve, nil)
	logins.RegisterLimiter = app.NewKeyedLimiter[string](rates.Register, nil)
	logins.NetLimiter = app.NewKeyedLimiter[string](rates.ResolveNet, nil)
	logins.InsertLimiter = app.NewKeyedLimiter[string](rates.InsertGlobal, nil)
	smsSender, err := newSMS(cfg, mail)
	if err != nil {
		return err
	}
	dedupeKey, err := cfg.DedupeKey()
	if err != nil {
		return err
	}
	courier := &app.CourierDispatcher{
		Logins: logins, Identities: kadmin, Profiles: repos.Profiles, Mailer: mail, SMS: smsSender,
		Dedupe: repos.Dispatches, DedupeKey: dedupeKey,
		RecipientRules: rates.CourierRecipient, SMSDailyBudget: cfg.SMSDailyBudget, SMSCountryDailyBudget: cfg.SMSCountryDailyBudget,
		Phase: logins.Phase, Clock: clock, Log: log,
	}

	// Machine-to-machine access (Hydra). The JWKS warm-up may fail while
	// Hydra starts; the machine plane then answers 503 until it is reachable.
	for _, w := range cfg.Warnings() {
		log.WarnContext(ctx, "configuration warning", "warning", w)
	}
	tagKey, err := cfg.ClientTagKey()
	if err != nil {
		return err
	}
	hydraAdmin, err := hydra.NewAdmin(cfg.HydraAdminURL, cfg.M2MAudience, tagKey, hc)
	if err != nil {
		return err
	}
	machineVerifier, err := hydra.NewVerifier(hydra.VerifierConfig{
		JWKSURL: cfg.M2MJWKSURL, Issuer: cfg.M2MIssuer, Audience: cfg.M2MAudience, Admin: hydraAdmin,
		ClientCacheTTL: cfg.M2MClientCacheTTL, Log: log,
	})
	if err != nil {
		return err
	}
	if err := machineVerifier.RefreshKeys(ctx); err != nil {
		log.WarnContext(ctx, "jwks warm-up failed; retrying on demand", "error", err.Error())
	}

	gate := &app.AdminGate{
		Identities: kadmin, Sessions: verifier, Tx: store, Clock: clock, Log: log,
		MaxAge: cfg.AdminSessionMaxAge, MFAGrace: cfg.AdminMFAGrace,
	}
	server := &httpapi.Server{
		Me: &app.MeService{Profiles: repos.Profiles, Logins: logins},
		Customers: &app.CustomerService{
			Authz: authz, Identities: kadmin, Profiles: repos.Profiles, Tx: store, Sessions: verifier, Log: log, Logins: logins,
		},
		Logins: logins,
		Admins: &app.AdminService{
			Authz: authz, Roles: ketoClient, Identities: kadmin, Tx: store, Idempotency: repos.Idempotency,
			Mailer: mail, Clock: clock, Log: log, InvitationTTL: cfg.InvitationTTL,
		},
		Audit: &app.AuditService{Authz: authz, Audit: repos.Audit},
		PersonalInfo: &app.PersonalInfoService{
			Authz: authz, Identities: kadmin, Keys: keys, Cache: dekCache, Tx: store,
			SubjectKeys: repos.SubjectKeys, Records: repos.PersonalInfo, Clock: clock, Log: log, NameTraits: kadmin,
			Logins:        logins,
			LookupLimiter: app.NewRateLimiter(app.LookupRateRules, nil),
			RevealLimiter: app.NewRateLimiter(app.RevealRateRules, nil),
			MaskedLimiter: app.NewRateLimiter(app.MaskedRateRules, nil),
		},
		Machine: &app.MachineService{Identities: kadmin, Audit: repos.Audit},
		ServiceClients: &app.ServiceClientService{
			Authz: authz, Clients: hydraAdmin, Verifier: machineVerifier, Tx: store, Idempotency: repos.Idempotency,
			Clock: clock, Log: log,
		},
	}
	metrics := httpapi.NewMetrics()
	public, err := httpapi.NewPublicHandler(httpapi.PublicDeps{
		Log: log, Metrics: metrics, Server: server, Verifier: verifier, Gate: gate, Authz: authz,
		AllowedOrigins: cfg.CORSAllowedOrigins, TrustedHops: cfg.TrustedProxyHops,
		MachineVerifier: machineVerifier, MachineClientLimiter: httpapi.NewMachineClientLimiter(cfg.M2MRateLimitPerMin, nil),
	})
	if err != nil {
		return fmt.Errorf("refusing to start: %w", err)
	}
	webhooks := httpapi.NewWebhookHandler(httpapi.WebhookDeps{
		Log: log, Metrics: metrics, APIKey: cfg.KratosWebhookAPIKey,
		Provisioning: &app.ProvisioningService{Profiles: repos.Profiles},
		Logins:       logins, Courier: courier, CourierAPIKey: cfg.KratosCourierAPIKey,
	})
	ops := httpapi.NewOpsHandler(log, metrics, []httpapi.Checker{
		{Name: "database", Check: store.Ping},
		{Name: "kratos_public", Check: verifier.Ready},
		{Name: "kratos_admin", Check: kadmin.Ready},
		{Name: "keto", Check: ketoClient.Ready},
	})

	servers := []*http.Server{
		newHTTPServer(cfg.HTTPAddr, public, ctx),
		newHTTPServer(cfg.WebhookAddr, webhooks, ctx),
		newHTTPServer(cfg.OpsAddr, ops, ctx),
	}
	g, gctx := errgroup.WithContext(ctx)
	for _, s := range servers {
		g.Go(func() error {
			log.InfoContext(ctx, "listening", "addr", s.Addr)
			if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return fmt.Errorf("listen %s: %w", s.Addr, err)
			}
			return nil
		})
	}
	g.Go(func() error {
		purgeIdempotencyKeys(gctx, repos.Idempotency, log)
		return nil
	})
	g.Go(func() error {
		purgeCourierDispatches(gctx, repos.Dispatches, repos.Logins, metrics, log)
		return nil
	})
	g.Go(func() error {
		sweepAdmins(gctx, gate, cfg.MFASweepInterval, log)
		return nil
	})
	if renewKeys != nil {
		g.Go(func() error {
			renewKeys(gctx)
			return nil
		})
	}
	g.Go(func() error {
		sweepDEKCache(gctx, dekCache)
		return nil
	})
	g.Go(func() error {
		machineVerifier.Run(gctx)
		return nil
	})
	g.Go(func() error {
		<-gctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownGracePeriod)
		defer cancel()
		var errs []error
		for _, s := range servers {
			errs = append(errs, s.Shutdown(sctx))
		}
		log.Info("shut down")
		return errors.Join(errs...)
	})
	return g.Wait()
}

func newHTTPServer(addr string, h http.Handler, base context.Context) *http.Server {
	return &http.Server{
		Addr: addr, Handler: h,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second,
		BaseContext: func(net.Listener) context.Context { return context.WithoutCancel(base) },
	}
}

// purgeCourierDispatches deletes courier de-duplication keys older than
// 24 h and samples the unbound login rows gauge, hourly.
func purgeCourierDispatches(ctx context.Context, repo app.CourierDispatchRepo, logins app.LoginIdentifierRepo, m *httpapi.Metrics, log *slog.Logger) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		if _, err := repo.Purge(ctx, time.Now().Add(-24*time.Hour)); err != nil && ctx.Err() == nil {
			log.WarnContext(ctx, "purge courier dispatches", "error", err.Error())
		}
		if n, err := logins.CountUnbound(ctx); err == nil {
			m.UnboundLogins(n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// purgeIdempotencyKeys deletes idempotency rows older than 24 h, hourly.
func purgeIdempotencyKeys(ctx context.Context, repo app.IdempotencyRepo, log *slog.Logger) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		n, err := repo.Purge(ctx, time.Now().Add(-app.IdempotencyTTL))
		if err != nil && ctx.Err() == nil {
			log.WarnContext(ctx, "purge idempotency keys", "error", err.Error())
		} else if n > 0 {
			log.InfoContext(ctx, "purged idempotency keys", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// sweepAdmins deactivates admins past the MFA enrolment deadline (S1),
// including admins that never call the API.
func sweepAdmins(ctx context.Context, gate *app.AdminGate, every time.Duration, log *slog.Logger) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		n, err := gate.Sweep(ctx)
		if err != nil && ctx.Err() == nil {
			log.WarnContext(ctx, "mfa sweep", "error", err.Error())
		} else if n > 0 {
			log.WarnContext(ctx, "mfa sweep deactivated admins", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// sweepDEKCache zeroes expired data keys every minute and all of them on
// shutdown, so unwrapped keys do not outlive their TTL in memory.
func sweepDEKCache(ctx context.Context, c *app.DEKCache) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			c.Purge()
			return
		case <-t.C:
			c.SweepExpired()
		}
	}
}

// keyRotation wires the operator key commands (DATABASE_URL, key manager).
func keyRotation(ctx context.Context, cfg platform.Config, log *slog.Logger, needKeys bool) (*app.KeyRotationService, func(), error) {
	if cfg.DatabaseURL == "" {
		return nil, nil, errors.New("DATABASE_URL is required")
	}
	svc := &app.KeyRotationService{Log: log}
	if needKeys {
		keys, _, err := newKeyManager(cfg, log)
		if err != nil {
			return nil, nil, err
		}
		svc.Keys = keys
	}
	pool, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, nil, err
	}
	repos := postgres.NewStore(pool).Repos()
	svc.SubjectKeys, svc.Logins = repos.SubjectKeys, repos.Logins
	return svc, pool.Close, nil
}

func keysCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{Use: "keys", Short: "PII data key operations (operator)"}
	var batch int
	rewrap := &cobra.Command{
		Use:   "rewrap",
		Short: "Re-wrap every customer data key with the newest KEK version (idempotent, resumable)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, log, err := loadConfig(stderr)
			if err != nil {
				return err
			}
			svc, done, err := keyRotation(cmd.Context(), cfg, log, true)
			if err != nil {
				return err
			}
			defer done()
			res, err := svc.Rewrap(cmd.Context(), batch)
			_, _ = fmt.Fprintf(stdout, "scanned=%d rewrapped=%d current=%d conflicts=%d failed=%d kek_latest_version=%d\n",
				res.Scanned, res.Rewrapped, res.Current, res.Conflicts, res.Failed, res.LatestVersion)
			if err != nil {
				return err
			}
			logins, ldone, err := loginCLI(cmd.Context(), cfg, log)
			if err != nil {
				return err
			}
			defer ldone()
			n, err := logins.Rewrap(cmd.Context())
			_, _ = fmt.Fprintf(stdout, "login_rewrapped=%d\n", n)
			return err
		},
	}
	rewrap.Flags().IntVar(&batch, "batch", app.DefaultRewrapBatch, "keys per batch")
	cmd.AddCommand(rewrap)
	return cmd
}

func piiCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{Use: "pii", Short: "Personal information operations (operator)"}
	cmd.AddCommand(&cobra.Command{
		Use:   "reapply-erasures",
		Short: "Delete data keys of customers erased after the key was created (run after any restore)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, log, err := loadConfig(stderr)
			if err != nil {
				return err
			}
			svc, done, err := keyRotation(cmd.Context(), cfg, log, false)
			if err != nil {
				return err
			}
			defer done()
			n, err := svc.ReapplyErasures(cmd.Context())
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(stdout, "erased_keys_deleted=%d\n", n)
			ln, err := svc.ReapplyLoginErasures(cmd.Context())
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(stdout, "erased_logins_deleted=%d\n", ln)
			return nil
		},
	})
	var dryRun, stripInvalid bool
	migrateNames := &cobra.Command{
		Use:   "migrate-kratos-names",
		Short: "Move customer names from Kratos traits into the encrypted personal info (idempotent)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, log, err := loadConfig(stderr)
			if err != nil {
				return err
			}
			svc, done, err := nameMigration(cmd.Context(), cfg, log)
			if err != nil {
				return err
			}
			defer done()
			svc.StripInvalid = stripInvalid
			res, err := svc.MigrateKratosNames(cmd.Context(), dryRun)
			prefix := ""
			if dryRun {
				prefix = "dry_run=true "
			}
			_, _ = fmt.Fprintf(stdout, "%sscanned=%d migrated=%d stripped_only=%d failed=%d\n",
				prefix, res.Scanned, res.Migrated, res.StrippedOnly, res.Failed)
			return err
		},
	}
	migrateNames.Flags().BoolVar(&dryRun, "dry-run", false, "report what would change without writing")
	migrateNames.Flags().BoolVar(&stripInvalid, "strip-invalid", false,
		"remove names that fail validation from Kratos without storing them (audited); default: keep them and count as failed")
	cmd.AddCommand(migrateNames)

	var loginDryRun bool
	migrateLogins := &cobra.Command{
		Use:   "migrate-kratos-logins",
		Short: "Replace legacy customer email traits with pseudonymous login identifiers (idempotent; ADR-0013)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, log, err := loadConfig(stderr)
			if err != nil {
				return err
			}
			logins, done, err := loginCLI(cmd.Context(), cfg, log)
			if err != nil {
				return err
			}
			defer done()
			kadmin := kratos.NewAdmin(cfg.KratosAdminURL, oryHTTP(cfg))
			svc := &app.LoginMigrationService{Logins: logins, Kratos: kadmin, Tx: logins.Tx, Log: log}
			res, err := svc.Migrate(cmd.Context(), loginDryRun)
			prefix := ""
			if loginDryRun {
				prefix = "dry_run=true "
			}
			_, _ = fmt.Fprintf(stdout, "%sscanned=%d migrated=%d reverified=%d skipped=%d failed=%d\n",
				prefix, res.Scanned, res.Migrated, res.Reverified, res.Skipped, res.Failed)
			if err == nil && res.Failed > 0 {
				err = fmt.Errorf("%d identities failed (see log; re-run after fixing)", res.Failed)
			}
			return err
		},
	}
	migrateLogins.Flags().BoolVar(&loginDryRun, "dry-run", false, "report what would change without writing")
	cmd.AddCommand(migrateLogins)

	var purgeDryRun bool
	var olderThan time.Duration
	purgeLogins := &cobra.Command{
		Use:   "purge-unbound-logins",
		Short: "Delete unclaimed login identifiers and logins of deleted identities (schedule daily)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, log, err := loadConfig(stderr)
			if err != nil {
				return err
			}
			logins, done, err := loginCLI(cmd.Context(), cfg, log)
			if err != nil {
				return err
			}
			defer done()
			res, err := logins.Purge(cmd.Context(), olderThan, purgeDryRun)
			prefix := ""
			if purgeDryRun {
				prefix = "dry_run=true "
			}
			_, _ = fmt.Fprintf(stdout, "%sunbound_deleted=%d rebound=%d orphan_deleted=%d\n",
				prefix, res.UnboundDeleted, res.Rebound, res.OrphanDeleted)
			return err
		},
	}
	purgeLogins.Flags().BoolVar(&purgeDryRun, "dry-run", false, "report what would change without writing")
	purgeLogins.Flags().DurationVar(&olderThan, "older-than", 24*time.Hour, "minimum age of an unbound entry")
	cmd.AddCommand(purgeLogins)
	return cmd
}

// loginCLI wires the login identifier service for operator commands
// (DATABASE_URL, key manager, Kratos admin).
func loginCLI(ctx context.Context, cfg platform.Config, log *slog.Logger) (*app.LoginIdentifierService, func(), error) {
	if cfg.DatabaseURL == "" {
		return nil, nil, errors.New("DATABASE_URL is required")
	}
	keys, _, err := newKeyManager(cfg, log)
	if err != nil {
		return nil, nil, err
	}
	pool, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, nil, err
	}
	svc, err := newLoginService(cfg, log, keys, postgres.NewStore(pool), kratos.NewAdmin(cfg.KratosAdminURL, oryHTTP(cfg)))
	if err != nil {
		pool.Close()
		return nil, nil, err
	}
	return svc, pool.Close, nil
}

// nameMigration wires `pii migrate-kratos-names` (DATABASE_URL, key
// manager, Kratos admin).
func nameMigration(ctx context.Context, cfg platform.Config, log *slog.Logger) (*app.NameMigrationService, func(), error) {
	if cfg.DatabaseURL == "" {
		return nil, nil, errors.New("DATABASE_URL is required")
	}
	keys, _, err := newKeyManager(cfg, log)
	if err != nil {
		return nil, nil, err
	}
	pool, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, nil, err
	}
	store := postgres.NewStore(pool)
	repos := store.Repos()
	kadmin := kratos.NewAdmin(cfg.KratosAdminURL, oryHTTP(cfg))
	pi := &app.PersonalInfoService{
		Identities: kadmin, Keys: keys, Cache: app.NewDEKCache(cfg.PIIDEKCacheSize, cfg.PIIDEKCacheTTL, nil), Tx: store,
		SubjectKeys: repos.SubjectKeys, Records: repos.PersonalInfo, Clock: app.SystemClock{}, Log: log,
	}
	return &app.NameMigrationService{PersonalInfo: pi, Kratos: kadmin, Log: log}, pool.Close, nil
}

func migrateCmd(stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{Use: "migrate", Short: "Database migrations"}
	cmd.AddCommand(&cobra.Command{
		Use:   "up",
		Short: "Apply all pending migrations (uses MIGRATE_DATABASE_URL)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, log, err := loadConfig(stderr)
			if err != nil {
				return err
			}
			if cfg.MigrateDatabaseURL == "" {
				return errors.New("MIGRATE_DATABASE_URL is required")
			}
			return postgres.MigrateUp(cmd.Context(), cfg.MigrateDatabaseURL, log)
		},
	})
	return cmd
}

func adminCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{Use: "admin", Short: "Operator commands"}
	var email, first, last string
	bootstrap := &cobra.Command{
		Use:   "bootstrap",
		Short: "Create the first super_admin and print a one-time recovery link (operator terminal only)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, log, err := loadConfig(stderr)
			if err != nil {
				return err
			}
			if cfg.DatabaseURL == "" {
				return errors.New("DATABASE_URL is required")
			}
			ctx := cmd.Context()
			pool, err := postgres.Open(ctx, cfg.DatabaseURL)
			if err != nil {
				return err
			}
			defer pool.Close()
			store := postgres.NewStore(pool)
			hc := oryHTTP(cfg)
			svc := &app.AdminService{
				Roles: keto.New(cfg.KetoReadURL, cfg.KetoWriteURL, hc), Identities: kratos.NewAdmin(cfg.KratosAdminURL, hc),
				Tx: store, Idempotency: store.Repos().Idempotency, Clock: app.SystemClock{}, Log: log,
				InvitationTTL: cfg.InvitationTTL,
			}
			res, err := svc.Bootstrap(ctx, email, identity.Name{First: first, Last: last})
			if err != nil {
				if errors.Is(err, app.ErrConflict) {
					return fmt.Errorf("an identity with email %q already exists; bootstrap only creates new admins", strings.ToLower(email))
				}
				return err
			}
			// Printed to the operator's terminal only; never logged.
			_, _ = fmt.Fprintf(stdout, "super_admin created: id=%s email=%s\n", res.Admin.ID, res.Admin.Email)
			_, _ = fmt.Fprintf(stdout, "Recovery link (one-time, expires %s):\n  %s\n", res.Recovery.ExpiresAt.UTC().Format(time.RFC3339), res.Recovery.Link)
			_, _ = fmt.Fprintf(stdout, "Recovery code: %s\n", res.Recovery.Code)
			_, _ = fmt.Fprintln(stdout, "Open the link, enter the code, then set a password and enrol TOTP in the same session (within 24h).")
			return nil
		},
	}
	bootstrap.Flags().StringVar(&email, "email", "", "admin email (required)")
	bootstrap.Flags().StringVar(&first, "first", "", "first name")
	bootstrap.Flags().StringVar(&last, "last", "", "last name")
	_ = bootstrap.MarkFlagRequired("email")
	cmd.AddCommand(bootstrap)
	return cmd
}

// clientsCmd registers machine clients from a trusted operator shell
// (M2M-FR-13). The secret is printed to stdout once and never logged.
func clientsCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{Use: "clients", Short: "Machine-to-machine service clients (operator)"}
	var name, owner string
	var scopes []string
	create := &cobra.Command{
		Use:   "create",
		Short: "Register a client_credentials client at Hydra and print its secret once (audited as the system actor)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, log, err := loadConfig(stderr)
			if err != nil {
				return err
			}
			if cfg.DatabaseURL == "" {
				return errors.New("DATABASE_URL is required")
			}
			if err := cfg.ValidateHydraAdmin(); err != nil {
				return err
			}
			if cfg.M2MAudience == "" {
				return errors.New("M2M_AUDIENCE is required")
			}
			tagKey, err := cfg.ClientTagKey()
			if err != nil {
				return err
			}
			hadmin, err := hydra.NewAdmin(cfg.HydraAdminURL, cfg.M2MAudience, tagKey, oryHTTP(cfg))
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			pool, err := postgres.Open(ctx, cfg.DatabaseURL)
			if err != nil {
				return err
			}
			defer pool.Close()
			store := postgres.NewStore(pool)
			svc := &app.ServiceClientService{
				Clients: hadmin, Tx: store,
				Idempotency: store.Repos().Idempotency, Clock: app.SystemClock{}, Log: log,
			}
			res, err := svc.CreateAsSystem(ctx, name, owner, scopes)
			if err != nil {
				var ve *app.ValidationError
				if errors.As(err, &ve) {
					parts := make([]string, len(ve.Fields))
					for i, f := range ve.Fields {
						parts[i] = f.Field + ": " + f.Code
					}
					return fmt.Errorf("invalid input (%s)", strings.Join(parts, ", "))
				}
				return err
			}
			c := res.Client
			// Printed to the operator's terminal only; never logged.
			_, _ = fmt.Fprintf(stdout, "service client created: client_id=%s name=%s scopes=%s\n",
				c.ClientID, c.Name, strings.Join(scopeStrings(c), " "))
			_, _ = fmt.Fprintf(stdout, "client_secret (shown once, store it in a secret manager now): %s\n", res.Secret)
			return nil
		},
	}
	create.Flags().StringVar(&name, "name", "", "client name, 3-64 chars [a-z0-9-] (required)")
	create.Flags().StringVar(&owner, "owner", "", "owner contact email (required)")
	create.Flags().StringSliceVar(&scopes, "scope", nil, "scope: customers:read | audit:read (repeatable, required)")
	_ = create.MarkFlagRequired("name")
	_ = create.MarkFlagRequired("owner")
	_ = create.MarkFlagRequired("scope")
	cmd.AddCommand(create)
	return cmd
}

func scopeStrings(c machine.ServiceClient) []string {
	out := make([]string, len(c.Scopes))
	for i, s := range c.Scopes {
		out[i] = string(s)
	}
	return out
}

// healthcheckCmd lets the distroless container probe itself (no shell/curl).
func healthcheckCmd() *cobra.Command {
	var url string
	cmd := &cobra.Command{
		Use:   "healthcheck",
		Short: "Exit 0 if the ops readiness endpoint answers 200",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 4*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return err
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("not ready: %d", resp.StatusCode)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&url, "url", "http://127.0.0.1:9090/readyz", "readiness URL")
	return cmd
}
