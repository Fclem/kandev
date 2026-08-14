package websocket

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// URL attributes that may carry a path-absolute reference and therefore need
// the proxy path prefix when the document is served through the port proxy.
// Source: https://html.spec.whatwg.org/multipage/indices.html#attributes-3
var rewritableURLAttrs = map[string]bool{
	"href":       true,
	"src":        true,
	"action":     true,
	"formaction": true,
	"cite":       true,
	"data":       true,
	"poster":     true,
	"background": true,
	"manifest":   true,
}

// styleTagName is the HTML element name we treat as a raw-text CSS body so we
// can pipe its contents through `rewriteCSSFragment`.
const styleTagName = "style"

// scriptTagName is a raw-text element per the HTML spec — its contents are
// not HTML-escaped. The golang.org/x/net/html tokenizer correctly returns the
// raw bytes in `Token.Data`, but `Token.String()` blindly HTML-escapes any
// text token's `Data`, which corrupts inline JS containing characters like
// `&` (e.g. `x & 1`, `a && b`, JSON with `&` in strings). We therefore emit
// `<script>` bodies via the raw `Data` directly. We don't rewrite anything
// inside a script body — the runtime shim handles network-facing APIs at
// runtime, which is more reliable than trying to parse JS statically.
const scriptTagName = "script"

// headTagName is the HTML element after whose opening tag we inject the proxy
// runtime shim, so it installs `fetch`/`XHR`/`WebSocket` overrides before any
// user JS can run.
const headTagName = "head"

