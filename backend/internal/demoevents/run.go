package demoevents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
)

type Options struct {
	DishID   string
	Scenario string
	Count    int
	Interval time.Duration
	Expiry   time.Duration
}

func (o Options) Validate() error {
	if o.Count < 1 || o.Count > 10000 {
		return errors.New("count must be between 1 and 10000")
	}
	if o.Interval < 0 || o.Interval > 24*time.Hour {
		return errors.New("interval must be between 0 and 24h")
	}
	if !validScenario(o.Scenario) {
		return errors.New("scenario must be normal, depletion, replenishment or expiring")
	}
	if o.Expiry < time.Second || o.Expiry > 720*time.Hour {
		return errors.New("expiry must be between 1s and 720h")
	}
	if o.DishID != "" {
		id, err := uuid.Parse(o.DishID)
		if err != nil || id == uuid.Nil {
			return errors.New("dish-id must be a nonzero UUID")
		}
	}
	return nil
}

func (c *Client) Run(ctx context.Context, password string, options Options, out io.Writer) (err error) {
	if err := options.Validate(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := c.Login(ctx, password); err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		logoutErr := c.Logout(cleanup)
		if err == nil {
			err = logoutErr
		}
	}()
	dish, err := c.dish(ctx, options.DishID)
	if err != nil {
		return err
	}
	if _, err = fmt.Fprintf(
		out,
		"demo scenario=%s dish_id=%s count=%d interval=%s\n",
		options.Scenario,
		dish.ID,
		options.Count,
		options.Interval,
	); err != nil {
		return err
	}
	for i := 0; i < options.Count; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := c.Step(ctx, dish.ID.String(), options.Scenario, options.Expiry, out); err != nil {
			return err
		}
		if i+1 < options.Count && options.Interval > 0 {
			timer := time.NewTimer(options.Interval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return nil
}
