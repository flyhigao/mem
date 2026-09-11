package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

type Config struct {
	ServerURL string `json:"server_url"`
	Token     string `json:"token"`
	Hotkey    string `json:"hotkey"`
	AutoPaste bool   `json:"auto_paste"`
	Notify    bool   `json:"notify"`
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
	Success bool     `json:"success"`
	Data    *Message `json:"data"`
	Error   string   `json:"error"`
}

type MemClient struct {
	cfg        Config
	httpClient *http.Client
}

func NewMemClient(cfg Config) *MemClient {
	return &MemClient{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// FetchLatestMessage retrieves the latest text from the relay server
func (c *MemClient) FetchLatestMessage() (*Message, error) {
	url := strings.TrimRight(c.cfg.ServerURL, "/") + "/api/v1/messages/latest"
	req, err := http.NewRequest(http.MethodGet, url, nil)
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
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	var res APIResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	if !res.Success {
		return nil, fmt.Errorf("api error: %s", res.Error)
	}

	if res.Data == nil {
		return nil, fmt.Errorf("no messages available")
	}

	return res.Data, nil
}

// PostMessage sends text to the relay server
func (c *MemClient) PostMessage(content string, source string) (*Message, error) {
	url := strings.TrimRight(c.cfg.ServerURL, "/") + "/api/v1/messages"
	payload := fmt.Sprintf(`{"content":%q,"source":%q}`, content, source)

	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	var res APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !res.Success {
		return nil, fmt.Errorf("api error: %s", res.Error)
	}

	return res.Data, nil
}

// SendNotification triggers a desktop notification if notify-send is present
func SendNotification(title, message string) {
	if _, err := exec.LookPath("notify-send"); err == nil {
		_ = exec.Command("notify-send", "-a", "Mem", "-t", "2000", title, message).Run()
	}
}
