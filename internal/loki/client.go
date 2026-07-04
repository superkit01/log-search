package loki

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// HTTPError is returned when Loki responds with a non-success HTTP status.
type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("loki: HTTP %d: %s", e.StatusCode, e.Message)
}

// Client 抽象 Loki 查询能力,便于测试注入 mock。
type Client interface {
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

func (c *httpClient) QueryRange(ctx context.Context, query string, start, end time.Time, limit int, direction Direction) (*QueryResult, error) {
	var qr queryRangeResponse
	err := c.get(ctx, "/loki/api/v1/query_range", url.Values{
		"query":     {query},
		"start":     {strconv.FormatInt(start.UnixNano(), 10)},
		"end":       {strconv.FormatInt(end.UnixNano(), 10)},
		"limit":     {strconv.Itoa(limit)},
		"direction": {string(direction)},
	}, &qr)
	if err != nil {
		return nil, err
	}
	res := &QueryResult{}
	for _, s := range qr.Result {
		for _, v := range s.Values {
			ns, err := strconv.ParseInt(v.Timestamp, 10, 64)
			if err != nil {
				continue
			}
			// Merge stream labels and structured metadata into a single label map.
			// If both contain the same key, metadata takes precedence (consistent with
			// Loki's structured metadata semantics).
			labels := make(map[string]string, len(s.Stream)+len(v.Metadata))
			for k, val := range s.Stream {
				labels[k] = val
			}
			for k, val := range v.Metadata {
				labels[k] = val // Overwrites stream label if key exists
			}
			res.Lines = append(res.Lines, Line{
				Time:   time.Unix(0, ns).UTC(),
				Raw:    v.Line,
				Labels: labels,
			})
		}
	}
	// Loki returns lines grouped per stream; merge into a single global time
	// order so multi-stream results read chronologically. FORWARD = oldest
	// first, BACKWARD = newest first. Stable so equal-timestamp lines keep the
	// order Loki returned within their stream.
	sort.SliceStable(res.Lines, func(i, j int) bool {
		if direction == Forward {
			return res.Lines[i].Time.Before(res.Lines[j].Time)
		}
		return res.Lines[i].Time.After(res.Lines[j].Time)
	})
	res.Total = len(res.Lines)
	return res, nil
}

func (c *httpClient) get(ctx context.Context, path string, params url.Values, out interface{}) error {
	u := c.baseURL + path
	if enc := params.Encode(); enc != "" {
		u += "?" + enc
	}
	log.Printf("[Loki Request] URL=%s", u)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := c.h.Do(req)
	if err != nil {
		return fmt.Errorf("loki request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("loki: read response body %s: %w", path, err)
	}

	// On a non-success HTTP status, Loki may return a non-JSON body (e.g. a
	// LogQL parse error as plain text). Build the HTTPError from whatever we
	// got WITHOUT trying to JSON-decode first, so the web layer can map the
	// status code through instead of falling back to 502 on a decode failure.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := string(body)
		var ar apiResponse
		if err := json.Unmarshal(body, &ar); err == nil {
			if ar.ErrorMsg != "" {
				msg = ar.ErrorMsg
			}
			if ar.ErrorType != "" {
				msg = ar.ErrorType + ": " + msg
			}
		}
		return &HTTPError{StatusCode: resp.StatusCode, Message: msg}
	}

	var ar apiResponse
	if err := json.Unmarshal(body, &ar); err != nil {
		return fmt.Errorf("loki: decode %s: %w (body=%q)", path, err, string(body))
	}
	if ar.Status != "success" {
		msg := ar.ErrorMsg
		if msg == "" {
			msg = string(body)
		}
		if ar.ErrorType != "" {
			msg = ar.ErrorType + ": " + msg
		}
		return &HTTPError{StatusCode: resp.StatusCode, Message: msg}
	}
	if out != nil {
		// Empty data field is valid (means no results), skip unmarshal
		if len(ar.Data) == 0 {
			return nil
		}
		if err := json.Unmarshal(ar.Data, out); err != nil {
			return fmt.Errorf("loki: decode data %s: %w (data=%q)", path, err, string(ar.Data))
		}
	}
	return nil
}
