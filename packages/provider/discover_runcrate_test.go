package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestDiscoverRuncrateMapsChatModels(t *testing.T) {
	const body = `{"object":"list","data":[
		{"id":"zai-org/GLM-5.3","name":"GLM 5.3","input_modalities":["text"],"output_modalities":["text"],
		 "context_length":1048576,"max_output_length":131072,"supported_features":["tools","json_mode","reasoning"],
		 "pricing":{"prompt":"0.0000005625","completion":"0.0000025","input":0.590625,"output":2.625}},
		{"id":"moonshotai/Kimi-K2.6","name":"Kimi K2.6","input_modalities":["text","image"],"output_modalities":["text"],
		 "context_length":262144,"max_output_length":16384,"supported_features":["tools","reasoning"],
		 "pricing":{"input":0.7875,"output":3.675}},
		{"id":"vendor/no-limits","input_modalities":["text"],"output_modalities":["text"],"supported_features":["tools"]},
		{"id":"zai-org/GLM-5.3","name":"Duplicate","input_modalities":["text"],"output_modalities":["text"],"supported_features":["tools"]},
		{"id":"vendor/no-tools","input_modalities":["text"],"output_modalities":["text"],"supported_features":["json_mode"]},
		{"id":"vendor/image-model","input_modalities":["text"],"output_modalities":["image"],"supported_features":["tools"]},
		{"id":"vendor/embedding-model","input_modalities":["text"],"output_modalities":["embeddings"],"supported_features":["tools"]},
		{"id":"vendor/speech-to-text","input_modalities":["audio"],"output_modalities":["text"],"supported_features":["tools"]},
		{"id":"","input_modalities":["text"],"output_modalities":["text"],"supported_features":["tools"]}
	]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("request path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("public catalog request carried a credential")
		}
		w.Header().Set("content-type", "application/json")
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	models, err := DiscoverRuncrate(context.Background(), srv.URL+"/v1/")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 3 {
		t.Fatalf("models = %+v", models)
	}

	glm := models[0]
	if glm.Provider != "runcrate" || glm.ID != "zai-org/GLM-5.3" || glm.DisplayName != "GLM 5.3" ||
		glm.ContextWindow != 1048576 || glm.MaxOutput != 131072 || !glm.Reasoning ||
		glm.PriceInput != 0.590625 || glm.PriceOutput != 2.625 ||
		glm.BaseURL != srv.URL+"/v1" || glm.Source != "live" || !slices.Equal(glm.Input, []string{"text"}) {
		t.Errorf("glm model = %+v", glm)
	}
	kimi := models[1]
	if kimi.ID != "moonshotai/Kimi-K2.6" || kimi.MaxOutput != 16384 || !slices.Equal(kimi.Input, []string{"text", "image"}) {
		t.Errorf("kimi model = %+v", kimi)
	}
	fallback := models[2]
	if fallback.DisplayName != "vendor/no-limits" || fallback.ContextWindow != 32768 || fallback.MaxOutput != 4096 || fallback.Reasoning {
		t.Errorf("fallback metadata = %+v", fallback)
	}

	SetLiveModels(models)
	t.Cleanup(func() { SetLiveModels(nil) })
	client := NewRuncrate("test-key", srv.URL+"/v1").(*openaiClient)
	wire, err := client.buildRequest(Request{Model: kimi.ID, Reasoning: "high"})
	if err != nil {
		t.Fatal(err)
	}
	if wire.Model != kimi.ID || wire.MaxCompletionTok == nil || *wire.MaxCompletionTok != 16384 || wire.ReasoningEffort != "high" {
		t.Errorf("wire request = %+v", wire)
	}
}

func TestDiscoverRuncrateReplacesSeed(t *testing.T) {
	SetLiveModels([]Model{{
		Provider: "runcrate", ID: "deepseek-ai/DeepSeek-V4.1-Flash", DisplayName: "DeepSeek V4.1 Flash",
		ContextWindow: 524288, MaxOutput: 65536, PriceInput: 0.2, Source: "live",
	}})
	t.Cleanup(func() { SetLiveModels(nil) })
	m, err := FindModel("runcrate", "deepseek-ai/DeepSeek-V4.1-Flash")
	if err != nil {
		t.Fatal(err)
	}
	if m.ContextWindow != 524288 || m.MaxOutput != 65536 || m.PriceInput != 0.2 {
		t.Errorf("seed metadata was not replaced: %+v", m)
	}
}

func TestDiscoverRuncrateErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"unavailable", http.StatusServiceUnavailable, "unavailable"},
		{"invalid-json", http.StatusOK, "not json"},
		{"missing-data", http.StatusOK, `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()
			if _, err := DiscoverRuncrate(context.Background(), srv.URL); err == nil {
				t.Fatal("expected discovery error")
			}
		})
	}
}
