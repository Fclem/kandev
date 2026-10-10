package streams

import (
	"strings"
	"testing"
)

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.6
func TestProviderMessageRequestID(t *testing.T) {
	text := SanitizeProviderMessage("request_id=req_short retry-after-ms=274579000")
	if strings.Contains(text, "req_short") || !strings.Contains(text, "274579000") {
		t.Fatalf("unsafe or incomplete projection: %s", text)
	}
}
