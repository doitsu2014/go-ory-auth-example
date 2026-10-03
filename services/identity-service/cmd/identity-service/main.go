// Command identity-service is the go-ory-auth-example domain service around
// identity: serve | migrate up | admin bootstrap | keys rewrap |
// pii reapply-erasures | healthcheck.
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
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/keto"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/kratos"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/localkms"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/mailer"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/openbao"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/postgres"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
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
	root.AddCommand(serveCmd(stderr), migrateCmd(stderr), adminCmd(stdout, stderr), keysCmd(stdout, stderr),
		piiCmd(stdout, stderr), healthcheckCmd())
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

// newKeyManager builds the PII key manager (DD-1). renew is the OpenBao
// token renewal loop, nil for the local provider.
func newKeyManager(cfg platform.Config, log *slog.Logger) (keys app.KeyManager, renew func(context.Context), err error) {
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
		})
		if err != nil {
			return nil, nil, err
		}
		return c, c.RenewLoop, nil
	}
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

	gate := &app.AdminGate{
		Identities: kadmin, Sessions: verifier, Tx: store, Clock: clock, Log: log,
		MaxAge: cfg.AdminSessionMaxAge, MFAGrace: cfg.AdminMFAGrace,
	}
	server := &httpapi.Server{
		Me: &app.MeService{Profiles: repos.Profiles},
		Customers: &app.CustomerService{
			Authz: authz, Identities: kadmin, Profiles: repos.Profiles, Tx: store, Sessions: verifier, Log: log,
		},
		Admins: &app.AdminService{
			Authz: authz, Roles: ketoClient, Identities: kadmin, Tx: store, Idempotency: repos.Idempotency,
			Mailer: mail, Clock: clock, Log: log, InvitationTTL: cfg.InvitationTTL,
		},
		Audit: &app.AuditService{Authz: authz, Audit: repos.Audit},
		PersonalInfo: &app.PersonalInfoService{
			Authz: authz, Identities: kadmin, Keys: keys, Cache: dekCache, Tx: store,
			SubjectKeys: repos.SubjectKeys, Records: repos.PersonalInfo, Clock: clock, Log: log,
			LookupLimiter: app.NewRateLimiter(app.LookupRateRules, nil),
			RevealLimiter: app.NewRateLimiter(app.RevealRateRules, nil),
			MaskedLimiter: app.NewRateLimiter(app.MaskedRateRules, nil),
		},
	}
	metrics := httpapi.NewMetrics()
	public, err := httpapi.NewPublicHandler(httpapi.PublicDeps{
		Log: log, Metrics: metrics, Server: server, Verifier: verifier, Gate: gate, Authz: authz,
		AllowedOrigins: cfg.CORSAllowedOrigins, TrustedHops: cfg.TrustedProxyHops,
	})
	if err != nil {
		return fmt.Errorf("refusing to start: %w", err)
	}
	webhooks := httpapi.NewWebhookHandler(httpapi.WebhookDeps{
		Log: log, Metrics: metrics, APIKey: cfg.KratosWebhookAPIKey,
		Provisioning: &app.ProvisioningService{Profiles: repos.Profiles},
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
	svc.SubjectKeys = postgres.NewStore(pool).Repos().SubjectKeys
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
			return nil
		},
	})
	return cmd
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
