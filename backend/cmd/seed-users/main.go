package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/sklyar-vlad/umirHack-flagman/database"
	"github.com/sklyar-vlad/umirHack-flagman/internal/config"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
	"go.uber.org/zap"
)

type seedUser struct {
	id           uuid.UUID
	name         string
	email        string
	role         string
	passwordHash string
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}

	log.Print("demo users seed completed")
}

func run() error {
	envFile := flag.String(
		"env",
		"../infrastructure/.env",
		"Database configuration",
	)
	flag.Parse()

	if err := godotenv.Load(*envFile); err != nil {
		return errors.New(
			"cannot load seed configuration; check -env file",
		)
	}

	users, err := loadDemoUsers()
	if err != nil {
		return err
	}

	dsn := os.Getenv("GOOSE_DBSTRING")
	if dsn == "" {
		return errors.New("GOOSE_DBSTRING is required for seed-users")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		60*time.Second,
	)
	defer cancel()

	ctx = requestctx.WithID(ctx, uuid.NewString())

	pool, err := database.NewPostgres(ctx, config.ConfigDatabase{
		PostgresURL: dsn,
	})
	if err != nil {
		return errors.New(
			"cannot connect to seed database; check GOOSE_DBSTRING and database availability",
		)
	}
	defer pool.Close()

	repo := repository.NewRepository(pool, zap.NewNop())

	return repo.WithinTx(ctx, nil, func(tx repository.DBTX) error {
		if _, err := tx.Exec(
			ctx,
			"SELECT pg_advisory_xact_lock($1)",
			int64(40401),
		); err != nil {
			return fmt.Errorf("lock seed: %w", err)
		}

		const query = `
			INSERT INTO users (
				id, name, email, password_hash, role, is_active
			)
			VALUES ($1, $2, $3, $4, $5, true)
			ON CONFLICT (id) DO NOTHING
		`

		for _, user := range users {
			result, err := tx.Exec(
				ctx,
				query,
				user.id.String(),
				user.name,
				user.email,
				user.passwordHash,
				user.role,
			)
			if err != nil {
				return fmt.Errorf(
					"insert demo user with role %s: %w",
					user.role,
					err,
				)
			}

			if result.RowsAffected() == 0 {
				continue
			}

			id := user.id

			if err := repo.AppendAudit(ctx, tx, &model.AuditEntry{
				ActorID:    nil,
				Action:     model.AuditCreate,
				EntityType: model.AuditUser,
				EntityID:   &id,
				After: &model.UserAuditSnapshot{
					Name:     user.name,
					Email:    user.email,
					Role:     user.role,
					IsActive: true,
					Revision: 1,
				},
			}); err != nil {
				return err
			}

			if err := repo.AppendOutbox(ctx, tx, &model.OutboxEvent{
				Type:    model.UsersChanged,
				ActorID: nil,
				UserIDs: []uuid.UUID{id},
			}); err != nil {
				return err
			}
		}

		return nil
	})
}

func loadDemoUsers() ([]seedUser, error) {
	definitions := []struct {
		id, name, email, role, passwordEnv string
	}{
		{
			"90000000-0000-4000-8000-000000000002",
			"Демо-администратор",
			"admin@flagman.invalid",
			"admin",
			"DEMO_ADMIN_PASSWORD",
		},
		{
			"90000000-0000-4000-8000-000000000003",
			"Демо-менеджер",
			"manager@flagman.invalid",
			"manager",
			"DEMO_MANAGER_PASSWORD",
		},
		{
			"90000000-0000-4000-8000-000000000004",
			"Демо-кладовщик",
			"storekeeper@flagman.invalid",
			"storekeeper",
			"DEMO_STOREKEEPER_PASSWORD",
		},
		{
			"90000000-0000-4000-8000-000000000005",
			"Демо-оператор заказов",
			"operator@flagman.invalid",
			"order_operator",
			"DEMO_ORDER_OPERATOR_PASSWORD",
		},
	}

	users := make([]seedUser, 0, len(definitions))

	for _, definition := range definitions {
		hash, err := auth.Hash(
			os.Getenv(definition.passwordEnv),
		)
		if err != nil {
			return nil, fmt.Errorf(
				"%s: %w",
				definition.passwordEnv,
				err,
			)
		}

		users = append(users, seedUser{
			id:           uuid.MustParse(definition.id),
			name:         definition.name,
			email:        definition.email,
			role:         definition.role,
			passwordHash: hash,
		})
	}

	return users, nil
}
