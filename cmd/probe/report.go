package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

var csvHeader = []string{
	"domain", "url", "http_code", "success", "reachable", "scheme", "attempts",
	"final_url", "redirects", "error_kind", "error", "latency_ms",
	"dataset_count", "url_occurrences", "distinct_urls", "usage_records", "checked_at",
}

func writeCSVFile(path string, results []Result) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// UTF-8 BOM，方便 Excel 直接打开中文域名。
	if _, err := f.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return err
	}
	w := csv.NewWriter(f)
	if err := w.Write(csvHeader); err != nil {
		return err
	}
	for _, r := range results {
		row := []string{
			r.Domain,
			r.URL,
			strconv.Itoa(r.HTTPCode),
			strconv.FormatBool(r.Success),
			strconv.FormatBool(r.Reachable),
			r.Scheme,
			r.Attempts,
			r.FinalURL,
			strconv.Itoa(r.Redirects),
			r.ErrorKind,
			truncate(oneLine(r.Error), 300),
			strconv.FormatInt(r.LatencyMS, 10),
			strconv.FormatInt(r.DatasetCount, 10),
			strconv.FormatInt(r.URLOccurrences, 10),
			strconv.FormatInt(r.DistinctURLs, 10),
			strconv.FormatInt(r.UsageRecords, 10),
			r.CheckedAt.UTC().Format(time.RFC3339),
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

type FailureItem struct {
	Domain       string `json:"domain"`
	HTTPCode     int    `json:"http_code"`
	ErrorKind    string `json:"error_kind"`
	Error        string `json:"error,omitempty"`
	Attempts     string `json:"attempts,omitempty"`
	DatasetCount int64  `json:"dataset_count"`
}

type SlowItem struct {
	Domain    string `json:"domain"`
	HTTPCode  int    `json:"http_code"`
	LatencyMS int64  `json:"latency_ms"`
	Success   bool   `json:"success"`
}

type Summary struct {
	Tool             string            `json:"tool"`
	Version          string            `json:"version"`
	GeneratedAt      string            `json:"generated_at"`
	StartedAt        string            `json:"started_at"`
	DurationSeconds  float64           `json:"duration_seconds"`
	Input            string            `json:"input"`
	Total            int               `json:"total"`
	Probed           int               `json:"probed"`
	Skipped          int               `json:"skipped"`
	Success          int               `json:"success"`
	Reachable        int               `json:"reachable"`
	SuccessRate      float64           `json:"success_rate"`
	ReachableRate    float64           `json:"reachable_rate"`
	AverageLatencyMS int64             `json:"average_latency_ms"`
	ByScheme         map[string]int    `json:"by_scheme"`
	ByStatusClass    map[string]int    `json:"by_status_class"`
	ByStatusCode     map[string]int    `json:"by_status_code"`
	ByErrorKind      map[string]int    `json:"by_error_kind"`
	TopFailures      []FailureItem     `json:"top_failures"`
	Slowest          []SlowItem        `json:"slowest"`
	Config           map[string]any    `json:"config"`
	Environment      map[string]string `json:"environment"`
}

func buildSummary(cfg Config, results []Result, started time.Time, duration time.Duration) Summary {
	s := Summary{
		Tool:            toolName,
		Version:         version,
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
		StartedAt:       started.UTC().Format(time.RFC3339),
		DurationSeconds: round2(duration.Seconds()),
		Input:           cfg.Input,
		Total:           len(results),
		ByScheme:        map[string]int{},
		ByStatusClass:   map[string]int{},
		ByStatusCode:    map[string]int{},
		ByErrorKind:     map[string]int{},
		Config: map[string]any{
			"concurrency":   cfg.Concurrency,
			"timeout":       cfg.Timeout.String(),
			"retries":       cfg.Retries,
			"max_redirects": cfg.MaxRedirects,
			"max_duration":  cfg.MaxDuration.String(),
			"tls_verify":    cfg.TLSVerify,
			"skip_private":  cfg.SkipPrivate,
			"limit":         cfg.Limit,
		},
		Environment: collectEnvironment(),
	}

	var failures []FailureItem
	var slow []SlowItem
	var latencySum int64
	for _, r := range results {
		skipped := isSkippedKind(r.ErrorKind)
		if skipped {
			s.Skipped++
		} else {
			s.Probed++
			if r.Success {
				s.Success++
			}
			if r.Reachable {
				s.Reachable++
			}
			latencySum += r.LatencyMS
			if !r.Success {
				failures = append(failures, FailureItem{
					Domain:       r.Domain,
					HTTPCode:     r.HTTPCode,
					ErrorKind:    r.ErrorKind,
					Error:        truncate(oneLine(r.Error), 200),
					Attempts:     r.Attempts,
					DatasetCount: r.DatasetCount,
				})
			}
		}

		scheme := r.Scheme
		if scheme == "" {
			scheme = "none"
		}
		s.ByScheme[scheme]++
		s.ByStatusCode[strconv.Itoa(r.HTTPCode)]++

		class := "no_response"
		switch {
		case r.HTTPCode >= 200 && r.HTTPCode < 300:
			class = "2xx"
		case r.HTTPCode >= 300 && r.HTTPCode < 400:
			class = "3xx"
		case r.HTTPCode >= 400 && r.HTTPCode < 500:
			class = "4xx"
		case r.HTTPCode >= 500 && r.HTTPCode < 600:
			class = "5xx"
		}
		s.ByStatusClass[class]++

		kind := r.ErrorKind
		if kind == "" {
			kind = "none"
		}
		s.ByErrorKind[kind]++

		slow = append(slow, SlowItem{Domain: r.Domain, HTTPCode: r.HTTPCode, LatencyMS: r.LatencyMS, Success: r.Success})
	}

	if s.Probed > 0 {
		s.SuccessRate = round4(float64(s.Success) / float64(s.Probed))
		s.ReachableRate = round4(float64(s.Reachable) / float64(s.Probed))
		s.AverageLatencyMS = latencySum / int64(s.Probed)
	}

	sort.Slice(failures, func(i, j int) bool {
		if failures[i].DatasetCount != failures[j].DatasetCount {
			return failures[i].DatasetCount > failures[j].DatasetCount
		}
		return failures[i].Domain < failures[j].Domain
	})
	if len(failures) > 100 {
		failures = failures[:100]
	}
	if failures == nil {
		failures = []FailureItem{}
	}
	s.TopFailures = failures

	sort.Slice(slow, func(i, j int) bool { return slow[i].LatencyMS > slow[j].LatencyMS })
	if len(slow) > 20 {
		slow = slow[:20]
	}
	s.Slowest = slow
	return s
}

func writeSummaryJSON(path string, s Summary) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func writeSummaryMarkdown(path string, s Summary) error {
	var b strings.Builder
	b.WriteString("# 数据集域名探活报告\n\n")
	fmt.Fprintf(&b, "- 生成时间（UTC）：%s\n", s.GeneratedAt)
	fmt.Fprintf(&b, "- 运行耗时：%s｜并发：%v｜单次超时：%v\n",
		time.Duration(s.DurationSeconds*float64(time.Second)).Round(time.Second),
		s.Config["concurrency"], s.Config["timeout"])
	fmt.Fprintf(&b, "- 总计：%d｜已探测：%d｜跳过：%d\n", s.Total, s.Probed, s.Skipped)
	fmt.Fprintf(&b, "- **成功（2xx/3xx）：%d（%.2f%%）**｜**可达（有 HTTP 响应）：%d（%.2f%%）**\n\n",
		s.Success, s.SuccessRate*100, s.Reachable, s.ReachableRate*100)

	b.WriteString("## 状态码分布\n\n| 类别 | 数量 |\n| --- | ---: |\n")
	for _, k := range []string{"2xx", "3xx", "4xx", "5xx", "no_response"} {
		fmt.Fprintf(&b, "| %s | %d |\n", k, s.ByStatusClass[k])
	}

	b.WriteString("\n## 结果类型 Top 15\n\n| 类型 | 数量 |\n| --- | ---: |\n")
	type kv struct {
		k string
		v int
	}
	var kinds []kv
	for k, v := range s.ByErrorKind {
		kinds = append(kinds, kv{k, v})
	}
	sort.Slice(kinds, func(i, j int) bool {
		if kinds[i].v != kinds[j].v {
			return kinds[i].v > kinds[j].v
		}
		return kinds[i].k < kinds[j].k
	})
	for i, item := range kinds {
		if i >= 15 {
			break
		}
		fmt.Fprintf(&b, "| %s | %d |\n", item.k, item.v)
	}

	b.WriteString("\n## 失败域名（按数据集数，Top 20）\n\n| 域名 | HTTP | 类型 | 数据集数 |\n| --- | ---: | --- | ---: |\n")
	for i, f := range s.TopFailures {
		if i >= 20 {
			break
		}
		fmt.Fprintf(&b, "| `%s` | %d | %s | %d |\n", escapeMarkdown(f.Domain), f.HTTPCode, f.ErrorKind, f.DatasetCount)
	}

	b.WriteString("\n> 说明：`success` 表示最终 HTTP 状态为 2xx/3xx；`reachable` 表示收到了任意 HTTP 响应（含 401/403/5xx）。\n")
	b.WriteString("> 失败原因可能是站点下线、证书错误、DNS 失效，也可能是目标站点封锁数据中心/IP 段。\n")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func collectEnvironment() map[string]string {
	env := map[string]string{
		"go_version": runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
	}
	if host, err := os.Hostname(); err == nil {
		env["hostname"] = host
	}
	for _, key := range []string{
		"GITHUB_ACTIONS", "GITHUB_WORKFLOW", "GITHUB_RUN_ID", "GITHUB_RUN_NUMBER",
		"GITHUB_SHA", "GITHUB_EVENT_NAME", "GITHUB_REPOSITORY", "RUNNER_NAME",
		"RUNNER_OS", "RUNNER_ARCH", "RUNNER_ENVIRONMENT", "CI",
	} {
		if v := os.Getenv(key); v != "" {
			env[key] = v
		}
	}
	return env
}

func isSkippedKind(kind string) bool {
	switch kind {
	case "skipped_private", "not_run", "aborted", "skipped_timeout":
		return true
	}
	return false
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.Join(strings.Fields(s), " ")
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}

func escapeMarkdown(s string) string {
	s = strings.ReplaceAll(s, "|", "/")
	return strings.ReplaceAll(s, "`", "'")
}

func round2(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}

func round4(v float64) float64 {
	return float64(int64(v*10000+0.5)) / 10000
}
