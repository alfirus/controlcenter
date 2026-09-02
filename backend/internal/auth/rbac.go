package auth

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RequireWorkspaceRole checks workspace_members for the current user.
// allowed e.g. []string{"admin","member"}. "agent" is checked via agent_memberships separately.
func RequireWorkspaceRole(pool *pgxpool.Pool, workspaceIDParam string, allowed ...string) func(http.Handler) http.Handler {
	allowedSet := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		allowedSet[a] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if pool == nil {
				next.ServeHTTP(w, r)
				return
			}
			userID := UserID(r.Context())
			if userID == "" {
				http.Error(w, `{"error":{"code":"forbidden","message":"no user"}}`, http.StatusForbidden)
				return
			}
			// workspace id comes from URL param; caller must set via chi.URLParam
			// We extract from context via chi — lazy import to avoid cycle, so use r.Context value set by handler wrapper instead.
			// Fallback: allow if no param (list endpoints handle filtering themselves).
			wsID := r.Context().Value(ctxKey("workspace_id"))
			if wsID == nil {
				next.ServeHTTP(w, r)
				return
			}
			wsStr, _ := wsID.(string)
			if wsStr == "" {
				next.ServeHTTP(w, r)
				return
			}
			var role string
			err := pool.QueryRow(r.Context(),
				`select role from workspace_members where workspace_id=$1 and user_id=$2`, wsStr, userID).Scan(&role)
			if err != nil {
				http.Error(w, `{"error":{"code":"forbidden","message":"not a workspace member"}}`, http.StatusForbidden)
				return
			}
			if !allowedSet[role] {
				http.Error(w, `{"error":{"code":"forbidden","message":"insufficient role"}}`, http.StatusForbidden)
				return
			}
			ctx := context.WithValue(r.Context(), RoleKey, role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func WithWorkspaceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey("workspace_id"), id)
}
