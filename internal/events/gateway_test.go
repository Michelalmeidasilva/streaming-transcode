package events

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewGatewayClientTrimsTrailingSlash(t *testing.T) {
	client := NewGatewayClient("http://example.com/api/v1/")
	if client.baseURL != "http://example.com/api/v1" {
		t.Fatalf("baseURL = %q", client.baseURL)
	}
}

func TestGatewayClientSendEventAndPatchVideo(t *testing.T) {
	var (
		eventBody map[string]any
		patchBody map[string]any
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/events":
			if err := json.NewDecoder(r.Body).Decode(&eventBody); err != nil {
				t.Fatalf("Decode POST body: %v", err)
			}
			w.WriteHeader(http.StatusAccepted)
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/upload-state/videos/v1":
			if err := json.NewDecoder(r.Body).Decode(&patchBody); err != nil {
				t.Fatalf("Decode PATCH body: %v", err)
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewGatewayClient(server.URL + "/api/v1/")
	if err := client.SendEvent(context.Background(), "ready", map[string]any{"videoId": "v1"}); err != nil {
		t.Fatalf("SendEvent() error = %v", err)
	}
	if err := client.PatchVideo(context.Background(), "v1", map[string]any{"status": "ready"}); err != nil {
		t.Fatalf("PatchVideo() error = %v", err)
	}
	if eventBody["eventType"] != "ready" {
		t.Fatalf("eventType = %v", eventBody["eventType"])
	}
	payload, ok := eventBody["payload"].(map[string]any)
	if !ok || payload["videoId"] != "v1" {
		t.Fatalf("payload = %#v", eventBody["payload"])
	}
	if patchBody["status"] != "ready" {
		t.Fatalf("patchBody = %#v", patchBody)
	}
}

func TestGatewayClientReturnsHTTPStatusErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			http.Error(w, "boom", http.StatusBadGateway)
			return
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewGatewayClient(server.URL)
	if err := client.SendEvent(context.Background(), "ready", map[string]any{}); err == nil {
		t.Fatalf("SendEvent() error = nil, want error")
	}
	if err := client.PatchVideo(context.Background(), "v1", map[string]any{}); err == nil {
		t.Fatalf("PatchVideo() error = nil, want error")
	}
}
