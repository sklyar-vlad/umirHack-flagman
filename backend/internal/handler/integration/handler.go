package integration

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	"github.com/sklyar-vlad/umirHack-flagman/internal/httpjson"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

type Service interface {
	Authenticate(context.Context, string) (dto.IntegrationPrincipal, error)
	Tokens(context.Context) ([]dto.IntegrationToken, error)
	CreateToken(context.Context, string) (dto.IntegrationToken, error)
	Revoke(context.Context, uuid.UUID) error
	CreateProduct(context.Context, dto.IntegrationPrincipal, *dto.ExternalProductRequest) (dto.Product, error)
	Submit(context.Context, dto.IntegrationPrincipal, dto.IncomingSupplyRequest) (dto.IncomingSupply, error)
	List(context.Context, string, int, int) (dto.ListResponse[dto.IncomingSupply], error)
	Accept(context.Context, uuid.UUID, dto.AcceptIncomingSupply) (dto.IncomingSupply, error)
}
type Handler struct{ service Service }

func NewHandler(s Service) *Handler         { return &Handler{service: s} }
func decode(c *gin.Context, body any) error { return httpjson.DecodeObject(c.Request.Body, body) }

func respond(c *gin.Context, status int, result any, err error) {
	c.Header("Cache-Control", "no-store")
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.JSON(status, result)
}

func (h *Handler) RequireToken(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	headers := c.Request.Header.Values("Authorization")
	if len(headers) != 1 || !strings.HasPrefix(headers[0], "Bearer ") {
		basehandler.WriteError(c, apperrors.ErrUnauthorized)
		return
	}
	p, err := h.service.Authenticate(c.Request.Context(), strings.TrimPrefix(headers[0], "Bearer "))
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.Set("integration.principal", p)
	c.Request = c.Request.WithContext(
		requestctx.WithActor(c.Request.Context(), model.Actor{ID: p.ActorID, Role: "admin"}),
	)
	c.Next()
}

func principal(c *gin.Context) dto.IntegrationPrincipal {
	return c.MustGet("integration.principal").(dto.IntegrationPrincipal)
}

func (h *Handler) Tokens(c *gin.Context) {
	result, err := h.service.Tokens(c.Request.Context())
	respond(c, 200, result, err)
}

func (h *Handler) CreateToken(c *gin.Context) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decode(c, &body); err != nil {
		basehandler.WriteError(c, err)
		return
	}
	result, err := h.service.CreateToken(c.Request.Context(), body.Name)
	respond(c, 201, result, err)
}

func (h *Handler) Revoke(c *gin.Context) {
	id, err := validations.ParseUUID("id", c.Param("id"))
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	if err = h.service.Revoke(c.Request.Context(), id); err != nil {
		basehandler.WriteError(c, err)
		return
	}
	c.Status(204)
}

func (h *Handler) CreateProduct(c *gin.Context) {
	var body dto.ExternalProductRequest
	if err := decode(c, &body); err != nil {
		basehandler.WriteError(c, err)
		return
	}
	result, err := h.service.CreateProduct(c.Request.Context(), principal(c), &body)
	respond(c, 201, result, err)
}

func (h *Handler) Submit(c *gin.Context) {
	var body dto.IncomingSupplyRequest
	if err := decode(c, &body); err != nil {
		basehandler.WriteError(c, err)
		return
	}
	result, err := h.service.Submit(c.Request.Context(), principal(c), body)
	respond(c, 201, result, err)
}

func (h *Handler) List(c *gin.Context) {
	q, err := validationsQuery(c)
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	result, err := h.service.List(c.Request.Context(), q.status, q.page, q.size)
	respond(c, 200, result, err)
}

type query struct {
	status     string
	page, size int
}

func validationsQuery(c *gin.Context) (query, error) {
	values := c.Request.URL.Query()
	if err := validations.ValidateQuery(values, "status", "page", "page_size"); err != nil {
		return query{}, err
	}
	p, err := validations.ParsePagination(values)
	if err != nil {
		return query{}, err
	}
	status, err := validations.ParseEnum(values, "status", "all", "all", "pending", "partial", "received")
	return query{status, p.Page, p.PageSize}, err
}

func (h *Handler) Accept(c *gin.Context) {
	id, err := validations.ParseUUID("id", c.Param("id"))
	if err != nil {
		basehandler.WriteError(c, err)
		return
	}
	var body dto.AcceptIncomingSupply
	if err = decode(c, &body); err != nil {
		basehandler.WriteError(c, err)
		return
	}
	result, err := h.service.Accept(c.Request.Context(), id, body)
	respond(c, 200, result, err)
}
