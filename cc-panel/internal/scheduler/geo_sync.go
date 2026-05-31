package scheduler

import (
	"context"
	"log"
	"time"

	"github.com/example/cc-panel/internal/geo"
)

func StartGeoCIDRSync(ctx context.Context, geoService *geo.Service, interval time.Duration) {
	if interval <= 0 {
		return
	}
	go func() {
		runGeoCIDRSync(ctx, geoService, interval, true)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runGeoCIDRSync(ctx, geoService, interval, false)
			}
		}
	}()
}

func runGeoCIDRSync(ctx context.Context, geoService *geo.Service, interval time.Duration, startup bool) {
	shouldRun, err := geoService.ShouldRunAutoSync(ctx, interval)
	if err != nil {
		log.Printf("geo auto sync check: %v", err)
		return
	}
	if !shouldRun {
		if startup {
			log.Printf("geo auto sync: waiting for next interval (%s)", interval)
		}
		return
	}
	result := geoService.RunScheduledDefaultWhitelistSync(ctx)
	if result.Error != "" {
		log.Printf("geo auto sync failed: %s", result.Error)
		return
	}
	if result.Skipped {
		log.Printf("geo auto sync skipped: %s", result.SkipReason)
		return
	}
	log.Printf("geo auto sync completed: changed=%v servers=%v", result.ChangedCountries, result.DeployedServers)
}
