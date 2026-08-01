package webauth

import (
	"strconv"
	"strings"
)

// ProbeJS returns the capture script injected into the sign-in page. The script
// calls the global function named deliver with an object carrying an
// {authorization} or {email} key as it observes them.
//
// host scopes the script to the OpenObserve instance. This is a security
// boundary, not a nicety: the injection mechanisms both callers use (the CDP
// Page.addScriptToEvaluateOnNewDocument, and o3's WKUserScript) evaluate the
// script in EVERY document, including a full-page or iframed identity provider
// on a completely different origin. An IdP that sends its own Authorization
// header over XHR or fetch would otherwise have that header captured, written
// to the user's keychain, and replayed on every request to the OpenObserve
// instance — a third party's bearer token handed to a different server. So the
// script does nothing at all unless location.hostname is host, or a subdomain
// of it (the same relation HostMatches applies to cookie domains). Any port in
// host is stripped first, since location.hostname carries none. An empty host
// disables capture entirely rather than capturing everywhere.
//
// It hooks BOTH window.fetch and XMLHttpRequest.prototype.setRequestHeader.
// Both are required: OpenObserve's web app issues its API requests through
// axios, which uses XHR, so a fetch-only hook captures nothing on a real
// instance and leaves only the short-lived session cookie. The Authorization
// header the SPA sends is the durable credential — typically
// Basic base64(email:token) — and it outlives the session cookie, so it is the
// more valuable of the two captures.
//
// The script is defensive to the point of paranoia — every step is wrapped in
// try/catch and the original function is always called — because it runs inside
// a page we do not control and must never break the user's ability to log in.
func ProbeJS(deliver, host string) string {
	js := strings.ReplaceAll(probeTemplate, "__DELIVER__", deliver)
	// strconv.Quote emits a valid JS string literal and escapes anything that
	// could break out of it, so a hostile "host" cannot inject code.
	return strings.ReplaceAll(js, "'__HOST__'", strconv.Quote(probeHost(host)))
}

// probeHost renders host the way location.hostname would: no port, lowercase,
// and an IPv6 literal kept in its brackets (hostname() strips them, but the DOM
// keeps them, and the gate compares the two directly).
func probeHost(host string) string {
	h := hostname(host)
	if strings.Contains(h, ":") {
		return "[" + h + "]"
	}
	return h
}

// probeTemplate is the injected script. '__HOST__' is replaced by a quoted JS
// string literal (not just its contents), so the surrounding quotes here are
// part of the placeholder.
const probeTemplate = `(function(){try{` +
	`var t='__HOST__';` +
	`var h='';try{h=String(location.hostname||'').toLowerCase();}catch(e){}` +
	`if(!t||!h||(h!==t&&h.slice(-(t.length+1))!=='.'+t)){return;}` +
	`function post(a){try{a=String(a||'');if(a){__DELIVER__(JSON.stringify({authorization:a}));}}catch(e){}}` +
	`try{var of=window.fetch;window.fetch=function(){try{` +
	`var hd=arguments[1]&&arguments[1].headers;` +
	`if(hd){var a=hd['Authorization']||hd['authorization']||(hd.get&&(hd.get('Authorization')||hd.get('authorization')));if(a)post(a);}` +
	`}catch(e){}return of.apply(this,arguments);};}catch(e){}` +
	`try{var XS=XMLHttpRequest.prototype.setRequestHeader;` +
	`XMLHttpRequest.prototype.setRequestHeader=function(k,v){try{` +
	`if(k&&String(k).toLowerCase()==='authorization')post(v);` +
	`}catch(e){}return XS.apply(this,arguments);};}catch(e){}` +
	`try{var raw=localStorage.getItem('user_info')||localStorage.getItem('userInfo');` +
	`if(raw){var j=JSON.parse(raw);var em=j.email||(j.data&&j.data.email);` +
	`if(em){__DELIVER__(JSON.stringify({email:em}));}}}catch(e){}` +
	`}catch(e){}})();`
