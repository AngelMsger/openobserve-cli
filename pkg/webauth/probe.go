package webauth

import "strings"

// ProbeJS returns the capture script injected into the sign-in page. The script
// calls the global function named deliver with an object carrying an
// {authorization} or {email} key as it observes them.
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
func ProbeJS(deliver string) string {
	return strings.ReplaceAll(probeTemplate, "__DELIVER__", deliver)
}

const probeTemplate = `(function(){try{` +
	`function post(a){try{a=String(a||'');if(a){__DELIVER__(JSON.stringify({authorization:a}));}}catch(e){}}` +
	`try{var of=window.fetch;window.fetch=function(){try{` +
	`var h=arguments[1]&&arguments[1].headers;` +
	`if(h){var a=h['Authorization']||h['authorization']||(h.get&&(h.get('Authorization')||h.get('authorization')));if(a)post(a);}` +
	`}catch(e){}return of.apply(this,arguments);};}catch(e){}` +
	`try{var XS=XMLHttpRequest.prototype.setRequestHeader;` +
	`XMLHttpRequest.prototype.setRequestHeader=function(k,v){try{` +
	`if(k&&String(k).toLowerCase()==='authorization')post(v);` +
	`}catch(e){}return XS.apply(this,arguments);};}catch(e){}` +
	`try{var raw=localStorage.getItem('user_info')||localStorage.getItem('userInfo');` +
	`if(raw){var j=JSON.parse(raw);var em=j.email||(j.data&&j.data.email);` +
	`if(em){__DELIVER__(JSON.stringify({email:em}));}}}catch(e){}` +
	`}catch(e){}})();`
