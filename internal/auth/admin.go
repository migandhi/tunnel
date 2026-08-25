package auth

import (
	"net"
	"net/http"

	"github.com/migandhi/tunnel-software/internal/security"
	"golang.org/x/crypto/bcrypt"
)

func BasicAuth(username, passHash string, limiter *security.Limiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		if ip == "" {
			ip = r.RemoteAddr
		}
		if !limiter.Allow(ip) {
			http.Error(w, "too many attempts, slow down", http.StatusTooManyRequests)
			return
		}
		u, p, ok := r.BasicAuth()
		if !ok || !ConstantTimeEqual(u, username) || bcrypt.CompareHashAndPassword([]byte(passHash), []byte(p)) != nil {
			w.Header().Set("WWW-Authenticate", `Basic realm="tunnel admin"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
