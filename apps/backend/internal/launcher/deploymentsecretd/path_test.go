package deploymentsecretd

import (
	"errors"
	"strings"
	"testing"
)

// TestValidateSocketPathAcceptsExactlyThePlatformLimit pins the boundary: Linux
// binds sun_path, which is 108 bytes including the terminator, so a path of
// exactly UnixSocketPathLimit bytes is bindable and one byte more is not.
func TestValidateSocketPathAcceptsExactlyThePlatformLimit(t *testing.T) {
	limit := "a" + strings.Repeat("b", UnixSocketPathLimit-1)
	if err := ValidateSocketPath(limit); err != nil {
		t.Fatalf("path of %d bytes rejected: %v", len(limit), err)
	}
	tooLong := limit + "b"
	err := ValidateSocketPath(tooLong)
	if err == nil {
		t.Fatalf("path of %d bytes accepted", len(tooLong))
	}
	if !errors.Is(err, ErrSocketPathTooLong) {
		t.Fatalf("error = %v, want ErrSocketPathTooLong", err)
	}
	if err := ValidateSocketPath(SocketPath); err != nil {
		t.Fatalf("the default socket path was rejected: %v", err)
	}
}

// TestValidateSocketPathNamesThePathTheLimitAndTheRemedy pins the operator-facing
// text: a failure that does not say which path was too long, how long it is, or
// what to do about it leaves the operator with a raw bind error.
func TestValidateSocketPathNamesThePathTheLimitAndTheRemedy(t *testing.T) {
	path := "/srv/" + strings.Repeat("deep-", 30) + "deployment-secret.sock"
	err := ValidateSocketPath(path)
	if err == nil {
		t.Fatal("an over-long path was accepted")
	}
	message := err.Error()
	for _, want := range []string{path, "bytes", "--socket"} {
		if !strings.Contains(message, want) {
			t.Fatalf("message %q does not name %q", message, want)
		}
	}
	if !strings.Contains(message, "107") {
		t.Fatalf("message %q does not name the platform limit", message)
	}
}
