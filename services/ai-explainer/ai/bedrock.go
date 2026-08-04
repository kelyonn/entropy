package ai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/entropy/pkg/resilience"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const modelID = "anthropic.claude-3-5-sonnet-20241022-v2:0"

var genericFallbacks = map[int]string{
	1: `**What happened:** The command failed with a general error.

**In plain terms:** Something went wrong, but the program did not say exactly what. Think of pressing a button and getting a red light with no label.

**What actually happened:** Exit code 1 usually means the program detected a problem and stopped on purpose.

**How to fix it:** Read the error text above the exit code. Check spelling, file paths, and whether required files exist.`,
	2: `**What happened:** The command failed because of incorrect usage or a syntax error.

**In plain terms:** Like sending a text with a typo — the receiver cannot understand the request.

**What actually happened:** Exit code 2 often means wrong flags, missing arguments, or invalid syntax.

**How to fix it:** Re-read the command. Compare it to the program's help output (try adding --help).`,
	126: `**What happened:** The command was found but could not be executed.

**In plain terms:** You found the right door, but it is locked.

**What actually happened:** Exit code 126 means the file exists but lacks execute permission or is not a runnable program.

**How to fix it:** Check file permissions with ls -l and ensure the binary is executable.`,
	127: `**What happened:** The command was not found.

**In plain terms:** You asked for a tool that is not in the toolbox.

**What actually happened:** Exit code 127 means the shell could not locate the program name you typed.

**How to fix it:** Check spelling. Verify the program is installed and on PATH.`,
	130: `**What happened:** The command was interrupted (Ctrl+C).

**In plain terms:** Like hanging up a phone call mid-sentence.

**What actually happened:** Exit code 130 means the process received SIGINT and stopped.

**How to fix it:** Re-run the command if you still need the result. Use interrupt only when you intend to stop.`,
	137: `**What happened:** The process was killed — likely out of memory (OOM).

**In plain terms:** The sandbox ran out of RAM, like a desk overflowing with papers until nothing fits.

**What actually happened:** Exit code 137 means SIGKILL, often because the container hit its 256 MB memory limit.

**How to fix it:** Use smaller data sets, stream instead of loading everything at once, or raise memory limits in production.`,
	139: `**What happened:** The program crashed with a segmentation fault.

**In plain terms:** The program tried to use memory it was not allowed to touch — like opening someone else's locked drawer.

**What actually happened:** Exit code 139 means SIGSEGV: invalid memory access, often a bug in native code.

**How to fix it:** Update or patch the faulty binary. In production, capture core dumps and stack traces.`,
	124: `**What happened:** The command timed out after 30 seconds.

**In plain terms:** A task took too long and was automatically cancelled, like a microwave stopping at max time.

**What actually happened:** The sandbox enforces a 30-second limit per command to keep sessions responsive.

**How to fix it:** Break long jobs into smaller steps, or run heavy work outside the sandbox.`,
	0: `**What happened:** The container stopped running even though the command may have appeared to succeed.

**In plain terms:** You turned off the engine of the car while still sitting in it — everything inside shuts down.

**What actually happened:** A command like kill -9 1 terminates the container's main process (PID 1). The sandbox shell dies with it.

**How to fix it:** Never kill PID 1 in production. Use graceful shutdown signals and process supervisors instead.`,
}

type BedrockClient struct {
	client  *bedrockruntime.Client
	rdb     *redis.Client
	logger  *zap.Logger
	breaker *resilience.CircuitBreaker
}

func NewBedrockClient(rdb *redis.Client, logger *zap.Logger) *BedrockClient {
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion("us-east-1"))
	if err != nil {
		logger.Warn("AWS config load failed; AI will use fallbacks", zap.Error(err))
	}
	var client *bedrockruntime.Client
	if err == nil {
		client = bedrockruntime.NewFromConfig(cfg)
	}
	return &BedrockClient{
		client: client, rdb: rdb, logger: logger,
		breaker: resilience.NewCircuitBreaker(3, 30*time.Second),
	}
}

func (b *BedrockClient) Fallback(exitCode int) string {
	if msg, ok := genericFallbacks[exitCode]; ok {
		return msg
	}
	return fmt.Sprintf(`**What happened:** Command failed with exit code %d.

**In plain terms:** The program stopped because something went wrong.

**What actually happened:** Inspect the error output and system limits for the specific cause.

**How to fix it:** Fix the underlying issue shown in stderr, then re-run.`, exitCode)
}

func (b *BedrockClient) FallbackForCommand(command string, exitCode int) string {
	cmd := strings.ToLower(strings.TrimSpace(command))
	switch {
	case strings.Contains(cmd, "kill -9 1"):
		return genericFallbacks[0]
	case strings.Contains(cmd, ":(){ :|:& };:") || strings.Contains(cmd, "fork"):
		return `**What happened:** A fork bomb tried to create endless copies of itself.

**In plain terms:** Like a photocopier that keeps copying its own copies until the room is full.

**What actually happened:** The shell function recursively spawns processes until the PID limit (100 in this sandbox) is hit.

**How to fix it:** Limit process counts with cgroups/PID limits. Kill runaway process trees. Never run untrusted shell snippets.`
	case strings.Contains(cmd, "oom") || strings.Contains(cmd, "dev/zero"):
		return genericFallbacks[137]
	case strings.Contains(cmd, "openssl speed"):
		return `**What happened:** A CPU stress test ran many heavy calculations at once.

**In plain terms:** Like asking every CPU core to sprint simultaneously.

**What actually happened:** The scenario spawned CPU-bound openssl workers to simulate high load.

**How to fix it:** Use rate limits, autoscaling, and CPU quotas so one tenant cannot starve others.`
	default:
		return b.Fallback(exitCode)
	}
}

