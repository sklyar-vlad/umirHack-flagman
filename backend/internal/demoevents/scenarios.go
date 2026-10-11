package demoevents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/sklyar-vlad/umirHack-flagman/internal/dto"
	"github.com/sklyar-vlad/umirHack-flagman/internal/validations"
)

const invalidResponseCode = "INVALID_RESPONSE"

func validScenario(scenario string) bool {
	switch scenario {
	case "normal", "depletion", "replenishment", "expiring":
		return true
	}
	return false
}

// Step emits at most one business mutation. Reads always refresh recipe and stock.
// Depletion consumes up to the API portion limit, then emits ordinary refusals.
func (c *Client) Step(ctx context.Context, dishID, scenario string, expiry time.Duration, out io.Writer) error {
	if !validScenario(scenario) {
		return errors.New("scenario must be normal, depletion, replenishment or expiring")
	}
	if expiry < time.Second || expiry > 720*time.Hour {
		return errors.New("expiry must be between 1s and 720h")
	}
	dish, err := c.dish(ctx, dishID)
	if err != nil {
		return err
	}
	if dish.ID == uuid.Nil || !dish.IsActive || dish.RecipeVersion == nil {
		return errors.New("selected demo dish must be active with a recipe")
	}
	if scenario == "depletion" {
		portions, err := strconv.ParseUint(dish.AvailablePortions, 10, 64)
		if err != nil {
			return &APIError{Code: invalidResponseCode}
		}
		if portions == 0 {
			portions = 1
		}
		if portions > validations.MaxPortions {
			portions = validations.MaxPortions
		}
		return c.order(ctx, &dish, int(portions), out)
	}
	var check dto.OrderCheck
	if err := c.request(ctx, "POST", "/orders/check", "", basket(&dish, 1), &check); err != nil {
		return err
	}
	if len(check.Issues) > 0 || len(check.Requirements) == 0 {
		return errors.New("technical: demo recipe changed or contains unavailable products")
	}
	if scenario == "normal" && check.CanCreate {
		return c.order(ctx, &dish, 1, out)
	}
	ttl := 72 * time.Hour
	if scenario == "expiring" {
		ttl = expiry
	}
	return c.supply(ctx, check.Requirements, ttl, out)
}

func (c *Client) supply(ctx context.Context, requirements []dto.Requirement, ttl time.Duration, out io.Writer) error {
	now := time.Now().UTC()
	expires := now.Add(ttl).Format(time.RFC3339Nano)
	key := uuid.NewString()
	input := dto.CreateSupplyRequest{
		Number:     "demo-" + key,
		ReceivedAt: now.Format(time.RFC3339Nano),
		Items:      make([]dto.CreateSupplyItem, 0, len(requirements)),
	}
	for _, req := range requirements {
		quantity, err := validations.ParseQuantity("quantity", req.RequiredQuantity)
		if err != nil || quantity <= 0 || req.ProductID == uuid.Nil || (req.Unit == "pcs" && quantity%1000 != 0) {
			return &APIError{Code: invalidResponseCode}
		}
		input.Items = append(
			input.Items,
			dto.CreateSupplyItem{
				ProductID: req.ProductID.String(),
				Quantity:  req.RequiredQuantity,
				ExpiresAt: &expires,
			},
		)
	}
	var result dto.SupplyDetail
	if err := c.request(ctx, "POST", "/supplies", key, input, &result); err != nil {
		return err
	}
	if result.ID == uuid.Nil {
		return &APIError{Code: invalidResponseCode}
	}
	if _, err := fmt.Fprintf(out, "supply created id=%s key=%s expires_at=%s\n", result.ID, key, expires); err != nil {
		return err
	}
	for i := range result.Items {
		item := &result.Items[i]
		if _, err := fmt.Fprintf(
			out,
			"batch id=%s product_id=%s expires_at=%s\n",
			item.Batch.ID,
			item.Product.ID,
			item.Batch.ExpiresAt.UTC().Format(time.RFC3339Nano),
		); err != nil {
			return err
		}
	}
	return nil
}