// runtimeShimTemplate is the JS bootstrap injected at the top of `<head>` for
// every HTML response we proxy. It overrides the network-facing browser APIs
// (fetch, XMLHttpRequest, WebSocket) plus the URL-mutating navigation APIs
// (history.pushState/replaceState, location.assign/replace) so root-absolute
// URLs requested at runtime — e.g. Next.js dynamic chunk imports, fetch calls,
// HMR WebSocket, and SPA-router pushState transitions — stay on the same proxy
// chain instead of escaping to the host origin.
//
// %q is replaced per-response with the proxy path (no trailing slash) using
// `fmt.Sprintf`, which emits a JS-safe double-quoted string literal.
//
// Concatenated across multiple Go strings only for readability; the resulting
// JS is still a single self-invoking expression with no internal whitespace.
const runtimeShimTemplate = `(function(){` +
	`var P=%q;` +
	`window.__kandevProxyPrefix=P;` +
	// Path rewriter: prefix path-absolute URLs that aren't already prefixed.
	// %s is the optional capability-append logic (empty when no capability is
	// minted, keeping the auth-disabled output byte-identical).
	`function r(u){if(typeof u!=='string')return u;if(!u||u.charAt(0)==='#'||u.indexOf('//')===0)return u;try{var ru=new URL(u,window.location.href);if(ru.protocol!=='http:'&&ru.protocol!=='https:'||ru.origin!==window.location.origin)return u}catch(e){return u}if(u.charAt(0)==='/'){if(u.indexOf(P)===0)return u;u=P+u}%sreturn u;}` +
	// Navigation rewriter: same prefixing WITHOUT the capability. Navigation
	// APIs (history.pushState, location.assign) put the URL in the address bar
	// and browser history; embedding a bearer there would leak it through
	// copied URLs, history, and cross-origin Referers. The subtree cookie
	// covers same-origin navigations instead.
	`function sc(u){var qi=u.indexOf('?');if(qi===-1)return u;var b=u.slice(0,qi);var q=u.slice(qi+1).split('&').filter(function(p){var k=p.split('=')[0];var d;try{d=decodeURIComponent(k)}catch(e){d=k}return d!=='kandev_cap'});return b+(q.length?'?'+q.join('&'):'')}` +
	`function rn(u){if(typeof u!=='string')return u;if(!u||u.charAt(0)!=='/'||(u.length>1&&u.charAt(1)==='/'))return u;if(u.indexOf(P)===0)return sc(u);return sc(P+u);}` +
	// norm(u): normalizes any URL-like input through the network rewriter
	// using the URL API, which applies the WHATWG parsing rules (leading
	// C0/space trimmed, embedded tabs/newlines removed, default ports and
	// userinfo normalized). Only http/https/ws/wss inputs that resolve to the
	// page's origin (the gateway, in an iframe) get their pathname rewritten
	// through r() with scheme+host preserved; cross-origin inputs, other
	// schemes (blob:, data:, file:, about:), and non-URL inputs pass through
	// unchanged. ws:/wss: origins are compared as http:/https: so WebSocket
	// URLs match the page origin.
	`function norm(u){var s=typeof u==='string'?u:(u&&typeof u==='object'&&typeof u.href==='string'?u.href:null);if(s===null)return u;var x;try{x=new URL(s,window.location.href)}catch(e){return u}var p=x.protocol;if(p!=='http:'&&p!=='https:'&&p!=='ws:'&&p!=='wss:')return u;var o=x.origin.replace(/^ws:/,'http:').replace(/^wss:/,'https:');if(o!==window.location.origin)return u;return x.origin+r(x.pathname+(x.search||'')+(x.hash||''));}` +
	// fetch — string, URL-object, and Request-object input forms.
	`var of=window.fetch;if(of){window.fetch=function(i,n){if(typeof i==='string'||(i&&typeof i==='object'&&typeof i.href==='string'&&!i.url))i=norm(i);else if(i&&typeof i==='object'&&typeof i.url==='string'){var nu=norm(i.url);if(nu!==i.url){try{i=new Request(nu,i)}catch(e){}}}return of.call(this,i,n)}}` +
	// XMLHttpRequest.open — 2nd arg is the URL (string or URL object).
	`var oo=XMLHttpRequest.prototype.open;XMLHttpRequest.prototype.open=function(m,u){arguments[1]=norm(u);return oo.apply(this,arguments)};` +
	// WebSocket — path-absolute ws/wss URLs need an explicit ws:// scheme + host since the constructor doesn't accept bare paths. String and URL-object inputs go through norm(); same-origin results are converted to the matching ws/wss scheme, fragments are dropped (WebSocket URLs cannot carry one), and network-relative and cross-origin inputs pass through untouched.
	`var OW=window.WebSocket;if(OW){function W(u,p){var n=norm(u);var l=window.location;var w=(l.protocol==='https:'?'wss:':'ws:')+'//'+l.host;var s=n;if(typeof s==='string'){if(s.charAt(0)==='/'&&s.charAt(1)!=='/'){s=w+s}else if(s!==u&&/^[a-z][a-z0-9+.-]*:\/\/[^/?#]*/i.test(s)){s=s.replace(/^[a-z][a-z0-9+.-]*:\/\/[^/?#]*/i,w)}var h=s.indexOf('#');if(h!==-1)s=s.slice(0,h)}return p?new OW(s,p):new OW(s)}W.prototype=OW.prototype;Object.getOwnPropertyNames(OW).forEach(function(k){try{W[k]=OW[k]}catch(e){}});window.WebSocket=W}` +
	// history.pushState / replaceState — SPA routers (Next.js, React Router, etc.) call these to change the URL on client-side navigation. Without rewriting, the URL bar drops the proxy prefix and a reload 404s. Uses rn() (no capability — see above).
	`['pushState','replaceState'].forEach(function(op){var orig=history[op];if(!orig)return;history[op]=function(s,t,u){if(typeof u==='string')u=rn(u);return orig.call(this,s,t,u)}});` +
	// location.assign / location.replace — direct navigation APIs, patched
	// best-effort with the prefix-only rewriter. Chromium exposes these as
	// non-writable own properties, so the patch can silently no-op there.
	// Anchor navigation needs no interception: static and MutationObserver
	// rewriting already prefix anchor hrefs, so the browser's own default
	// navigation stays inside the proxy subtree and application click
	// handlers (including delegated document listeners) keep full control.
	// Known limitation: an anchor created and clicked programmatically
	// (anchor.click()) in the same task can navigate before the
	// MutationObserver microtask rewrites its href; ordinary user clicks are
	// unaffected because the observer delivers before the next task.
	`['assign','replace'].forEach(function(op){var orig=location[op];if(!orig)return;try{location[op]=function(u){if(typeof u==='string')u=rn(u);return orig.call(location,u)}}catch(e){}});` +
	// MutationObserver: rewrite URL attributes on every element that is
	// inserted or has its URL attribute mutated. Navigation attributes
	// (anchor/area/base href, form action, button/input formaction, metadata
	// links with no fetching rel token) use the prefix-only rewriter (and any
	// previously issued capability is stripped); subresource attributes use
	// the capability-bearing one. Dynamic style attributes get their
	// root-absolute url() references rewritten, and dynamic meta-refresh
	// content gets its url= target prefixed. Dynamic iframe srcdoc is not
	// runtime-rewritten (static-only, documented): its children inherit the
	// subtree cookies, so only cookie-less dynamic srcdoc escapes — an
	// accepted edge. Covers the cases the network-API patches miss, notably
	// `ReactDOM.preload()` and any framework that builds DOM nodes with
	// absolute paths after the initial HTML has been parsed.
	`var ATTRS=['href','src','action','formaction','cite','data','poster','background','manifest','srcset','style','content'];` +
	`function lf(el){var rel=el.rel;if(typeof rel!=='string')return true;var toks=rel.toLowerCase().split(/\s+/);for(var i=0;i<toks.length;i++){if(toks[i]==='stylesheet'||toks[i]==='icon'||toks[i]==='manifest'||toks[i]==='preload'||toks[i]==='modulepreload'||toks[i]==='prefetch'||toks[i]==='dns-prefetch'||toks[i]==='preconnect'||toks[i]==='shortcut'||toks[i]==='apple-touch-icon')return true}return false}` +
	`function mref(v){var ml=v.toLowerCase();var m=/(^|;)\s*url=/.exec(ml);if(!m)return v;var mi=m.index+m[0].length-4;var after=v.slice(mi+4);var lead=after.replace(/^[ \t\n\r\f]+/,'');var off=after.length-lead.length;var t=lead;var q='';var rest='';if(t.charAt(0)===\''||t.charAt(0)==='"'){q=t.charAt(0);var e=t.indexOf(q,1);if(e<0)return v;t=t.slice(1,e);rest=lead.slice(e)}else{var sp=t.search(/[; \t\n\r\f]/);if(sp>=0){t=t.slice(0,sp);rest=lead.slice(sp)}}return v.slice(0,mi+4)+after.slice(0,off)+q+rn(t)+rest}` +
	`function rwa(el,a){if(!el.getAttribute||!el.hasAttribute(a))return;var v=el.getAttribute(a);if(typeof v!=='string')return;var nav=(a==='href'&&(el.tagName==='A'||el.tagName==='AREA'||el.tagName==='BASE'||(el.tagName==='LINK'&&!lf(el))))||(a==='action'&&el.tagName==='FORM')||(a==='formaction'&&(el.tagName==='BUTTON'||el.tagName==='INPUT'));var rr=nav?rn:r;var nv;if(a==='srcset'){nv=v.split(',').map(function(p){var f=p.trim().split(/\s+/);if(f[0])f[0]=rr(f[0]);return f.join(' ')}).join(', ')}else if(a==='style'){nv=v.replace(/url\(\s*(['"]?)([^'")]+)/g,function(m,q,u){return 'url('+q+r(u)+')'})}else if(a==='content'&&el.tagName==='META'){nv=mref(v)}else{nv=rr(v)}if(nv!==v)el.setAttribute(a,nv)}` +
	`function rwe(el){if(!el||el.nodeType!==1)return;for(var i=0;i<ATTRS.length;i++)rwa(el,ATTRS[i])}` +
	`var OBS=['href','src','action','formaction','cite','data','poster','background','manifest','srcset','rel','style','content','http-equiv'];` +
	`var MO=window.MutationObserver;if(MO&&document.documentElement){try{new MO(function(rs){for(var i=0;i<rs.length;i++){var rec=rs[i];if(rec.type==='attributes'){rwe(rec.target)}else{for(var j=0;j<rec.addedNodes.length;j++){var n=rec.addedNodes[j];rwe(n);if(n.querySelectorAll){var nl=n.querySelectorAll('[href],[src],[action],[formaction],[cite],[data],[poster],[background],[manifest],[srcset],[style],[content]');for(var k=0;k<nl.length;k++)rwe(nl[k])}}}}}).observe(document.documentElement,{childList:true,subtree:true,attributes:true,attributeFilter:OBS})}catch(e){}}` +
	// Console forwarding: pipe iframe console output back to the parent frame via postMessage so it surfaces in the kandev UI alongside other preview events. The message targets the gateway origin only (the preview is served by the gateway, same-origin with the Kandev UI), never a wildcard, so a cross-origin embedding parent cannot receive proxied app console arguments. Errors and stacks are coerced to strings; objects are JSON-cloned where possible. We continue calling the original method so the iframe's own DevTools still shows everything.
	`var LV=['log','warn','error','info','debug'];LV.forEach(function(lv){var orig=console[lv];if(!orig)return;console[lv]=function(){try{var out=[];for(var i=0;i<arguments.length;i++){var a=arguments[i];if(a instanceof Error){out.push('Error: '+a.message+(a.stack?'\n'+a.stack:''))}else if(typeof a==='object'&&a!==null){try{out.push(JSON.parse(JSON.stringify(a)))}catch(e){out.push(String(a))}}else{out.push(a)}}window.parent.postMessage({source:'kandev-inspector',type:'console',payload:{level:lv,args:out}},window.location.origin)}catch(e){}return orig.apply(console,arguments)}});` +
	`})();`

