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

	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, ""))

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

	got := string(rewriteCSSURLs([]byte(in), proxyPrefix, ""))

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
		Header: http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
		Body:   io.NopCloser(strings.NewReader(body)),
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
		Header: http.Header{"Content-Type": []string{"text/css"}},
		Body:   io.NopCloser(strings.NewReader(body)),
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
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   io.NopCloser(strings.NewReader(body)),
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
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, ""))

	// The shim must appear exactly once, immediately after the `<head>` open tag
	// (so it executes before any other script that may follow).
	const marker = "window.fetch="
	if strings.Count(got, marker) != 1 {
		t.Fatalf("expected exactly one runtime shim, got %d copies\n%s",
			strings.Count(got, marker), got)
	}
	headIdx := strings.Index(got, "<head>")
	titleIdx := strings.Index(got, "<title>")
	shimIdx := strings.Index(got, marker)
	if headIdx >= shimIdx || shimIdx >= titleIdx {
		t.Fatalf("shim must come between <head> and <title>: head=%d shim=%d title=%d\n%s",
			headIdx, shimIdx, titleIdx, got)
	}
	// The prefix must be baked into the JS literal.
	if !strings.Contains(got, `var P="/port-proxy/abc/3001"`) {
		t.Fatalf("shim missing baked-in prefix:\n%s", got)
	}
}

func TestRewriteHTMLURLs_NoHeadStillRewritesURLs(t *testing.T) {
	// Documents without a <head> are rare but possible. We don't bother
	// injecting the shim in that case (no good anchor point), but URL
	// rewriting must still work.
	in := `<a href="/foo">x</a>`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, ""))
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

	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, ""))

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

	// Both patches must reuse the existing path rewriter `r()` rather than
	// rolling their own prefix logic.
	for _, needle := range []string{
		`u=r(u);return orig.call(this,s,t,u)`, // history APIs
		`u=r(u);return orig.call(location,u)`, // location APIs
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

// The subtree capability must ride on every rewritten URL: plain URLs get it
// as the first query parameter, URLs that already carry a query get it
// appended. Network-relative and absolute URLs stay untouched.
func TestRewriteHTMLURLs_AppendsCapabilityToRewrittenURLs(t *testing.T) {
	in := `<a href="/foo">x</a>` +
		`<script type="module" src="/src/main.tsx?t=123"></script>` +
		`<img srcset="/a.png 1x, //cdn.example.com/b.png 2x">` +
		`<link rel="manifest" href="/manifest.webmanifest">` +
		`<a href="/docs#installation">docs</a>`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-123"))

	mustContain(t, got, `href="/port-proxy/abc/3001/foo?kandev_cap=cap-123"`)
	// The tokenizer HTML-escapes the separator inside the attribute value.
	mustContain(t, got, `src="/port-proxy/abc/3001/src/main.tsx?t=123&amp;kandev_cap=cap-123"`)
	mustContain(t, got, `href="/port-proxy/abc/3001/manifest.webmanifest?kandev_cap=cap-123"`)
	mustContain(t, got, `srcset="/port-proxy/abc/3001/a.png?kandev_cap=cap-123 1x, //cdn.example.com/b.png 2x"`)
	// A fragment must stay after the capability query.
	mustContain(t, got, `href="/port-proxy/abc/3001/docs?kandev_cap=cap-123#installation"`)
}

// The capability is appended to rewritten CSS url() and @import references too,
// so cookie-less CSS loads stay authorized in the same contexts as the
// manifest fetch.
func TestRewriteCSSURLs_AppendsCapability(t *testing.T) {
	in := `@import "/theme.css"; .x { background: url("/img/bg.png"); }`
	got := string(rewriteCSSURLs([]byte(in), proxyPrefix, "cap-456"))

	mustContain(t, got, `@import "/port-proxy/abc/3001/theme.css?kandev_cap=cap-456"`)
	mustContain(t, got, `url("/port-proxy/abc/3001/img/bg.png?kandev_cap=cap-456")`)
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
		`<img src="data:image/png;base64,AAA">`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-789"))

	mustContain(t, got, `href="manifest.webmanifest?kandev_cap=cap-789"`)
	mustContain(t, got, `href="page?kandev_cap=cap-789"`)
	mustContain(t, got, `srcset="a.png?kandev_cap=cap-789 1x, ./b.png?kandev_cap=cap-789 2x"`)
	mustContain(t, got, `href="javascript:alert(1)"`)
	mustContain(t, got, `href="mailto:x@y.dev"`)
	mustContain(t, got, `href="//cdn.example.com/lib.js"`)
	mustContain(t, got, `href="#section"`)
	mustContain(t, got, `src="data:image/png;base64,AAA"`)
}

// The runtime shim appends the capability to every URL its path rewriter
// produces, preserving existing query strings and never double-appending.
func TestRuntimeShim_AppendsCapabilityToRewrittenURLs(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "cap-shim")
	mustContain(t, shim, `var K="cap-shim";`)
	mustContain(t, shim, `x.indexOf("kandev_cap=")===-1`)
	mustContain(t, shim, `x+=(x.indexOf('?')===-1?'?':'&')+"kandev_cap="+K`)
	// Existing query strings are preserved by the "&" branch.
	mustContain(t, shim, `x.indexOf('?')===-1?'?':'&'`)
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

func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("output missing %q\noutput: %s", needle, haystack)
	}
}
