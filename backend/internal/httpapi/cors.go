package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"sniping_engine/internal/config"
)

func corsMiddleware(cfg config.CorsConfig, next http.Handler) http.Handler {
	allowHeaders := []string{"Content-Type", "Authorization"}
	allowMethods := []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	maxAge := 600

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowedOrigin := ""
		for _, o := range cfg.AllowOrigins {
			if o == "*" {
				allowedOrigin = "*"
				break
			}
			if strings.EqualFold(o, origin) {
				allowedOrigin = origin
				break
			}
		}

		if allowedOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			if cfg.AllowCredentials {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			w.Header().Set("Access-Control-Allow-Headers", strings.Join(allowHeaders, ", "))
			w.Header().Set("Access-Control-Allow-Methods", strings.Join(allowMethods, ", "))
			w.Header().Set("Access-Control-Max-Age", strconv.Itoa(maxAge))
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