// runtimeShim returns the runtime shim's JavaScript body with the given proxy
// prefix baked in. %q produces a JS-safe double-quoted string literal (slashes
// and alphanumerics need no escaping, which matches every prefix we emit).
//
// When `capability` is non-empty, the shim's network path rewriter appends the
// subtree capability to every URL it rewrites (fetch/XHR/WebSocket and
// dynamically injected DOM), so those requests stay authorized in contexts
// where cookies are not sent. Navigation APIs (history/location) deliberately
// use a prefix-only rewriter: a bearer in the address bar or history would leak
// through copied URLs and cross-origin Referers, and the subtree cookie covers
// same-origin navigations.
//
// The shim is served as an external same-origin JavaScript file by the port
// proxy (reserved path __kandev_runtime_shim.js) and injected as a <script
// src> tag: an app's Content-Security-Policy (script-src 'self') blocks inline
// scripts but allows a same-origin script.
func runtimeShim(prefix, capability string) string {
	return fmt.Sprintf(runtimeShimTemplate, prefix, shimCapabilityJS(capability))
}

// runtimeShimPath is the reserved per-subtree path the port proxy serves the
// runtime shim JavaScript from.
const runtimeShimPath = "__kandev_runtime_shim.js"

// runtimeShimTag is the <script src> element injected at the top of <head> for
// every HTML response we proxy; the browser loads the shim body from the
// proxy's reserved path. When a capability is minted it rides on the src so
// the shim itself loads even in cookie-less contexts. When the response's
// Content-Security-Policy uses a script nonce, the tag carries it (validated
// against the CSP nonce grammar and HTML-escaped) so nonce- and
// strict-dynamic-based policies still allow the shim.
func runtimeShimTag(prefix, capability, nonce string) string {
	src := prefix + "/" + runtimeShimPath
	if capability != "" {
		src += "?" + proxyCapabilityQueryParam + "=" + capability
	}
	nonceAttr := ""
	if nonce != "" && validCSPNonce(nonce) {
		nonceAttr = ` nonce="` + html.EscapeString(nonce) + `"`
	}
	return `<script src="` + src + `"` + nonceAttr + `></script>`
}

