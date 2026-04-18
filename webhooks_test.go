package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeWebhooksAPI struct {
	hooks []Webhook
	err   error
}

func (f *fakeWebhooksAPI) ListWebhooks(ctx context.Context, pubID string) ([]Webhook, error) {
	return f.hooks, f.err
}

func TestRunWebhooks_EmptyList(t *testing.T) {
	api := &fakeWebhooksAPI{}
	out, err := runWebhooks(context.Background(), api, "pub_1", time.Now())
	if err != nil {
		t.Fatalf("runWebhooks: %v", err)
	}
	if len(out.Webhooks) != 0 {
		t.Errorf("Webhooks = %d, want 0", len(out.Webhooks))
	}
	if out.ActiveCount != 0 || out.FailingCount != 0 {
		t.Errorf("counts = (%d, %d), want 0, 0", out.ActiveCount, out.FailingCount)
	}
}

func TestRunWebhooks_CountsActiveAndFailing(t *testing.T) {
	now := time.Date(2026, 4, 18, 20, 0, 0, 0, time.UTC)
	api := &fakeWebhooksAPI{hooks: []Webhook{
		{ID: "w1", Status: "active", LastDeliveryStatus: "success"},
		{ID: "w2", Status: "active", LastDeliveryStatus: "failure"},
		{ID: "w3", Status: "paused", LastDeliveryStatus: "success"},
		{ID: "w4", Status: "active", LastDeliveryStatus: "unknown"},
	}}

	out, err := runWebhooks(context.Background(), api, "pub_1", now)
	if err != nil {
		t.Fatalf("runWebhooks: %v", err)
	}
	if len(out.Webhooks) != 4 {
		t.Fatalf("Webhooks = %d, want 4", len(out.Webhooks))
	}
	if out.ActiveCount != 3 {
		t.Errorf("ActiveCount = %d, want 3", out.ActiveCount)
	}
	if out.FailingCount != 1 {
		t.Errorf("FailingCount = %d, want 1", out.FailingCount)
	}
}

func TestRunWebhooks_FetchedAtIsNow(t *testing.T) {
	now := time.Date(2026, 4, 18, 20, 0, 0, 0, time.UTC)
	out, _ := runWebhooks(context.Background(), &fakeWebhooksAPI{}, "pub_1", now)
	if !out.FetchedAt.Equal(now.UTC()) {
		t.Errorf("FetchedAt = %v, want %v", out.FetchedAt, now.UTC())
	}
}

func TestRunWebhooks_ListErrorPropagates(t *testing.T) {
	sentinel := errors.New("nope")
	api := &fakeWebhooksAPI{err: sentinel}
	_, err := runWebhooks(context.Background(), api, "pub_1", time.Now())
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want to wrap %v", err, sentinel)
	}
}
