package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type ConfigLogger struct {
	Env string
}

type ConfigServer struct {
	Host         string
	Addr         string
	Port         string
	ReadTimeout  int
	WriteTimeout int
	IdleTimeout  int
	Middleware   string
}

type ConfigDatabase struct {
	PostgresURL string
}

type ConfigAuth struct {
	FrontendOrigin string
}

type config struct {
	Server       ConfigServer
	Logger       ConfigLogger
	Database     ConfigDatabase
	Auth         ConfigAuth
	MetricsToken string
}

func NewConfig() (config, error) {
	if err := godotenv.Load(); err != nil {
		return config{}, err
	}

	metricsToken := os.Getenv("METRICS_TOKEN")
	if path := os.Getenv("METRICS_TOKEN_FILE"); path != "" {
		if metricsToken != "" {
			return config{}, fmt.Errorf("configure only one metrics token source")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return config{}, fmt.Errorf("cannot read metrics token file")
		}
		metricsToken = strings.TrimSpace(string(data))
		if len(metricsToken) < 32 {
			return config{}, fmt.Errorf("metrics token requires at least 32 bytes")
		}
	}
	readTimeout, _ := strconv.Atoi(getEnv("READ_TIMEOUT", "10"))
	writeTimeout, _ := strconv.Atoi(getEnv("WRITE_TIMEOUT", "10"))
	idleTimeout, _ := strconv.Atoi(getEnv("IDLE_TIMEOUT", "60"))

	cfg := config{
		MetricsToken: metricsToken,
		Server: ConfigServer{
			Host:         getEnv("HOST", ""),
			Addr:         getEnv("ADDR", ""),
			Port:         getEnv("PORT", "8080"),
			ReadTimeout:  readTimeout,
			WriteTimeout: writeTimeout,
			IdleTimeout:  idleTimeout,
			Middleware:   getEnv("MIDDLEWARE", ""),
		},
		Logger: ConfigLogger{
			Env: getEnv("ENV", "production"),
		},
		Database: ConfigDatabase{
			PostgresURL: getEnv("POSTGRES_URL", "postgres://admin:admin@db:5432/flagman"),
		},
		Auth: ConfigAuth{FrontendOrigin: getEnv("FRONTEND_ORIGIN", "http://flagman.localhost")},
	}

	return cfg, nil
}

func getEnv(key, def string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return def
}
