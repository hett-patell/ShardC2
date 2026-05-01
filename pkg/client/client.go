package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{
		baseURL:    baseURL + "/api/v1",
		token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

type HealthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

type StatsResponse struct {
	TotalBots  int `json:"total_bots"`
	ActiveBots int `json:"active_bots"`
}

type Bot struct {
	ID       string `json:"id"`
	Hostname string `json:"hostname"`
	IP       string `json:"ip_address"`
	OS       string `json:"os"`
	Status   string `json:"status"`
}

type BotsResponse struct {
	Bots []Bot `json:"bots"`
}

type ValidateRequest struct {
	Type   string `json:"type"`
	Config string `json:"config"`
}

type ValidateResponse struct {
	TotalTargets   int      `json:"total_targets"`
	BlockedTargets int      `json:"blocked_targets"`
	PolicyWarnings []string `json:"policy_warnings"`
	CanLaunch      bool     `json:"can_launch"`
}

func (c *Client) Health() (HealthResponse, error) {
	var resp HealthResponse
	if err := c.get("/health", &resp); err != nil {
		return resp, err
	}
	return resp, nil
}

func (c *Client) Stats() (StatsResponse, error) {
	var resp StatsResponse
	if err := c.get("/stats", &resp); err != nil {
		return resp, err
	}
	return resp, nil
}

func (c *Client) ListBots() ([]Bot, error) {
	var resp BotsResponse
	if err := c.get("/bots/", &resp); err != nil {
		return nil, err
	}
	return resp.Bots, nil
}

func (c *Client) ValidateCampaign(req ValidateRequest) (ValidateResponse, error) {
	var resp ValidateResponse
	if err := c.post("/campaigns/validate", req, &resp); err != nil {
		return resp, err
	}
	return resp, nil
}

func (c *Client) get(path string, result interface{}) error {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	return c.do(req, result)
}

func (c *Client) post(path string, body interface{}, result interface{}) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, result)
}

func (c *Client) do(req *http.Request, result interface{}) error {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("api error %d: %s", resp.StatusCode, string(body))
	}

	if result != nil && len(body) > 0 {
		if err := json.Unmarshal(body, result); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}