func (b *BedrockClient) ExplainCrash(ctx context.Context, command, stderr string, exitCode int) (string, error) {
	var buf strings.Builder
	err := b.ExplainCrashStream(ctx, command, stderr, exitCode, func(token string, done bool, _ bool) {
		if !done {
			buf.WriteString(token)
		}
	})
	if err != nil {
		return b.FallbackForCommand(command, exitCode), err
	}
	return buf.String(), nil
}

func (b *BedrockClient) ExplainCrashStream(ctx context.Context, command, stderr string, exitCode int, onToken func(token string, done bool, fallback bool)) error {
	return b.streamPrompt(ctx, command, stderr, exitCode, teachingPrompt(command, stderr, exitCode), onToken)
}

func (b *BedrockClient) ExplainSimplerStream(ctx context.Context, command, stderr string, exitCode int, onToken func(token string, done bool, fallback bool)) error {
	prompt := fmt.Sprintf(`Explain this command failure as if to a smart 14-year-old who has never used Linux.

Command: %s
Exit code: %d
Error output: %s

Use very short sentences. No jargon without a one-phrase explanation in parentheses.
Structure:
1. One sentence summary
2. Everyday analogy (one sentence)
3. What went wrong (two short sentences max)
4. What to do next (one sentence)

No emoji. Under 150 words.`, command, exitCode, truncate(stderr, 1000))
	return b.streamPrompt(ctx, command, stderr, exitCode, prompt, onToken)
}

func (b *BedrockClient) ExplainQuestionStream(ctx context.Context, command, stderr string, exitCode int, question string, onToken func(token string, done bool, fallback bool)) error {
	prompt := fmt.Sprintf(`You are Entropy, a patient systems instructor.

A learner asked a follow-up question about a command failure.

Command: %s
Exit code: %d
Error output: %s
Learner question: %s

Answer the question directly in plain language. If you use a technical term, define it in parentheses.
No emoji. Under 200 words.`, command, exitCode, truncate(stderr, 1000), question)
	return b.streamPrompt(ctx, command+":q:"+question, stderr, exitCode, prompt, onToken)
}

func teachingPrompt(command, stderr string, exitCode int) string {
	return fmt.Sprintf(`You are Entropy, a systems engineering instructor explaining a command failure to someone who may not be technical.

Command: %s
Exit code: %d
Error output: %s

Use this exact structure with markdown headings:
**What happened:** (one plain sentence)
**In plain terms:** (everyday analogy, one sentence)
**What actually happened:** (2-3 short sentences, define any jargon in parentheses)
**How to fix it:** (1-2 practical steps)

No emoji. Under 250 words.`, command, exitCode, truncate(stderr, 1000))
}

func (b *BedrockClient) streamPrompt(ctx context.Context, command, stderr string, exitCode int, prompt string, onToken func(token string, done bool, fallback bool)) error {
	cacheKey := b.cacheKey(command, stderr, exitCode)
	if cached, err := b.rdb.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
		b.rdb.Incr(ctx, "metric:cache_hits")
		onToken(cached, false, false)
		onToken("", true, false)
		return nil
	}

	if b.client == nil || !b.breaker.Allow() {
		if b.client != nil {
			b.rdb.Incr(ctx, "metric:bedrock_circuit_open")
		}
		fb := b.FallbackForCommand(command, exitCode)
		onToken(fb, false, true)
		onToken("", true, true)
		return fmt.Errorf("bedrock client unavailable")
	}

	body, _ := json.Marshal(map[string]interface{}{
		"anthropic_version": "bedrock-2023-05-31",
		"max_tokens":        500,
		"temperature":       0.7,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	})

	model := modelID
	stream, err := b.client.InvokeModelWithResponseStream(ctx, &bedrockruntime.InvokeModelWithResponseStreamInput{
		ModelId:     &model,
		Body:        body,
		ContentType: strPtr("application/json"),
	})
	if err != nil {
		b.breaker.RecordFailure()
		fb := b.FallbackForCommand(command, exitCode)
		onToken(fb, false, true)
		onToken("", true, true)
		return err
	}
	defer stream.GetStream().Close()

	var full bytes.Buffer
	for event := range stream.GetStream().Events() {
		switch v := event.(type) {
		case *types.ResponseStreamMemberChunk:
			var chunk map[string]interface{}
			if json.Unmarshal(v.Value.Bytes, &chunk) != nil {
				continue
			}
			if chunk["type"] == "content_block_delta" {
				if delta, ok := chunk["delta"].(map[string]interface{}); ok {
					if text, ok := delta["text"].(string); ok && text != "" {
						full.WriteString(text)
						onToken(text, false, false)
					}
				}
			}
		}
	}
	if err := stream.GetStream().Err(); err != nil {
		b.breaker.RecordFailure()
		if full.Len() == 0 {
			fb := b.FallbackForCommand(command, exitCode)
			onToken(fb, false, true)
			onToken("", true, true)
			return err
		}
	}

	explanation := full.String()
	if explanation != "" {
		b.breaker.RecordSuccess()
		b.rdb.Set(ctx, cacheKey, explanation, 24*time.Hour)
	}
	onToken("", true, false)
	return nil
}

func (b *BedrockClient) cacheKey(command, stderr string, exitCode int) string {
	normalized := strings.ToLower(strings.TrimSpace(command)) + "::" + truncate(stderr, 50) + fmt.Sprintf("::%d", exitCode)
	hash := sha256.Sum256([]byte(normalized))
	return "ai:cache:" + hex.EncodeToString(hash[:])
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func strPtr(s string) *string { return &s }
