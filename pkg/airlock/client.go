package airlock

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: http.DefaultClient,
	}
}

func (c *Client) Spawn(ctx context.Context, sessionID, existingContainerID, image string) (string, error) {
	body, _ := json.Marshal(SpawnRequest{
		SessionID:           sessionID,
		ExistingContainerID: existingContainerID,
		Image:               image,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/internal/spawn", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("spawn failed: %s", string(b))
	}
	var out SpawnResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.ContainerID, nil
}

func (c *Client) ExecStream(ctx context.Context, req ExecRequest, onChunk func(ExecChunk) error) error {
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/internal/exec", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/x-ndjson")
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("exec failed: %s", string(b))
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var chunk ExecChunk
		if err := json.Unmarshal(sc.Bytes(), &chunk); err != nil {
			continue
		}
		if err := onChunk(chunk); err != nil {
			return err
		}
	}
	return sc.Err()
}

func (c *Client) Kill(ctx context.Context, kill KillRequest) error {
	body, _ := json.Marshal(kill)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/internal/kill", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("kill failed: %s", string(b))
	}
	return nil
}

func (c *Client) Stats(ctx context.Context, containerID string) (StatsResponse, error) {
	body, _ := json.Marshal(StatsRequest{ContainerID: containerID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/internal/stats", bytes.NewReader(body))
	if err != nil {
		return StatsResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return StatsResponse{}, err
	}
	defer resp.Body.Close()
	var out StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return StatsResponse{}, err
	}
	return out, nil
}

func (c *Client) Stop(ctx context.Context, stop StopRequest) error {
	body, _ := json.Marshal(stop)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/internal/stop", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("stop failed: %s", string(b))
	}
	return nil
}

func (c *Client) Commit(ctx context.Context, containerID, tag string) (string, error) {
	body, _ := json.Marshal(CommitRequest{ContainerID: containerID, Tag: tag})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/internal/commit", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("commit failed: %s", string(b))
	}
	var out CommitResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Tag, nil
}

func (c *Client) SaveImageGzip(ctx context.Context, tag string) ([]byte, error) {
	body, _ := json.Marshal(SaveImageRequest{Tag: tag})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/internal/save-image", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("save failed: %s", string(b))
	}
	return io.ReadAll(resp.Body)
}

func (c *Client) LoadImageGzip(ctx context.Context, data []byte) error {
	body, _ := json.Marshal(LoadImageRequest{Data: data})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/internal/load-image", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("load failed: %s", string(b))
	}
	return nil
}
