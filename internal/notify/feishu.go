package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// FeishuClient sends notifications via Feishu (Lark) webhook.
type FeishuClient struct {
	webhookURL string
	httpClient *http.Client
}

// NewFeishuClient creates a new FeishuClient with the given webhook URL.
func NewFeishuClient(webhookURL string) *FeishuClient {
	return &FeishuClient{
		webhookURL: webhookURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// feishuMessage represents the Feishu interactive card payload.
type feishuMessage struct {
	MsgType string     `json:"msg_type"`
	Card    feishuCard `json:"card"`
}

type feishuCard struct {
	Header   feishuHeader    `json:"header"`
	Elements []feishuElement `json:"elements"`
}

type feishuHeader struct {
	Title    feishuText `json:"title"`
	Template string     `json:"template"`
}

type feishuElement struct {
	Tag  string     `json:"tag"`
	Text feishuText `json:"text"`
}

type feishuText struct {
	Tag     string `json:"tag"`
	Content string `json:"content"`
}

// Send sends an interactive card message with the given title and content.
func (c *FeishuClient) Send(ctx context.Context, title, content string) error {
	return c.sendCard(ctx, title, content, "blue")
}

// SendAlert sends an alert card with a color based on the level.
// Supported levels: "critical" (red), "warning" (orange), "info" (green).
func (c *FeishuClient) SendAlert(ctx context.Context, level, title, detail string) error {
	template := levelToTemplate(level)
	body := fmt.Sprintf("**[%s]** %s", level, detail)
	return c.sendCard(ctx, title, body, template)
}

// sendCard posts an interactive card to the Feishu webhook.
func (c *FeishuClient) sendCard(ctx context.Context, title, content, template string) error {
	msg := feishuMessage{
		MsgType: "interactive",
		Card: feishuCard{
			Header: feishuHeader{
				Title:    feishuText{Tag: "plain_text", Content: title},
				Template: template,
			},
			Elements: []feishuElement{
				{
					Tag:  "div",
					Text: feishuText{Tag: "lark_md", Content: content},
				},
			},
		},
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("notify: marshal feishu message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.webhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("notify: create feishu request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("notify: send feishu request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("notify: feishu webhook returned status %d", resp.StatusCode)
	}
	return nil
}

// levelToTemplate maps alert levels to Feishu card header template colors.
func levelToTemplate(level string) string {
	switch level {
	case "critical":
		return "red"
	case "warning":
		return "orange"
	case "info":
		return "green"
	default:
		return "red"
	}
}
