package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sklyar-vlad/umirHack-flagman/internal/demoevents"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, getenv func(string) string, out, errOut io.Writer) int {
	flags := flag.NewFlagSet("demo-events", flag.ContinueOnError)
	flags.SetOutput(errOut)
	destination := flags.String("demo-url", "", "explicit origin of the dedicated demo API (no default)")
	origin := flags.String("origin", "", "exact configured frontend origin")
	confirm := flags.Bool("confirm-demo", false, "confirm that the selected API uses a dedicated demo database")
	options := demoevents.Options{}
	flags.StringVar(&options.DishID, "dish-id", "", "dish UUID; default: first active recipe (pinned for the run)")
	flags.StringVar(&options.Scenario, "scenario", "normal", "normal, depletion, replenishment or expiring")
	flags.IntVar(&options.Count, "count", 10, "maximum business mutations, including refusals (1-10000)")
	flags.DurationVar(&options.Interval, "interval", time.Second, "delay between events (0-24h)")
	flags.DurationVar(&options.Expiry, "expiry", 30*time.Second, "expiring scenario batch lifetime (1s-720h)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(errOut, "unexpected positional arguments")
		return 2
	}
	if !*confirm {
		fmt.Fprintln(errOut, "--confirm-demo is required for the explicitly selected demo destination")
		return 2
	}
	if err := options.Validate(); err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	client, err := demoevents.NewClient(getenv("ENV"), *destination, *origin)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	err = client.Run(ctx, getenv("DEMO_MANAGER_PASSWORD"), options, out)
	if errors.Is(err, context.Canceled) {
		fmt.Fprintln(errOut, "demo-events stopped")
		return 130
	}
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	return 0
}
