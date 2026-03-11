package fetcher

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/damonleelcx/gpu-k8s-infra/api/v1alpha1"
	"github.com/redis/go-redis/v9"
)

// Fetcher gets current (and optionally historical) metric values for scaling.
type Fetcher struct {
	prometheusURL string
	httpClient    *http.Client
	redisClient   *redis.Client
}

// NewFetcher creates a Fetcher. prometheusURL and redisAddr can be empty to disable those backends.
func NewFetcher(prometheusURL, redisAddr string) *Fetcher {
	f := &Fetcher{
		prometheusURL: prometheusURL,
		httpClient:    &http.Client{Timeout: 10 * time.Second},
	}
	if redisAddr != "" {
		f.redisClient = redis.NewClient(&redis.Options{Addr: redisAddr})
	}
	return f
}

// GetCurrent runs the Prometheus query or Redis lookup and returns the current value for the metric.
func (f *Fetcher) GetCurrent(ctx context.Context, m v1alpha1.MetricSpec) (float64, error) {
	switch m.Type {
	case v1alpha1.MetricTypeQPS, v1alpha1.MetricTypeGPUUtilization:
		if f.prometheusURL == "" || m.PrometheusQuery == "" {
			return 0, fmt.Errorf("prometheus not configured or PrometheusQuery empty")
		}
		return f.queryPrometheus(ctx, m.PrometheusQuery)
	case v1alpha1.MetricTypeQueueLength:
		if f.redisClient == nil || m.QueueConfig == nil {
			return 0, fmt.Errorf("redis not configured or QueueConfig empty")
		}
		return f.getQueueLength(ctx, m.QueueConfig)
	default:
		return 0, fmt.Errorf("unknown metric type %q", m.Type)
	}
}

// GetHistorical returns recent values for the metric (for prediction). Only supports Prometheus for now.
func (f *Fetcher) GetHistorical(ctx context.Context, query string, lookbackSeconds int, stepSeconds int) ([]float64, error) {
	if f.prometheusURL == "" {
		return nil, nil
	}
	start := time.Now().Add(-time.Duration(lookbackSeconds) * time.Second)
	end := time.Now()
	u, _ := url.Parse(f.prometheusURL + "/api/v1/query_range")
	q := u.Query()
	q.Set("query", query)
	q.Set("start", fmt.Sprintf("%d", start.Unix()))
	q.Set("end", fmt.Sprintf("%d", end.Unix()))
	q.Set("step", fmt.Sprintf("%ds", stepSeconds))
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result prometheusRangeResponse
	if err := jsonDecode(resp.Body, &result); err != nil {
		return nil, err
	}
	return parseRangeValues(&result), nil
}

func (f *Fetcher) queryPrometheus(ctx context.Context, query string) (float64, error) {
	u, _ := url.Parse(f.prometheusURL + "/api/v1/query")
	q := u.Query()
	q.Set("query", query)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, err
	}
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var result prometheusQueryResponse
	if err := jsonDecode(resp.Body, &result); err != nil {
		return 0, err
	}
	return parseSingleValue(&result), nil
}

func (f *Fetcher) getQueueLength(ctx context.Context, q *v1alpha1.QueueMetricConfig) (float64, error) {
	keyType := q.KeyType
	if keyType == "" {
		keyType = "list"
	}
	switch keyType {
	case "list":
		n, err := f.redisClient.LLen(ctx, q.Key).Result()
		return float64(n), err
	case "stream":
		n, err := f.redisClient.XLen(ctx, q.Key).Result()
		return float64(n), err
	case "set":
		n, err := f.redisClient.SCard(ctx, q.Key).Result()
		return float64(n), err
	default:
		n, err := f.redisClient.LLen(ctx, q.Key).Result()
		return float64(n), err
	}
}

// Prometheus API response types (minimal).
type prometheusQueryResponse struct {
	Data struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Value []interface{} `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

type prometheusRangeResponse struct {
	Data struct {
		Result []struct {
			Values [][]interface{} `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

func parseSingleValue(r *prometheusQueryResponse) float64 {
	if len(r.Data.Result) == 0 {
		return 0
	}
	// Instant query: value is [timestamp, "value"]
	if len(r.Data.Result[0].Value) < 2 {
		return 0
	}
	switch v := r.Data.Result[0].Value[1].(type) {
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	default:
		return 0
	}
}

func parseRangeValues(r *prometheusRangeResponse) []float64 {
	var out []float64
	for _, series := range r.Data.Result {
		for _, pair := range series.Values {
			if len(pair) < 2 {
				continue
			}
			switch v := pair[1].(type) {
			case string:
				f, _ := strconv.ParseFloat(v, 64)
				out = append(out, f)
			}
		}
	}
	return out
}
