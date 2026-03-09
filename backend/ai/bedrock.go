package ai

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "strings"
    "time"

    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
    "github.com/redis/go-redis/v9"
    "go.uber.org/zap"
)

const modelID = "anthropic.claude-3-5-sonnet-20241022-v2:0"

type BedrockClient struct {
    client *bedrockruntime.Client
    rdb    *redis.Client
    logger *zap.Logger
}

func NewBedrockClient(rdb *redis.Client, logger *zap.Logger) *BedrockClient {
    cfg, _ := config.LoadDefaultConfig(context.Background(),
        config.WithRegion("us-east-1"))

    return &BedrockClient{
        client: bedrockruntime.NewFromConfig(cfg),
        rdb:    rdb,
        logger: logger,
    }
}

func (b *BedrockClient) ExplainCrash(ctx context.Context, command, stderr string, exitCode int) (string, error) {
    // Check Redis cache first
    cacheKey := b.cacheKey(command, stderr)
    if cached, err := b.rdb.Get(ctx, cacheKey).Result(); err == nil {
        b.logger.Info("AI cache HIT", zap.String("command", command))
        b.rdb.Incr(ctx, "metric:cache_hits")
        return cached, nil
    }

    // Build prompt
    prompt := fmt.Sprintf(`You are Entropy, an AI assistant that helps developers learn from system crashes.

A command was executed in a sandboxed Linux environment and it crashed.

Command: %s
Exit Code: %d
Stderr: %s

Please explain:
1. What this command does
2. WHY it crashed (root cause)
3. What EXACTLY happened at the OS level
4. How to PREVENT this in production
5. A safe alternative if applicable

Keep the explanation educational, clear, and under 300 words.`, command, exitCode, stderr)

    body, _ := json.Marshal(map[string]interface{}{
        "anthropic_version": "bedrock-2023-05-31",
        "max_tokens":        500,
        "temperature":       0.7,
        "messages": []map[string]string{
            {"role": "user", "content": prompt},
        },
    })

    model := modelID
    resp, err := b.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
        ModelId:     &model,
        Body:        body,
        ContentType: stringPtr("application/json"),
    })
    if err != nil {
        return "", fmt.Errorf("bedrock call failed: %w", err)
    }

    var result map[string]interface{}
    json.Unmarshal(resp.Body, &result)

    explanation := ""
    if content, ok := result["content"].([]interface{}); ok && len(content) > 0 {
        if item, ok := content[0].(map[string]interface{}); ok {
            explanation = item["text"].(string)
        }
    }

    // Cache for 24 hours
    b.rdb.Set(ctx, cacheKey, explanation, 24*time.Hour)
    b.logger.Info("AI cache SET", zap.String("command", command))

    return explanation, nil
}

func (b *BedrockClient) cacheKey(command, stderr string) string {
    normalized := strings.ToLower(strings.TrimSpace(command)) + "::" + stderr[:min(50, len(stderr))]
    hash := sha256.Sum256([]byte(normalized))
    return "ai:cache:" + hex.EncodeToString(hash[:])
}

func stringPtr(s string) *string { return &s }
func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}
