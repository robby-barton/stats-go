package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"

	"github.com/robby-barton/stats-go/internal/database"
)

type Config struct {
	Env          string
	DBParams     *database.DBParams // nil → use SQLite
	DeployScript string
}

const defaultPGPort = 5432

// SetupConfig loads environment configuration. DBParams is only populated when
// PostgreSQL is configured: if no PG_* variable is set at all, DBParams is nil
// and the database layer falls back to SQLite. A partial Postgres configuration
// (some PG_* variables set, required ones missing, or a malformed port) is a
// configuration error and returns an error rather than silently misbehaving.
func SetupConfig() (*Config, error) {
	env := os.Getenv("API_ENV")
	local := env == "" || env == "local"

	if local {
		godotenv.Load(".env")
	}

	params, err := dbParamsFromEnv()
	if err != nil {
		return nil, err
	}

	return &Config{
		Env:          env,
		DBParams:     params,
		DeployScript: os.Getenv("DEPLOY_SCRIPT"),
	}, nil
}

// dbParamsFromEnv builds DBParams from PG_* environment variables. It returns
// nil (use SQLite) when none are set, and an error on partial/malformed configs.
func dbParamsFromEnv() (*database.DBParams, error) {
	host := os.Getenv("PG_HOST")
	portStr := os.Getenv("PG_PORT")
	user := os.Getenv("PG_USER")
	password := os.Getenv("PG_PASSWORD")
	dbName := os.Getenv("PG_DBNAME")
	sslMode := os.Getenv("PG_SSLMODE")

	if host == "" && portStr == "" && user == "" && password == "" &&
		dbName == "" && sslMode == "" {
		return nil, nil //nolint:nilnil // nil DBParams is the documented SQLite fallback signal
	}

	var missing []string
	if host == "" {
		missing = append(missing, "PG_HOST")
	}
	if user == "" {
		missing = append(missing, "PG_USER")
	}
	if dbName == "" {
		missing = append(missing, "PG_DBNAME")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf(
			"postgres is partially configured; missing required env vars: %v "+
				"(unset all PG_* variables to use SQLite instead)", missing)
	}

	port := int64(defaultPGPort)
	if portStr != "" {
		parsed, err := strconv.ParseInt(portStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid PG_PORT %q: %w", portStr, err)
		}
		port = parsed
	}

	return &database.DBParams{
		Host:     host,
		Port:     port,
		User:     user,
		Password: password,
		DBName:   dbName,
		SSLMode:  sslMode,
	}, nil
}
