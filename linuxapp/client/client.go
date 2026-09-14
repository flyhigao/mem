package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ServerURL    string `json:"server_url"`
	Token        string `json:"token"`
	Source       string `json:"source,omitempty"`
	DeviceName   string `json:"device_name,omitempty"`
	Hotkey       string `json:"hotkey,omitempty"`
	AutoPaste    bool   `json:"auto_paste"`
	Notify       bool   `json:"notify"`
	PollInterval int    `json:"poll_interval,omitempty"`
}

func (c *Config) GetSource() string {
	if c.Source != "" {
		return c.Source
	}
	if c.DeviceName != "" {
		return c.DeviceName
	}
	return "Linux"
}

func (c *Config) GetPollInterval() time.Duration {
	if c.PollInterval > 0 {
		return time.Duration(c.PollInterval) * time.Second
	}
	return 3 * time.Second
}

type Message struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	TokenID   *int64    `json:"token_id"`
	TokenName string    `json:"token_name"`
	Content   string    `json:"content"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
}

type APIResponse struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
}

type MemClient struct {
	cfg        Config
	httpClient *http.Client
}

func NewMemClient(cfg Config) *MemClient {
	return &MemClient{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Ping checks server reachable status and measures latency
func (c *MemClient) Ping() (time.Duration, error) {
	pingURL := strings.TrimRight(c.cfg.ServerURL, "/") + "/api/v1/ping"
	start := time.Now()

	req, err := http.NewRequest(http.MethodGet, pingURL, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to build ping request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("ping failed: %w", err)
	}
	defer resp.Body.Close()

	duration := time.Since(start)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return duration, fmt.Errorf("server returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	return duration, nil
}

// FetchLatestMessage retrieves the latest text from the relay server
func (c *MemClient) FetchLatestMessage() (*Message, error) {
	reqURL := strings.TrimRight(c.cfg.ServerURL, "/") + "/api/v1/messages/latest"
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var res APIResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	if !res.Success {
		return nil, fmt.Errorf("api error: %s", res.Error)
	}

	if len(res.Data) == 0 || string(res.Data) == "null" {
		return nil, fmt.Errorf("no messages available")
	}

	var msg Message
	if err := json.Unmarshal(res.Data, &msg); err != nil {
		return nil, fmt.Errorf("failed to decode message: %w", err)
	}

	return &msg, nil
}

// PostMessage sends text to the relay server
func (c *MemClient) PostMessage(content string, source string) (*Message, error) {
	reqURL := strings.TrimRight(c.cfg.ServerURL, "/") + "/api/v1/messages"
	if source == "" {
		source = c.cfg.GetSource()
	}

	payloadMap := map[string]string{
		"content": content,
		"source":  source,
	}
	payloadBytes, err := json.Marshal(payloadMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, reqURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("server returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var res APIResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !res.Success {
		return nil, fmt.Errorf("api error: %s", res.Error)
	}

	var msg Message
	if err := json.Unmarshal(res.Data, &msg); err != nil {
		return nil, fmt.Errorf("failed to decode message: %w", err)
	}

	return &msg, nil
}

// FetchMessages retrieves message history with pagination
func (c *MemClient) FetchMessages(limit int, offset int, beforeID int64) ([]Message, error) {
	baseURL := strings.TrimRight(c.cfg.ServerURL, "/") + "/api/v1/messages"
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid server url: %w", err)
	}

	q := u.Query()
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}
	if beforeID > 0 {
		q.Set("before_id", strconv.FormatInt(beforeID, 10))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var res APIResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	if !res.Success {
		return nil, fmt.Errorf("api error: %s", res.Error)
	}

	var messages []Message
	if len(res.Data) > 0 && string(res.Data) != "null" {
		if err := json.Unmarshal(res.Data, &messages); err != nil {
			return nil, fmt.Errorf("failed to parse messages: %w", err)
		}
	}
	return messages, nil
}

// DeleteMessage removes a message by ID
func (c *MemClient) DeleteMessage(id int64) error {
	reqURL := fmt.Sprintf("%s/api/v1/messages/%d", strings.TrimRight(c.cfg.ServerURL, "/"), id)
	req, err := http.NewRequest(http.MethodDelete, reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to build delete request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var res APIResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return fmt.Errorf("failed to parse JSON response: %w", err)
	}

	if !res.Success {
		return fmt.Errorf("api error: %s", res.Error)
	}

	return nil
}

// TestConnection checks server ping and validates token against latest message endpoint
func (c *MemClient) TestConnection() (*Message, error) {
	if _, err := c.Ping(); err != nil {
		return nil, fmt.Errorf("server unreachable (%s): %w", c.cfg.ServerURL, err)
	}

	msg, err := c.FetchLatestMessage()
	if err != nil {
		if strings.Contains(err.Error(), "no messages available") {
			return nil, nil // Connected and authenticated, just no messages yet
		}
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	return msg, nil
}

// SendNotification triggers a desktop notification if notify-send is present
func SendNotification(title, message string) {
	if _, err := exec.LookPath("notify-send"); err == nil {
		_ = exec.Command("notify-send", "-a", "Mem", "-t", "2500", title, message).Run()
	}
}
