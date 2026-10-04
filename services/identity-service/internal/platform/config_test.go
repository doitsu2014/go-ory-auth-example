package platform

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func validConfig() Config {
	return Config{
		DatabaseURL: "postgres://x", KratosWebhookAPIKey: strings.Repeat("k", 32),
		CORSAllowedOrigins: []string{"http://localhost:5173"}, AdminSessionMaxAge: 12 * time.Hour,
		AdminMFAGrace: 24 * time.Hour, InvitationTTL: 24 * time.Hour, ShutdownGracePeriod: 15 * time.Second,
		MFASweepInterval: 5 * time.Minute,
		AppEnv:           "production", PIIKMSProvider: KMSProviderOpenBao, PIIOpenBaoAddr: "https://bao.internal:8200",
		PIIOpenBaoTokenFile: "/run/openbao/token", PIIOpenBaoKEKName: "identity-pii-kek", PIIOpenBaoBidxKeyName: "identity-pii-bidx",
		PIIOpenBaoTimeout: 2 * time.Second, PIIDEKCacheTTL: 5 * time.Minute, PIIDEKCacheSize: 10000,
		HydraAdminURL: "https://hydra-admin.internal:4445", M2MJWKSURL: "https://auth.example.com/.well-known/jwks.json",
		M2MIssuer: "https://auth.example.com", M2MAudience: "identity-service", M2MClientCacheTTL: 30 * time.Second,
		M2MRateLimitPerMin: 600, M2MClientTagKey: key32("t"),
		PIIOpenBaoLoginHMACKeyName: "identity-login-pseudonym", PIIOpenBaoLoginKEKName: "identity-login-kek",
		KratosCourierAPIKey: strings.Repeat("c", 32), CourierDedupeSecret: key32("d"), LoginMigrationPhase: "transition",
		LoginPhoneDefaultCountry: "84", LoginPhoneAllowedCountries: []string{"84"},
		LoginSignInRate: "20/1m,200/24h", LoginAccountRate: "10/15m,50/24h", LoginRegisterRate: "5/1m,30/24h", LoginInsertGlobalRate: "120/1m",
		CourierRecipientRate: "5/1h,20/24h", SMSProvider: SMSProviderDisabled, SMSDailyBudget: 1000,
		SMSCountryDailyBudget: 1000, LoginNetRate: "300/1m,5000/24h",
	}
}

