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
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		return config{}, fmt.Errorf("cannot load dotenv configuration")
	}

	metricsToken, err := secretEnv("METRICS_TOKEN")
	if err != nil {
		return config{}, err
	}
	if metricsToken != "" && len(metricsToken) < 32 {
		return config{}, fmt.Errorf("metrics token requires at least 32 bytes")
	}
	postgresURL, err := secretEnv("POSTGRES_URL")
	if err != nil {
		return config{}, err
	}
	if getEnv("ENV", "production") == "production" {
		if postgresURL == "" {
			return config{}, fmt.Errorf("production requires explicit database configuration")
		}
		if !strings.HasPrefix(os.Getenv("FRONTEND_ORIGIN"), "https://") {
			return config{}, fmt.Errorf("production requires an explicit HTTPS frontend origin")
		}
	}
	if postgresURL == "" {
		postgresURL = "postgres://admin:admin@db:5432/flagman"
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
			PostgresURL: postgresURL,
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

// secretEnv accepts one runtime secret source without exposing its contents or path.
func secretEnv(name string) (string, error) {
	value := os.Getenv(name)
	path := os.Getenv(name + "_FILE")
	if path == "" {
		return value, nil
	}
	if value != "" {
		return "", fmt.Errorf("configure only one %s source", name)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot read %s file", name)
	}
	value = strings.TrimSpace(string(data))
	if value == "" {
		return "", fmt.Errorf("empty %s file", name)
	}
	return value, nil
}
