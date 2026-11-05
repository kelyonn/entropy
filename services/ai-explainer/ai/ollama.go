package ai

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/entropy/pkg/resilience"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type OllamaClient struct {
	httpClient *http.Client
	ollamaURL  string
	modelID    string

	rdb     *redis.Client
	logger  *zap.Logger
	breaker *resilience.CircuitBreaker
}

func NewOllamaClient(rdb *redis.Client, logger *zap.Logger, ollamaURL, modelID string) *OllamaClient {
	ollamaURL = strings.TrimSpace(ollamaURL)
	modelID = strings.TrimSpace(modelID)
	return &OllamaClient{
		httpClient: &http.Client{Timeout: 300 * time.Second},
		ollamaURL:  ollamaURL,
		modelID:    modelID,
		rdb:        rdb,
		logger:     logger,
		breaker:    resilience.NewCircuitBreaker(3, 30*time.Second),
	}
}

func (o *OllamaClient) Fallback(exitCode int) string {
	if msg, ok := genericFallbacks[exitCode]; ok {
		return msg
	}
	return fmt.Sprintf(`**What happened:** Command failed with exit code %d.

**In plain terms:** The program stopped because something went wrong.

What actually happened: Inspect the error output and system limits for the specific cause.

How to fix it: Fix the underlying issue shown in stderr, then re-run.`, exitCode)
}

func (o *OllamaClient) FallbackForCommand(command string, exitCode int) string {
	cmd := strings.ToLower(strings.TrimSpace(command))
	switch {
	case strings.Contains(cmd, "kill -9 1"):
		return genericFallbacks[0]
	case strings.Contains(cmd, ":(){ :|:& };:") || strings.Contains(cmd, "fork"):
		return `**What happened:** A fork bomb tried to create endless copies of itself.

**In plain terms:** Like a photocopier that keeps copying its own copies until the room is full.

What actually happened: The shell function recursively spawns processes until the PID limit (100 in this sandbox) is hit.

How to fix it: Limit process counts with cgroups/PID limits. Kill runaway process trees. Never run untrusted shell snippets.`
	case strings.Contains(cmd, "oom") || strings.Contains(cmd, "dev/zero"):
		return genericFallbacks[137]
	case strings.Contains(cmd, "openssl speed"):
		return `**What happened:** A CPU stress test ran many heavy calculations at once.

**In plain terms:** Like asking every CPU core to sprint simultaneously.

What actually happened:** The scenario spawned CPU-bound openssl workers to simulate high load.

How to fix it:** Use rate limits, autoscaling, and CPU quotas so one tenant cannot starve others.`
	default:
		return o.Fallback(exitCode)
	}
}

func (o *OllamaClient) ExplainCrashStream(ctx context.Context, command, stderr string, exitCode int, onToken func(token string, done bool, fallback bool)) error {
	return o.streamPrompt(ctx, command, stderr, exitCode, teachingPrompt(command, stderr, exitCode), onToken)
}

func (o *OllamaClient) ExplainSimplerStream(ctx context.Context, command, stderr string, exitCode int, onToken func(token string, done bool, fallback bool)) error {
	prompt := fmt.Sprintf(`Explain this command failure as if to a smart 14-year-old who has never used Linux.

Command: %s
Exit: %d
Error output: %s

Use very short sentences. No jargon without a one-phrase explanation in parentheses.
Structure:
1. One sentence summary
2. Everyday analogy (one sentence)
3. What went wrong (two short sentences max)
4. What to do next (one sentence)

No emoji. Under 150 words.`, command, exitCode, truncate(stderr, maxPromptOutput))
	return o.streamPrompt(ctx, command, stderr, exitCode, prompt, onToken)
}

func (o *OllamaClient) ExplainQuestionStream(ctx context.Context, command, stderr string, exitCode int, question string, onToken func(token string, done bool, fallback bool)) error {
	prompt := fmt.Sprintf(`You are Entropy, a patient systems instructor.

A learner asked a follow-up question about a command failure.

Command: %s
Exit: %d
Error output: %s
Learner question: %s

Answer the question directly in plain language. If you use a technical term, define it in parentheses.
No emoji. Under 200 words.`, command, exitCode, truncate(stderr, maxPromptOutput), question)

	return o.streamPrompt(ctx, command+":q:"+question, stderr, exitCode, prompt, onToken)
}

func (o *OllamaClient) streamPrompt(
	ctx context.Context,
	command, stderr string,
	exitCode int,
	prompt string,
	onToken func(token string, done bool, fallback bool),
) error {
	cacheKey := o.cacheKey(command, stderr, exitCode)
	if cached, err := o.rdb.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
		o.rdb.Incr(ctx, "metric:cache_hits")
		onToken(cached, false, false)
		onToken("", true, false)
		return nil
	}

	if o.ollamaURL == "" || o.modelID == "" || !o.breaker.Allow() {
		if o.ollamaURL != "" {
			o.rdb.Incr(ctx, "metric:ollama_circuit_open")
		}
		fb := o.FallbackForCommand(command, exitCode)
		onToken(fb, false, true)
		onToken("", true, true)
		return fmt.Errorf("ollama provider unavailable")
	}

	body, _ := json.Marshal(map[string]interface{}{
		"model":  o.modelID,
		"prompt": prompt,
		"stream": true,
		"options": map[string]interface{}{
			"temperature": 0.7,
			"num_predict": 500,
		},
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.ollamaURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		o.breaker.RecordFailure()
		fb := o.FallbackForCommand(command, exitCode)
		onToken(fb, false, true)
		onToken("", true, true)
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		o.breaker.RecordFailure()
		fb := o.FallbackForCommand(command, exitCode)
		onToken(fb, false, true)
		onToken("", true, true)
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		o.breaker.RecordFailure()
		fb := o.FallbackForCommand(command, exitCode)
		onToken(fb, false, true)
		onToken("", true, true)
		return fmt.Errorf("ollama http status %s", resp.Status)
	}

	scanner := bufio.NewScanner(resp.Body)
	// Ollama chunks can be larger than the default token size.
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	var full strings.Builder
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var evt struct {
			Response string `json:"response"`
			Done     bool   `json:"done"`
		}
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			continue
		}
		if evt.Response != "" {
			full.WriteString(evt.Response)
			onToken(evt.Response, false, false)
		}
		if evt.Done {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		o.breaker.RecordFailure()
		if full.Len() == 0 {
			fb := o.FallbackForCommand(command, exitCode)
			onToken(fb, false, true)
			onToken("", true, true)
			return err
		}
	}

	explanation := full.String()
	if explanation != "" {
		o.breaker.RecordSuccess()
		o.rdb.Set(ctx, cacheKey, explanation, 24*time.Hour)
	}
	onToken("", true, false)
	return nil
}

func (o *OllamaClient) cacheKey(command, stderr string, exitCode int) string {
	normalized := strings.ToLower(strings.TrimSpace(command)) + "::" + truncate(stderr, maxPromptOutput) + fmt.Sprintf("::%d", exitCode)
	hash := sha256.Sum256([]byte(normalized))
	return "ai:cache:" + hex.EncodeToString(hash[:])
}
