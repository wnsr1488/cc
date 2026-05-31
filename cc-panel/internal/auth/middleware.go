package auth

import (
	"net/http"
	"strings"

	"github.com/example/cc-panel/internal/httpx"
)

func Middleware(service *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				httpx.WriteError(w, http.StatusUnauthorized, "missing bearer token")
				return
			}
			claims, err := service.ParseToken(strings.TrimPrefix(header, "Bearer "))
			if err != nil {
				httpx.WriteError(w, http.StatusUnauthorized, "invalid token")
				return
			}
			user, err := service.FindByID(r.Context(), claims.UserID)
			if err != nil {
				httpx.WriteError(w, http.StatusUnauthorized, "user not found")
				return
			}
			next.ServeHTTP(w, r.WithContext(ContextWithUser(r.Context(), user)))
		})
	}
}
