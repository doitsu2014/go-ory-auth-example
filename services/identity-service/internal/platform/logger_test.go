package platform

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// TestT12_LogRedaction asserts secrets never reach the log output (06-security T12).
func TestT12_LogRedaction(t *testing.T) {
	var buf bytes.Buffer
	log := NewLogger(&buf, "debug")
	log = log.With("Authorization", "Bearer ory_st_secret1")
	log.Info("request",
		"cookie", "ory_kratos_session=secret2",
		"X-Session-Token", "secret3",
		"password", "secret4",
		"code", "secret5",
		"recovery_link", "http://x/recovery?secret6",
		"email", "secret7@example.com",
		slog.Group("headers", "authorization", "secret8", "user_agent", "curl"),
		"identity_id", "5d9c2c61-6a1e-4b8f-9b8a-2f9d6f0c1e11",
	)
	out := buf.String()
	for i := 1; i <= 8; i++ {
		if strings.Contains(out, "secret"+string(rune('0'+i))) {
			t.Fatalf("secret%d leaked: %s", i, out)
		}
	}
	if !strings.Contains(out, "5d9c2c61-6a1e-4b8f-9b8a-2f9d6f0c1e11") || !strings.Contains(out, "curl") {
		t.Fatalf("non-sensitive attrs must be kept: %s", out)
	}
	if strings.Count(out, Redacted) != 8 {
		t.Fatalf("want 8 redactions: %s", out)
	}
}
