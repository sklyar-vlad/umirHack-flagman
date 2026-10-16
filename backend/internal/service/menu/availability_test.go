package menu

import (
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/model"
)

func TestCalculateAvailability(t *testing.T) {
	ingredient := func(required, available, expired string) model.RecipeIngredient {
		return model.RecipeIngredient{
			ProductID:          uuid.New(),
			ProductName:        "Курица",
			Unit:               "g",
			QuantityPerPortion: required,
			AvailableQuantity:  available,
			ExpiredQuantity:    expired,
		}
	}
	tests := []struct {
		name     string
		active   bool
		recipe   []model.RecipeIngredient
		portions string
		reasons  []string
	}{
		{
			"limiting ingredient",
			true,
			[]model.RecipeIngredient{ingredient("120.000", "1200.000", "0"), ingredient("300.000", "899.999", "0")},
			"2",
			[]string{},
		},
		{"exact decimal boundary", true, []model.RecipeIngredient{ingredient("0.333", "0.999", "0")}, "3", []string{}},
		{
			"no chicken",
			true,
			[]model.RecipeIngredient{ingredient("300", "0", "0")},
			"0",
			[]string{"ingredient_shortage"},
		},
		{
			"expired cream",
			true,
			[]model.RecipeIngredient{ingredient("100", "0", "900")},
			"0",
			[]string{"only_expired_stock"},
		},
		{
			"mixed insufficient",
			true,
			[]model.RecipeIngredient{ingredient("100", "20", "900")},
			"0",
			[]string{"ingredient_shortage"},
		},
		{
			"two reasons",
			true,
			[]model.RecipeIngredient{ingredient("100", "0", "0"), ingredient("20", "0", "100")},
			"0",
			[]string{"ingredient_shortage", "only_expired_stock"},
		},
		{"missing recipe", true, nil, "0", []string{"recipe_missing"}},
		{"inactive", false, []model.RecipeIngredient{ingredient("1", "100", "0")}, "0", []string{"dish_inactive"}},
		{
			"aggregate exceeds int64",
			true,
			[]model.RecipeIngredient{ingredient("0.001", "99999999999999999999.999", "0")},
			"99999999999999999999999",
			[]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CalculateAvailability(tt.active, tt.recipe)
			if err != nil {
				t.Fatal(err)
			}
			if result.AvailablePortions != tt.portions || result.IsAvailable != (tt.portions != "0") ||
				!reflect.DeepEqual(result.UnavailabilityReasons, tt.reasons) {
				t.Fatalf("unexpected result: %+v", result)
			}
			if result.Shortages == nil {
				t.Fatal("shortages must serialize as []")
			}
		})
	}
	result, err := CalculateAvailability(true, []model.RecipeIngredient{ingredient("100", "20.123", "900")})
	if err != nil {
		t.Fatal(err)
	}
	shortage := result.Shortages[0]
	if shortage.MissingQuantity != "79.877" || shortage.RequiredQuantity != "100.000" ||
		shortage.ExpiredQuantity != "900.000" {
		t.Fatalf("incorrect shortage: %+v", shortage)
	}
}

func TestCalculateAvailabilityRejectsInvalidQuantities(t *testing.T) {
	for _, raw := range []string{"0", "-1", "NaN", "1e3", "1.0001", "+1", "1."} {
		_, err := CalculateAvailability(
			true,
			[]model.RecipeIngredient{{QuantityPerPortion: raw, AvailableQuantity: "100", ExpiredQuantity: "0"}},
		)
		if err == nil {
			t.Errorf("accepted recipe quantity %q", raw)
		}
	}
	_, err := CalculateAvailability(
		true,
		[]model.RecipeIngredient{{QuantityPerPortion: "1", AvailableQuantity: "-1", ExpiredQuantity: "0"}},
	)
	if err == nil {
		t.Fatal("accepted negative stock")
	}
}
