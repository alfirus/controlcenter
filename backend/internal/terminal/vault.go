package terminal

import (
	"context"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// fetchVaultSecret resolves a vault_ref to raw secret value.
//
// vault_ref forms supported:
//   vault://<name>           -> env VAULT_<NAME> (dots/hyphens → underscore, uppercased)
//   env://VAR_NAME            -> env VAR_NAME
//   file:///path              -> file contents
//   <raw> with "BEGIN "       -> treated as raw PEM (rejected elsewhere, but returned here for error path)
// For Supabase Vault (vault.secrets), try SELECT decrypted_secret FROM vault.decrypted_secrets WHERE name=$1
func fetchVaultSecret(ctx context.Context, pool *pgxpool.Pool, vaultRef string) (string, error) {
	if vaultRef == "" {
		return "", nil
	}
	if strings.HasPrefix(vaultRef, "vault://") {
		name := strings.TrimPrefix(vaultRef, "vault://")
		envKey := "VAULT_" + strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(name, "/", "_"), "-", "_"))
		if v := os.Getenv(envKey); v != "" {
			return v, nil
		}
		if v := os.Getenv(name); v != "" {
			return v, nil
		}
		// try Supabase Vault table if available
		if pool != nil {
			var secret string
			err := pool.QueryRow(ctx, `select decrypted_secret from vault.decrypted_secrets where name=$1`, name).Scan(&secret)
			if err == nil && secret != "" {
				return secret, nil
			}
		}
		// fallback: return empty to allow caller to error cleanly
		return "", nil
	}
	if strings.HasPrefix(vaultRef, "env://") {
		return os.Getenv(strings.TrimPrefix(vaultRef, "env://")), nil
	}
	if strings.HasPrefix(vaultRef, "file://") {
		b, err := os.ReadFile(strings.TrimPrefix(vaultRef, "file://"))
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	// plain env fallback
	if v := os.Getenv(vaultRef); v != "" {
		return v, nil
	}
	return vaultRef, nil
}