// validCSPNonce accepts only the CSP nonce grammar: base64/base64url
// characters with optional padding. A malformed nonce-shaped token from an
// app-controlled policy must never be interpolated into the injected markup.
func validCSPNonce(nonce string) bool {
	if nonce == "" || len(nonce) > 256 {
		return false
	}
	seenContent := false
	padding := 0
	for i := 0; i < len(nonce); i++ {
		if !cspNonceChar(nonce[i]) {
			return false
		}
		if nonce[i] == '=' {
			padding++
			if padding > 2 {
				return false
			}
		} else {
			if padding > 0 {
				return false // padding only at the very end (ab=c invalid)
			}
			seenContent = true
		}
	}
	// An all-padding token ("=", "==") is not a valid nonce value.
	return seenContent
}

// cspNonceChar reports whether a byte is valid in a CSP nonce (base64 /
// base64url alphabet).
func cspNonceChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
		c == '+' || c == '/' || c == '-' || c == '_' || c == '='
}

// scriptNonceFromCSP extracts the first VALID script-src nonce from a
// Content-Security-Policy value, e.g. `script-src 'self' 'nonce-abc123'`.
// Invalid nonce-shaped tokens are skipped so a later valid one is used.
func scriptNonceFromCSP(policy string) string {
	for _, directive := range strings.Split(policy, ";") {
		fields := strings.Fields(strings.TrimSpace(directive))
		if len(fields) == 0 || !strings.EqualFold(fields[0], "script-src") && !strings.EqualFold(fields[0], "script-src-elem") {
			continue
		}
		for _, source := range fields[1:] {
			if nonce, ok := strings.CutPrefix(source, "'nonce-"); ok {
				if nonce, ok = strings.CutSuffix(nonce, "'"); ok && validCSPNonce(nonce) {
					return nonce
				}
			}
		}
	}
	return ""
}

// responseScriptNonce returns the script nonce an app Content-Security-Policy
// requires, from either the response header or a CSP meta tag in the body.
// Meta tags are scanned with the HTML tokenizer: attributes are decoded
// (entity-encoded policies work), keys match exactly (data-content decoys do
// not), any attribute order is accepted, and every meta is examined so a
// nonce-free policy cannot hide a later nonce-bearing one.
func responseScriptNonce(resp *http.Response, body []byte) string {
	for _, policy := range resp.Header.Values("Content-Security-Policy") {
		if nonce := scriptNonceFromCSP(policy); nonce != "" {
			return nonce
		}
	}
	tok := html.NewTokenizer(bytes.NewReader(body))
	for {
		tt := tok.Next()
		switch tt {
		case html.ErrorToken:
			return ""
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := tok.TagName()
			if !bytes.Equal(name, []byte("meta")) || !hasAttr {
				continue
			}
			var equiv, content string
			for {
				key, val, more := tok.TagAttr()
				switch {
				case bytes.EqualFold(key, []byte("http-equiv")):
					equiv = string(val)
				case bytes.EqualFold(key, []byte("content")):
					content = string(val)
				}
				if !more {
					break
				}
			}
			if !strings.EqualFold(strings.TrimSpace(equiv), "content-security-policy") {
				continue
			}
			if nonce := scriptNonceFromCSP(content); nonce != "" {
				return nonce
			}
		}
	}
}

// shimCapabilityJS returns the JavaScript spliced into the shim's path
// rewriter so runtime-rewritten URLs carry the subtree capability. It appends
// the capability as a query parameter: any URL fragment is split off first so
// the query lands before it and existing query strings are preserved. The
// append is unconditional — an app's own kandev_cap parameter must not
// suppress the issued capability, and the gateway accepts any valid value
// among duplicates. Empty when no capability is minted, keeping the
// auth-disabled output byte-identical.
func shimCapabilityJS(capability string) string {
	if capability == "" {
		return ""
	}
	param := proxyCapabilityQueryParam + "="
	// The append is idempotent for the EXACT issued capability: a value that
	// already carries kandev_cap=K is not touched (this stops the
	// MutationObserver from looping on its own rewrites), while an app's own
	// different kandev_cap value still gets the issued capability appended —
	// the gateway accepts any valid value among duplicates.
	return fmt.Sprintf(`var K=%q;var fr='';var fh=u.indexOf('#');if(fh!==-1){fr=u.slice(fh);u=u.slice(0,fh)}if(u.indexOf(%q+K)===-1)u+=(u.indexOf('?')===-1?'?':'&')+%q+K;return u+fr;`,
		capability, param, param)
}

