package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/redis/go-redis/v9"
)

// PrometheusHandler exposes a minimal Prometheus text endpoint by reading the
// metric keys already produced by Session Manager’s runtime.
func PrometheusHandler(rdb *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		active := activeSessions(ctx, rdb)
		totalCommands := metric(ctx, rdb, "metric:total_commands")
		totalCrashes := metric(ctx, rdb, "metric:total_crashes")

		errorRate := 0.0
		if totalCommands > 0 {
			errorRate = totalCrashes / totalCommands * 100
		}

		aiLatencyMs := metric(ctx, rdb, "metric:ai_latency_ms")
		aiTokenCount := metric(ctx, rdb, "metric:ai_token_count")
		containerCPU := metric(ctx, rdb, "metric:container_cpu")
		containerMemory := metric(ctx, rdb, "metric:container_memory")
		snapshotSaves := metric(ctx, rdb, "metric:snapshot_saves")
		sessionCreationFailures := metric(ctx, rdb, "metric:session_creation_failures")

		w.Header().Set("Content-Type", "text/plain; version=0.0.4")

		fmt.Fprintln(w, "# HELP entropy_active_sessions Active sandbox sessions")
		fmt.Fprintln(w, "# TYPE entropy_active_sessions gauge")
		fmt.Fprintf(w, "entropy_active_sessions %f\n", active)

		fmt.Fprintln(w, "# HELP entropy_total_commands Total command executions (monotonic counter)")
		fmt.Fprintln(w, "# TYPE entropy_total_commands counter")
		fmt.Fprintf(w, "entropy_total_commands %f\n", totalCommands)

		fmt.Fprintln(w, "# HELP entropy_total_crashes Total crashes encountered (monotonic counter)")
		fmt.Fprintln(w, "# TYPE entropy_total_crashes counter")
		fmt.Fprintf(w, "entropy_total_crashes %f\n", totalCrashes)

		fmt.Fprintln(w, "# HELP entropy_error_rate_percent Crash rate percentage over commands")
		fmt.Fprintln(w, "# TYPE entropy_error_rate_percent gauge")
		fmt.Fprintf(w, "entropy_error_rate_percent %f\n", errorRate)

		fmt.Fprintln(w, "# HELP entropy_ai_explanation_latency_ms AI explanation latency, last observed value")
		fmt.Fprintln(w, "# TYPE entropy_ai_explanation_latency_ms gauge")
		fmt.Fprintf(w, "entropy_ai_explanation_latency_ms %f\n", aiLatencyMs)

		fmt.Fprintln(w, "# HELP entropy_ai_token_count AI token count (monotonic counter)")
		fmt.Fprintln(w, "# TYPE entropy_ai_token_count counter")
		fmt.Fprintf(w, "entropy_ai_token_count %f\n", aiTokenCount)

		fmt.Fprintln(w, "# HELP entropy_container_cpu_percent Latest container CPU utilization percentage")
		fmt.Fprintln(w, "# TYPE entropy_container_cpu_percent gauge")
		fmt.Fprintf(w, "entropy_container_cpu_percent %f\n", containerCPU)

		fmt.Fprintln(w, "# HELP entropy_container_memory_percent Latest container memory utilization percentage")
		fmt.Fprintln(w, "# TYPE entropy_container_memory_percent gauge")
		fmt.Fprintf(w, "entropy_container_memory_percent %f\n", containerMemory)

		fmt.Fprintln(w, "# HELP entropy_snapshot_saves Total snapshot saves (monotonic counter)")
		fmt.Fprintln(w, "# TYPE entropy_snapshot_saves counter")
		fmt.Fprintf(w, "entropy_snapshot_saves %f\n", snapshotSaves)

		fmt.Fprintln(w, "# HELP entropy_session_creation_failures Total session creation failures (monotonic counter)")
		fmt.Fprintln(w, "# TYPE entropy_session_creation_failures counter")
		fmt.Fprintf(w, "entropy_session_creation_failures %f\n", sessionCreationFailures)
	}
}

func metric(ctx context.Context, rdb *redis.Client, key string) float64 {
	value, err := rdb.Get(ctx, key).Result()
	if err != nil {
		return 0
	}
	var out float64
	_, _ = fmt.Sscanf(value, "%f", &out)
	return out
}

func activeSessions(ctx context.Context, rdb *redis.Client) float64 {
	var cursor uint64
	var active float64
	for {
		keys, next, err := rdb.Scan(ctx, cursor, "session:*", 100).Result()
		if err != nil {
			return active
		}
		for _, key := range keys {
			raw, err := rdb.Get(ctx, key).Bytes()
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

