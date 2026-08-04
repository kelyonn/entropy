package storage

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

func (c *Client) PutLocal(_ context.Context, key string, data []byte) error {
	dir := filepath.Join("data", "snapshots", filepath.Dir(key))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join("data", "snapshots", key), data, 0o644)
}

func (c *Client) GetLocal(_ context.Context, key string) ([]byte, error) {
	return os.ReadFile(filepath.Join("data", "snapshots", key))
}

func (c *Client) PutBestEffort(ctx context.Context, key string, data []byte) (backend string, err error) {
	if c.Enabled() {
		if err := c.Put(ctx, key, data); err == nil {
			return "s3", nil
		} else {
			return "", err
		}
	}
	if err := c.PutLocal(ctx, key, data); err != nil {
		return "", err
	}
	return "local", nil
}

func (c *Client) PutBestEffortWithRetry(ctx context.Context, key string, data []byte, retries int) (backend string, err error) {
	if !c.Enabled() {
		return c.PutBestEffort(ctx, key, data)
	}
	for i := 0; i <= retries; i++ {
		err = c.Put(ctx, key, data)
		if err == nil {
			return "s3", nil
		}
		c.logger.Warn("S3 upload failed",
			zap.Int("attempt", i+1),
			zap.Int("max_attempts", retries+1),
			zap.Error(err))
		if i < retries {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(i+1) * 500 * time.Millisecond):
			}
		}
	}
	return "", err
}

func (c *Client) GetBestEffort(ctx context.Context, key string) ([]byte, error) {
	if c.Enabled() {
		return c.Get(ctx, key)
	}
	return c.GetLocal(ctx, key)
}
