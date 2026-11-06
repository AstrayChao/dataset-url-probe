package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Target 是输入清单中的一行。
type Target struct {
	Raw            string
	DatasetCount   int64
	URLOccurrences int64
	DistinctURLs   int64
	UsageRecords   int64
	Line           int
}

var domainHeaderNames = map[string]bool{
	"domain": true, "域名": true, "host": true, "hostname": true,
	"url": true, "网址": true, "dataset_url": true, "链接": true,
}

var countColumns = map[string]int{
	"dataset_count":   1,
	"数据集数":            1,
	"url_occurrences": 2,
	"url 出现次数":        2,
	"distinct_urls":   3,
	"不同 url 数":        3,
	"usage_records":   4,
	"使用记录数":           4,
}

// loadTargets 支持 CSV / TSV / 纯文本清单。
func loadTargets(path string, limit int) ([]Target, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("%s 是空文件", path)
	}

	var targets []Target
	switch strings.ToLower(filepath.Ext(path)) {
	case ".csv", ".tsv":
		sep := ','
		if strings.EqualFold(filepath.Ext(path), ".tsv") {
			sep = '\t'
		}
		targets, err = parseDelimited(data, sep)
	default:
		targets, err = parseLines(data)
	}
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(targets) > limit {
		targets = targets[:limit]
	}
	return targets, nil
}

func parseDelimited(data []byte, sep rune) ([]Target, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = sep
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("解析 CSV 失败: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("CSV 没有数据行")
	}

	header := records[0]
	hasHeader := false
	for _, cell := range header {
		if domainHeaderNames[strings.ToLower(strings.TrimSpace(cell))] {
			hasHeader = true
			break
		}
	}

	colDomain, colCount, colURL, colDistinct, colUsage := 0, 1, 2, 3, 4
	if hasHeader {
		for i, cell := range header {
			name := strings.ToLower(strings.TrimSpace(cell))
			if domainHeaderNames[name] {
				colDomain = i
			}
			if c, ok := countColumns[name]; ok {
				switch c {
				case 1:
					colCount = i
				case 2:
					colURL = i
				case 3:
					colDistinct = i
				case 4:
					colUsage = i
				}
			}
		}
	}

	var targets []Target
	start := 0
	if hasHeader {
		start = 1
	}
	for i := start; i < len(records); i++ {
		row := records[i]
		if colDomain >= len(row) {
			continue
		}
		raw := strings.TrimSpace(row[colDomain])
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		targets = append(targets, Target{
			Raw:            raw,
			DatasetCount:   atoi64(row, colCount),
			URLOccurrences: atoi64(row, colURL),
			DistinctURLs:   atoi64(row, colDistinct),
			UsageRecords:   atoi64(row, colUsage),
			Line:           i + 1,
		})
	}
	return targets, nil
}

func parseLines(data []byte) ([]Target, error) {
	lines := strings.Split(string(data), "\n")
	var targets []Target
	for i, line := range lines {
		s := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		s = strings.TrimPrefix(s, "\ufeff")
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		targets = append(targets, Target{Raw: s, Line: i + 1})
	}
	return targets, nil
}

func atoi64(row []string, idx int) int64 {
	if idx < 0 || idx >= len(row) {
		return 0
	}
	s := strings.TrimSpace(row[idx])
	if s == "" {
		return 0
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return v
}
