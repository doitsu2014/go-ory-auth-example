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