// urlInCSSPattern matches `url(...)` invocations in CSS where the argument is
// a path-absolute URL we want to rewrite. The argument can be optionally
// wrapped in single or double quotes and may be surrounded by whitespace.
//
//	url(/foo)
//	url('/foo')
//	url("/foo")
//
// Network-relative (`//host/foo`) and absolute (`http://...`) are left alone.
var urlInCSSPattern = regexp.MustCompile(`url\(\s*(['"]?)(/[^/'"][^'")]*)`)

// urlRelativeCSSPattern matches `url(...)` invocations whose argument is a
// same-origin relative reference. The first character is not "/", so scheme
// (`http:`, `data:`) and network-relative (`//host`) references are excluded
// here; the shared rewriter leaves those untouched anyway.
var urlRelativeCSSPattern = regexp.MustCompile(`url\(\s*(['"]?)([^/'"][^'")]*)`)

// importInCSSPattern matches `@import "/foo";` style root-absolute imports.
var importInCSSPattern = regexp.MustCompile(`@import\s+(['"])(/[^/'"][^'"]*)['"]`)

// importRelativeCSSPattern matches `@import "foo.css";` style relative imports.
var importRelativeCSSPattern = regexp.MustCompile(`@import\s+(['"])([^/'"][^'"]*)['"]`)

