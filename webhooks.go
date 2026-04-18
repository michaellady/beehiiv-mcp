package main

import (
	"context"
	"fmt"
	"time"
)

type webhooksAPI interface {
	ListWebhooks(ctx context.Context, pubID string) ([]Webhook, error)
}

type webhooksOutput struct {
	Webhooks     []Webhook `json:"webhooks"`
	ActiveCount  int       `json:"active_count"`
	FailingCount int       `json:"failing_count"`
	FetchedAt    time.Time `json:"fetched_at"`
}

func runWebhooks(ctx context.Context, api webhooksAPI, pubID string, now time.Time) (webhooksOutput, error) {
	hooks, err := api.ListWebhooks(ctx, pubID)
	if err != nil {
		return webhooksOutput{}, fmt.Errorf("list webhooks: %w", err)
	}
	out := webhooksOutput{
		Webhooks:  hooks,
		FetchedAt: now.UTC(),
	}
	for _, h := range hooks {
		if h.Status == "active" {
			out.ActiveCount++
		}
		if h.LastDeliveryStatus == "failure" {
			out.FailingCount++
		}
	}
	return out, nil
}
