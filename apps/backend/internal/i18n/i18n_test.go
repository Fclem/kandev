package i18n

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ in, want string }{
		{"en", "en"},
		{"pseudo", "pseudo"},
		{"", "en"},
		{"fr", "en"},
		{"EN", "en"},
	} {
		if got := Normalize(tc.in); got != tc.want {
			t.Fatalf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTranslatesAndFallsBack(t *testing.T) {
	t.Parallel()
	en := T("en", "webapp.shellUnavailable")
	if en == "webapp.shellUnavailable" || en == "" {
		t.Fatalf("en catalog did not resolve the key, got %q", en)
	}
	// The pseudo catalog must differ from en; that is what makes it usable as the
	// externalization oracle.
	if pseudo := T("pseudo", "webapp.shellUnavailable"); pseudo == en {
		t.Fatalf("pseudo message should differ from en, both %q", en)
	}
	// An unknown locale falls back to en rather than erroring or blanking.
	if got := T("klingon", "webapp.shellUnavailable"); got != en {
		t.Fatalf("unknown locale = %q, want en fallback %q", got, en)
	}
	// A missing key degrades to the key itself, never empty.
	if got := T("en", "does.not.exist"); got != "does.not.exist" {
		t.Fatalf("missing key = %q, want the key echoed back", got)
	}
}

func TestCatalogsHaveMatchingKeys(t *testing.T) {
	t.Parallel()
	// Every en key must exist in pseudo, otherwise the QA locale silently shows
	// English and stops proving that a string was externalized.
	for _, key := range Keys() {
		if T("pseudo", key) == key {
			t.Fatalf("pseudo catalog is missing key %q", key)
		}
	}
}

func TestFromRequest(t *testing.T) {
	t.Parallel()

	t.Run("cookie wins", func(t *testing.T) {
		t.Parallel()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.AddCookie(&http.Cookie{Name: LocaleCookie, Value: "pseudo"})
		r.Header.Set("Accept-Language", "en")
		if got := FromRequest(r); got != "pseudo" {
			t.Fatalf("got %q, want pseudo", got)
		}
	})

	t.Run("invalid cookie ignored", func(t *testing.T) {
		t.Parallel()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.AddCookie(&http.Cookie{Name: LocaleCookie, Value: "klingon"})
		if got := FromRequest(r); got != DefaultLocale {
			t.Fatalf("got %q, want %q", got, DefaultLocale)
		}
	})

	t.Run("accept-language honored by q-value", func(t *testing.T) {
		t.Parallel()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Accept-Language", "fr;q=0.9, pseudo;q=1.0")
		if got := FromRequest(r); got != "pseudo" {
			t.Fatalf("got %q, want pseudo", got)
		}
	})

	t.Run("region subtag matches base locale", func(t *testing.T) {
		t.Parallel()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Accept-Language", "en-GB")
		if got := FromRequest(r); got != "en" {
			t.Fatalf("got %q, want en", got)
		}
	})

	t.Run("nil request is safe", func(t *testing.T) {
		t.Parallel()
		if got := FromRequest(nil); got != DefaultLocale {
			t.Fatalf("got %q, want %q", got, DefaultLocale)
		}
	})
}

func TestTf_SubstitutesPlaceholders(t *testing.T) {
	got := Tf(DefaultLocale, "share.pageTitleWithTask", map[string]string{"title": "Fix login"})
	if want := "Fix login — kandev share"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestTf_LeavesUnknownPlaceholderVerbatim(t *testing.T) {
	// Blanking it would silently drop words; leaving it makes the bug visible.
	if got := Tf(DefaultLocale, "share.pageTitleWithTask", map[string]string{"other": "x"}); !strings.Contains(got, "{{title}}") {
		t.Fatalf("placeholder should survive, got %q", got)
	}
}

func TestTf_WithoutVarsMatchesT(t *testing.T) {
	if Tf(DefaultLocale, "share.brandTag", nil) != T(DefaultLocale, "share.brandTag") {
		t.Fatal("Tf with no vars should equal T")
	}
}

func TestContextLocaleRoundTrip(t *testing.T) {
	ctx := ContextWithLocale(context.Background(), "pseudo")
	if got := FromContext(ctx); got != "pseudo" {
		t.Fatalf("got %q, want pseudo", got)
	}
}

func TestFromContext_DefaultsWithoutLocale(t *testing.T) {
	// Pollers and schedulers never pass through the HTTP layer.
	if got := FromContext(context.Background()); got != DefaultLocale {
		t.Fatalf("got %q, want %q", got, DefaultLocale)
	}
	//nolint:staticcheck // explicitly covering the nil-context path
	if got := FromContext(nil); got != DefaultLocale {
		t.Fatalf("nil context: got %q, want %q", got, DefaultLocale)
	}
}

func TestContextWithLocale_CoercesUnsupported(t *testing.T) {
	ctx := ContextWithLocale(context.Background(), "kl-KL")
	if got := FromContext(ctx); got != DefaultLocale {
		t.Fatalf("got %q, want %q", got, DefaultLocale)
	}
}