// rewriteProxyResponse mutates an `http.Response` from agentctl in place,
// rewriting root-absolute URLs to be prefixed by `proxyPrefix` so the iframe's
// asset/XHR/import requests come back through the same port proxy instead of
// hitting the host page's origin. Returns nil and leaves the response untouched
// for content types we don't rewrite (everything except HTML and CSS today).
//
// `proxyPrefix` is the public URL path that fronts this proxy on the gateway,
// e.g. "/port-proxy/<sessionId>/<port>" (no trailing slash). It is prepended to
// matched URLs that start with a single "/" — see `rewriteAbsolutePath`.
//
// `capability`, when non-empty, is appended to every rewritten URL as
// `kandev_cap=<capability>`. It is the short-lived subtree credential minted
// after the preview document authenticated; embedding it in the asset URLs
// keeps the browser's subresource fetches authorized even when they cannot
// carry cookies (the <link rel="manifest"> fetch, sandboxed iframes).
func rewriteProxyResponse(resp *http.Response, proxyPrefix, capability string) error {
	// 1xx/204/304 responses carry no body (304's Content-Length describes the
	// selected representation, not a rewritten payload): leave them untouched.
	if resp.StatusCode < 200 || resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotModified {
		return nil
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	var rewrite func([]byte, string, string, string) []byte
	switch {
	case strings.Contains(ct, "text/html"):
		rewrite = rewriteHTMLURLs
	case strings.Contains(ct, "text/css"):
		rewrite = rewriteCSSURLs
	default:
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}

	nonce := responseScriptNonce(resp, body)
	modified := rewrite(body, proxyPrefix, capability, nonce)
	resp.Body = io.NopCloser(bytes.NewReader(modified))
	resp.Header.Del("Content-Encoding")
	resp.Header.Set("Content-Length", strconv.Itoa(len(modified)))
	resp.ContentLength = int64(len(modified))
	return nil
}

// withCapability appends the capability query parameter to a rewritten URL,
// using "&" when the URL already carries a query string. A URL fragment, when
// present, stays last — the capability must precede it or the browser would
// treat it as part of the fragment and never send it.
func withCapability(rewritten, capability string) string {
	if capability == "" {
		return rewritten
	}
	path, fragment, hasFragment := strings.Cut(rewritten, "#")
	sep := "?"
	if strings.ContainsRune(path, '?') {
		sep = "&"
	}
	path += sep + proxyCapabilityQueryParam + "=" + capability
	if hasFragment {
		path += "#" + fragment
	}
	return path
}

// rewriteAbsolutePath turns a path-absolute URL ("/foo") into a proxied path
// ("<prefix>/foo"). Returns the input unchanged for non-rewritable cases:
// empty strings, network-relative URLs (`//host`), schemes (`http:`, `data:`,
// `mailto:`, etc.), or relative paths (`foo`, `./foo`, `../foo`).
func rewriteAbsolutePath(rawURL, prefix, capability string) string {
	if len(rawURL) < 1 || rawURL[0] != '/' {
		return rawURL
	}
	if len(rawURL) >= 2 && rawURL[1] == '/' {
		return rawURL // network-relative
	}
	return withCapability(prefix+rawURL, capability)
}

// htmlRewriteState tracks per-document position during a single
// `rewriteHTMLURLs` pass. Pulled out to its own type so each token-handling
// case stays short and the top-level loop's cyclomatic complexity stays under
// the lint budget.
type htmlRewriteState struct {
	out          *bytes.Buffer
	prefix       string
	capability   string
	shim         string
	nonce        string
	depth        int
	inStyle      bool
	inScript     bool
	shimInjected bool
}

// onStartTag emits the (URL-rewritten) start tag and updates raw-text
// element tracking. Also injects the runtime shim immediately after `<head>`.
func (s *htmlRewriteState) onStartTag(token html.Token) {
	rewriteTokenURLs(&token, s.prefix, s.capability, s.nonce, s.depth)
	s.out.WriteString(token.String())
	if !s.shimInjected && token.Data == headTagName {
		s.out.WriteString(s.shim)
		s.shimInjected = true
	}
	switch token.Data {
	case styleTagName:
		s.inStyle = true
	case scriptTagName:
		s.inScript = true
	}
}

// onEndTag clears raw-text element tracking, then writes the end tag.
func (s *htmlRewriteState) onEndTag(token html.Token) {
	switch token.Data {
	case styleTagName:
		s.inStyle = false
	case scriptTagName:
		s.inScript = false
	}
	s.out.WriteString(token.String())
}

// onTextToken writes text content with the right escaping for the current
// element context: rewrite CSS URLs inside `<style>`, emit raw bytes inside
// `<script>` (Token.String would HTML-escape and corrupt JS), and otherwise
// fall back to the default Token.String() entity-encoding.
func (s *htmlRewriteState) onTextToken(token html.Token) {
	switch {
	case s.inStyle:
		s.out.WriteString(rewriteCSSFragment(token.Data, s.prefix, s.capability))
	case s.inScript:
		s.out.WriteString(token.Data)
	default:
		s.out.WriteString(token.String())
	}
}

// rewriteHTMLURLs walks the HTML document and rewrites every rewritable URL
// attribute (`href`, `src`, …) plus `srcset` values and inline `style="…"`
// `url(...)` references. `<style>` content is run through the CSS URL
// rewriter; `<script>` content is emitted unchanged (the runtime shim
// handles network-facing APIs at runtime).
//
// Falls back to returning the input unchanged if tokenization fails midway, so
// a malformed page never blocks the response.
func rewriteHTMLURLs(body []byte, prefix, capability, nonce string) []byte {
	return rewriteHTMLURLsAtDepth(body, prefix, capability, nonce, 0)
}

// maxSRCDocDepth bounds recursive srcdoc rewriting so a deeply nested inline
// document cannot exhaust the stack or balloon memory.
const maxSRCDocDepth = 5

func rewriteHTMLURLsAtDepth(body []byte, prefix, capability, nonce string, depth int) []byte {
	tok := html.NewTokenizer(bytes.NewReader(body))
	var out bytes.Buffer
	out.Grow(len(body) + 256 + len(runtimeShimTemplate))
	s := &htmlRewriteState{out: &out, prefix: prefix, capability: capability, shim: runtimeShimTag(prefix, capability, nonce), nonce: nonce, depth: depth}
	for {
		tt := tok.Next()
		if tt == html.ErrorToken {
			if tok.Err() == io.EOF {
				return out.Bytes()
			}
			return body
		}
		token := tok.Token()
		switch token.Type {
		case html.StartTagToken:
			s.onStartTag(token)
		case html.SelfClosingTagToken:
			rewriteTokenURLs(&token, prefix, capability, nonce, depth)
			out.WriteString(token.String())
		case html.EndTagToken:
			s.onEndTag(token)
		case html.TextToken:
			s.onTextToken(token)
		default:
			out.WriteString(token.String())
		}
	}
}

// rewriteTokenURLs walks a single token's attributes and rewrites any URL-
// shaped attribute value in place. Navigation attributes — anchor/area/base
// href, form action, button/input formaction — get the proxy prefix but NOT
// the capability: clicking them navigates the top-level frame, so a bearer in
// the URL would land in the address bar, history, bookmarks, and cross-origin
// Referers (the subtree cookie authorizes the navigation instead). Subresource
// attributes (script/img/stylesheet/manifest/iframe src-href, srcset, …) get
// the capability so cookie-less fetches stay authorized.
func rewriteTokenURLs(token *html.Token, prefix, capability, nonce string, depth int) {
	if token.Type != html.StartTagToken && token.Type != html.SelfClosingTagToken {
		return
	}
	// Metadata links (rel=canonical, alternate, license, author, …) are not
	// fetched; a capability in their href would leak through the emitted
	// canonical URL. rel values that DO fetch (stylesheet, icon, manifest,
	// preload, modulepreload, …) keep the capability.
	metaLink := token.Data == "link" && !isFetchingLinkRel(relValue(token))
	for i, attr := range token.Attr {
		key := strings.ToLower(attr.Key)
		token.Attr[i].Val = rewriteTokenAttr(token, key, attr.Val, prefix, capability, nonce, depth, metaLink)
	}
}

// rewriteTokenAttr rewrites one URL-shaped attribute value of a token,
// applying the navigation/subresource capability distinction.
func rewriteTokenAttr(token *html.Token, key, val, prefix, capability, nonce string, depth int, metaLink bool) string {
	switch {
	case key == "href" && (isNavHref(token.Data) || metaLink), key == "action" && token.Data == "form", key == "formaction" && (token.Data == "button" || token.Data == "input"):
		return rewriteURLReference(val, prefix, "")
	case key == "srcdoc":
		// Inline child document: its root-absolute references resolve
		// against the child, which inherits the proxy origin, so rewrite
		// them like a nested page. Bounded so nested srcdoc cannot
		// exhaust the stack.
		if depth < maxSRCDocDepth {
			return string(rewriteHTMLURLsAtDepth([]byte(val), prefix, capability, nonce, depth+1))
		}
		return val
	case key == attrContent && token.Data == "meta" && isMetaRefresh(token):
		return rewriteMetaRefresh(val, prefix)
	case rewritableURLAttrs[key]:
		return rewriteURLReference(val, prefix, capability)
	case key == "srcset":
		return rewriteSrcSet(val, prefix, capability)
	case key == "style":
		return rewriteCSSFragment(val, prefix, capability)
	}
	return val
}

// isNavHref reports whether an element's href is a navigation reference.
func isNavHref(tag string) bool {
	return tag == "a" || tag == "area" || tag == "base"
}

// relValue extracts the normalized rel attribute value of a token.
func relValue(token *html.Token) string {
	for _, attr := range token.Attr {
		if strings.EqualFold(attr.Key, "rel") {
			return strings.ToLower(strings.TrimSpace(attr.Val))
		}
	}
	return ""
}

// attrContent is the HTML "content" attribute key (meta refresh targets).
const attrContent = "content"

// fetchingLinkRels are link rel tokens whose href the browser fetches; a rel
// value with ANY fetching token is a subresource. Metadata-only rels (no
// fetching token) must not carry the capability.
var fetchingLinkRels = map[string]bool{
	"stylesheet": true, "icon": true, "shortcut": true, "apple-touch-icon": true,
	"manifest": true, "preload": true, "modulepreload": true, "prefetch": true,
	"dns-prefetch": true, "preconnect": true,
}

// isFetchingLinkRel reports whether a link rel value (a whitespace-separated
// token list, per the HTML spec) contains any fetching token.
func isFetchingLinkRel(rel string) bool {
	for _, token := range strings.Fields(rel) {
		if fetchingLinkRels[strings.ToLower(token)] {
			return true
		}
	}
	return false
}

// isMetaRefresh reports whether a meta token is an http-equiv=refresh.
func isMetaRefresh(token *html.Token) bool {
	for _, attr := range token.Attr {
		if strings.EqualFold(attr.Key, "http-equiv") && strings.EqualFold(strings.TrimSpace(attr.Val), "refresh") {
			return true
		}
	}
	return false
}

// rewriteMetaRefresh prefixes the root-absolute navigation target inside a
// meta refresh content value (no capability — it is a navigation). The "url"
// token is recognized only at a field boundary — the start of the value or
// right after any ";" separator, case-insensitive, ignoring surrounding
// whitespace — so unrelated text like `noturl=/evil` is never rewritten and a
// multi-field value like `5;foo=bar; url=/next` is handled. The target is
// parsed exactly once, respecting an optional quote; any text after it is
// preserved untouched.
func rewriteMetaRefresh(content, prefix string) string {
	lower := strings.ToLower(content)
	idx := -1
	if strings.HasPrefix(lower, "url=") {
		idx = 0
	} else {
		for from := 0; from < len(lower); {
			semi := strings.IndexByte(lower[from:], ';')
			if semi < 0 {
				break
			}
			from += semi + 1
			rest := lower[from:]
			trimmed := strings.TrimLeft(rest, " \t\n\r\f")
			if strings.HasPrefix(trimmed, "url=") {
				idx = from + (len(rest) - len(trimmed))
				break
			}
		}
	}
	if idx < 0 {
		return content
	}
	rest := content[idx+4:]
	trimmed := strings.TrimLeft(rest, " \t\n\r\f")
	offset := len(rest) - len(trimmed)
	target, tail, quote := parseRefreshTarget(trimmed)
	return content[:idx+4] + rest[:offset] + quote + rewriteURLReference(target, prefix, "") + tail
}

// parseRefreshTarget splits a meta refresh target off the text after `url=`,
// respecting an optional quote. The tail keeps the closing quote and any
// remaining text (including nested url= tokens) untouched.
func parseRefreshTarget(trimmed string) (target, tail, quote string) {
	if trimmed == "" {
		return "", "", ""
	}
	if trimmed[0] != '\'' && trimmed[0] != '"' {
		if end := strings.IndexAny(trimmed, "; \t"); end >= 0 {
			return trimmed[:end], trimmed[end:], ""
		}
		return trimmed, "", ""
	}
	quote = trimmed[:1]
	if end := strings.IndexByte(trimmed[1:], quote[0]); end >= 0 {
		return trimmed[1 : 1+end], trimmed[1+end:], quote
	}
	return trimmed[1:], "", quote
}

// rewriteURLReference rewrites a URL-shaped attribute value. Path-absolute
// URLs get the proxy prefix plus the capability. Same-origin relative URLs
// keep their own resolution — the document already lives in the proxy subtree,
// so the browser lands inside it — but still get the capability appended so
// cookie-less fetches (a relative <link rel="manifest">) stay authorized.
// Classification runs against a WHATWG-normalized copy (ASCII tabs/newlines
// removed anywhere, leading C0/space trimmed — the browser does the same when
// parsing): a scheme-bearing or network-relative reference hidden behind
// whitespace or embedded controls is left untouched rather than leaking the
// capability to an external origin. Empty, fragment-only, and scheme-bearing
// values are never modified.
func rewriteURLReference(rawURL, prefix, capability string) string {
	normalized := normalizeURLForClassification(rawURL)
	if normalized == "" || normalized[0] == '#' || hasURLScheme(normalized) || strings.HasPrefix(normalized, "//") {
		return rawURL
	}
	if normalized[0] == '/' {
		return rewriteAbsolutePath(normalized, prefix, capability)
	}
	if capability == "" {
		return rawURL
	}
	return withCapability(rawURL, capability)
}

// normalizeURLForClassification returns a copy of a URL reference with the
// WHATWG parser's forgiving adjustments applied: ASCII tabs and newlines are
// removed anywhere in the string, and leading C0/space characters are trimmed.
// Only the result is used for classification; the original bytes are preserved
// when the reference is returned unchanged.
func normalizeURLForClassification(raw string) string {
	noControls := strings.Map(func(r rune) rune {
		switch r {
		case '\t', '\n', '\r':
			return -1
		}
		return r
	}, raw)
	return strings.TrimLeftFunc(noControls, func(r rune) bool { return r <= ' ' })
}

// hasURLScheme reports whether a URL reference starts with a scheme per
// RFC 3986 — ALPHA *( ALPHA / DIGIT / "+" / "-" / "." ) followed by ":"
// (http:, data:, javascript:, mailto:, …). A colon in a position where a
// scheme could not start (a digit or a non-scheme character first, or after a
// path/query/fragment delimiter) makes the reference relative instead.
func hasURLScheme(raw string) bool {
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		switch {
		case c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			continue
		case c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.':
			if i == 0 {
				return false // schemes start with an ASCII letter
			}
			continue
		case c == ':':
			return i > 0
		default:
			return false
		}
	}
	return false
}

