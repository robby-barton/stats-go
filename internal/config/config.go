package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"

	"github.com/robby-barton/stats-go/internal/database"
)

type Config struct {
	Env              string
	DBParams         *database.DBParams
	RevalidateSecret string
}

func SetupConfig() *Config {
	env := os.Getenv("API_ENV")
	local := env == "" || env == "local"

	if local {
		godotenv.Load(".env")
	}

	port, _ := strconv.ParseInt(os.Getenv("PG_PORT"), 10, 64)

	return &Config{
		Env: env,
		DBParams: &database.DBParams{
			Host:     os.Getenv("PG_HOST"),
			Port:     port,
			User:     os.Getenv("PG_USER"),
			Password: os.Getenv("PG_PASSWORD"),
			DBName:   os.Getenv("PG_DBNAME"),
			SSLMode:  os.Getenv("PG_SSLMODE"),
		},
		RevalidateSecret: os.Getenv("REVALIDATE_SECRET"),
	}
}