func TestPLIValidateLogin(t *testing.T) {
	for name, mut := range map[string]func(*Config){
		"courier key = webhook key": func(c *Config) { c.KratosCourierAPIKey = c.KratosWebhookAPIKey },
		"short dedupe secret":       func(c *Config) { c.CourierDedupeSecret = "c2hvcnQ=" },
		"bad phase":                 func(c *Config) { c.LoginMigrationPhase = "done" },
		"bad country":               func(c *Config) { c.LoginPhoneAllowedCountries = []string{"+84"} },
		"bad rate":                  func(c *Config) { c.LoginSignInRate = "20/forever" },
		"sms sink in production":    func(c *Config) { c.SMSProvider = SMSProviderSink },
		"sms http without url":      func(c *Config) { c.SMSProvider = SMSProviderHTTP },
		"login key = pii key":       func(c *Config) { c.PIIOpenBaoLoginKEKName = c.PIIOpenBaoKEKName },
	} {
		c := validConfig()
		mut(&c)
		if err := c.ValidateServe(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	c := validConfig()
	c.AppEnv, c.PIIOpenBaoAddr, c.SMSProvider = "local", "http://openbao:8200", SMSProviderSink
	if err := c.ValidateServe(); err != nil {
		t.Fatalf("sink locally: %v", err)
	}
}

func TestValidateServe(t *testing.T) {
	if err := validConfig().ValidateServe(); err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*Config){
		"no cors":           func(c *Config) { c.CORSAllowedOrigins = nil },
		"wildcard cors":     func(c *Config) { c.CORSAllowedOrigins = []string{"*"} },
		"admin max age 13h": func(c *Config) { c.AdminSessionMaxAge = 13 * time.Hour },
		"shutdown 0":        func(c *Config) { c.ShutdownGracePeriod = 0 },
		"short webhook key": func(c *Config) { c.KratosWebhookAPIKey = "k" },
		"no database":       func(c *Config) { c.DatabaseURL = "" },
		"migrate w/o dsn":   func(c *Config) { c.MigrateOnStart = true },
		"invite ttl 25h":    func(c *Config) { c.InvitationTTL = 25 * time.Hour },
		"negative hops":     func(c *Config) { c.TrustedProxyHops = -1 },
		// §9 A15 provider guards.
		"http openbao in production": func(c *Config) { c.PIIOpenBaoAddr = "http://openbao:8200" },
		"http openbao in staging":    func(c *Config) { c.AppEnv, c.PIIOpenBaoAddr = "staging", "http://openbao:8200" },
		"missing ca file":            func(c *Config) { c.PIIOpenBaoCAFile = "/nonexistent/ca.pem" },
		"no token file":              func(c *Config) { c.PIIOpenBaoTokenFile = "" },
		"unknown provider":           func(c *Config) { c.PIIKMSProvider = "aws" },
		"unknown app env":            func(c *Config) { c.AppEnv = "prod" },
		"local kms in production": func(c *Config) {
			c.PIIKMSProvider, c.PIILocalKEK, c.PIILocalBidxKey = KMSProviderLocal, key32("a"), key32("b")
		},
		"local kms bad key": func(c *Config) {
			c.AppEnv, c.PIIKMSProvider, c.PIILocalKEK, c.PIILocalBidxKey = "local", KMSProviderLocal, "c2hvcnQ=", key32("b")
		},
		"same kek and bidx name": func(c *Config) { c.PIIOpenBaoBidxKeyName = c.PIIOpenBaoKEKName },
		"cache ttl 1h":           func(c *Config) { c.PIIDEKCacheTTL = time.Hour },
		// §6 A11 machine-to-machine guards.
		"http hydra admin":  func(c *Config) { c.HydraAdminURL = "http://hydra:4445" },
		"http jwks":         func(c *Config) { c.M2MJWKSURL = "http://hydra:4444/.well-known/jwks.json" },
		"http issuer":       func(c *Config) { c.M2MIssuer = "http://localhost:4444" },
		"http jwks staging": func(c *Config) { c.AppEnv, c.M2MJWKSURL = "staging", "http://hydra:4444/.well-known/jwks.json" },
		"issuer not a url":  func(c *Config) { c.M2MIssuer = "localhost:4444" },
		"issuer with query": func(c *Config) { c.M2MIssuer = "https://auth.example.com?x=1" },
		"no audience":       func(c *Config) { c.M2MAudience = "" },
		"client cache 31s":  func(c *Config) { c.M2MClientCacheTTL = 31 * time.Second },
		"client cache 0":    func(c *Config) { c.M2MClientCacheTTL = 0 },
		"m2m rate limit 0":  func(c *Config) { c.M2MRateLimitPerMin = 0 },
		"admin url ftp":     func(c *Config) { c.HydraAdminURL = "ftp://hydra:4445" },
		"no client tag key": func(c *Config) { c.M2MClientTagKey = "" },
		"short client tag key": func(c *Config) {
			c.M2MClientTagKey = base64.StdEncoding.EncodeToString([]byte(strings.Repeat("t", 31)))
		},
		"client tag key not base64": func(c *Config) { c.M2MClientTagKey = strings.Repeat("!", 64) },
	}
	for name, mutate := range tests {
		c := validConfig()
		mutate(&c)
		if err := c.ValidateServe(); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func key32(fill string) string {
	return base64.StdEncoding.EncodeToString([]byte(strings.Repeat(fill, 32)))
}

func TestValidatePIIAllowsDevProviders(t *testing.T) {
	c := validConfig()
	c.AppEnv, c.PIIOpenBaoAddr = "local", "http://openbao:8200"
	if err := c.ValidateServe(); err != nil {
		t.Fatalf("http openbao is fine locally: %v", err)
	}
	for _, env := range []string{"local", "test"} {
		c := validConfig()
		c.AppEnv, c.PIIKMSProvider, c.PIILocalKEK, c.PIILocalBidxKey = env, KMSProviderLocal, key32("a"), key32("b")
		if err := c.ValidateServe(); err != nil {
			t.Fatalf("local kms in %s: %v", env, err)
		}
	}
}

func TestValidateM2MAllowsHTTPLocally(t *testing.T) {
	for _, env := range []string{"local", "test"} {
		c := validConfig()
		c.AppEnv, c.HydraAdminURL, c.M2MJWKSURL, c.M2MIssuer = env, "http://hydra:4445", "http://hydra:4444/.well-known/jwks.json", "http://localhost:4444"
		if err := c.ValidateServe(); err != nil {
			t.Fatalf("http hydra in %s: %v", env, err)
		}
	}
}

func TestClientTagKeyEncodings(t *testing.T) {
	raw := []byte(strings.Repeat("k", 32))
	for _, v := range []string{base64.StdEncoding.EncodeToString(raw), base64.RawURLEncoding.EncodeToString(raw)} {
		c := validConfig()
		c.M2MClientTagKey = v
		if b, err := c.ClientTagKey(); err != nil || string(b) != string(raw) {
			t.Fatalf("%q: %v", v, err)
		}
	}
}

func TestWarningsTrustedProxyHops(t *testing.T) {
	c := validConfig() // production, hops 0
	if w := c.Warnings(); len(w) != 1 || !strings.Contains(w[0], "TRUSTED_PROXY_HOPS") {
		t.Fatalf("production with 0 hops must warn: %v", w)
	}
	c.TrustedProxyHops = 1
	if w := c.Warnings(); len(w) != 0 {
		t.Fatalf("hops set: %v", w)
	}
	c.TrustedProxyHops, c.AppEnv = 0, "local"
	if w := c.Warnings(); len(w) != 0 {
		t.Fatalf("local: %v", w)
	}
}
