// Package platform holds process wiring helpers: config, logging, telemetry.
package platform

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
)

// Config is the 12-factor environment configuration.
type Config struct {
	HTTPAddr    string `env:"HTTP_ADDR" envDefault:":8080"`
	WebhookAddr string `env:"WEBHOOK_ADDR" envDefault:":8081"`
	OpsAddr     string `env:"OPS_ADDR" envDefault:":9090"`

	DatabaseURL        string `env:"DATABASE_URL"`
	MigrateDatabaseURL string `env:"MIGRATE_DATABASE_URL"`
	MigrateOnStart     bool   `env:"MIGRATE_ON_START" envDefault:"false"`

	KratosPublicURL string        `env:"KRATOS_PUBLIC_URL" envDefault:"http://localhost:4433"`
	KratosAdminURL  string        `env:"KRATOS_ADMIN_URL" envDefault:"http://127.0.0.1:4434"`
	KetoReadURL     string        `env:"KETO_READ_URL" envDefault:"http://127.0.0.1:4466"`
	KetoWriteURL    string        `env:"KETO_WRITE_URL" envDefault:"http://127.0.0.1:4467"`
	OryTimeout      time.Duration `env:"ORY_TIMEOUT" envDefault:"2s"`

	KratosWebhookAPIKey string `env:"KRATOS_WEBHOOK_API_KEY"`

	CORSAllowedOrigins []string `env:"CORS_ALLOWED_ORIGINS" envSeparator:","`

	SMTPURL     string `env:"SMTP_URL" envDefault:"smtp://127.0.0.1:1025"`
	SMTPFrom    string `env:"SMTP_FROM" envDefault:"no-reply@go-ory-auth-example.local"`
	AdminWebURL string `env:"ADMIN_WEB_URL" envDefault:"http://localhost:5173"`

	LogLevel     string `env:"LOG_LEVEL" envDefault:"info"`
	OTLPEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`

	SessionCacheSize    int           `env:"SESSION_CACHE_SIZE" envDefault:"10000"`
	AdminSessionMaxAge  time.Duration `env:"ADMIN_SESSION_MAX_AGE" envDefault:"12h"`
	AdminMFAGrace       time.Duration `env:"ADMIN_MFA_ENROLLMENT_GRACE" envDefault:"24h"`
	InvitationTTL       time.Duration `env:"ADMIN_INVITATION_TTL" envDefault:"24h"`
	ShutdownGracePeriod time.Duration `env:"SHUTDOWN_GRACE_PERIOD" envDefault:"15s"`
	// TrustedProxyHops is the number of trusted reverse proxies in front of
	// the service; 0 ignores X-Forwarded-For.
	TrustedProxyHops int           `env:"TRUSTED_PROXY_HOPS" envDefault:"0"`
	MFASweepInterval time.Duration `env:"ADMIN_MFA_SWEEP_INTERVAL" envDefault:"5m"`

	// AppEnv is production | staging | local | test. It defaults to
	// production so that development-only switches fail closed.
	AppEnv string `env:"APP_ENV" envDefault:"production"`

	// Personal information encryption (intent 261003-encrypt-user-pii).
	PIIKMSProvider        string        `env:"PII_KMS_PROVIDER" envDefault:"openbao"`
	PIIOpenBaoAddr        string        `env:"PII_OPENBAO_ADDR" envDefault:"http://127.0.0.1:8200"`
	PIIOpenBaoTokenFile   string        `env:"PII_OPENBAO_TOKEN_FILE"`
	PIIOpenBaoKEKName     string        `env:"PII_OPENBAO_KEK_NAME" envDefault:"identity-pii-kek"`
	PIIOpenBaoBidxKeyName string        `env:"PII_OPENBAO_BIDX_KEY_NAME" envDefault:"identity-pii-bidx"`
	PIIOpenBaoTimeout     time.Duration `env:"PII_OPENBAO_TIMEOUT" envDefault:"2s"`
	// PIIOpenBaoCAFile optionally replaces the system roots with a PEM CA
	// bundle for the OpenBao TLS connection.
	PIIOpenBaoCAFile string `env:"PII_OPENBAO_CA_FILE"`
	// PIILocalKEK / PIILocalBidxKey are base64 32-byte keys for
	// PII_KMS_PROVIDER=local (APP_ENV local|test only). Secrets: never log.
	PIILocalKEK     string        `env:"PII_LOCAL_KEK"`
	PIILocalBidxKey string        `env:"PII_LOCAL_BIDX_KEY"`
	PIIDEKCacheTTL  time.Duration `env:"PII_DEK_CACHE_TTL" envDefault:"5m"`
	PIIDEKCacheSize int           `env:"PII_DEK_CACHE_SIZE" envDefault:"10000"`

	// Machine-to-machine access with Ory Hydra (intent 261003-add-ory-hydra).
	// The admin API is unauthenticated: reachable by identity-service only.
	HydraAdminURL string `env:"HYDRA_ADMIN_URL" envDefault:"http://127.0.0.1:4445"`
	M2MJWKSURL    string `env:"M2M_JWKS_URL" envDefault:"http://localhost:4444/.well-known/jwks.json"`
	// M2MIssuer must equal the token iss claim exactly (no normalisation).
	M2MIssuer          string        `env:"M2M_ISSUER" envDefault:"http://localhost:4444"`
	M2MAudience        string        `env:"M2M_AUDIENCE" envDefault:"identity-service"`
	M2MClientCacheTTL  time.Duration `env:"M2M_CLIENT_CACHE_TTL" envDefault:"30s"`
	M2MRateLimitPerMin int           `env:"M2M_RATE_LIMIT_PER_MIN" envDefault:"600"`
	// M2MClientTagKey is a base64 key (≥ 32 bytes) for the HMAC integrity
	// tag on managed Hydra clients (security S4). Secret: never log.
	M2MClientTagKey string `env:"M2M_CLIENT_TAG_KEY"`

	// Pseudonymous login identifiers (ADR-0013).
	PIIOpenBaoLoginHMACKeyName string `env:"PII_OPENBAO_LOGIN_HMAC_KEY_NAME" envDefault:"identity-login-pseudonym"`
	PIIOpenBaoLoginKEKName     string `env:"PII_OPENBAO_LOGIN_KEK_NAME" envDefault:"identity-login-kek"`
	// KratosCourierAPIKey authenticates the Kratos http courier; it must
	// differ from KRATOS_WEBHOOK_API_KEY (A4). Secret: never log.
	KratosCourierAPIKey string `env:"KRATOS_COURIER_API_KEY"`
	// CourierDedupeSecret is a base64 key (>= 32 bytes) for the courier
	// de-duplication hash (A9). Secret: never log.
	CourierDedupeSecret string `env:"COURIER_DEDUPE_SECRET"`
	// LoginMigrationPhase is transition (legacy plaintext customers exist)
	// or complete (A2, DD-15).
	LoginMigrationPhase        string   `env:"LOGIN_MIGRATION_PHASE" envDefault:"transition"`
	LoginPhoneDefaultCountry   string   `env:"LOGIN_PHONE_DEFAULT_COUNTRY" envDefault:"84"`
	LoginPhoneAllowedCountries []string `env:"LOGIN_PHONE_ALLOWED_COUNTRIES" envSeparator:"," envDefault:"84"`
	LoginResolveRate           string   `env:"LOGIN_RESOLVE_RATE" envDefault:"20/1m,200/24h"`
	LoginRegisterRate          string   `env:"LOGIN_REGISTER_RATE" envDefault:"5/1m,30/24h"`
	// LoginResolveNetRate is the aggregate limit per /24 (IPv4) or /48
	// (IPv6) network, all purposes together (SEC-C03).
	LoginResolveNetRate   string `env:"LOGIN_RESOLVE_NET_RATE" envDefault:"300/1m,5000/24h"`
	LoginInsertGlobalRate string `env:"LOGIN_INSERT_GLOBAL_RATE" envDefault:"120/1m"`
	CourierRecipientRate  string `env:"COURIER_RECIPIENT_RATE" envDefault:"5/1h,20/24h"`
	// SMSProvider is sink (local/test: Mailpit), http or disabled.
	SMSProvider    string `env:"SMS_PROVIDER" envDefault:"disabled"`
	SMSDailyBudget int    `env:"SMS_DAILY_BUDGET" envDefault:"1000"`
	// SMSCountryDailyBudget caps SMS per calling code per 24 h (A3).
	SMSCountryDailyBudget int    `env:"SMS_COUNTRY_DAILY_BUDGET" envDefault:"1000"`
	SMSHTTPURL            string `env:"SMS_HTTP_URL"`
	// SMSHTTPToken is the provider bearer token. Secret: never log.
	SMSHTTPToken string `env:"SMS_HTTP_TOKEN"`
}

// SMS providers.
const (
	SMSProviderSink     = "sink"
	SMSProviderHTTP     = "http"
	SMSProviderDisabled = "disabled"
)

// LoginRates are the parsed login and courier rate rules.
type LoginRates struct {
	Resolve, Register, ResolveNet, InsertGlobal, CourierRecipient []app.RateRule
}

// ParseLoginRates parses the LOGIN_*_RATE / COURIER_RECIPIENT_RATE settings.
func (c Config) ParseLoginRates() (LoginRates, error) {
	var r LoginRates
	var errs []error
	for _, x := range []struct {
		name, val string
		dst       *[]app.RateRule
	}{
		{"LOGIN_RESOLVE_RATE", c.LoginResolveRate, &r.Resolve},
		{"LOGIN_REGISTER_RATE", c.LoginRegisterRate, &r.Register},
		{"LOGIN_RESOLVE_NET_RATE", c.LoginResolveNetRate, &r.ResolveNet},
		{"LOGIN_INSERT_GLOBAL_RATE", c.LoginInsertGlobalRate, &r.InsertGlobal},
		{"COURIER_RECIPIENT_RATE", c.CourierRecipientRate, &r.CourierRecipient},
	} {
		rules, err := app.ParseRateRules(x.val)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", x.name, err))
		}
		*x.dst = rules
	}
	return r, errors.Join(errs...)
}

// DedupeKey decodes COURIER_DEDUPE_SECRET (base64, >= 32 bytes).
func (c Config) DedupeKey() ([]byte, error) {
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(c.CourierDedupeSecret); err == nil && len(b) >= 32 {
			return b, nil
		}
	}
	return nil, errors.New("COURIER_DEDUPE_SECRET must be a base64 key of at least 32 bytes")
}

// ValidateLogin checks the login identifier settings (serve).
func (c Config) ValidateLogin() error {
	var errs []error
	if len(c.KratosCourierAPIKey) < 16 {
		errs = append(errs, errors.New("KRATOS_COURIER_API_KEY is required (>= 16 chars)"))
	} else if c.KratosCourierAPIKey == c.KratosWebhookAPIKey {
		errs = append(errs, errors.New("KRATOS_COURIER_API_KEY must differ from KRATOS_WEBHOOK_API_KEY"))
	}
	if _, err := c.DedupeKey(); err != nil {
		errs = append(errs, err)
	}
	if _, ok := app.ParseMigrationPhase(c.LoginMigrationPhase); !ok {
		errs = append(errs, errors.New("LOGIN_MIGRATION_PHASE must be transition or complete"))
	}
	for _, cc := range append([]string{c.LoginPhoneDefaultCountry}, c.LoginPhoneAllowedCountries...) {
		if len(cc) == 0 || len(cc) > 3 || strings.Trim(cc, "0123456789") != "" || cc[0] == '0' {
			errs = append(errs, errors.New("LOGIN_PHONE_* must be calling codes (1-3 digits)"))
			break
		}
	}
	if _, err := c.ParseLoginRates(); err != nil {
		errs = append(errs, err)
	}
	if c.SMSDailyBudget <= 0 || c.SMSCountryDailyBudget <= 0 {
		errs = append(errs, errors.New("SMS_DAILY_BUDGET and SMS_COUNTRY_DAILY_BUDGET must be > 0"))
	}
	switch c.SMSProvider {
	case SMSProviderDisabled:
	case SMSProviderSink:
		if !c.DevEnv() {
			errs = append(errs, errors.New("SMS_PROVIDER=sink is only allowed when APP_ENV is local or test"))
		}
	case SMSProviderHTTP:
		if c.SMSHTTPURL == "" || c.SMSHTTPToken == "" {
			errs = append(errs, errors.New("SMS_HTTP_URL and SMS_HTTP_TOKEN are required for SMS_PROVIDER=http"))
		}
	default:
		errs = append(errs, errors.New("SMS_PROVIDER must be sink, http or disabled"))
	}
	return errors.Join(errs...)
}

// ClientTagKey decodes M2M_CLIENT_TAG_KEY (standard or URL-safe base64,
// padded or not) and requires at least 32 bytes.
func (c Config) ClientTagKey() ([]byte, error) {
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(c.M2MClientTagKey); err == nil && len(b) >= 32 {
			return b, nil
		}
	}
	return nil, errors.New("M2M_CLIENT_TAG_KEY must be a base64 key of at least 32 bytes")
}

// Warnings lists risky but allowed settings, logged at startup.
func (c Config) Warnings() []string {
	var out []string
	if !c.DevEnv() && c.TrustedProxyHops == 0 {
		out = append(out, "TRUSTED_PROXY_HOPS=0 outside local/test: behind a proxy every caller shares the proxy address, "+
			"so per-IP limits (machine-plane failures) apply to all callers together")
	}
	return out
}

// KMS providers.
const (
	KMSProviderOpenBao = "openbao"
	KMSProviderLocal   = "local"
)

// DevEnv reports whether APP_ENV allows development-only providers.
func (c Config) DevEnv() bool { return c.AppEnv == "local" || c.AppEnv == "test" }

// ValidatePII checks the key manager settings (serve and the key CLI).
func (c Config) ValidatePII() error {
	var errs []error
	switch c.AppEnv {
	case "production", "staging", "local", "test":
	default:
		errs = append(errs, errors.New("APP_ENV must be production, staging, local or test"))
	}
	switch c.PIIKMSProvider {
	case KMSProviderOpenBao:
		u, err := url.Parse(c.PIIOpenBaoAddr)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errs = append(errs, errors.New("PII_OPENBAO_ADDR must be an http(s) URL"))
		} else if u.Scheme == "http" && !c.DevEnv() {
			errs = append(errs, errors.New("PII_OPENBAO_ADDR must use https unless APP_ENV is local or test"))
		}
		if c.PIIOpenBaoCAFile != "" {
			if fi, err := os.Stat(c.PIIOpenBaoCAFile); err != nil || fi.IsDir() {
				errs = append(errs, errors.New("PII_OPENBAO_CA_FILE must be a readable PEM file"))
			}
		}
		if c.PIIOpenBaoTokenFile == "" {
			errs = append(errs, errors.New("PII_OPENBAO_TOKEN_FILE is required for PII_KMS_PROVIDER=openbao"))
		}
		names := map[string]bool{c.PIIOpenBaoKEKName: true, c.PIIOpenBaoBidxKeyName: true,
			c.PIIOpenBaoLoginHMACKeyName: true, c.PIIOpenBaoLoginKEKName: true}
		if names[""] || len(names) != 4 {
			errs = append(errs, errors.New("PII_OPENBAO_KEK_NAME, PII_OPENBAO_BIDX_KEY_NAME, PII_OPENBAO_LOGIN_HMAC_KEY_NAME and PII_OPENBAO_LOGIN_KEK_NAME must be set and distinct"))
		}
		if c.PIIOpenBaoTimeout <= 0 {
			errs = append(errs, errors.New("PII_OPENBAO_TIMEOUT must be > 0"))
		}
	case KMSProviderLocal:
		if !c.DevEnv() {
			errs = append(errs, errors.New("PII_KMS_PROVIDER=local is only allowed when APP_ENV is local or test"))
		}
		if !is32Base64(c.PIILocalKEK) || !is32Base64(c.PIILocalBidxKey) {
			errs = append(errs, errors.New("PII_LOCAL_KEK and PII_LOCAL_BIDX_KEY must be base64 32-byte keys"))
		} else if c.PIILocalKEK == c.PIILocalBidxKey {
			errs = append(errs, errors.New("PII_LOCAL_KEK and PII_LOCAL_BIDX_KEY must differ"))
		}
	default:
		errs = append(errs, errors.New("PII_KMS_PROVIDER must be openbao or local"))
	}
	if c.PIIDEKCacheTTL <= 0 || c.PIIDEKCacheTTL > 15*time.Minute {
		errs = append(errs, errors.New("PII_DEK_CACHE_TTL must be in (0, 15m]"))
	}
	if c.PIIDEKCacheSize <= 0 {
		errs = append(errs, errors.New("PII_DEK_CACHE_SIZE must be > 0"))
	}
	return errors.Join(errs...)
}

func is32Base64(s string) bool {
	b, err := base64.StdEncoding.DecodeString(s)
	return err == nil && len(b) == 32
}

// ValidateHydraAdmin checks the Hydra admin URL (serve and the clients CLI).
func (c Config) ValidateHydraAdmin() error {
	return c.httpsURL("HYDRA_ADMIN_URL", c.HydraAdminURL)
}

// ValidateM2M checks the machine-to-machine settings (§6 A11): https for
// the issuer, JWKS and admin URLs unless APP_ENV is local or test.
func (c Config) ValidateM2M() error {
	errs := []error{c.ValidateHydraAdmin(), c.httpsURL("M2M_JWKS_URL", c.M2MJWKSURL), c.httpsURL("M2M_ISSUER", c.M2MIssuer)}
	if u, err := url.Parse(c.M2MIssuer); err == nil && (u.RawQuery != "" || u.Fragment != "" || u.User != nil) {
		errs = append(errs, errors.New("M2M_ISSUER must not carry a query, fragment or user info"))
	}
	if c.M2MAudience == "" {
		errs = append(errs, errors.New("M2M_AUDIENCE is required"))
	}
	if _, err := c.ClientTagKey(); err != nil {
		errs = append(errs, err)
	}
	if c.M2MClientCacheTTL <= 0 || c.M2MClientCacheTTL > 30*time.Second {
		errs = append(errs, errors.New("M2M_CLIENT_CACHE_TTL must be in (0, 30s]"))
	}
	if c.M2MRateLimitPerMin <= 0 {
		errs = append(errs, errors.New("M2M_RATE_LIMIT_PER_MIN must be > 0"))
	}
	return errors.Join(errs...)
}

func (c Config) httpsURL(name, v string) error {
	u, err := url.Parse(v)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("%s must be an http(s) URL", name)
	}
	if u.Scheme == "http" && !c.DevEnv() {
		return fmt.Errorf("%s must use https unless APP_ENV is local or test", name)
	}
	return nil
}

// LoadConfig parses the environment.
func LoadConfig() (Config, error) {
	var c Config
	if err := env.Parse(&c); err != nil {
		return c, fmt.Errorf("config: %w", err)
	}
	return c, nil
}

// ValidateServe checks settings required by `serve`. Startup fails closed
// when a secret is missing.
func (c Config) ValidateServe() error {
	var errs []error
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if len(c.KratosWebhookAPIKey) < 16 {
		errs = append(errs, errors.New("KRATOS_WEBHOOK_API_KEY is required (>= 16 chars)"))
	}
	if c.MigrateOnStart && c.MigrateDatabaseURL == "" {
		errs = append(errs, errors.New("MIGRATE_DATABASE_URL is required when MIGRATE_ON_START=true"))
	}
	if len(c.CORSAllowedOrigins) == 0 {
		errs = append(errs, errors.New("CORS_ALLOWED_ORIGINS is required (admin web origin)"))
	}
	for _, o := range c.CORSAllowedOrigins {
		if o == "*" || o == "" {
			errs = append(errs, errors.New("CORS_ALLOWED_ORIGINS must list explicit origins"))
		}
	}
	if c.AdminSessionMaxAge <= 0 || c.AdminSessionMaxAge > 12*time.Hour {
		errs = append(errs, errors.New("ADMIN_SESSION_MAX_AGE must be in (0, 12h]"))
	}
	if c.AdminMFAGrace <= 0 || c.AdminMFAGrace > 24*time.Hour {
		errs = append(errs, errors.New("ADMIN_MFA_ENROLLMENT_GRACE must be in (0, 24h]"))
	}
	if c.ShutdownGracePeriod <= 0 {
		errs = append(errs, errors.New("SHUTDOWN_GRACE_PERIOD must be > 0"))
	}
	if c.TrustedProxyHops < 0 {
		errs = append(errs, errors.New("TRUSTED_PROXY_HOPS must be >= 0"))
	}
	if c.MFASweepInterval <= 0 {
		errs = append(errs, errors.New("ADMIN_MFA_SWEEP_INTERVAL must be > 0"))
	}
	if c.InvitationTTL <= 0 || c.InvitationTTL > 24*time.Hour {
		errs = append(errs, errors.New("ADMIN_INVITATION_TTL must be in (0, 24h]"))
	}
	if err := c.ValidatePII(); err != nil {
		errs = append(errs, err)
	}
	if err := c.ValidateM2M(); err != nil {
		errs = append(errs, err)
	}
	if err := c.ValidateLogin(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
