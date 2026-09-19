package web

import (
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
	"time"
)

// modelViewerUpstream is the CDN that serves the WotLK-era character models.
// Its responses carry no CORS headers and return 403 to any request that sends
// an Origin header, so the assets are only usable from a same-origin proxy.
const modelViewerUpstream = "https://wow.zamimg.com/modelviewer/"

// modelViewerProxy forwards a narrow, allow-listed slice of the model viewer
// under this server's own origin:
//
//	GET /modelviewer/viewer.min.js          → viewer bundle
//	GET /modelviewer/auto/...               → meta/model/texture payloads
//
// Everything else is rejected, so the route cannot be used as a general proxy.
func modelViewerProxy() http.Handler {
	target, err := url.Parse(modelViewerUpstream)
	if err != nil {
		panic(err)
	}
	proxy := &httputil.ReverseProxy{
		Director: func(r *http.Request) {
			incoming := r.URL.Path
			switch {
			case incoming == "/modelviewer/viewer.min.js":
				r.URL.Path = "/modelviewer/live/viewer/viewer.min.js"
			case strings.HasPrefix(incoming, "/modelviewer/auto/"):
				// The viewer bundle asks for "<contentPath>meta/...", TheraWoW-style,
				// while the CDN actually serves those files under /live/.
				r.URL.Path = "/modelviewer/live/" + strings.TrimPrefix(incoming, "/modelviewer/auto/")
			}
			r.URL.Scheme = target.Scheme
			r.URL.Host = target.Host
			r.Host = target.Host
			r.Header.Del("Origin")
			r.Header.Del("Referer")
			r.Header.Del("Cookie")
			r.Header.Set("Accept-Encoding", "gzip")
		},
		ModifyResponse: func(res *http.Response) error {
			// The viewer's own files are immutable per game patch; meta JSON is
			// stable too. Cache hard so the proxy is not on the hot path.
			res.Header.Set("Cache-Control", "public, max-age=2592000, immutable")
			res.Header.Del("Set-Cookie")
			res.Header.Del("Strict-Transport-Security")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, e error) {
			problem(w, http.StatusBadGateway, "Model viewer unavailable")
		},
		Transport: &http.Transport{
			Proxy:                 nil,
			DialContext:           (&net.Dialer{Timeout: 8 * time.Second}).DialContext,
			MaxIdleConns:          16,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   8 * time.Second,
			ResponseHeaderTimeout: 15 * time.Second,
			ForceAttemptHTTP2:     true,
		},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			problem(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		// Reject traversal on the raw (still escaped) path before cleaning, so a
		// request such as /modelviewer/auto/../../etc/passwd cannot be normalised
		// into something that looks allow-listed.
		raw := r.URL.EscapedPath()
		if strings.Contains(raw, "..") || strings.Contains(raw, "%2e%2e") || strings.Contains(raw, "\\") {
			problem(w, http.StatusBadRequest, "Invalid path")
			return
		}
		p := path.Clean(raw)
		switch {
		case p == "/modelviewer/viewer.min.js":
		case strings.HasPrefix(p, "/modelviewer/auto/"):
		default:
			problem(w, http.StatusNotFound, "Not found")
			return
		}
		proxy.ServeHTTP(w, r)
	})
}
