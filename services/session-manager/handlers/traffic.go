package handlers

import (
	"strconv"
	"strings"
)

// trafficSample is the latest second of production-stack traffic.
type trafficSample struct {
	RPS      int     `json:"rps"`
	ErrorPct float64 `json:"errorPct"`
	P50      int     `json:"p50"`
	P95      int     `json:"p95"`
}

func (c *conn) stackActive() bool {
	c.labMu.Lock()
	defer c.labMu.Unlock()
	return c.lab != nil && c.lab.lab.Stack && c.lab.state == labActive
}

// parseTraffic reads "epoch total ok errors p50 p95" from traffic.log.
func parseTraffic(line string) (trafficSample, bool) {
	f := strings.Fields(strings.TrimSpace(line))
	if len(f) != 6 {
		return trafficSample{}, false
	}
	n := make([]int, 6)
	for i, v := range f {
		x, err := strconv.Atoi(v)
		if err != nil {
			return trafficSample{}, false
		}
		n[i] = x
	}
	s := trafficSample{RPS: n[1], P50: n[4], P95: n[5]}
	if n[1] > 0 {
		s.ErrorPct = float64(n[3]) * 100 / float64(n[1])
	}
	return s, true
}

func (c *conn) readTraffic() (trafficSample, bool) {
	out, code, err := c.execQuiet(c.ctx, "tail -n 1 /var/log/stack/traffic.log 2>/dev/null")
	if err != nil || code != 0 {
		return trafficSample{}, false
	}
	return parseTraffic(out)
}
