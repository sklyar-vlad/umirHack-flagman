package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	integrationrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/integration"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

const roleAdmin = "admin"

type Transactions interface {
	AppendAudit(context.Context, repository.DBTX, *model.AuditEntry) error
	WithinTx(context.Context, *pgx.TxOptions, func(repository.DBTX) error) error
	AppendOutbox(context.Context, repository.DBTX, *model.OutboxEvent) error
}
type Catalog interface {
	CreateProductWithinTransaction(context.Context, repository.DBTX, *dto.CreateProductRequest) (dto.Product, error)
	GetProduct(context.Context, uuid.UUID) (dto.Product, error)
}

type SupplyCreator interface {
	CreateWithinTransaction(
		context.Context,
		repository.DBTX,
		uuid.UUID,
		dto.CreateSupplyRequest,
	) (model.CreatedSupply, error)
}
type (
	SupplyValidator func(context.Context, repository.DBTX, dto.CreateSupplyRequest) error
	Service         struct {
		tx       Transactions
		repo     *integrationrepo.Repository
		catalog  Catalog
		supplies SupplyCreator
		validate SupplyValidator
	}
)

func NewService(tx Transactions, c Catalog, s SupplyCreator, validate SupplyValidator) *Service {
	return &Service{tx: tx, repo: integrationrepo.NewRepository(), catalog: c, supplies: s, validate: validate}
}

func field(name, message string) error { return &validations.FieldError{Field: name, Message: message} }

func text(value string, maxRunes int) bool {
	return utf8.ValidString(value) && strings.TrimSpace(value) != "" && utf8.RuneCountInString(value) <= maxRunes &&
		!strings.ContainsRune(value, 0)
}

func hashBody(value any) []byte {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return sum[:]
}

func (s *Service) Authenticate(ctx context.Context, raw string) (dto.IntegrationPrincipal, error) {
	if !strings.HasPrefix(raw, "flagman_") || len(raw) != 51 {
		return dto.IntegrationPrincipal{}, apperrors.ErrUnauthorized
	}
	hash := sha256.Sum256([]byte(raw))
	var p dto.IntegrationPrincipal
	err := s.tx.WithinTx(ctx, &pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(q repository.DBTX) error {
		var err error
		p, err = s.repo.Authenticate(ctx, q, hash[:], false)
		return err
	})
	return p, err
}

func (s *Service) internal(ctx context.Context, admin bool, fn func(repository.DBTX, model.Actor) error) error {
	a, ok := requestctx.Actor(ctx)
	if !ok {
		return apperrors.ErrUnauthorized
	}
	if a.Role != roleAdmin && (admin || a.Role != "manager" && a.Role != "storekeeper") {
		return apperrors.ErrForbidden
	}
	return s.tx.WithinTx(ctx, nil, func(q repository.DBTX) error {
		if err := s.repo.LockActor(ctx, q, a.ID, admin); err != nil {
			return err
		}
		return fn(q, a)
	})
}

func (s *Service) external(
	ctx context.Context,
	p dto.IntegrationPrincipal,
	fn func(context.Context, repository.DBTX) error,
) error {
	return s.tx.WithinTx(ctx, nil, func(q repository.DBTX) error {
		if err := s.repo.LockToken(ctx, q, p); err != nil {
			return err
		}
		return fn(requestctx.WithActor(ctx, model.Actor{ID: p.ActorID, Role: roleAdmin}), q)
	})
}

func (s *Service) Tokens(ctx context.Context) ([]dto.IntegrationToken, error) {
	var result []dto.IntegrationToken
	err := s.internal(ctx, true, func(q repository.DBTX, _ model.Actor) error {
		var err error
		result, err = s.repo.Tokens(ctx, q)
		return err
	})
	return result, err
}

func (s *Service) CreateToken(ctx context.Context, name string) (dto.IntegrationToken, error) {
	name = strings.TrimSpace(name)
	if !text(name, 100) {
		return dto.IntegrationToken{}, field("name", "Название должно содержать 1–100 символов")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return dto.IntegrationToken{}, err
	}
	token := "flagman_" + base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	var result dto.IntegrationToken
	err := s.internal(ctx, true, func(q repository.DBTX, a model.Actor) error {
		var err error
		result, err = s.repo.CreateToken(ctx, q, a.ID, name, hash[:])
		if err != nil {
			return err
		}
		return s.tx.AppendAudit(
			ctx,
			q,
			&model.AuditEntry{
				ActorID:    &a.ID,
				Action:     model.AuditCreate,
				EntityType: model.AuditIntegrationToken,
				EntityID:   &result.ID,
				After:      &model.WorkflowAuditSnapshot{Name: name, Status: "active"},
			},
		)
	})
	if err != nil {
		return dto.IntegrationToken{}, err
	}
	result.Token = token
	return result, nil
}

