package elasticsearch

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"log-search/internal/storage"
)

const pitKeepAlive = "2m"

type pageCursor struct {
	PIT   string            `json:"pit"`
	Sort  []json.RawMessage `json:"sort"`
	Query string            `json:"query"`
}

// QueryRangePage retains the original range and exact ES sort values, including
// 64-bit sequence values and the PIT-unique _shard_doc tiebreaker.
func (c *httpClient) QueryRangePage(ctx context.Context, filters storage.QueryFilters, start, end time.Time, limit int, direction storage.Direction, cursor string) (*storage.QueryResult, error) {
	query := buildESQueryWithFields(filters, c.fields)
	boolean := query["bool"].(map[string]interface{})
	boolean["must"] = append(boolean["must"].([]interface{}), map[string]interface{}{
		"range": map[string]interface{}{"@timestamp": map[string]interface{}{
			"gte": start.UTC().Format(time.RFC3339Nano), "lte": end.UTC().Format(time.RFC3339Nano),
		}},
	})
	index := buildTimeRangeIndex(strings.ReplaceAll(c.index, "{namespace}", filters.Namespace), filters.Env, start.In(c.location), end.In(c.location))
	binding, _ := json.Marshal([]interface{}{index, query, direction})
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(binding))
	state := pageCursor{}
	if cursor != "" {
		if !strings.HasPrefix(cursor, "es1:") || len(cursor) > 12000 {
			return nil, badCursor()
		}
		data, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(cursor, "es1:"))
		if err != nil || json.Unmarshal(data, &state) != nil || state.PIT == "" || len(state.Sort) != 3 || state.Query != fingerprint {
			return nil, badCursor()
		}
	} else {
		state.Query = fingerprint
		var opened struct {
			ID string `json:"id"`
		}
		err := c.pitRequest(ctx, http.MethodPost, "/"+index+"/_pit?keep_alive="+pitKeepAlive+"&ignore_unavailable=true", nil, &opened)
		// ES 7.16 can return an NPE when ignore_unavailable removes every target.
		// Retry strictly to confirm index_not_found rather than hiding a real 500.
		if isEmptyPITFailure(err) {
			err = c.pitRequest(ctx, http.MethodPost, "/"+index+"/_pit?keep_alive="+pitKeepAlive+"&ignore_unavailable=false", nil, &opened)
		}
		if err != nil {
			var httpErr *storage.HTTPError
			if errors.As(err, &httpErr) && httpErr.StatusCode == 404 && strings.Contains(httpErr.Message, "index_not_found_exception") {
				return &storage.QueryResult{Lines: []storage.Line{}, Page: &storage.PageInfo{}}, nil
			}
			return nil, err
		}
		if opened.ID == "" {
			return nil, fmt.Errorf("es: open PIT returned no id")
		}
		state.PIT = opened.ID
	}
	body := map[string]interface{}{
		"query": query, "size": limit, "track_total_hits": false,
		"pit": map[string]interface{}{"id": state.PIT, "keep_alive": pitKeepAlive},
		"sort": []interface{}{
			map[string]interface{}{"@timestamp": map[string]interface{}{"order": sortOrder(direction)}},
			map[string]interface{}{"sequence": map[string]interface{}{"order": sortOrder(direction), "missing": "_last", "unmapped_type": "long"}},
			map[string]interface{}{"_shard_doc": sortOrder(direction)},
		},
	}
	if len(state.Sort) > 0 {
		body["search_after"] = state.Sort
	}
	var result searchResult
	if err := c.search(ctx, "", body, &result); err != nil {
		c.closePIT(ctx, state.PIT)
		var httpErr *storage.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return nil, &storage.HTTPError{StatusCode: http.StatusNotFound, Message: "查询快照已失效，请重新查询。" + httpErr.Message}
		}
		return nil, err
	}
	if result.PITID != "" {
		state.PIT = result.PITID
	}
	res, err := c.convertResult(result)
	if err != nil {
		c.closePIT(ctx, state.PIT)
		return nil, err
	}
	res.Page = &storage.PageInfo{}
	// Continue from the last raw hit, even when malformed source timestamps were skipped.
	if len(result.Hits.Hits) >= limit {
		state.Sort = result.Hits.Hits[len(result.Hits.Hits)-1].Sort
		if len(state.Sort) != 3 {
			c.closePIT(ctx, state.PIT)
			return nil, fmt.Errorf("es: paginated response missing sort values")
		}
		encoded, err := json.Marshal(state)
		if err != nil {
			c.closePIT(ctx, state.PIT)
			return nil, err
		}
		res.Page.Continuation = "es1:" + base64.RawURLEncoding.EncodeToString(encoded)
	} else {
		c.closePIT(ctx, state.PIT)
	}
	return res, nil
}

func badCursor() error {
	return &storage.HTTPError{StatusCode: http.StatusBadRequest, Message: "分页游标无效或查询条件已改变，请重新查询"}
}

func isEmptyPITFailure(err error) bool {
	var httpErr *storage.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusInternalServerError {
		return false
	}
	var response struct {
		Error struct {
			Type   string `json:"type"`
			Reason string `json:"reason"`
		} `json:"error"`
	}
	return json.Unmarshal([]byte(httpErr.Message), &response) == nil &&
		response.Error.Type == "null_pointer_exception" &&
		response.Error.Reason == "Point in time parameter must be not null"
}

func (c *httpClient) closePIT(ctx context.Context, id string) {
	// Best effort; abandoned searches are also released automatically after keep_alive.
	_ = c.pitRequest(ctx, http.MethodDelete, "/_pit", map[string]string{"id": id}, nil)
}

func (c *httpClient) pitRequest(ctx context.Context, method, path string, body interface{}, out interface{}) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = strings.NewReader(string(data))
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("es PIT request: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &storage.HTTPError{StatusCode: resp.StatusCode, Message: string(data)}
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}
