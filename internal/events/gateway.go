package events

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type GatewayClient struct {
	baseURL string
	http    *http.Client
}

func NewGatewayClient(baseURL string) *GatewayClient {
	return &GatewayClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *GatewayClient) SendEvent(ctx context.Context, eventType string, payload map[string]any) error {
	body := map[string]any{"eventType": eventType, "payload": payload}
	return c.postJSON(ctx, "/events", body, http.StatusAccepted)
}

func (c *GatewayClient) PatchVideo(ctx context.Context, videoID string, patch map[string]any) error {
	path := "/upload-state/videos/" + videoID
	return c.patchJSON(ctx, path, patch)
}

func (c *GatewayClient) postJSON(ctx context.Context, path string, body any, expected int) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != expected {
		return fmt.Errorf("POST %s returned %s", path, resp.Status)
	}
	return nil
}

func (c *GatewayClient) patchJSON(ctx context.Context, path string, body any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("PATCH %s returned %s", path, resp.Status)
	}
	return nil
}
