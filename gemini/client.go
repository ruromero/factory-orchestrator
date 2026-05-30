package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Client struct {
	apiKey  string
	baseURL string // override for testing; empty uses production URL
	http    *http.Client
}

type generateRequest struct {
	Contents []content `json:"contents"`
}

type content struct {
	Parts []part `json:"parts"`
}

type part struct {
	Text string `json:"text"`
}

type generateResponse struct {
	Candidates []struct {
		Content struct {
			Parts []part `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

// Usage holds token counts returned by the Gemini API.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		http:   &http.Client{},
	}
}

func (c *Client) Generate(ctx context.Context, model, prompt string) (string, error) {
	text, _, err := c.GenerateWithUsage(ctx, model, prompt)
	return text, err
}

func (c *Client) endpoint(model string) string {
	base := "https://generativelanguage.googleapis.com"
	if c.baseURL != "" {
		base = c.baseURL
	}
	return fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", base, model, c.apiKey)
}

// GenerateWithUsage works like Generate but also returns token usage metadata.
func (c *Client) GenerateWithUsage(ctx context.Context, model, prompt string) (string, Usage, error) {
	url := c.endpoint(model)

	req := generateRequest{
		Contents: []content{{Parts: []part{{Text: prompt}}}},
	}

	data, err := json.Marshal(req)
	if err != nil {
		return "", Usage{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", Usage{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", Usage{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", Usage{}, fmt.Errorf("gemini api: %d: %s", resp.StatusCode, body)
	}

	var genResp generateResponse
	if err := json.NewDecoder(resp.Body).Decode(&genResp); err != nil {
		return "", Usage{}, fmt.Errorf("decode gemini response: %w", err)
	}

	if len(genResp.Candidates) == 0 || len(genResp.Candidates[0].Content.Parts) == 0 {
		return "", Usage{}, fmt.Errorf("empty gemini response")
	}

	usage := Usage{
		PromptTokens:     genResp.UsageMetadata.PromptTokenCount,
		CompletionTokens: genResp.UsageMetadata.CandidatesTokenCount,
	}

	return genResp.Candidates[0].Content.Parts[0].Text, usage, nil
}
