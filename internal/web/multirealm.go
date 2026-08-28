package web

import (
	"net/http"
	"strings"
)

const realmCookie = "portal_realm"

// MultiRealm routes every request to a realm-bound Server. Each Server owns
// that realm's character/world pools and SOAP client while auth remains shared.
func MultiRealm(defaultKey string, secureCookie bool, realms map[string]http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimSpace(r.URL.Query().Get("realm"))
		explicit := key != ""
		if key == "" {
			if cookie, err := r.Cookie(realmCookie); err == nil {
				key = cookie.Value
			}
		}
		handler, ok := realms[key]
		if !ok {
			if explicit {
				http.Error(w, "Unknown realm", http.StatusBadRequest)
				return
			}
			key, handler = defaultKey, realms[defaultKey]
		}
		if handler == nil {
			http.Error(w, "Realm unavailable", http.StatusServiceUnavailable)
			return
		}
		if explicit {
			http.SetCookie(w, &http.Cookie{Name: realmCookie, Value: key, Path: "/", MaxAge: 365 * 24 * 60 * 60, HttpOnly: true, Secure: secureCookie, SameSite: http.SameSiteLaxMode})
		}
		handler.ServeHTTP(w, r)
	})
}
