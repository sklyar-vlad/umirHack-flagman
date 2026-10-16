package main

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	"github.com/sklyar-vlad/umirHack-flagman/database"
	"github.com/sklyar-vlad/umirHack-flagman/internal/config"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
	log.Print("database schema is ready")
}

func run() error {
	dsn := os.Getenv("POSTGRES_URL")
	if dsn == "" {
		return errors.New("POSTGRES_URL is required for database initialization")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := database.NewPostgres(ctx, config.ConfigDatabase{PostgresURL: dsn})
	if err != nil {
		return errors.New("cannot connect to database for initialization")
	}
	defer pool.Close()
	if err := database.Migrate(ctx, pool); err != nil {
		return errors.New("database initialization failed; check schema compatibility")
	}
	return nil
}
