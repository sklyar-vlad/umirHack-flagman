package supply

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/idempotency"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type Repository interface {
	FindProducts(
		context.Context,
		repository.DBTX,
		[]uuid.UUID,
	) (map[uuid.UUID]model.SupplyProduct, error)

	NumberExists(
		context.Context,
		repository.DBTX,
		string,
	) (bool, error)
	InsertSupply(context.Context, repository.DBTX, model.NewSupply) (time.Time, error)
	InsertBatch(context.Context, repository.DBTX, *model.NewSupplyBatch) error
	InsertReceipt(context.Context, repository.DBTX, *model.NewSupplyReceipt) error
	FindDetails(context.Context, repository.DBTX, uuid.UUID) (*model.SupplyDetails, error)
}

type Transactions interface {
	WithinTx(context.Context, *pgx.TxOptions, func(repository.DBTX) error) error
	AppendAudit(context.Context, repository.DBTX, *model.AuditEntry) error
	AppendOutbox(context.Context, repository.DBTX, *model.OutboxEvent) error
}

type Service struct {
	repo         Repository
	transactions Transactions
	idempotency  *idempotency.Service
	now          func() time.Time
}

func NewService(repo Repository, transactions Transactions, idem *idempotency.Service) *Service {
	return &Service{
		repo:         repo,
		transactions: transactions,
		idempotency:  idem,
		now:          time.Now,
	}
}

type Basics struct {
	Number     string
	ReceivedAt time.Time
	ServerTime time.Time
	Items      []Line
}

type Line struct {
	ProductID     uuid.UUID
	ProductName   string
	QuantityMilli int64
	Unit          string
	Expiry        model.BatchExpiry
}

func (s *Service) ValidateBasics(
	ctx context.Context,
	tx repository.DBTX,
	input dto.CreateSupplyRequest,
) (Basics, error) {
	prepared, err := prepareSupplyInput(input)
	if err != nil {
		return Basics{}, err
	}
	number, receivedAt := prepared.Number, prepared.ReceivedAt
	items, ids := prepared.Items, prepared.ProductIDs

	products, err := s.repo.FindProducts(ctx, tx, ids)
	if err != nil {
		return Basics{}, err
	}

	serverTime := s.now().UTC()

	if receivedAt.After(serverTime) {
		return Basics{}, &validations.FieldError{
			Field:   "received_at",
			Message: "Дата приёмки не может быть в будущем",
		}
	}

	for i := range items {
		product, exists := products[items[i].ProductID]

		if !exists || !product.IsActive {
			return Basics{}, &validations.FieldError{
				Field: fmt.Sprintf("items[%d].product_id", i),
				Message: "Продукт должен существовать " +
					"и быть активным",
			}
		}

		switch product.Unit {
		case "g", "ml":
		case "pcs":
			if items[i].QuantityMilli%1000 != 0 {
				return Basics{}, &validations.FieldError{
					Field: fmt.Sprintf("items[%d].quantity", i),
					Message: "Для штучного продукта " +
						"количество должно быть целым",
				}
			}
		default:
			return Basics{}, fmt.Errorf(
				"unknown product unit %q",
				product.Unit,
			)
		}

		items[i].Unit = product.Unit
		items[i].ProductName = product.Name

		expiry, err := resolveExpiry(
			fmt.Sprintf("items[%d]", i),
			input.Items[i],
			product,
			receivedAt,
			serverTime,
		)
		if err != nil {
			return Basics{}, err
		}
		items[i].Expiry = expiry
	}

	exists, err := s.repo.NumberExists(ctx, tx, number)
	if err != nil {
		return Basics{}, err
	}

	if exists {
		return Basics{}, fmt.Errorf(
			"%w: supply number already exists",
			apperrors.ErrConflict,
		)
	}

	return Basics{
		Number:     number,
		ReceivedAt: receivedAt,
		ServerTime: serverTime,
		Items:      items,
	}, nil
}
