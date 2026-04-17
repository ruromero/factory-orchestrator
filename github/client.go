package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Client struct {
	token string
	owner string
	repo  string
	http  *http.Client
}

type Issue struct {
	Number int     `json:"number"`
	Title  string  `json:"title"`
	Body   string  `json:"body"`
	Labels []Label `json:"labels"`
}

type Label struct {
	Name string `json:"name"`
}

type Comment struct {
	ID   int    `json:"id"`
	Body string `json:"body"`
	User struct {
		Login string `json:"login"`
	} `json:"user"`
}

func NewClient(token, owner, repo string) *Client {
	return &Client{
		token: token,
		owner: owner,
		repo:  repo,
		http:  &http.Client{},
	}
}

func (c *Client) ListIssuesByLabel(ctx context.Context, label string) ([]Issue, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues?labels=%s&state=open", c.owner, c.repo, label)
	var issues []Issue
	if err := c.get(ctx, url, &issues); err != nil {
		return nil, fmt.Errorf("list issues: %w", err)
	}
	return issues, nil
}

func (c *Client) AddLabel(ctx context.Context, issueNumber int, label string) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d/labels", c.owner, c.repo, issueNumber)
	body := map[string][]string{"labels": {label}}
	return c.post(ctx, url, body, nil)
}

func (c *Client) RemoveLabel(ctx context.Context, issueNumber int, label string) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d/labels/%s", c.owner, c.repo, issueNumber, label)
	return c.delete(ctx, url)
}

func (c *Client) CreateComment(ctx context.Context, issueNumber int, body string) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d/comments", c.owner, c.repo, issueNumber)
	payload := map[string]string{"body": body}
	return c.post(ctx, url, payload, nil)
}

func (c *Client) ListComments(ctx context.Context, issueNumber int) ([]Comment, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d/comments", c.owner, c.repo, issueNumber)
	var comments []Comment
	if err := c.get(ctx, url, &comments); err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	return comments, nil
}

func (c *Client) CreateIssue(ctx context.Context, title, body string, labels []string) (Issue, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues", c.owner, c.repo)
	payload := map[string]any{
		"title":  title,
		"body":   body,
		"labels": labels,
	}
	var issue Issue
	if err := c.post(ctx, url, payload, &issue); err != nil {
		return Issue{}, fmt.Errorf("create issue: %w", err)
	}
	return issue, nil
}

// GetFileContent fetches the contents of a file from the repo's default branch.
func (c *Client) GetFileContent(ctx context.Context, path string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", c.owner, c.repo, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github.raw+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return "", fmt.Errorf("file not found: %s", path)
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("get file %s: %d: %s", path, resp.StatusCode, body)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// FileExists checks if a file exists in the repo's default branch.
func (c *Client) FileExists(ctx context.Context, path string) (bool, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", c.owner, c.repo, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.http.Do(req)
	if err != nil {
		return false, err
	}
	resp.Body.Close()

	if resp.StatusCode == 200 {
		return true, nil
	}
	if resp.StatusCode == 404 {
		return false, nil
	}
	return false, fmt.Errorf("check file %s: status %d", path, resp.StatusCode)
}

func (c *Client) get(ctx context.Context, url string, result any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	return c.do(req, result)
}

func (c *Client) post(ctx context.Context, url string, body any, result any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	return c.do(req, result)
}

func (c *Client) delete(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	return c.do(req, nil)
}

func (c *Client) do(req *http.Request, result any) error {
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if req.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github api %s %s: %d: %s", req.Method, req.URL.Path, resp.StatusCode, body)
	}

	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}
