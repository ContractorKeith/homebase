package hub

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

const cookieName = "homebase_key"

// keyOK compares in constant time; hashing first hides the key's length too.
func (s *Server) keyOK(got string) bool {
	if got == "" || s.key == "" {
		return false
	}
	a, b := sha256.Sum256([]byte(got)), sha256.Sum256([]byte(s.key))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}

// requestKey is the key a request carries: a Bearer header or the cookie.
func requestKey(r *http.Request) string {
	if h, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		return strings.TrimSpace(h)
	}
	if c, err := r.Cookie(cookieName); err == nil {
		return c.Value
	}
	return ""
}

// auth gives 401 to any request without the key. Visiting /?key=<key> sets the cookie and
// redirects to / so the key does not stay in the address bar.
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" && r.Method == http.MethodGet && s.keyOK(r.URL.Query().Get("key")) {
			http.SetCookie(w, &http.Cookie{Name: cookieName, Value: r.URL.Query().Get("key"),
				Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 365 * 24 * 3600})
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		if !s.keyOK(requestKey(r)) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="homebase"`)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized: send the key as a Bearer token or open /?key=<key>"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
