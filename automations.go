package main

import (
	"context"
	"fmt"
	"time"
)

// automationsAPI is the slice of the beehiiv API surface runAutomations uses.
type automationsAPI interface {
	ListAutomations(ctx context.Context, pubID string) ([]Automation, error)
	GetJourneyStats(ctx context.Context, pubID, autoID string) (JourneyStats, error)
	ListAutomationEmails(ctx context.Context, pubID, autoID string) ([]AutomationEmail, error)
}

type automationsInput struct {
	IncludeEmails bool `json:"include_emails,omitempty"`
}

type automationsOutput struct {
	Automations []AutomationReport `json:"automations"`
	FetchedAt   time.Time          `json:"fetched_at"`
}

// AutomationReport bundles an Automation with its aggregate journey stats and
// (optionally) its ordered email sequence.
type AutomationReport struct {
	Automation
	JourneyStats JourneyStats      `json:"journey_stats"`
	Emails       []AutomationEmail `json:"emails,omitempty"`
}

func runAutomations(ctx context.Context, api automationsAPI, pubID string, in automationsInput) (automationsOutput, error) {
	autos, err := api.ListAutomations(ctx, pubID)
	if err != nil {
		return automationsOutput{}, fmt.Errorf("list automations: %w", err)
	}

	reports := make([]AutomationReport, 0, len(autos))
	for _, a := range autos {
		js, err := api.GetJourneyStats(ctx, pubID, a.ID)
		if err != nil {
			return automationsOutput{}, fmt.Errorf("journey stats for %s: %w", a.ID, err)
		}
		rep := AutomationReport{Automation: a, JourneyStats: js}

		if in.IncludeEmails {
			emails, err := api.ListAutomationEmails(ctx, pubID, a.ID)
			if err != nil {
				return automationsOutput{}, fmt.Errorf("emails for %s: %w", a.ID, err)
			}
			rep.Emails = emails
		}
		reports = append(reports, rep)
	}

	return automationsOutput{
		Automations: reports,
		FetchedAt:   time.Now().UTC(),
	}, nil
}
