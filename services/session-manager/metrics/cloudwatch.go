package metrics

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type Exporter struct {
	rdb        *redis.Client
	cloudwatch *cloudwatch.Client
	namespace  string
	identity   string
	logger     *zap.Logger
}

func NewExporter(ctx context.Context, rdb *redis.Client, logger *zap.Logger) *Exporter {
	namespace := os.Getenv("CLOUDWATCH_NAMESPACE")
	if namespace == "" {
		return nil
	}
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		logger.Warn("CloudWatch exporter disabled", zap.Error(err))
		return nil
	}
	identity, _ := os.Hostname()
	return &Exporter{
		rdb: rdb, cloudwatch: cloudwatch.NewFromConfig(cfg),
		namespace: namespace, identity: identity, logger: logger,
	}
}

func (e *Exporter) Start(ctx context.Context) {
	if e == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			e.exportIfLeader(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (e *Exporter) exportIfLeader(ctx context.Context) {
	const lockKey = "metric:cloudwatch:leader"
	acquired, err := e.rdb.SetNX(ctx, lockKey, e.identity, 75*time.Second).Result()
	if err != nil {
		return
	}
	if !acquired {
		owner, getErr := e.rdb.Get(ctx, lockKey).Result()
		if getErr != nil || owner != e.identity {
			return
		}
		e.rdb.Expire(ctx, lockKey, 75*time.Second)
	}

	active := e.activeSessions(ctx)
	commands := e.metric(ctx, "metric:total_commands")
	crashes := e.metric(ctx, "metric:total_crashes")
	errorRate := 0.0
	if commands > 0 {
		errorRate = crashes / commands * 100
	}

	data := []types.MetricDatum{
		datum("ActiveSessions", active, types.StandardUnitCount),
		datum("CommandExecutionCount", commands, types.StandardUnitCount),
		datum("ErrorRate", errorRate, types.StandardUnitPercent),
		datum("AIExplanationLatency", e.metric(ctx, "metric:ai_latency_ms"), types.StandardUnitMilliseconds),
		datum("AIExplanationTokenCount", e.metric(ctx, "metric:ai_token_count"), types.StandardUnitCount),
		datum("ContainerCPUUtilization", e.metric(ctx, "metric:container_cpu"), types.StandardUnitPercent),
		datum("ContainerMemoryUtilization", e.metric(ctx, "metric:container_memory"), types.StandardUnitPercent),
		datum("SnapshotSaveCount", e.metric(ctx, "metric:snapshot_saves"), types.StandardUnitCount),
		datum("SessionCreationFailures", e.metric(ctx, "metric:session_creation_failures"), types.StandardUnitCount),
	}
	data = append(data, e.sessionSummaryData(ctx)...)
	if _, err := e.cloudwatch.PutMetricData(ctx, &cloudwatch.PutMetricDataInput{
		Namespace:  aws.String(e.namespace),
		MetricData: data,
	}); err != nil {
		e.logger.Warn("CloudWatch metric export failed", zap.Error(err))
	}
}

func (e *Exporter) sessionSummaryData(ctx context.Context) []types.MetricDatum {
	data := make([]types.MetricDatum, 0, 60)
	for index := 0; index < 20; index++ {
		raw, err := e.rdb.RPop(ctx, "metric:session_summary_events").Bytes()
		if err != nil {
			break
		}
		var summary struct {
			CommandsRun        int `json:"commandsRun"`
			CrashesEncountered int `json:"crashesEncountered"`
			DurationSeconds    int `json:"durationSeconds"`
		}
		if json.Unmarshal(raw, &summary) != nil {
			continue
		}
		data = append(data,
			datum("SessionCommandsRun", float64(summary.CommandsRun), types.StandardUnitCount),
			datum("SessionCrashesEncountered", float64(summary.CrashesEncountered), types.StandardUnitCount),
			datum("SessionDuration", float64(summary.DurationSeconds), types.StandardUnitSeconds),
		)
	}
	return data
}

func (e *Exporter) activeSessions(ctx context.Context) float64 {
	var cursor uint64
	var active float64
	for {
		keys, next, err := e.rdb.Scan(ctx, cursor, "session:*", 100).Result()
		if err != nil {
			return active
		}
		for _, key := range keys {
			raw, err := e.rdb.Get(ctx, key).Bytes()
			if err != nil {
				continue
			}
			var value struct {
				Status string `json:"status"`
			}
			if json.Unmarshal(raw, &value) == nil && value.Status != "TERMINATED" && value.Status != "TERMINATING" {
				active++
			}
		}
		cursor = next
		if cursor == 0 {
			return active
		}
	}
}

func (e *Exporter) metric(ctx context.Context, key string) float64 {
	value, err := e.rdb.Get(ctx, key).Result()
	if err != nil {
		return 0
	}
	number, _ := strconv.ParseFloat(value, 64)
	return number
}

func datum(name string, value float64, unit types.StandardUnit) types.MetricDatum {
	return types.MetricDatum{
		MetricName: aws.String(name),
		Value:      aws.Float64(value),
		Unit:       unit,
		Timestamp:  aws.Time(time.Now().UTC()),
	}
}
