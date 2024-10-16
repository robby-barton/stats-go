package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/writer"
)

type Config struct {
	Env              string
	DBParams         *database.DBParams
	RevalidateSecret string
	DOConfig         *writer.DOConfig
	Local            bool
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
		DOConfig: &writer.DOConfig{
			Key:      os.Getenv("DO_KEY"),
			Secret:   os.Getenv("DO_SECRET"),
			Endpoint: os.Getenv("DO_ENDPOINT"),
			Bucket:   os.Getenv("DO_BUCKET"),
			APIToken: os.Getenv("DO_API_TOKEN"),
			CDNID:    os.Getenv("DO_CDN_ID"),
		},
		RevalidateSecret: os.Getenv("REVALIDATE_SECRET"),
		Local:            local,
	}
}
