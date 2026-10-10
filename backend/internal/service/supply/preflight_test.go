package supply

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	apperrors "github.com/sklyar-vlad/umirHack-flagman/internal/errors"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

func TestSupplyPreflightBeforeDatabaseAccess(t *testing.T) {
	expires := "2030-10-11T12:00:00Z"
	valid := dto.CreateSupplyRequest{
		Number: "104", ReceivedAt: "2030-10-10T12:00:00Z",
		Items: []dto.CreateSupplyItem{{ProductID: uuid.NewString(), Quantity: "1", ExpiresAt: &expires}},
	}
	for _, test := range []struct {
		name, raw, field string
		change           func(*dto.CreateSupplyRequest)
	}{
		{name: "malformed JSON", raw: `{"number":`},
		{name: "trailing JSON", raw: `{} {}`},
		{name: "unknown field", raw: `{"actor_id":"forged"}`},
		{name: "null", raw: `null`},
		{name: "wrong field type", raw: `{"number":104}`},
		{name: "empty number", field: "number", change: func(i *dto.CreateSupplyRequest) { i.Number = "  " }},
		{name: "date syntax", field: "received_at", change: func(i *dto.CreateSupplyRequest) { i.ReceivedAt = "invalid" }},
		{name: "empty items", field: "items", change: func(i *dto.CreateSupplyRequest) { i.Items = nil }},
		{name: "invalid product UUID", field: "items[0].product_id", change: func(i *dto.CreateSupplyRequest) { i.Items[0].ProductID = "invalid" }},
		{name: "zero quantity", field: "items[0].quantity", change: func(i *dto.CreateSupplyRequest) { i.Items[0].Quantity = "0" }},
		{name: "expiry syntax", field: "items[0].expires_at", change: func(i *dto.CreateSupplyRequest) { v := "invalid"; i.Items[0].ExpiresAt = &v }},
		{name: "expiry shape", field: "items[0].produced_at", change: func(i *dto.CreateSupplyRequest) { i.Items[0].ExpiresAt = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := valid
			input.Items = append([]dto.CreateSupplyItem(nil), valid.Items...)
			body := []byte(test.raw)
			if test.change != nil {
				test.change(&input)
				var err error
				body, err = json.Marshal(input)
				if err != nil {
					t.Fatal(err)
				}
			}
			// Nil dependencies ensure an early rejection never attempts a transaction or cache lookup.
			s := NewService(nil, nil, nil)
			response, err := s.CreateIdempotent(context.Background(), uuid.New(), uuid.New(), body,
				func(context.Context, repository.DBTX, *model.CreatedSupply) (json.RawMessage, error) {
					t.Fatal("early invalid input reached the renderer")
					return nil, nil
				})
			if response.Status != 0 {
				t.Fatal("early failure returned a stored response")
			}
			if test.field == "" {
				if !errors.Is(err, apperrors.ErrInvalidJSON) {
					t.Fatalf("expected invalid JSON, got %v", err)
				}
			} else {
				var field *validations.FieldError
				if !errors.As(err, &field) || field.Field != test.field {
					t.Fatalf("expected field %s, got %v", test.field, err)
				}
			}
		})
	}
}
