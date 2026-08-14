package websocket

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

const proxyPrefix = "/port-proxy/abc/3001"

func TestRewriteAbsolutePath(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"path-absolute", "/foo/bar.js", proxyPrefix + "/foo/bar.js"},
		{"root", "/", proxyPrefix + "/"},
		{"network-relative skipped", "//cdn.example.com/x", "//cdn.example.com/x"},
		{"absolute http skipped", "http://example.com/x", "http://example.com/x"},
		{"relative skipped", "foo.js", "foo.js"},
		{"dot-relative skipped", "./foo.js", "./foo.js"},
		{"parent-relative skipped", "../foo.js", "../foo.js"},
		{"data-uri skipped", "data:image/png;base64,xyz", "data:image/png;base64,xyz"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := rewriteAbsolutePath(c.in, proxyPrefix, "")
			if got != c.want {
				t.Fatalf("rewriteAbsolutePath(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestRewriteHTMLURLs(t *testing.T) {
	in := `<!DOCTYPE html>
<html>
<head>
<link rel="stylesheet" href="/styles/main.css">
<link rel="stylesheet" href="//cdn.example.com/lib.css">
<script src="/static/app.js"></script>
<style>body { background: url(/img/bg.png); }</style>
</head>
<body>
<img src="/img/logo.png" srcset="/img/logo@2x.png 2x, https://cdn.example.com/logo.png 3x">
<a href="/about">About</a>
<a href="http://external.example.com/x">External</a>
<form action="/submit"><input formaction="/quick"></form>
<div style="background: url('/bg.jpg');"></div>
</body>
</html>`

	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "", ""))

	mustContain(t, got, `href="/port-proxy/abc/3001/styles/main.css"`)
	mustContain(t, got, `href="//cdn.example.com/lib.css"`)
	mustContain(t, got, `src="/port-proxy/abc/3001/static/app.js"`)
	mustContain(t, got, `src="/port-proxy/abc/3001/img/logo.png"`)
	mustContain(t, got, `/port-proxy/abc/3001/img/logo@2x.png 2x`)
	mustContain(t, got, `https://cdn.example.com/logo.png 3x`)
	mustContain(t, got, `href="/port-proxy/abc/3001/about"`)
	mustContain(t, got, `href="http://external.example.com/x"`)
	mustContain(t, got, `action="/port-proxy/abc/3001/submit"`)
	mustContain(t, got, `formaction="/port-proxy/abc/3001/quick"`)
	// Inline style="url('/bg.jpg')" — html package HTML-escapes single quotes
	// on serialization, so check for the rewritten path without the quote.
	mustContain(t, got, `/port-proxy/abc/3001/bg.jpg`)
	// Inline <style> block should be rewritten via rewriteCSSFragment.
	mustContain(t, got, `url(/port-proxy/abc/3001/img/bg.png)`)
}

func TestRewriteCSSURLs(t *testing.T) {
	in := `@import "/theme.css";
@import url("/print.css");
.bg { background: url(/img/bg.png) no-repeat; }
.cdn { background: url("//cdn.example.com/x.png"); }
.abs { background: url(http://example.com/x.png); }
.rel { background: url(foo.png); }`

	got := string(rewriteCSSURLs([]byte(in), proxyPrefix, "", ""))

	mustContain(t, got, `@import "/port-proxy/abc/3001/theme.css"`)
	mustContain(t, got, `url("/port-proxy/abc/3001/print.css")`)
	mustContain(t, got, `url(/port-proxy/abc/3001/img/bg.png)`)
	mustContain(t, got, `url("//cdn.example.com/x.png")`)
	mustContain(t, got, `url(http://example.com/x.png)`)
	mustContain(t, got, `url(foo.png)`)
}

func TestRewriteProxyResponse_HTML(t *testing.T) {
	body := `<a href="/x">x</a>`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	if err := rewriteProxyResponse(resp, proxyPrefix, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(got), `href="/port-proxy/abc/3001/x"`) {
		t.Fatalf("HTML not rewritten: %q", got)
	}
	if resp.ContentLength != int64(len(got)) {
		t.Fatalf("ContentLength mismatch: %d vs %d", resp.ContentLength, len(got))
	}
}