// rewriteSrcSet rewrites each candidate URL in a `srcset` attribute. The
// value format is `url [descriptor], url [descriptor], …` per the HTML spec.
func rewriteSrcSet(value, prefix, capability string) string {
	parts := strings.Split(value, ",")
	for i, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		// Split into URL and optional descriptor (separated by whitespace).
		fields := strings.Fields(trimmed)
		if len(fields) == 0 {
			continue
		}
		fields[0] = rewriteURLReference(fields[0], prefix, capability)
		parts[i] = strings.Join(fields, " ")
	}
	return strings.Join(parts, ", ")
}

// rewriteCSSURLs rewrites url(/...) and @import "/..." occurrences inside a
// standalone CSS document.
func rewriteCSSURLs(body []byte, prefix, capability, _ string) []byte {
	return []byte(rewriteCSSFragment(string(body), prefix, capability))
}

// rewriteCSSFragment rewrites CSS URL references inside an arbitrary string
// (either a full CSS file or the contents of an inline style attribute).
// Path-absolute references get the proxy prefix plus the capability; relative
// references keep their resolution but still get the capability appended so
// cookie-less CSS loads stay authorized. Scheme-bearing, network-relative, and
// fragment-only references are left untouched.
func rewriteCSSFragment(css, prefix, capability string) string {
	css = urlInCSSPattern.ReplaceAllStringFunc(css, func(match string) string {
		sub := urlInCSSPattern.FindStringSubmatch(match)
		// sub: full match, quote, url
		if len(sub) != 3 {
			return match
		}
		return "url(" + sub[1] + rewriteAbsolutePath(sub[2], prefix, capability)
	})
	css = urlRelativeCSSPattern.ReplaceAllStringFunc(css, func(match string) string {
		sub := urlRelativeCSSPattern.FindStringSubmatch(match)
		if len(sub) != 3 {
			return match
		}
		return "url(" + sub[1] + rewriteURLReference(sub[2], prefix, capability)
	})
	css = importInCSSPattern.ReplaceAllStringFunc(css, func(match string) string {
		sub := importInCSSPattern.FindStringSubmatch(match)
		if len(sub) != 3 {
			return match
		}
		return "@import " + sub[1] + rewriteAbsolutePath(sub[2], prefix, capability) + sub[1]
	})
	css = importRelativeCSSPattern.ReplaceAllStringFunc(css, func(match string) string {
		sub := importRelativeCSSPattern.FindStringSubmatch(match)
		if len(sub) != 3 {
			return match
		}
		return "@import " + sub[1] + rewriteURLReference(sub[2], prefix, capability) + sub[1]
	})
	return css
}
