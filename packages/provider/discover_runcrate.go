package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
)

// DiscoverRuncrate lists tool-capable chat models from Runcrate's public
// catalog. Image, video, audio and embedding models are excluded. Runcrate
// reports pricing.input and pricing.output in USD per 1M tokens.
func DiscoverRuncrate(ctx context.Context, baseURL string) ([]Model, error) {
	if baseURL == "" {
		baseURL = runcrateDefaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("runcrate discover: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("runcrate discover http %d", resp.StatusCode)
	}
	var page struct {
		Data []struct {
			ID                string   `json:"id"`
			Name              string   `json:"name"`
			InputModalities   []string `json:"input_modalities"`
			OutputModalities  []string `json:"output_modalities"`
			ContextLength     int      `json:"context_length"`
			MaxOutputLength   int      `json:"max_output_length"`
			SupportedFeatures []string `json:"supported_features"`
			Pricing           struct {
				Input  float64 `json:"input"`
				Output float64 `json:"output"`
			} `json:"pricing"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&page); err != nil {
		return nil, fmt.Errorf("runcrate discover parse: %w", err)
	}
	if page.Data == nil {
		return nil, fmt.Errorf("runcrate discover response is missing data")
	}
	out := make([]Model, 0, len(page.Data))
	seen := make(map[string]bool)
	for _, d := range page.Data {
		id := strings.TrimSpace(d.ID)
		chat := slices.Equal(d.OutputModalities, []string{"text"}) && slices.Contains(d.InputModalities, "text")
		if id == "" || seen[id] || !chat || !slices.Contains(d.SupportedFeatures, "tools") {
			continue
		}
		seen[id] = true
		display := d.Name
		if display == "" {
			display = id
		}
		window, output := d.ContextLength, d.MaxOutputLength
		if window <= 0 {
			window = customDiscoveryContextWindow
		}
		if output <= 0 || output > window {
			output = min(customDiscoveryMaxOutput, window)
		}
		input := []string{"text"}
		if slices.Contains(d.InputModalities, "image") {
			input = append(input, "image")
		}
		out = append(out, Model{
			Provider:         "runcrate",
			ID:               id,
			DisplayName:      display,
			Input:            input,
			ContextWindow:    window,
			MaxOutput:        output,
			Reasoning:        slices.Contains(d.SupportedFeatures, "reasoning"),
			AdaptiveThinking: usesAdaptiveThinking(Model{ID: id}),
			PriceInput:       max(d.Pricing.Input, 0),
			PriceOutput:      max(d.Pricing.Output, 0),
			BaseURL:          baseURL,
			Source:           "live",
		})
	}
	return out, nil
}
