package config

import "os"

type Config struct {
	DatabaseURL          string
	SupabaseJWTSecret    string
	SupabaseURL          string
	Port                 string
	GithubAppID          string
	GithubPrivateKey     string
	GithubWebhookSecret  string
	GoogleClientID       string
	GoogleClientSecret   string
	GoogleRedirectURL    string
}

func Load() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	return Config{
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		SupabaseJWTSecret:   os.Getenv("SUPABASE_JWT_SECRET"),
		SupabaseURL:         os.Getenv("SUPABASE_URL"),
		Port:                port,
		GithubAppID:         os.Getenv("GITHUB_APP_ID"),
		GithubPrivateKey:    os.Getenv("GITHUB_PRIVATE_KEY"),
		GithubWebhookSecret: os.Getenv("GITHUB_WEBHOOK_SECRET"),
		GoogleClientID:      os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret:  os.Getenv("GOOGLE_CLIENT_SECRET"),
		GoogleRedirectURL:   os.Getenv("GOOGLE_REDIRECT_URL"),
	}
}