func (s *Service) Revoke(ctx context.Context, id uuid.UUID) error {
	return s.internal(ctx, true, func(q repository.DBTX, a model.Actor) error {
		found, err := s.repo.Token(ctx, q, id)
		if err != nil {
			return err
		}
		if found.RevokedAt != nil {
			return nil
		}
		if err := s.repo.Revoke(ctx, q, id); err != nil {
			return err
		}
		return s.tx.AppendAudit(
			ctx,
			q,
			&model.AuditEntry{
				ActorID:    &a.ID,
				Action:     model.AuditUpdate,
				EntityType: model.AuditIntegrationToken,
				EntityID:   &id,
				Before:     &model.WorkflowAuditSnapshot{Name: found.Name, Status: "active"},
				After:      &model.WorkflowAuditSnapshot{Name: found.Name, Status: "revoked"},
			},
		)
	})
}

func (s *Service) CreateProduct(
	ctx context.Context,
	p dto.IntegrationPrincipal,
	input *dto.ExternalProductRequest,
) (dto.Product, error) {
	if !text(input.ExternalID, 100) {
		return dto.Product{}, field("external_id", "Укажите внешний идентификатор до 100 символов")
	}
	var result dto.Product
	var replay uuid.UUID
	err := s.external(ctx, p, func(ctx context.Context, q repository.DBTX) error {
		if err := s.repo.LockExternal(ctx, q, p.TokenID, "product", input.ExternalID); err != nil {
			return err
		}
		id, hash, err := s.repo.ProductMapping(ctx, q, p.TokenID, input.ExternalID)
		if err == nil {
			if !bytes.Equal(hash, hashBody(input)) {
				return apperrors.ErrIdempotencyConflict
			}
			replay = id
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		result, err = s.catalog.CreateProductWithinTransaction(ctx, q, &input.Product)
		if err != nil {
			return err
		}
		if err := s.repo.MapProduct(ctx, q, p.TokenID, input.ExternalID, hashBody(input), result.ID); err != nil {
			return err
		}
		return s.tx.AppendOutbox(
			ctx,
			q,
			&model.OutboxEvent{Type: model.ProductsChanged, ActorID: &p.ActorID, ProductIDs: []uuid.UUID{result.ID}},
		)
	})
	if err != nil {
		return dto.Product{}, err
	}
	if replay != uuid.Nil {
		return s.catalog.GetProduct(requestctx.WithActor(ctx, model.Actor{ID: p.ActorID, Role: roleAdmin}), replay)
	}
	return result, nil
}

func (s *Service) Submit(
	ctx context.Context,
	p dto.IntegrationPrincipal,
	input dto.IncomingSupplyRequest,
) (dto.IncomingSupply, error) {
	if !text(input.ExternalID, 100) {
		return dto.IncomingSupply{}, field("external_id", "Укажите внешний идентификатор до 100 символов")
	}
	if !text(input.Number, 80) {
		return dto.IncomingSupply{}, field("number", "Укажите номер до 80 символов")
	}
	if err := validations.ValidateCount("items", len(input.Items)); err != nil {
		return dto.IncomingSupply{}, err
	}
	var result dto.IncomingSupply
	err := s.external(ctx, p, func(ctx context.Context, q repository.DBTX) error {
		if err := s.repo.LockExternal(ctx, q, p.TokenID, "supply", input.ExternalID); err != nil {
			return err
		}
		id, hash, err := s.repo.FindIncoming(ctx, q, p.TokenID, input.ExternalID)
		if err == nil {
			if !bytes.Equal(hash, hashBody(input)) {
				return apperrors.ErrIdempotencyConflict
			}
			result, err = s.repo.GetIncoming(ctx, q, id, false)
			return err
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		check := dto.CreateSupplyRequest{
			Number:     "INBOX-" + uuid.NewString(),
			ReceivedAt: time.Now().UTC().Format(time.RFC3339Nano),
			Items:      []dto.CreateSupplyItem{},
		}
		for _, item := range input.Items {
			expiry := item.ExpiresAt
			check.Items = append(
				check.Items,
				dto.CreateSupplyItem{ProductID: item.ProductID.String(), Quantity: item.Quantity, ExpiresAt: &expiry},
			)
		}
		if err := s.validate(ctx, q, check); err != nil {
			return err
		}
		id, err = s.repo.InsertIncoming(ctx, q, p.TokenID, input, hashBody(input))
		if err != nil {
			return err
		}
		result, err = s.repo.GetIncoming(ctx, q, id, false)
		if err != nil {
			return err
		}
		if err := s.tx.AppendAudit(
			ctx,
			q,
			&model.AuditEntry{
				ActorID:    &p.ActorID,
				Action:     model.AuditCreate,
				EntityType: model.AuditIncomingSupply,
				EntityID:   &id,
				After:      incomingSnapshot(&result),
			},
		); err != nil {
			return err
		}

		return s.tx.AppendOutbox(ctx, q, &model.OutboxEvent{Type: model.IncomingSuppliesChanged, ActorID: &p.ActorID})
	})
	return result, err
}

func (s *Service) List(
	ctx context.Context,
	status string,
	page, size int,
) (dto.ListResponse[dto.IncomingSupply], error) {
	var result dto.ListResponse[dto.IncomingSupply]
	if _, err := validations.PaginationOffset(dto.Pagination{Page: page, PageSize: size}); err != nil {
		return result, err
	}
	if status != "all" && status != "pending" && status != "partial" && status != "received" {
		return result, field("status", "Неизвестный статус")
	}
	err := s.internal(ctx, false, func(q repository.DBTX, _ model.Actor) error {
		var err error
		result, err = s.repo.ListIncoming(ctx, q, status, page, size)
		return err
	})
	return result, err
}

func (s *Service) Accept(
	ctx context.Context,
	id uuid.UUID,
	input dto.AcceptIncomingSupply,
) (dto.IncomingSupply, error) {
	if input.ExpectedRevision < 1 {
		return dto.IncomingSupply{}, field("expected_revision", "Укажите текущую ревизию")
	}
	if err := validations.ValidateCount("items", len(input.Items)); err != nil {
		return dto.IncomingSupply{}, err
	}
	var result dto.IncomingSupply
	err := s.internal(ctx, false, func(q repository.DBTX, a model.Actor) error {
		doc, err := s.repo.GetIncoming(ctx, q, id, true)
		if err != nil {
			return err
		}
		if doc.Revision != input.ExpectedRevision {
			return &apperrors.RevisionConflict{CurrentRevision: doc.Revision}
		}
		if doc.Status == "received" {
			return apperrors.ErrConflict
		}
		byID := map[uuid.UUID]*dto.IncomingSupplyItem{}
		for i := range doc.Items {
			line := &doc.Items[i]
			byID[line.ID] = line
		}
		seen := map[uuid.UUID]bool{}
		receipt := dto.CreateSupplyRequest{
			Number:     fmt.Sprintf("EXT-%s-%d", doc.ID, doc.Revision),
			ReceivedAt: time.Now().UTC().Format(time.RFC3339Nano),
			Items:      []dto.CreateSupplyItem{},
		}
		for i, item := range input.Items {
			line, ok := byID[item.ID]
			if !ok || seen[item.ID] {
				return field(fmt.Sprintf("items[%d].id", i), "Строка отсутствует или повторяется")
			}
			seen[item.ID] = true
			amount, err := validations.ParseQuantity(fmt.Sprintf("items[%d].quantity", i), item.Quantity)
			if err != nil {
				return err
			}
			expected, err := validations.ParseQuantity("quantity", line.Quantity)
			if err != nil {
				return err
			}
			accepted, err := validations.ParseQuantity("accepted_quantity", line.AcceptedQuantity)
			if err != nil {
				return err
			}
			if amount <= 0 || amount > expected-accepted {
				return field(
					fmt.Sprintf("items[%d].quantity", i),
					"Принимаемое количество должно быть больше нуля и не превышать остаток поставки",
				)
			}
			expiry := line.ExpiresAt.UTC().Format(time.RFC3339Nano)
			receipt.Items = append(
				receipt.Items,
				dto.CreateSupplyItem{ProductID: line.ProductID.String(), Quantity: item.Quantity, ExpiresAt: &expiry},
			)
		}
		created, err := s.supplies.CreateWithinTransaction(ctx, q, a.ID, receipt)
		if err != nil {
			return err
		}
		for _, item := range input.Items {
			if err := s.repo.AcceptLine(ctx, q, item.ID, item.Quantity); err != nil {
				return err
			}
		}
		if err := s.repo.FinishReceipt(ctx, q, id, created.ID); err != nil {
			return err
		}
		result, err = s.repo.GetIncoming(ctx, q, id, false)
		if err != nil {
			return err
		}
		return s.tx.AppendAudit(
			ctx,
			q,
			&model.AuditEntry{
				ActorID:    &a.ID,
				Action:     model.AuditUpdate,
				EntityType: model.AuditIncomingSupply,
				EntityID:   &id,
				Before:     incomingSnapshot(&doc),
				After:      incomingSnapshot(&result),
			},
		)
	})
	return result, err
}

func incomingSnapshot(doc *dto.IncomingSupply) *model.WorkflowAuditSnapshot {
	result := &model.WorkflowAuditSnapshot{
		Name:       doc.Number,
		Status:     doc.Status,
		Revision:   doc.Revision,
		ExternalID: doc.ExternalID,
		Items:      []model.WorkflowLineAudit{},
	}
	for i := range doc.Items {
		item := &doc.Items[i]
		result.Items = append(
			result.Items,
			model.WorkflowLineAudit{
				ID:               item.ID,
				ProductID:        item.ProductID,
				Quantity:         item.Quantity,
				AcceptedQuantity: item.AcceptedQuantity,
			},
		)
	}
	return result
}
