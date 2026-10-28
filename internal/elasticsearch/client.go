package elasticsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
	_ "time/tzdata"

	"log-search/internal/storage"
)

// FieldMapping selects exact-match fields without changing existing indices.
type FieldMapping struct {
	Env             string `yaml:"env"`
	App             string `yaml:"app"`
	Level           string `yaml:"level"`
	TraceID         string `yaml:"trace_id"`
	UppercaseLevels bool   `yaml:"uppercase_levels"`
}

type Options struct {
	Fields        FieldMapping
	IndexTimezone string
}

type Client interface {
	QueryRange(ctx context.Context, filters storage.QueryFilters, start, end time.Time, limit int, direction storage.Direction) (*storage.QueryResult, error)
}

type httpClient struct {
	baseURL    string
	index      string
	fields     FieldMapping
	location   *time.Location
	httpClient *http.Client
}

// NewHTTPClient 创建直连 ES HTTP API 的客户端。
func NewHTTPClient(rawURL string, index string, timeout time.Duration, options ...Options) Client {
	opts := Options{}
	if len(options) > 0 {
		opts = options[0]
	}
	if opts.Fields.Env == "" {
		opts.Fields.Env = "env.keyword"
	}
	if opts.Fields.App == "" {
		opts.Fields.App = "app.keyword"
	}
	if opts.Fields.Level == "" {
		opts.Fields.Level = "level.keyword"
	}
	if opts.Fields.TraceID == "" {
		opts.Fields.TraceID = "tid.keyword"
	}
	location, _ := time.LoadLocation(opts.IndexTimezone)
	if location == nil {
		location = time.UTC
	}
	return &httpClient{
		baseURL:    strings.TrimRight(rawURL, "/"),
		index:      index,
		fields:     opts.Fields,
		location:   location,
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (c *httpClient) QueryRange(ctx context.Context, filters storage.QueryFilters, start, end time.Time, limit int, direction storage.Direction) (*storage.QueryResult, error) {
	esQuery := buildESQueryWithFields(filters, c.fields)
	boolQuery := esQuery["bool"].(map[string]interface{})
	mustList := boolQuery["must"].([]interface{})

	mustList = append(mustList, map[string]interface{}{
		"range": map[string]interface{}{
			"@timestamp": map[string]interface{}{
				"gte": start.UTC().Format(time.RFC3339Nano),
				"lte": end.UTC().Format(time.RFC3339Nano),
			},
		},
	})

	boolQuery["must"] = mustList

	// 构建请求
	searchBody := map[string]interface{}{
		"query": esQuery,
		"size":  limit,
		"sort": []map[string]interface{}{
			{"@timestamp": map[string]interface{}{"order": sortOrder(direction)}},
		},
	}

	var result searchResult
	err := c.search(ctx, buildTimeRangeIndex(strings.ReplaceAll(c.index, "{namespace}", filters.Namespace), filters.Env, start.In(c.location), end.In(c.location)), searchBody, &result)
	if err != nil {
		return nil, err
	}

	// 转换结果
	return c.convertResult(result)
}

func sortOrder(direction storage.Direction) string {
	if direction == storage.Forward {
		return "asc"
	}
	return "desc"
}

func (c *httpClient) search(ctx context.Context, index string, body map[string]interface{}, out *searchResult) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	searchURL := fmt.Sprintf("%s/%s/_search?ignore_unavailable=true&allow_no_indices=true", c.baseURL, index)
	log.Printf("[ES Request] URL=%s body=%s", searchURL, string(data))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, searchURL, strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("es request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("es: read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := string(respBody)
		var errResp struct {
			Error struct {
				Reason string `json:"root_cause"`
			} `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Reason != "" {
			msg = errResp.Error.Reason
		}
		return &storage.HTTPError{StatusCode: resp.StatusCode, Message: msg}
	}

	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("es: decode response: %w (body=%s)", err, string(respBody))
	}

	return nil
}

func (c *httpClient) convertResult(result searchResult) (*storage.QueryResult, error) {
	res := &storage.QueryResult{Lines: []storage.Line{}}

	for _, hit := range result.Hits.Hits {
		source := hit.Source

		// 提取时间戳
		timestamp, _ := source["@timestamp"].(string)
		ts, err := time.Parse(time.RFC3339Nano, timestamp)
		if err != nil {
			continue
		}

		// 提取日志消息
		message := ""
		if msg, ok := source["message"].(string); ok {
			message = msg
		}

		// 提取标签
		labels := make(map[string]string)
		for k, v := range source {
			if k == "@timestamp" || k == "message" {
				continue
			}
			if strVal, ok := v.(string); ok {
				labels[k] = strVal
			}
		}

		if trace, ok := source["traceId"].(string); ok && trace != "" {
			labels["tid"] = trace
		}
		if labels["app"] == "" {
			labels["app"], _ = source["serviceName"].(string)
		}
		if stack, ok := source["stack_trace"].(string); ok && stack != "" {
			message += "\n" + stack
		}
		res.Lines = append(res.Lines, storage.Line{
			Index:  hit.Index,
			ID:     hit.ID,
			Time:   ts.UTC(),
			Raw:    message,
			Labels: labels,
		})
	}

	// 注意: ES 返回的结果已经按指定排序返回，不需要再次排序
	// sort.SliceStable(res.Lines, func(i, j int) bool {
	// 	if direction == storage.Forward {
	// 		return res.Lines[i].Time.Before(res.Lines[j].Time)
	// 	}
	// 	return res.Lines[i].Time.After(res.Lines[j].Time)
	// })

	res.Total = len(res.Lines)
	return res, nil
}

// buildTimeRangeIndex 根据时间范围和 env 构建索引名
// 支持环境和日期占位符，枚举包含 start/end 的全部日历日期。
func buildTimeRangeIndex(indexPattern string, env string, start, end time.Time) string {
	// 替换 {env} 占位符
	if env != "" && strings.Contains(indexPattern, "{env}") {
		indexPattern = strings.ReplaceAll(indexPattern, "{env}", env)
	}

	if !strings.Contains(indexPattern, "{") {
		return indexPattern
	}

	var indices []string
	seen := map[string]bool{}
	day := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	for !day.After(end) {
		name := formatIndexName(indexPattern, day)
		if !seen[name] {
			indices = append(indices, name)
			seen[name] = true
		}
		day = day.AddDate(0, 0, 1)
	}
	return strings.Join(indices, ",")
}

func formatIndexName(pattern string, t time.Time) string {
	// 替换日期格式占位符
	pattern = strings.ReplaceAll(pattern, "{yyyyMMdd}", t.Format("20060102"))
	result := strings.ReplaceAll(pattern, "{yyyy.MM.dd}", t.Format("2006.01.02"))
	result = strings.ReplaceAll(result, "{yyyy.MM}", t.Format("2006.01"))
	result = strings.ReplaceAll(result, "{yyyy}", t.Format("2006"))
	return result
}

// ES 搜索响应结构
type searchResult struct {
	Hits struct {
		Total struct {
			Value int `json:"value"`
		} `json:"total"`
		Hits []struct {
			Source map[string]interface{} `json:"_source"`
			Index  string                 `json:"_index"`
			ID     string                 `json:"_id"`
		} `json:"hits"`
	} `json:"hits"`
}
