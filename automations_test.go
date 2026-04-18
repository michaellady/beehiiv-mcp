package main

import (
	"context"
	"errors"
	"testing"
)

type fakeAutomationsAPI struct {
	list         []Automation
	listErr      error
	journeys     map[string]JourneyStats
	journeysErr  error
	emails       map[string][]AutomationEmail
	emailsErr    error
	emailCalls   int
	journeyCalls int
}

func (f *fakeAutomationsAPI) ListAutomations(ctx context.Context, pubID string) ([]Automation, error) {
	return f.list, f.listErr
}
func (f *fakeAutomationsAPI) GetJourneyStats(ctx context.Context, pubID, autoID string) (JourneyStats, error) {
	f.journeyCalls++
	if f.journeysErr != nil {
		return JourneyStats{}, f.journeysErr
	}
	return f.journeys[autoID], nil
}
func (f *fakeAutomationsAPI) ListAutomationEmails(ctx context.Context, pubID, autoID string) ([]AutomationEmail, error) {
	f.emailCalls++
	if f.emailsErr != nil {
		return nil, f.emailsErr
	}
	return f.emails[autoID], nil
}

func TestRunAutomations_EmptyList(t *testing.T) {
	api := &fakeAutomationsAPI{}
	out, err := runAutomations(context.Background(), api, "pub_1", automationsInput{})
	if err != nil {
		t.Fatalf("runAutomations: %v", err)
	}
	if len(out.Automations) != 0 {
		t.Errorf("Automations = %d, want 0", len(out.Automations))
	}
	if out.FetchedAt.IsZero() {
		t.Errorf("FetchedAt should be set")
	}
}

func TestRunAutomations_FetchesJourneyStatsPerAutomation(t *testing.T) {
	api := &fakeAutomationsAPI{
		list: []Automation{
			{ID: "a1", Name: "Welcome", Status: "active", Trigger: "subscription_created", EmailCount: 5},
			{ID: "a2", Name: "Upsell", Status: "paused", Trigger: "tag_added", EmailCount: 3},
		},
		journeys: map[string]JourneyStats{
			"a1": {Active: 87, Completed: 412, Exited: 23},
			"a2": {Active: 10, Completed: 40, Exited: 5},
		},
	}

	out, err := runAutomations(context.Background(), api, "pub_1", automationsInput{})
	if err != nil {
		t.Fatalf("runAutomations: %v", err)
	}
	if len(out.Automations) != 2 {
		t.Fatalf("Automations = %d, want 2", len(out.Automations))
	}
	if out.Automations[0].JourneyStats.Active != 87 {
		t.Errorf("a1 Active = %d, want 87", out.Automations[0].JourneyStats.Active)
	}
	if out.Automations[1].JourneyStats.Completed != 40 {
		t.Errorf("a2 Completed = %d, want 40", out.Automations[1].JourneyStats.Completed)
	}
	if api.journeyCalls != 2 {
		t.Errorf("journeyCalls = %d, want 2", api.journeyCalls)
	}
	if api.emailCalls != 0 {
		t.Errorf("emailCalls = %d, want 0 when IncludeEmails is false", api.emailCalls)
	}
	if len(out.Automations[0].Emails) != 0 {
		t.Errorf("Emails should be empty when IncludeEmails=false; got %d", len(out.Automations[0].Emails))
	}
}

func TestRunAutomations_IncludeEmailsFetchesPerAutomation(t *testing.T) {
	api := &fakeAutomationsAPI{
		list: []Automation{
			{ID: "a1", Name: "Welcome", Status: "active", EmailCount: 2},
		},
		journeys: map[string]JourneyStats{"a1": {Active: 10}},
		emails: map[string][]AutomationEmail{
			"a1": {
				{ID: "e1", Subject: "Welcome!", Position: 1, DelayDays: 0, Opens: 408, OpenRate: 0.82, Clicks: 101, ClickRate: 0.20},
				{ID: "e2", Subject: "Deep dive", Position: 2, DelayDays: 3, Opens: 380, OpenRate: 0.76, Clicks: 80, ClickRate: 0.16},
			},
		},
	}

	out, err := runAutomations(context.Background(), api, "pub_1", automationsInput{IncludeEmails: true})
	if err != nil {
		t.Fatalf("runAutomations: %v", err)
	}
	if api.emailCalls != 1 {
		t.Errorf("emailCalls = %d, want 1", api.emailCalls)
	}
	if len(out.Automations[0].Emails) != 2 {
		t.Errorf("Emails = %d, want 2", len(out.Automations[0].Emails))
	}
	if out.Automations[0].Emails[1].Subject != "Deep dive" {
		t.Errorf("Emails[1].Subject = %q", out.Automations[0].Emails[1].Subject)
	}
}

func TestRunAutomations_ListErrorPropagates(t *testing.T) {
	sentinel := errors.New("upstream down")
	api := &fakeAutomationsAPI{listErr: sentinel}
	_, err := runAutomations(context.Background(), api, "pub_1", automationsInput{})
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want to wrap %v", err, sentinel)
	}
}

func TestRunAutomations_JourneyErrorPropagates(t *testing.T) {
	sentinel := errors.New("journey unavailable")
	api := &fakeAutomationsAPI{
		list:        []Automation{{ID: "a1"}},
		journeysErr: sentinel,
	}
	_, err := runAutomations(context.Background(), api, "pub_1", automationsInput{})
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want to wrap %v", err, sentinel)
	}
}
