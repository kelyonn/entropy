package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// ErrGeneratorUnavailable means no model is configured or the circuit is open.
var ErrGeneratorUnavailable = errors.New("generator unavailable")

// JSONCompleter returns one complete JSON document for a prompt. It backs
// the lab generator, which validates the result before anything is used.
type JSONCompleter interface {
	CompleteJSON(ctx context.Context, prompt string, maxTokens int) (string, error)
}

// CompleteJSON asks Ollama for a JSON-constrained, non-streaming completion.
func (o *OllamaClient) CompleteJSON(ctx context.Context, prompt string, maxTokens int) (string, error) {
	if o.ollamaURL == "" || o.modelID == "" || !o.breaker.Allow() {
		return "", ErrGeneratorUnavailable
	}
	if maxTokens <= 0 || maxTokens > 4000 {
		maxTokens = 3000
	}
	body, _ := json.Marshal(map[string]interface{}{
		"model":  o.modelID,
		"prompt": prompt,
		"stream": false,
		"format": "json",
		"options": map[string]interface{}{
			"temperature": 0.4,
			"num_predict": maxTokens,
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.ollamaURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.httpClient.Do(req)
	if err != nil {
		o.breaker.RecordFailure()
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		o.breaker.RecordFailure()
		return "", fmt.Errorf("ollama http status %s", resp.Status)
	}
	var out struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		o.breaker.RecordFailure()
		return "", err
	}
	o.breaker.RecordSuccess()
	return out.Response, nil
}
