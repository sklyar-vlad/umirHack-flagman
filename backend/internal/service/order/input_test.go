package order

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

func TestNormalizeBasketMergesDishes(t *testing.T) {
	id := "abcdefab-cdef-abcd-efab-cdefabcdefab"
	other := uuid.NewString()
	version := 2
	request := dto.OrderRequest{Items: []dto.OrderRequestItem{
		{DishID: id, Portions: 400},
		{DishID: other, Portions: 1},
		{DishID: strings.ToUpper(id), Portions: 600, RecipeVersion: &version},
	}}
	items, err := normalizeBasket(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].dishID.String() != id || items[0].portions != 1000 ||
		items[1].dishID.String() != other ||
		items[1].portions != 1 {
		t.Fatalf("unexpected merged basket: %+v", items)
	}
	if items[0].recipeVersion == nil || *items[0].recipeVersion != version || items[1].recipeVersion != nil {
		t.Fatalf("unexpected versions: %+v", items)
	}
	if request.Items[0].Portions != 400 || request.Items[0].RecipeVersion != nil {
		t.Fatal("normalization mutated the request")
	}
}

func TestNormalizeBasketRejectsInvalidInput(t *testing.T) {
	id := uuid.NewString()
	zero, one, two := 0, 1, 2
	tooMany := make([]dto.OrderRequestItem, validations.MaxEntries+1)
	for i := range tooMany {
		tooMany[i] = dto.OrderRequestItem{DishID: id, Portions: 1}
	}
	cases := []struct {
		name  string
		items []dto.OrderRequestItem
		field string
	}{
		{"empty", nil, "items"},
		{"too many entries", tooMany, "items"},
		{"invalid UUID", []dto.OrderRequestItem{{DishID: "bad", Portions: 1}}, "items[0].dish_id"},
		{"nil UUID", []dto.OrderRequestItem{{DishID: uuid.Nil.String(), Portions: 1}}, "items[0].dish_id"},
		{"zero portions", []dto.OrderRequestItem{{DishID: id}}, "items[0].portions"},
		{"negative portions", []dto.OrderRequestItem{{DishID: id, Portions: -1}}, "items[0].portions"},
		{"excessive portions", []dto.OrderRequestItem{{DishID: id, Portions: 1001}}, "items[0].portions"},
		{
			"merged limit",
			[]dto.OrderRequestItem{{DishID: id, Portions: 1000}, {DishID: id, Portions: 1}},
			"items[1].portions",
		},
		{
			"invalid version",
			[]dto.OrderRequestItem{{DishID: id, Portions: 1, RecipeVersion: &zero}},
			"items[0].recipe_version",
		},
		{
			"conflicting versions",
			[]dto.OrderRequestItem{
				{DishID: id, Portions: 1, RecipeVersion: &one},
				{DishID: id, Portions: 1, RecipeVersion: &two},
			},
			"items[1].recipe_version",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := normalizeBasket(dto.OrderRequest{Items: tc.items})
			var fieldError *validations.FieldError
			if !errors.As(err, &fieldError) || fieldError.Field != tc.field {
				t.Fatalf("expected error on %s, got %v", tc.field, err)
			}
		})
	}
}

func TestOrderRequestRequiresIntegerPortions(t *testing.T) {
	for _, value := range []string{"1.5", "1.0", "\"2\"", "true", "9223372036854775808"} {
		t.Run(value, func(t *testing.T) {
			var request dto.OrderRequest
			if err := json.Unmarshal(
				[]byte(`{"items":[{"dish_id":"`+uuid.NewString()+`","portions":`+value+`}]}`),
				&request,
			); err == nil {
				t.Fatalf("accepted invalid integer portions %s", value)
			}
		})
	}
}
