package websocket

import (
	"os"
	"strings"
	"testing"
)

// TestDumpShimProbe writes the REAL runtime shim (with the probe export
// injected) to /tmp/shim_probe.js so the node validator can exercise the
// actual functions r/rn/sc/norm/hcp/rwaStyle/srcsetParts/mref exactly as
// shipped, including the observer-loop idempotency behavior.
func TestDumpShimProbe(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "cap.ABC-_123")
	idx := strings.LastIndex(shim, "})();")
	if idx < 0 {
		t.Fatal("shim template does not end with the IIFE close")
	}
	probe := shim[:idx] + `window.__kandev__={r:r,rn:rn,sc:sc,norm:norm,rwaStyle:rwaStyle,srcsetParts:srcsetParts,mref:mref};` + shim[idx:]
	if err := os.WriteFile("/tmp/shim_probe.js", []byte(probe), 0o644); err != nil {
		t.Fatalf("write probe: %v", err)
	}
}