func TestRewriteProxyResponse_CSS(t *testing.T) {
	body := `body { background: url(/bg.png); }`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/css"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	if err := rewriteProxyResponse(resp, proxyPrefix, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(got), `url(/port-proxy/abc/3001/bg.png)`) {
		t.Fatalf("CSS not rewritten: %q", got)
	}
}

func TestRewriteProxyResponse_OtherContentTypeUnchanged(t *testing.T) {
	body := `{"href":"/foo"}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	if err := rewriteProxyResponse(resp, proxyPrefix, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := io.ReadAll(resp.Body)
	if string(got) != body {
		t.Fatalf("non-HTML/CSS response was modified: %q", got)
	}
}

func TestRewriteHTMLURLs_InjectsRuntimeShim(t *testing.T) {
	in := `<!DOCTYPE html><html><head><title>x</title></head><body></body></html>`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "", ""))

	// The shim script tag must appear exactly once, immediately after the
	// `<head>` open tag (so it executes before any other script that may
	// follow), and load from the proxy's reserved same-origin path.
	const marker = `<script src="/port-proxy/abc/3001/__kandev_runtime_shim.js"></script>`
	if strings.Count(got, marker) != 1 {
		t.Fatalf("expected exactly one runtime shim tag, got %d copies\n%s",
			strings.Count(got, marker), got)
	}
	headIdx := strings.Index(got, "<head>")
	titleIdx := strings.Index(got, "<title>")
	shimIdx := strings.Index(got, marker)
	if headIdx >= shimIdx || shimIdx >= titleIdx {
		t.Fatalf("shim must come between <head> and <title>: head=%d shim=%d title=%d\n%s",
			headIdx, shimIdx, titleIdx, got)
	}
	// With a capability minted, the shim src carries it so the shim loads even
	// in cookie-less contexts.
	withCap := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-shim", ""))
	if !strings.Contains(withCap, `__kandev_runtime_shim.js?kandev_cap=cap-shim`) {
		t.Fatalf("capability-bearing shim src missing the capability:\n%s", withCap)
	}
	// The prefix must be baked into the served shim body.
	shim := runtimeShim(proxyPrefix, "")
	if !strings.Contains(shim, `var P="/port-proxy/abc/3001";`) {
		t.Fatalf("shim body missing the baked-in prefix:\n%s", shim)
	}
}

func TestRewriteHTMLURLs_NoHeadStillRewritesURLs(t *testing.T) {
	// Documents without a <head> are rare but possible. We don't bother
	// injecting the shim in that case (no good anchor point), but URL
	// rewriting must still work.
	in := `<a href="/foo">x</a>`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "", ""))
	if !strings.Contains(got, `href="/port-proxy/abc/3001/foo"`) {
		t.Fatalf("URL not rewritten: %q", got)
	}
	if strings.Contains(got, "window.fetch=") {
		t.Fatalf("unexpected shim in headless document: %q", got)
	}
}

func TestRewriteHTMLURLs_PreservesScriptContentVerbatim(t *testing.T) {
	// Inline scripts must not be HTML-escaped — `&`, `<`, `>` are valid JS
	// tokens (bitwise/logical operators, comparisons, JSON characters in
	// embedded payloads, etc.) and escaping them corrupts the JS.
	in := `<!DOCTYPE html><html><head></head><body>` +
		`<script>var a = 1 & 2; var b = a && true; var c = "<x>"; var d = {"k":"&"};</script>` +
		`<script src="/static/app.js"></script>` +
		`</body></html>`

	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "", ""))

	// Inline script body must come through unescaped.
	for _, needle := range []string{
		`var a = 1 & 2;`,
		`var b = a && true;`,
		`var c = "<x>";`,
		`var d = {"k":"&"};`,
	} {
		mustContain(t, got, needle)
	}

	// External script `src` is still rewritten.
	mustContain(t, got, `src="/port-proxy/abc/3001/static/app.js"`)

	// Sanity: none of the inline-script characters got HTML-escaped.
	for _, forbidden := range []string{`&amp;`, `&lt;x&gt;`} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("script body was HTML-escaped (%q present):\n%s", forbidden, got)
		}
	}
}

func TestRuntimeShim_InstallsMutationObserver(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "")

	// MutationObserver is installed so dynamically-inserted DOM nodes (e.g.
	// Next.js `ReactDOM.preload()` for fonts) get their URL attributes
	// rewritten too, not just whatever was in the initial HTML.
	mustContain(t, shim, `new MO(function(rs)`)
	mustContain(t, shim, `.observe(document.documentElement,{childList:true,subtree:true,attributes:true,attributeFilter:ATTRS})`)

	// The attribute list mirrors the static HTML rewriter's coverage so the
	// runtime path doesn't silently miss attributes the static path catches.
	mustContain(t, shim, `'href','src','action','formaction','cite','data','poster','background','manifest','srcset'`)

	// srcset has its own splitter (whitespace-separated descriptors).
	mustContain(t, shim, `if(a==='srcset')`)
}

func TestRuntimeShim_ExposesProxyPrefixToInspector(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "")

	// The inspector script uses this to report app-local routes in annotation
	// prompts instead of the gateway's /port-proxy/... path.
	mustContain(t, shim, `window.__kandevProxyPrefix=P;`)
}

func TestRuntimeShim_ForwardsConsoleToParent(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "")

	// Console levels are intercepted so iframe diagnostics surface in the
	// parent window's console alongside other preview events.
	mustContain(t, shim, `var LV=['log','warn','error','info','debug'];`)
	mustContain(t, shim, `window.parent.postMessage({source:'kandev-inspector',type:'console',payload:{level:lv,args:out}}`)

	// Original method is still invoked so the iframe's own DevTools shows
	// the same output.
	mustContain(t, shim, `return orig.apply(console,arguments)`)
}

func TestRuntimeShim_PatchesNavigationAPIs(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "")

	// Patches history.pushState and history.replaceState so SPA routers keep
	// the proxy prefix in the URL bar on client-side navigation.
	mustContain(t, shim, `'pushState','replaceState'`)
	mustContain(t, shim, `history[op]=function(s,t,u)`)

	// Patches location.assign and location.replace so imperative navigation
	// goes through the same rewriter.
	mustContain(t, shim, `'assign','replace'`)
	mustContain(t, shim, `location[op]=function(u)`)

	// Both patches must reuse the prefix-only navigation rewriter rn() rather
	// than rolling their own prefix logic — and must NOT carry the capability
	// (the subtree cookie covers same-origin navigations; a bearer in the
	// address bar/history would leak).
	for _, needle := range []string{
		`u=rn(u);return orig.call(this,s,t,u)`, // history APIs
		`u=rn(u);return orig.call(location,u)`, // location APIs
	} {
		mustContain(t, shim, needle)
	}
}

func TestRewriteSrcSet(t *testing.T) {
	in := "/a.png 1x, /b.png 2x, //cdn.example.com/c.png 3x"
	got := rewriteSrcSet(in, proxyPrefix, "")
	want := "/port-proxy/abc/3001/a.png 1x, /port-proxy/abc/3001/b.png 2x, //cdn.example.com/c.png 3x"
	if got != want {
		t.Fatalf("rewriteSrcSet = %q, want %q", got, want)
	}
}

// The subtree capability must ride on every rewritten SUBRESOURCE URL: plain
// URLs get it as the first query parameter, URLs that already carry a query
// get it appended. Network-relative and absolute URLs stay untouched, and
// navigation references (anchor href) get the prefix without the capability.
func TestRewriteHTMLURLs_AppendsCapabilityToRewrittenURLs(t *testing.T) {
	in := `<a href="/foo">x</a>` +
		`<script type="module" src="/src/main.tsx?t=123"></script>` +
		`<img srcset="/a.png 1x, //cdn.example.com/b.png 2x">` +
		`<link rel="manifest" href="/manifest.webmanifest">` +
		`<a href="/docs#installation">docs</a>`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-123", ""))

	// Navigation hrefs are prefixed but capability-free.
	mustContain(t, got, `href="/port-proxy/abc/3001/foo"`)
	// The tokenizer HTML-escapes the separator inside the attribute value.
	mustContain(t, got, `src="/port-proxy/abc/3001/src/main.tsx?t=123&amp;kandev_cap=cap-123"`)
	mustContain(t, got, `href="/port-proxy/abc/3001/manifest.webmanifest?kandev_cap=cap-123"`)
	mustContain(t, got, `srcset="/port-proxy/abc/3001/a.png?kandev_cap=cap-123 1x, //cdn.example.com/b.png 2x"`)
	// A fragment must stay after the capability query on subresources.
	mustContain(t, got, `href="/port-proxy/abc/3001/docs#installation"`)
}

// The capability is appended to rewritten CSS url() and @import references too,
// so cookie-less CSS loads stay authorized in the same contexts as the
// manifest fetch. Relative references get the capability appended as well;
// scheme-bearing and network-relative references stay untouched.
func TestRewriteCSSURLs_AppendsCapability(t *testing.T) {
	in := `@import "/theme.css"; .x { background: url("/img/bg.png"); }` +
		`.y { background: url(rel.png); } @import "print.css";` +
		`.z { background: url(http://cdn.example.com/x.png); }`
	got := string(rewriteCSSURLs([]byte(in), proxyPrefix, "cap-456", ""))

	mustContain(t, got, `@import "/port-proxy/abc/3001/theme.css?kandev_cap=cap-456"`)
	mustContain(t, got, `url("/port-proxy/abc/3001/img/bg.png?kandev_cap=cap-456")`)
	mustContain(t, got, `url(rel.png?kandev_cap=cap-456)`)
	mustContain(t, got, `@import "print.css?kandev_cap=cap-456"`)
	mustContain(t, got, `url(http://cdn.example.com/x.png)`)
}

// hasURLScheme follows RFC 3986 scheme grammar: a scheme starts with an ASCII
// letter and continues with letters, digits, +, -, or . until ":". A colon in
// a non-scheme position (digit first, or after a path/query delimiter) means
// the reference is relative.
func TestHasURLScheme(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"http://x", true},
		{"https://x", true},
		{"data:image/png;base64,AAA", true},
		{"javascript:alert(1)", true},
		{"mailto:x@y.dev", true},
		{"v1:chunk.js", true}, // RFC-valid scheme
		{"C:foo", true},
		{"foo", false},
		{"./foo", false},
		{"../foo", false},
		{"foo/bar:baz", false}, // colon after a path delimiter
		{"1x:y", false},        // scheme must start with a letter
		{"?a=b", false},
		{"#frag", false},
	}
	for _, tc := range cases {
		if got := hasURLScheme(tc.raw); got != tc.want {
			t.Errorf("hasURLScheme(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}

// stripCapabilityParam removes only the capability pair from a raw query,
// matching percent-encoded spellings and preserving every other parameter's
// bytes and order.
func TestStripCapabilityParam(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"absent", "a=1&b=2", "a=1&b=2"},
		{"plain", "a=1&kandev_cap=x&b=2", "a=1&b=2"},
		{"first", "kandev_cap=x&a=1", "a=1"},
		{"last", "a=1&kandev_cap=x", "a=1"},
		{"encoded key", "a=1&%6bandev_cap=x", "a=1"},
		{"encoded key full", "a=1&%6B%61%6E%64%65%76%5F%63%61%70=x", "a=1"},
		{"no value", "kandev_cap", ""},
		{"preserves order and bytes", "b=2&a=%2F&kandev_cap=x&c=3", "b=2&a=%2F&c=3"},
		{"value contains cap name", "a=kandev_cap=x", "a=kandev_cap=x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripCapabilityParam(tc.in); got != tc.want {
				t.Fatalf("stripCapabilityParam(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// stripReservedProxyParams additionally removes the ?token= PAT credential the
// gateway consumes, in any encoding, while preserving everything else.
func TestStripReservedProxyParams(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"absent", "a=1&b=2", "a=1&b=2"},
		{"cap", "a=1&kandev_cap=x", "a=1"},
		{"token", "a=1&token=pat-secret&b=2", "a=1&b=2"},
		{"encoded token", "a=1&%74oken=pat&b=2", "a=1&b=2"},
		{"both", "token=pat&a=1&kandev_cap=x", "a=1"},
		{"value contains names", "a=kandev_cap=x&b=token=y", "a=kandev_cap=x&b=token=y"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripReservedProxyParams(tc.in); got != tc.want {
				t.Fatalf("stripReservedProxyParams(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// Same-origin relative references (a relative <link rel="manifest">, relative
// srcset entries) resolve inside the proxy subtree but carry no cookie on
// fetch; they must still get the capability appended. Scheme-bearing,
// network-relative, and fragment-only values stay untouched.
func TestRewriteHTMLURLs_AppendsCapabilityToRelativeReferences(t *testing.T) {
	in := `<link rel="manifest" href="manifest.webmanifest">` +
		`<a href="page">x</a>` +
		`<img srcset="a.png 1x, ./b.png 2x">` +
		`<a href="javascript:alert(1)">js</a>` +
		`<a href="mailto:x@y.dev">mail</a>` +
		`<a href="//cdn.example.com/lib.js">cdn</a>` +
		`<a href="#section">frag</a>` +
		`<img src="data:image/png;base64,AAA">` +
		`<a href=" https://evil.example/x">spaced-scheme</a>` +
		`<a href="&#10;//evil.example/y">newline-netrel</a>` +
		`<a href=" /rooted">spaced-root</a>` +
		`<form action="/submit"><button formaction="/alt"></button></form>` +
		`<a href="ht&#10;tps://evil2.example/z">embedded-tab-scheme</a>`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-789", ""))

	mustContain(t, got, `href="manifest.webmanifest?kandev_cap=cap-789"`)
	// Navigation references: prefixed, capability-free.
	mustContain(t, got, `<a href="page">x</a>`)
	mustContain(t, got, `action="/port-proxy/abc/3001/submit"`)
	mustContain(t, got, `formaction="/port-proxy/abc/3001/alt"`)
	mustContain(t, got, `srcset="a.png?kandev_cap=cap-789 1x, ./b.png?kandev_cap=cap-789 2x"`)
	mustContain(t, got, `href="javascript:alert(1)"`)
	mustContain(t, got, `href="mailto:x@y.dev"`)
	mustContain(t, got, `href="//cdn.example.com/lib.js"`)
	mustContain(t, got, `href="#section"`)
	mustContain(t, got, `src="data:image/png;base64,AAA"`)
	// Leading whitespace must not make an external URL look relative: the
	// browser trims it and goes external, so the capability must not be
	// appended.
	mustContain(t, got, `href=" https://evil.example/x"`)
	// The tokenizer decodes &#10; to a literal newline; the network-relative
	// URL behind it must still be left untouched.
	mustContain(t, got, "href=\"\n//evil.example/y\"")
	// A spaced root-absolute anchor reference is rewritten from its trimmed
	// form, capability-free (navigation).
	mustContain(t, got, `href="/port-proxy/abc/3001/rooted"`)
	// Embedded tab/newline inside the scheme: WHATWG normalization reveals an
	// external https URL, so the capability must not be appended.
	mustContain(t, got, "href=\"ht\ntps://evil2.example/z\"")
}

// The runtime shim appends the capability to every URL its network path
// rewriter produces: fragments stay last, existing query strings are preserved,
// and the append is skipped only for an exact kandev_cap query key. WebSocket
// URLs (string and same-origin URL-object inputs) go through the same rewriter.
// Navigation APIs (history/location) use a prefix-only rewriter so the bearer
// never lands in the address bar, history, or cross-origin Referers.
func TestRuntimeShim_AppendsCapabilityToRewrittenURLs(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "cap-shim")
	mustContain(t, shim, `var K="cap-shim";`)
	// Fragment split: the query lands before any #fragment.
	mustContain(t, shim, `var fh=x.indexOf('#');if(fh!==-1){fr=x.slice(fh);x=x.slice(0,fh)}`)
	// Exact-key guard: only an actual ?kandev_cap=/&kandev_cap= key suppresses
	// the append, not substrings like /kandev_cap=note or ?name=kandev_cap=.
	mustContain(t, shim, `if(!/([?&])kandev_cap=/.test(x))`)
	mustContain(t, shim, `x+=(x.indexOf('?')===-1?'?':'&')+"kandev_cap="+K`)
	// WebSocket wrapper rewrites string AND URL-object inputs through norm(),
	// which carries the capability; same-origin ws/wss origins are compared
	// as http/https via the URL API and the scheme is swapped for the
	// matching ws/wss form.
	mustContain(t, shim, `'//'+l.host`)
	mustContain(t, shim, `s.replace(/^[a-z][a-z0-9+.-]*:\/\/[^/?#]*/i,w)`)
	mustContain(t, shim, `x.origin.replace(/^ws:/,'http:').replace(/^wss:/,'https:')`)
	mustContain(t, shim, `new URL(s,window.location.href)`)
	// fetch/XHR/WS all route URL-like inputs through norm() (URL objects,
	// same-origin absolute strings).
	mustContain(t, shim, `typeof i==='string'||(i&&typeof i==='object'&&typeof i.href==='string'&&!i.url)`)
	mustContain(t, shim, `arguments[1]=norm(u)`)
	// Navigation APIs use the prefix-only rewriter, never the capability.
	mustContain(t, shim, `u=rn(u);return orig.call(this,s,t,u)`)
	mustContain(t, shim, `u=rn(u);return orig.call(location,u)`)
	mustContain(t, shim, `function rn(u){if(typeof u!=='string')return u;if(!u||u.charAt(0)!=='/'||(u.length>1&&u.charAt(1)==='/'))return u;if(u.indexOf(P)===0)return u;return P+u;}`)
	// Anchor navigation needs no click interception: the MutationObserver
	// prefixes hrefs, so the browser's own default navigation stays inside
	// the subtree and app click handlers keep control.
	if strings.Contains(shim, "addEventListener('click'") {
		t.Fatal("click interception must not be installed (it breaks app delegation)")
	}
	// norm() rewrites only http/https/ws/wss inputs; other schemes pass through.
	mustContain(t, shim, `var p=x.protocol;if(p!=='http:'&&p!=='https:'&&p!=='ws:'&&p!=='wss:')return u;`)
	// MutationObserver distinguishes navigation attributes (rn, incl.
	// metadata links) from subresource attributes (r), and scans the full
	// attribute set on inserted subtrees.
	mustContain(t, shim, `el.tagName==='A'||el.tagName==='AREA'||el.tagName==='BASE'||(el.tagName==='LINK'&&(el.rel==='canonical'||el.rel==='alternate'))`)
	mustContain(t, shim, `var rr=nav?rn:r;`)
	mustContain(t, shim, `'[href],[src],[action],[formaction],[cite],[data],[poster],[background],[manifest],[srcset]'`)
}

// The navigation rewriter must not carry the capability even when one is
// minted: the bearer stays out of the address bar and browser history.
func TestRuntimeShim_NavigationRewriterOmitsCapability(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "cap-shim")
	// rn() is the prefix-only form: no capability splice, no K reference.
	mustContain(t, shim, `function rn(u){if(typeof u!=='string')return u;if(!u||u.charAt(0)!=='/'||(u.length>1&&u.charAt(1)==='/'))return u;if(u.indexOf(P)===0)return u;return P+u;}`)
	// history and location rewrite through rn(), never r().
	mustContain(t, shim, `history[op]=function(s,t,u){if(typeof u==='string')u=rn(u);return orig.call(this,s,t,u)}`)
	mustContain(t, shim, `location[op]=function(u){if(typeof u==='string')u=rn(u);return orig.call(location,u)}`)
	if strings.Contains(shim, `history[op]=function(s,t,u){if(typeof u==='string')u=r(u)`) {
		t.Fatal("history APIs must not use the capability-bearing rewriter")
	}
}

// Without a capability the shim is byte-identical to the pre-auth output: no
// capability logic is spliced into the path rewriter.
func TestRuntimeShim_WithoutCapabilityStaysByteIdentical(t *testing.T) {
	withCap := runtimeShim(proxyPrefix, "")
	withoutCap := fmt.Sprintf(runtimeShimTemplate, proxyPrefix, "")
	if withCap != withoutCap {
		t.Fatal("empty capability must not alter the shim output")
	}
	if strings.Contains(withCap, "kandev_cap") {
		t.Fatalf("shim without capability mentions kandev_cap:\n%s", withCap)
	}
}

// Bodyless responses (1xx/204/304) must pass through untouched: no body
// rewrite, no synthesized Content-Length, no capability query rewriting.
func TestRewriteProxyResponse_LeavesBodylessResponsesUntouched(t *testing.T) {
	for _, status := range []int{http.StatusNoContent, http.StatusNotModified, http.StatusContinue} {
		resp := &http.Response{
			StatusCode: status,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
		}
		resp.Header.Set("Content-Type", "text/html; charset=utf-8")
		resp.Header.Set("Content-Length", "100") // 304: describes the selected representation
		if err := rewriteProxyResponse(resp, proxyPrefix, "cap-304"); err != nil {
			t.Fatalf("status %d: unexpected error: %v", status, err)
		}
		if got := resp.Header.Get("Content-Length"); got != "100" {
			t.Fatalf("status %d: Content-Length = %q, want original %q", status, got, "100")
		}
	}
}

// Capability-bearing rewritten responses must be uncacheable (per-user body +
// Set-Cookie) and must not leak the embedded capability through the Referer
// header to external origins. The headers are applied by the gateway's
// ModifyResponse for every response class (JS/JSON/redirects included), which
// is covered by the end-to-end proxy test; the rewriter itself performs the
// body rewrite only.
func TestRewriteProxyResponse_DoesNotSetCacheHeadersItself(t *testing.T) {
	body := `<!doctype html><html><head><link rel="manifest" href="/manifest.webmanifest"></head><body></body></html>`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	resp.Header.Set("Content-Type", "text/html; charset=utf-8")
	if err := rewriteProxyResponse(resp, proxyPrefix, "cap-cache"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Header.Get("Cache-Control") != "" || resp.Header.Get("Referrer-Policy") != "" {
		t.Fatalf("cache/referrer headers must be applied by ModifyResponse, not the rewriter: %v", resp.Header)
	}
}

// Inline iframe documents (srcdoc) inherit the proxy origin; their
// root-absolute references must be rewritten like a nested page, with the
// capability on subresources and none on navigation.
func TestRewriteHTMLURLs_RewritesSrcdocDocuments(t *testing.T) {
	in := `<iframe srcdoc="&lt;a href=&quot;/page&quot;&gt;x&lt;/a&gt;&lt;img src=&quot;/logo.png&quot;&gt;"></iframe>`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-srcdoc", ""))
	// The nested document is rewritten (navigation no cap, subresource cap);
	// the serializer re-escapes the srcdoc attribute value.
	mustContain(t, got, `srcdoc="&lt;a href=&#34;/port-proxy/abc/3001/page&#34;&gt;x&lt;/a&gt;&lt;img src=&#34;/port-proxy/abc/3001/logo.png?kandev_cap=cap-srcdoc&#34;&gt;"`)
}

// Meta refresh navigation targets must be re-anchored on the proxy subtree
// without a capability.
func TestRewriteHTMLURLs_RewritesMetaRefresh(t *testing.T) {
	in := `<meta http-equiv="refresh" content="5; url=/next">` +
		`<meta http-equiv="refresh" content="0;url='https://external.example/x'">`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-meta", ""))

	mustContain(t, got, `content="5; url=/port-proxy/abc/3001/next"`)
	mustContain(t, got, `content="0;url=&#39;https://external.example/x&#39;"`)
}

// Metadata links (rel=canonical and other non-fetching rels) must not carry
// the capability; fetching rels (stylesheet, manifest, preload, …) keep it.
func TestRewriteHTMLURLs_CanonicalLinkOmitsCapability(t *testing.T) {
	in := `<link rel="canonical" href="/canonical">` +
		`<link rel="stylesheet" href="/theme.css">` +
		`<link rel="manifest" href="/manifest.webmanifest">` +
		`<link rel="alternate" href="/feed.xml">`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-link", ""))

	mustContain(t, got, `rel="canonical" href="/port-proxy/abc/3001/canonical"`)
	mustContain(t, got, `rel="alternate" href="/port-proxy/abc/3001/feed.xml"`)
	mustContain(t, got, `rel="stylesheet" href="/port-proxy/abc/3001/theme.css?kandev_cap=cap-link"`)
	mustContain(t, got, `rel="manifest" href="/port-proxy/abc/3001/manifest.webmanifest?kandev_cap=cap-link"`)
}

// When the app's Content-Security-Policy uses a script nonce, the injected
// shim tag must carry it so nonce- and strict-dynamic-based policies allow the
// shim; the meta-tag form is honored too.
func TestRewriteHTMLURLs_ShimTagCarriesCSPNonce(t *testing.T) {
	in := `<!DOCTYPE html><html><head><title>x</title></head><body></body></html>`
	// Header-based policy.
	headerResp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":            {"text/html"},
			"Content-Security-Policy": {"default-src 'self'; script-src 'self' 'nonce-app123'"},
		},
		Body: io.NopCloser(strings.NewReader(in)),
	}
	if err := rewriteProxyResponse(headerResp, proxyPrefix, "cap-nonce"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := io.ReadAll(headerResp.Body)
	mustContain(t, string(got), `<script src="/port-proxy/abc/3001/__kandev_runtime_shim.js?kandev_cap=cap-nonce" nonce="app123"></script>`)

	// Meta-tag policy.
	metaResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/html"}},
		Body: io.NopCloser(strings.NewReader(
			`<!DOCTYPE html><html><head><meta http-equiv="Content-Security-Policy" content="script-src 'self' 'nonce-meta456'"><title>x</title></head><body></body></html>`)),
	}
	if err := rewriteProxyResponse(metaResp, proxyPrefix, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	gotMeta, _ := io.ReadAll(metaResp.Body)
	mustContain(t, string(gotMeta), `nonce="meta456"`)

	// No nonce in the policy: the tag stays plain.
	plain := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":            {"text/html"},
			"Content-Security-Policy": {"default-src 'self'"},
		},
		Body: io.NopCloser(strings.NewReader(in)),
	}
	if err := rewriteProxyResponse(plain, proxyPrefix, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	gotPlain, _ := io.ReadAll(plain.Body)
	if strings.Contains(string(gotPlain), "nonce=") {
		t.Fatalf("shim tag must not carry a nonce when the policy has none:\n%s", gotPlain)
	}
}

// Meta refresh targets: quoted URLs with embedded semicolons or spaces must be
// preserved; unquoted targets stop at the delimiter.
func TestRewriteMetaRefresh_QuoteAndSemicolonAware(t *testing.T) {
	in := `<meta http-equiv="refresh" content="0; url='/next;v=1'">` +
		`<meta http-equiv="refresh" content="3; url=/plain">`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-mq", ""))

	mustContain(t, got, `content="0; url=&#39;/port-proxy/abc/3001/next;v=1&#39;"`)
	mustContain(t, got, `content="3; url=/port-proxy/abc/3001/plain"`)
}

func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("output missing %q\noutput: %s", needle, haystack)
	}
}
