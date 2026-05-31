package scheduler

import (
	"context"
	"log"
	"time"

	"github.com/example/cc-panel/internal/monitor"
	"github.com/example/cc-panel/internal/policy"
)

func StartMonitorCollect(ctx context.Context, monitorService *monitor.Service, policyService *policy.Service, interval time.Duration) {
	if interval <= 0 {
		return
	}
	go func() {
		runMonitorCollect(ctx, monitorService, policyService)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runMonitorCollect(ctx, monitorService, policyService)
			}
		}
	}()
}

func runMonitorCollect(ctx context.Context, monitorService *monitor.Service, policyService *policy.Service) {
	result, err := monitorService.CollectAll(ctx)
	if err != nil {
		log.Printf("monitor collect all: %v", err)
		return
	}
	if result.Failed > 0 {
		log.Printf("monitor collect completed: ok=%d failed=%d", result.Collected, result.Failed)
	} else {
		log.Printf("monitor collect completed: ok=%d", result.Collected)
	}
	if policyService == nil {
		return
	}
	count, err := policyService.ExecuteAll(ctx)
	if err != nil {
		log.Printf("policy execute all: %v", err)
		return
	}
	if count > 0 {
		log.Printf("policy execute all completed: events=%d", count)
	}
}
