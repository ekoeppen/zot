package provider

import (
	"context"
	"testing"
)

type routeCaptureClient struct {
	name   string
	models []string
}

func (c *routeCaptureClient) Name() string { return c.name }
func (c *routeCaptureClient) Stream(_ context.Context, req Request) (<-chan Event, error) {
	c.models = append(c.models, req.Model)
	out := make(chan Event)
	close(out)
	return out, nil
}

func TestModelRouterDispatchesByModelAPI(t *testing.T) {
	completions := &routeCaptureClient{name: "xai"}
	responses := &routeCaptureClient{name: "xai"}
	router := NewModelRouter("xai", completions, map[string]Client{
		APIResponses: responses,
	})

	for _, model := range []string{"grok-4.5", "grok-build-0.1"} {
		stream, err := router.Stream(context.Background(), Request{Model: model})
		if err != nil {
			t.Fatal(err)
		}
		for range stream {
		}
	}

	if len(responses.models) != 1 || responses.models[0] != "grok-4.5" {
		t.Fatalf("Responses models = %v", responses.models)
	}
	if len(completions.models) != 1 || completions.models[0] != "grok-build-0.1" {
		t.Fatalf("Completions models = %v", completions.models)
	}
}

func TestModelRouterRejectsMissingAPIClient(t *testing.T) {
	router := NewModelRouter("xai", &routeCaptureClient{name: "xai"}, nil)
	if _, err := router.Stream(context.Background(), Request{Model: "grok-4.5"}); err == nil {
		t.Fatal("expected missing API client error")
	}
}
