package writeoff_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	basehandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	authhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/auth"
	integrationhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/integration"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	catalogrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/catalog"
	dashboardrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/dashboard"
	offersrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/hotoffers"
	menurepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/menu"
	orderrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/order"
	supplyrepo "github.com/sklyar-vlad/umirHack-flagman/internal/repository/supply"
	"github.com/sklyar-vlad/umirHack-flagman/internal/requestctx"
	catalogservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/catalog"
	dashboardservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/dashboard"
	offersservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/hotoffers"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/idempotency"
	integrationservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/integration"
	orderservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/order"
	supplyservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/supply"
	"go.uber.org/zap"
)

func (f *fixture) integrations() *integrationservice.Service {
	base := repository.NewRepository(f.pool, zap.NewNop())
	catalog := catalogservice.NewService(catalogrepo.NewRepository(), base, base)
	supply := supplyservice.NewService(supplyrepo.NewRepository(), base, idempotency.NewService(base))
	return integrationservice.NewService(
		base,
		catalog,
		supply,
		func(ctx context.Context, q repository.DBTX, input dto.CreateSupplyRequest) error {
			_, err := supply.ValidateBasics(ctx, q, input)
			return err
		},
	)
}

func (f *fixture) external(t *testing.T) (*integrationservice.Service, dto.IntegrationToken, dto.IntegrationPrincipal) {
	t.Helper()
	service := f.integrations()
	token, err := service.CreateToken(f.actor(), "Тестовый поставщик")
	if err != nil {
		t.Fatal(err)
	}
	p, err := service.Authenticate(f.ctx, token.Token)
	if err != nil {
		t.Fatal(err)
	}
	return service, token, p
}

