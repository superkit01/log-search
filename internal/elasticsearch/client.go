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

	"log-search/internal/storage"
)

// Client 抽象 ES 查询能力,便于测试注入 mock。
type Client interface {
	QueryRange(ctx context.Context, query string, start, end time.Time, limit int, direction storage.Direction) (*storage.QueryResult, error)
}

type httpClient struct {
	baseURL    string
	index      string
	httpClient *http.Client
}

// NewHTTPClient 创建直连 ES HTTP API 的客户端。
func NewHTTPClient(rawURL string, index string, timeout time.Duration) Client {
	return &httpClient{
		baseURL:    strings.TrimRight(rawURL, "/"),
		index:      index,
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (c *httpClient) QueryRange(ctx context.Context, query string, start, end time.Time, limit int, direction storage.Direction) (*storage.QueryResult, error) {
	// 解析 LogQL 查询
	parsed, err := ParseLogQL(query)
	if err != nil {
		return nil, fmt.Errorf("parse LogQL: %w", err)
	}

	// 构建 ES 查询
	esQuery, err := ToESQuery(parsed)
	if err != nil {
		return nil, fmt.Errorf("build ES query: %w", err)
	}

	// 添加时间范围
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
	err = c.search(ctx, buildTimeRangeIndex(c.index, start, end), searchBody, &result)
	if err != nil {
		return nil, err
	}

	// 转换结果
	return c.convertResult(result, parsed)
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

	searchURL := fmt.Sprintf("%s/%s/_search", c.baseURL, index)
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

func (c *httpClient) convertResult(result searchResult, parsed *ParsedQuery) (*storage.QueryResult, error) {
	res := &storage.QueryResult{}

	for _, hit := range result.Hits.Hits {
		source := hit.Source

		// 提取时间戳
		ts, err := time.Parse(time.RFC3339Nano, source["@timestamp"].(string))
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

		res.Lines = append(res.Lines, storage.Line{
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

// buildTimeRangeIndex 根据时间范围构建索引名
// 支持 "logs-{yyyy.MM.dd}" 格式，会根据 start/end 时间生成索引列表
func buildTimeRangeIndex(indexPattern string, start, end time.Time) string {
	if !strings.Contains(indexPattern, "{") {
		return indexPattern
	}

	// 如果时间跨度小于一天，使用 start 时间
	if end.Sub(start) < 24*time.Hour {
		return formatIndexName(indexPattern, start)
	}

	// 对于跨天查询，使用逗号分隔的索引列表（ES 支持）
	// 这里简化处理，返回通配符模式
	return formatIndexName(indexPattern, start) + "," + formatIndexName(indexPattern, end)
}

func formatIndexName(pattern string, t time.Time) string {
	// 替换日期格式占位符
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
		} `json:"hits"`
	} `json:"hits"`
}
