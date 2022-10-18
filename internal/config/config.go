package config

import (
	"os"
	"strconv"

	"github.com/robby-barton/stats-api/internal/database"

	"github.com/joho/godotenv"
)

type Config struct {
	DBParams *database.DBParams
}

func SetupConfig() *Config {
	env := os.Getenv("API_ENV")

	if "" == env || "local" == env {
		godotenv.Load(".env")
	}

	port, _ := strconv.ParseInt(os.Getenv("PG_PORT"), 10, 64)

	return &Config{
		DBParams: &database.DBParams{
			Host:     os.Getenv("PG_HOST"),
			Port:     port,
			User:     os.Getenv("PG_USER"),
			Password: os.Getenv("PG_PASSWORD"),
			DBName:   os.Getenv("PG_DBNAME"),
			SSLMode:  os.Getenv("PG_SSLMODE"),
		},
	}
}