func (f *fixture) incoming(
	t *testing.T,
	service *integrationservice.Service,
	p dto.IntegrationPrincipal,
	quantity string,
) dto.IncomingSupply {
	t.Helper()
	doc, err := service.Submit(
		f.ctx,
		p,
		dto.IncomingSupplyRequest{
			ExternalID: uuid.NewString(),
			Number:     "П-1042",
			Items: []dto.IncomingSupplyLine{
				{
					ProductID: f.product,
					Quantity:  quantity,
					ExpiresAt: time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339Nano),
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func (f *fixture) testDish(t *testing.T, product uuid.UUID, quantity string) uuid.UUID {
	t.Helper()
	dish, version := uuid.New(), uuid.New()
	f.exec(t, `INSERT INTO dishes(id,name) VALUES($1,'Тестовое блюдо')`, dish)
	f.exec(t, `INSERT INTO recipe_versions(id,dish_id,version,created_by) VALUES($1,$2,1,$3)`, version, dish, f.user)
	f.exec(
		t,
		`INSERT INTO recipe_items(recipe_version_id,product_id,quantity_per_portion) VALUES($1,$2,$3)`,
		version,
		product,
		quantity,
	)
	return dish
}

func TestExternalProductsPendingPartialAndProductFlow(t *testing.T) {
	f := newFixture(t)
	service, _, principal := f.external(t)
	input := dto.ExternalProductRequest{
		ExternalID: "product-1",
		Product: dto.CreateProductRequest{
			Name:        "Внешний продукт",
			CategoryID:  f.category,
			Unit:        "ml",
			DisplayUnit: "ml",
		},
	}
	product, err := service.CreateProduct(f.ctx, principal, &input)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := service.CreateProduct(f.ctx, principal, &input)
	if err != nil || replay.ID != product.ID {
		t.Fatal("product duplicated", replay, err)
	}
	input.Product.Name = "Другое тело"
	if _, err = service.CreateProduct(f.ctx, principal, &input); !errors.Is(err, apperrors.ErrIdempotencyConflict) {
		t.Fatal(err)
	}
	docInput := dto.IncomingSupplyRequest{
		ExternalID: "delivery-1",
		Number:     "П-1",
		Items: []dto.IncomingSupplyLine{
			{
				ProductID: product.ID,
				Quantity:  "10.000",
				ExpiresAt: time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339Nano),
			},
		},
	}
	doc, err := service.Submit(f.ctx, principal, docInput)
	if err != nil || doc.Status != "pending" {
		t.Fatal(doc, err)
	}
	replayDoc, err := service.Submit(f.ctx, principal, docInput)
	if err != nil || doc.ID != replayDoc.ID {
		t.Fatal("incoming duplicated", err)
	}
	var stock string
	if err = f.pool.QueryRow(f.ctx, `SELECT COALESCE(sum(remaining_quantity),0)::text FROM batches WHERE product_id=$1`, product.ID).
		Scan(&stock); err != nil ||
		stock != "0" {
		t.Fatal("pending affected stock", stock, err)
	}
	filter := model.AnalyticsFilter{Page: 1, PageSize: 50, ProductID: &product.ID}
	if _, total, err := f.analytics().ProductFlow(f.actor(), filter); err != nil || total != 0 {
		t.Fatal("pending affected report", total, err)
	}
	accept := dto.AcceptIncomingSupply{
		ExpectedRevision: 1,
		Items:            []dto.AcceptIncomingSupplyItem{{ID: doc.Items[0].ID, Quantity: "4"}},
	}
	partial, err := service.Accept(f.actor(), doc.ID, accept)
	if err != nil || partial.Status != "partial" || partial.Items[0].AcceptedQuantity != "4.000" ||
		len(partial.ReceiptIDs) != 1 {
		t.Fatal(partial, err)
	}
	if _, err = service.Accept(f.actor(), doc.ID, accept); err == nil {
		t.Fatal("stale acceptance succeeded")
	}
	accept.ExpectedRevision = partial.Revision
	accept.Items[0].Quantity = "7"
	if _, err = service.Accept(f.actor(), doc.ID, accept); err == nil {
		t.Fatal("over-receipt succeeded")
	}
	accept.Items[0].Quantity = "6"
	full, err := service.Accept(f.actor(), doc.ID, accept)
	if err != nil || full.Status != "received" || len(full.ReceiptIDs) != 2 {
		t.Fatal(full, err)
	}
	dish := f.testDish(t, product.ID, "2")
	base := repository.NewRepository(f.pool, zap.NewNop())
	orders := orderservice.NewCreateService(orderrepo.NewRepository(), base, idempotency.NewService(base))
	body := []byte(fmt.Sprintf(`{"items":[{"dish_id":"%s","portions":2,"recipe_version":1}]}`, dish))
	created, err := orders.CreateIdempotent(f.actor(), f.user, uuid.New(), body)
	if err != nil || created.Status != 201 {
		t.Fatal(created.Status, err, string(created.Body))
	}
	var batch uuid.UUID
	if err = f.pool.QueryRow(f.ctx, `SELECT id FROM batches WHERE product_id=$1 AND remaining_quantity>0 ORDER BY id LIMIT 1`, product.ID).
		Scan(&batch); err != nil {
		t.Fatal(err)
	}
	writeBody := []byte(fmt.Sprintf(`{"batch_id":"%s","quantity":"1","reason":"Порча"}`, batch))
	if result, err := f.writeoff.CreateIdempotent(
		f.actor(),
		f.user,
		uuid.New(),
		writeBody,
	); err != nil ||
		result.Status != 201 {
		t.Fatal(result.Status, err)
	}
	rows, total, err := f.analytics().ProductFlow(f.actor(), filter)
	if err != nil || total != 1 || len(rows) != 1 || rows[0].ReceivedQuantity != "10.000" ||
		rows[0].OrderedQuantity != "4.000" ||
		rows[0].WrittenOffQuantity != "1.000" {
		t.Fatal(rows, total, err)
	}
	var owner uuid.UUID
	if err = f.pool.QueryRow(f.ctx, `SELECT created_by FROM supplies WHERE id=$1`, full.ReceiptIDs[0]).
		Scan(&owner); err != nil ||
		owner != f.user {
		t.Fatal("acceptor missing", owner, err)
	}
}

func TestIncomingConcurrentAcceptanceAndRevokedToken(t *testing.T) {
	f := newFixture(t)
	service, token, p := f.external(t)
	doc := f.incoming(t, service, p, "10")
	request := dto.AcceptIncomingSupply{
		ExpectedRevision: doc.Revision,
		Items:            []dto.AcceptIncomingSupplyItem{{ID: doc.Items[0].ID, Quantity: "6"}},
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; _, err := service.Accept(f.actor(), doc.ID, request); results <- err }()
	}
	close(start)
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else {
			var conflict *apperrors.RevisionConflict
			if !errors.As(err, &conflict) {
				t.Fatal(err)
			}
		}
	}
	if success != 1 {
		t.Fatal("double receipt", success)
	}
	var amount string
	if err := f.pool.QueryRow(f.ctx, `SELECT sum(accepted_quantity)::text FROM incoming_supply_items WHERE incoming_supply_id=$1`, doc.ID).
		Scan(&amount); err != nil ||
		amount != "6.000" {
		t.Fatal(amount, err)
	}
	if err := service.Revoke(f.actor(), token.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(f.ctx, token.Token); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal(err)
	}
	if _, err := service.Submit(
		f.ctx,
		p,
		dto.IncomingSupplyRequest{
			ExternalID: "after-revoke",
			Number:     "П-2",
			Items: []dto.IncomingSupplyLine{
				{ProductID: f.product, Quantity: "1", ExpiresAt: time.Now().Add(time.Hour).Format(time.RFC3339)},
			},
		},
	); !errors.Is(
		err,
		apperrors.ErrUnauthorized,
	) {
		t.Fatal(err)
	}
	queue, err := service.List(f.actor(), "partial", 1, 20)
	if err != nil || queue.Total != 1 {
		t.Fatal(queue, err)
	}
	tokens, err := service.Tokens(f.actor())
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(tokens)
	if strings.Contains(string(encoded), token.Token) || strings.Contains(string(encoded), "token_hash") {
		t.Fatal("secret disclosed")
	}
}

func TestIncomingRolesExpiredLinesAndAtomicRollback(t *testing.T) {
	f := newFixture(t)
	service, _, p := f.external(t)
	doc := f.incoming(t, service, p, "5")
	waiter := uuid.New()
	f.exec(
		t,
		`INSERT INTO users(id,name,email,password_hash,role) VALUES($1,'Waiter','waiter@example.test',$2,'order_operator')`,
		waiter,
		f.hash,
	)
	ctx := requestctx.WithActor(f.ctx, model.Actor{ID: waiter, Role: "order_operator"})
	accept := dto.AcceptIncomingSupply{
		ExpectedRevision: 1,
		Items:            []dto.AcceptIncomingSupplyItem{{ID: doc.Items[0].ID, Quantity: "1"}},
	}
	if _, err := service.Accept(ctx, doc.ID, accept); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal(err)
	}
	if _, err := service.List(ctx, "all", 1, 20); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal(err)
	}
	if _, err := service.CreateToken(ctx, "wrong"); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal(err)
	}
	f.exec(
		t,
		`UPDATE incoming_supply_items SET expires_at=clock_timestamp()-interval '1 hour' WHERE incoming_supply_id=$1`,
		doc.ID,
	)
	if _, err := service.Accept(f.actor(), doc.ID, accept); err == nil {
		t.Fatal("expired stock received")
	}
	var qty string
	if err := f.pool.QueryRow(f.ctx, `SELECT accepted_quantity::text FROM incoming_supply_items WHERE id=$1`, doc.Items[0].ID).
		Scan(&qty); err != nil ||
		qty != "0.000" {
		t.Fatal(qty, err)
	}
	var count int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM incoming_supply_receipts WHERE incoming_supply_id=$1`, doc.ID).
		Scan(&count); err != nil ||
		count != 0 {
		t.Fatal("partial transaction", count, err)
	}
}

func TestHotOfferManagerPublicationAlsoAppliesToDashboard(t *testing.T) {
	f := newFixture(t)
	dish := f.testDish(t, f.product, "1")

	f.exec(t, `UPDATE batches SET expires_at=clock_timestamp()+interval '7 days' WHERE id=$1`, f.batch)
	base := repository.NewRepository(f.pool, zap.NewNop())
	preferences := offersservice.NewPreferenceService(base)
	manager := uuid.New()
	f.exec(
		t,
		`INSERT INTO users(id,name,email,password_hash,role) VALUES($1,'Manager','second@example.test',$2,'manager')`,
		manager,
		f.hash,
	)
	managerCtx := requestctx.WithActor(f.ctx, model.Actor{ID: manager, Role: "manager"})
	yes, no := true, false
	pref, err := preferences.Save(
		managerCtx,
		dish,
		dto.UpdateHotOffer{Published: &yes, Manual: &yes, Message: "Предлагать к обеду"},
	)
	if err != nil || pref.Revision != 1 {
		t.Fatal(pref, err)
	}
	offers := offersservice.NewService(offersrepo.NewRepository(), menurepo.NewRepository(), base)
	waiterCtx := requestctx.WithActor(f.ctx, model.Actor{ID: f.user, Role: "order_operator"})
	list, err := offers.List(waiterCtx, model.HotOffersFilter{Page: 1, PageSize: 20})
	if err != nil || list.Total != 1 || list.Items[0].Message != "Предлагать к обеду" || !list.Items[0].Manual {
		t.Fatal(list, err)
	}
	if _, err = preferences.Save(
		waiterCtx,
		dish,
		dto.UpdateHotOffer{Published: &no, Manual: &yes, ExpectedRevision: 1},
	); !errors.Is(
		err,
		apperrors.ErrForbidden,
	) {
		t.Fatal(err)
	}
	if _, err = preferences.Save(
		managerCtx,
		dish,
		dto.UpdateHotOffer{Published: &no, Manual: &yes, ExpectedRevision: 1},
	); err != nil {
		t.Fatal(err)
	}
	list, err = offers.List(waiterCtx, model.HotOffersFilter{Page: 1, PageSize: 20})
	if err != nil || list.Total != 0 {
		t.Fatal("hidden offer leaked", list, err)
	}
	dashboard := dashboardservice.NewService(dashboardrepo.NewRepository(), base)
	summary, err := dashboard.Get(waiterCtx, "order_operator")
	if err != nil || len(summary.HotOffers) != 0 {
		t.Fatal("dashboard leaked hidden offer", summary.HotOffers, err)
	}
	managerList, err := offers.List(managerCtx, model.HotOffersFilter{Page: 1, PageSize: 20})
	if err != nil || managerList.Total != 1 || managerList.Items[0].Published {
		t.Fatal(managerList, err)
	}
}

func TestIntegrationHTTPUsesBearerAndAllowsIndependentSupplierOrigin(t *testing.T) {
	f := newFixture(t)
	service, token, _ := f.external(t)
	h := integrationhandler.NewHandler(service)
	r := gin.New()
	r.Use(basehandler.RequestID(), authhandler.NewHandler(f.service).CORS())
	g := r.Group("/api/v1/integration", h.RequireToken)
	g.POST("/supplies", h.Submit)
	preflight := httptest.NewRequest(http.MethodOptions, "/api/v1/integration/supplies", http.NoBody)
	preflight.Header.Set("Origin", "http://127.0.0.1:5184")
	preflight.Header.Set("Access-Control-Request-Headers", "authorization,content-type")
	preflight.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, preflight)
	if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal(w.Code, w.Header())
	}
	for _, authorization := range []string{"", "Bearer " + token.Token} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/integration/supplies", strings.NewReader(`{}`))
		req.Header.Set("Origin", "http://127.0.0.1:5184")
		req.Header.Set("Authorization", authorization)
		req.AddCookie(&http.Cookie{Name: "flagman_session", Value: "cookie-is-not-an-api-token"})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		expected := 422
		if authorization == "" {
			expected = 401
		}
		if w.Code != expected {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
