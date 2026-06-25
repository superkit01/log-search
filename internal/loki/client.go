package loki

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client 抽象 Loki 查询能力,便于测试注入 mock。
type Client interface {
	Labels(ctx context.Context, start, end time.Time) ([]string, error)
	LabelValues(ctx context.Context, name string, start, end time.Time) ([]string, error)
	QueryRange(ctx context.Context, query string, start, end time.Time, limit int, direction Direction) (*QueryResult, error)
}

type httpClient struct {
	baseURL string
	h       *http.Client
}

// NewHTTPClient 创建直连 Loki HTTP API 的客户端。
func NewHTTPClient(rawURL string, timeout time.Duration) Client {
	return &httpClient{
		baseURL: strings.TrimRight(rawURL, "/"),
		h:       &http.Client{Timeout: timeout},
	}
}

func (c *httpClient) Labels(ctx context.Context, start, end time.Time) ([]string, error) {
	var out []string
	err := c.get(ctx, "/loki/api/v1/labels", url.Values{
		"start": {strconv.FormatInt(start.UnixNano(), 10)},
		"end":   {strconv.FormatInt(end.UnixNano(), 10)},
	}, &out)
	return out, err
}

func (c *httpClient) LabelValues(ctx context.Context, name string, start, end time.Time) ([]string, error) {
	var out []string
	path := fmt.Sprintf("/loki/api/v1/label/%s/values", url.PathEscape(name))
	err := c.get(ctx, path, url.Values{
		"start": {strconv.FormatInt(start.UnixNano(), 10)},
		"end":   {strconv.FormatInt(end.UnixNano(), 10)},
	}, &out)
	return out, err
}

func (c *httpClient) QueryRange(ctx context.Context, query string, start, end time.Time, limit int, direction Direction) (*QueryResult, error) {
	var streams []stream
	err := c.get(ctx, "/loki/api/v1/query_range", url.Values{
		"query":     {query},
		"start":     {strconv.FormatInt(start.UnixNano(), 10)},
		"end":       {strconv.FormatInt(end.UnixNano(), 10)},
		"limit":     {strconv.Itoa(limit)},
		"direction": {string(direction)},
	}, &streams)
	if err != nil {
		return nil, err
	}
	res := &QueryResult{}
	for _, s := range streams {
		for _, v := range s.Values {
			ns, err := strconv.ParseInt(v[0], 10, 64)
			if err != nil {
				continue
			}
			res.Lines = append(res.Lines, Line{
				Time:   time.Unix(0, ns).UTC(),
				Raw:    v[1],
				Labels: s.Stream,
			})
		}
	}
	res.Total = len(res.Lines)
	return res, nil
}

func (c *httpClient) get(ctx context.Context, path string, params url.Values, out interface{}) error {
	u := c.baseURL + path
	if enc := params.Encode(); enc != "" {
		u += "?" + enc
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := c.h.Do(req)
	if err != nil {
		return fmt.Errorf("loki request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var ar apiResponse
	if err := json.Unmarshal(body, &ar); err != nil {
		return fmt.Errorf("loki: decode %s: %w (body=%q)", path, err, string(body))
	}
	if ar.Status != "success" {
		return fmt.Errorf("loki: %s: %s", ar.ErrorType, ar.ErrorMsg)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("loki: HTTP %d: %s", resp.StatusCode, ar.ErrorMsg)
	}
	if out != nil {
		if err := json.Unmarshal(ar.Data, out); err != nil {
			return fmt.Errorf("loki: decode data %s: %w", path, err)
		}
	}
	return nil
}
