package main

import (
	"os"
	"testing"
)

// TestAllDomainsNormalize 确保仓库内的域名清单都能被解析或给出明确错误。
func TestAllDomainsNormalize(t *testing.T) {
	const path = "../../data/domains.csv"
	if _, err := os.Stat(path); err != nil {
		t.Skip("domains.csv not present")
	}
	targets, err := loadTargets(path, 0)
	if err != nil {
		t.Fatalf("loadTargets: %v", err)
	}
	if len(targets) < 1000 {
		t.Fatalf("unexpectedly few targets: %d", len(targets))
	}

	var invalid []string
	for _, target := range targets {
		if _, err := normalize(target.Raw); err != nil {
			invalid = append(invalid, target.Raw+" => "+err.Error())
		}
	}
	t.Logf("targets=%d invalid=%d", len(targets), len(invalid))
	for i, item := range invalid {
		if i >= 20 {
			break
		}
		t.Logf("invalid: %s", item)
	}
}
