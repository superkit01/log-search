package elasticsearch

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ParsedQuery 表示解析后的 LogQL 查询
type ParsedQuery struct {
	LabelFilters map[string]string   // 精确匹配: env="prod"
	LabelRegex   map[string]string   // 正则匹配: level=~"warn|error"
	Keyword      string              // |= "keyword"
	TraceID      string              // | tid="..."
}

// ParseLogQL 解析 LogQL 查询字符串
// 格式: {env="prod", app="api", level=~"warn|error"} |= "keyword" | tid="..."
func ParseLogQL(query string) (*ParsedQuery, error) {
	pq := &ParsedQuery{
		LabelFilters: make(map[string]string),
		LabelRegex:   make(map[string]string),
	}

	// 提取标签选择器部分 {env="prod", ...}
	labelSelector := extractLabelSelector(query)
	if labelSelector != "" {
		if err := parseLabelSelector(labelSelector, pq); err != nil {
			return nil, err
		}
	}

	// 解析管道过滤器的处理逻辑继续
	pipelineFilters := extractPipelineFilters(query)
	for _, filter := range pipelineFilters {
		if err := parsePipelineFilter(filter, pq); err != nil {
			return nil, err
		}
	}

	return pq, nil
}

// 正则表达式用于提取标签选择器和管道过滤器
var labelSelectorRe = regexp.MustCompile(`^\{([^}]*)\}`)
var pipelineFilterRe = regexp.MustCompile(`\|\s*(.+?)$`)

func extractLabelSelector(query string) string {
	matches := labelSelectorRe.FindStringSubmatch(query)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func extractPipelineFilters(query string) []string {
	// 移除标签选择器部分
	rest := labelSelectorRe.ReplaceAllString(query, "")
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return nil
	}

	// 分割多个管道过滤器
	parts := strings.Split(rest, "|")
	filters := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			filters = append(filters, part)
		}
	}
	return filters
}

// 标签解析器处理精确和正则匹配
var labelFilterRe = regexp.MustCompile(`(\w+)=~?"([^"]*)"`)
var labelRegexRe = regexp.MustCompile(`(\w+)=~"([^"]*)"`)

func parseLabelSelector(selector string, pq *ParsedQuery) error {
	// 分割逗号分隔的标签
	parts := strings.Split(selector, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// 尝试匹配正则表达式 level=~"..."
		if matches := labelRegexRe.FindStringSubmatch(part); len(matches) == 3 {
			pq.LabelRegex[matches[1]] = matches[2]
			continue
		}

		// 尝试匹配精确匹配 env="..."
		if matches := labelFilterRe.FindStringSubmatch(part); len(matches) == 3 {
			pq.LabelFilters[matches[1]] = matches[2]
			continue
		}

		return fmt.Errorf("invalid label filter: %s", part)
	}
	return nil
}

// 管道过滤器解析逻辑
var keywordRe = regexp.MustCompile(`\|=\s*"([^"]*)"`)
var traceIDRe = regexp.MustCompile(`tid\s*=\s*"([^"]*)"`)

func parsePipelineFilter(filter string, pq *ParsedQuery) error {
	// 检查关键字过滤器 |= "keyword"
	if matches := keywordRe.FindStringSubmatch(filter); len(matches) == 2 {
		pq.Keyword = matches[1]
		return nil
	}

	// 检查 trace ID 过滤器 | tid="..."
	if matches := traceIDRe.FindStringSubmatch(filter); len(matches) == 2 {
		pq.TraceID = matches[1]
		return nil
	}

	return fmt.Errorf("unsupported pipeline filter: %s", filter)
}

// ToESQuery 将解析后的查询转换为 ES 查询
func ToESQuery(pq *ParsedQuery) (map[string]interface{}, error) {
	query := map[string]interface{}{
		"bool": map[string]interface{}{
			"must": []interface{}{},
		},
	}

	mustList := query["bool"].(map[string]interface{})["must"].([]interface{})

	// 添加时间范围（会在调用处添加）
	// 添加标签精确匹配
	// ES 默认将字符串字段映射为 text，精确匹配需要使用 .keyword 子字段
	for key, value := range pq.LabelFilters {
		mustList = append(mustList, map[string]interface{}{
			"term": map[string]interface{}{
				key + ".keyword": value,
			},
		})
	}

	// 添加标签正则匹配
	for key, pattern := range pq.LabelRegex {
		mustList = append(mustList, map[string]interface{}{
			"regexp": map[string]interface{}{
				key: pattern,
			},
		})
	}

	// 添加关键词搜索
	if pq.Keyword != "" {
		mustList = append(mustList, map[string]interface{}{
			"query_string": map[string]interface{}{
				"query": pq.Keyword,
			},
		})
	}

	// 添加 trace ID 过滤
	if pq.TraceID != "" {
		mustList = append(mustList, map[string]interface{}{
			"term": map[string]interface{}{
				"tid.keyword": pq.TraceID,
			},
		})
	}

	query["bool"].(map[string]interface{})["must"] = mustList

	return query, nil
}

// ToESQueryJSON 返回 ES 查询的 JSON 字符串
func ToESQueryJSON(pq *ParsedQuery) (string, error) {
	query, err := ToESQuery(pq)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(query)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
