package config

import (
	"os"
	"strconv"

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
	CookieDomain string
	CookieSecure bool
}

type config struct {
	Server   ConfigServer
	Logger   ConfigLogger
	Database ConfigDatabase
	Auth     ConfigAuth
}

func NewConfig() (config, error) {
	if err := godotenv.Load(); err != nil {
		return config{}, err
	}

	readTimeout, _ := strconv.Atoi(getEnv("READ_TIMEOUT", "10"))
	writeTimeout, _ := strconv.Atoi(getEnv("WRITE_TIMEOUT", "10"))
	idleTimeout, _ := strconv.Atoi(getEnv("IDLE_TIMEOUT", "60"))
	cookieSecure, _ := strconv.ParseBool(getEnv("COOKIE_SECURE", "false"))

	cfg := config{
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
			PostgresURL: getEnv("POSTGRES_URL", "postgres://admin:admin@db:5432/self-dev"),
		},
		Auth: ConfigAuth{
			CookieDomain: getEnv("COOKIE_DOMAIN", ""),
			CookieSecure: cookieSecure,
		},
	}

	return cfg, nil
}

func getEnv(key, def string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return def
}
