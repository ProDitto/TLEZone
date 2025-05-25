package middleware

import (
	"context"
	"net/http"
	"strings"
	"tle_zone_v2/internal/common"
	"tle_zone_v2/internal/common/security"
	"tle_zone_v2/internal/domain/model"

	"github.com/go-chi/jwtauth/v5"
)

type contextKey string

const (
	UserIDCtxKey   contextKey = "userID"
	UserRoleCtxKey contextKey = "userRole"
)

func Authenticator(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, claims, err := jwtauth.FromContext(r.Context()) // Extracts token from Authorization header

		if err != nil {
			if strings.Contains(err.Error(), "token not found") || token == nil {
				common.RespondWithError(w, http.StatusUnauthorized, "Authorization token required")
			} else {
				common.RespondWithError(w, http.StatusUnauthorized, "Invalid token: "+err.Error())
			}
			return
		}

		if token == nil {
			common.RespondWithError(w, http.StatusUnauthorized, "Invalid token")
			return
		}

		userID, err := security.GetUserIDFromClaims(claims)
		if err != nil {
			common.RespondWithError(w, http.StatusUnauthorized, "Invalid token claims: "+err.Error())
			return
		}
		userRole, err := security.GetUserRoleFromClaims(claims)
		if err != nil {
			common.RespondWithError(w, http.StatusUnauthorized, "Invalid token claims: "+err.Error())
			return
		}

		ctx := context.WithValue(r.Context(), UserIDCtxKey, userID)
		ctx = context.WithValue(ctx, UserRoleCtxKey, userRole)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func AdminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role, ok := r.Context().Value(UserRoleCtxKey).(string)
		if !ok || role != model.RoleAdmin {
			common.RespondWithError(w, http.StatusForbidden, "Admin access required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Helper to get user ID from context
func GetUserIDFromContext(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(UserIDCtxKey).(string)
	return userID, ok
}

// Helper to get user role from context
func GetUserRoleFromContext(ctx context.Context) (string, bool) {
	userRole, ok := ctx.Value(UserRoleCtxKey).(string)
	return userRole, ok
}
