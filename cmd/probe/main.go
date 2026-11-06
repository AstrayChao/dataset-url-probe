package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, handled, err := parseConfig(os.Args[1:])
	if err != nil {
		return err
	}
	if handled {
		return nil
	}

	started := time.Now()
	if err := os.MkdirAll(cfg.OutDir, 0o755); err != nil {
		return fmt.Errorf("创建输出目录失败: %w", err)
	}

	targets, err := loadTargets(cfg.Input, cfg.Limit)
	if err != nil {
		return fmt.Errorf("读取输入失败: %w", err)
	}
	if len(targets) == 0 {
		return fmt.Errorf("没有从 %s 读到任何域名", cfg.Input)
	}
	if !cfg.Quiet {
		fmt.Fprintf(os.Stderr, "loaded %d targets from %s\n", len(targets), cfg.Input)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if cfg.MaxDuration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.MaxDuration)
		defer cancel()
	}

	results := runProbes(ctx, targets, cfg)
	duration := time.Since(started)

	resultsName := "results.csv"
	summaryJSONName := "summary.json"
	summaryMDName := "summary.md"
	if cfg.Dated {
		// 用服务器本地日期命名，便于和 systemd timer 的本地时区对齐。
		stamp := started.Format("2006-01-02")
		resultsName = "results-" + stamp + ".csv"
		summaryJSONName = "summary-" + stamp + ".json"
		summaryMDName = "summary-" + stamp + ".md"
	}

	resultsPath := filepath.Join(cfg.OutDir, resultsName)
	summaryJSONPath := filepath.Join(cfg.OutDir, summaryJSONName)
	summaryMDPath := filepath.Join(cfg.OutDir, summaryMDName)

	if err := writeCSVFile(resultsPath, results); err != nil {
		return fmt.Errorf("写 CSV 失败: %w", err)
	}
	summary := buildSummary(cfg, results, started, duration)
	if err := writeSummaryJSON(summaryJSONPath, summary); err != nil {
		return fmt.Errorf("写 summary.json 失败: %w", err)
	}
	if err := writeSummaryMarkdown(summaryMDPath, summary); err != nil {
		return fmt.Errorf("写 summary.md 失败: %w", err)
	}

	fmt.Fprintf(os.Stderr,
		"done in %s: total=%d probed=%d success=%d (%.2f%%) reachable=%d (%.2f%%)\n",
		duration.Round(time.Second), summary.Total, summary.Probed, summary.Success,
		summary.SuccessRate*100, summary.Reachable, summary.ReachableRate*100)
	fmt.Fprintf(os.Stderr, "wrote %s, %s, %s\n", resultsPath, summaryJSONPath, summaryMDPath)
	return nil
}

func runProbes(ctx context.Context, targets []Target, cfg Config) []Result {
	results := make([]Result, len(targets))
	for i := range targets {
		results[i] = Result{
			Domain:         targets[i].Raw,
			DatasetCount:   targets[i].DatasetCount,
			URLOccurrences: targets[i].URLOccurrences,
			DistinctURLs:   targets[i].DistinctURLs,
			UsageRecords:   targets[i].UsageRecords,
			Line:           targets[i].Line,
			ErrorKind:      "not_run",
		}
	}

	prober := newProber(cfg)
	jobs := make(chan int, cfg.Concurrency*2)
	var wg sync.WaitGroup
	var completed int64
	started := time.Now()

	for w := 0; w < cfg.Concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				results[idx] = prober.Probe(ctx, targets[idx])
				n := atomic.AddInt64(&completed, 1)
				if !cfg.Quiet && (n%200 == 0 || int(n) == len(targets)) {
					printProgress(n, int64(len(targets)), started)
				}
			}
		}()
	}

feed:
	for i := range targets {
		select {
		case jobs <- i:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()
	if !cfg.Quiet {
		fmt.Fprintln(os.Stderr)
	}

	skippedKind := "aborted"
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		skippedKind = "skipped_timeout"
	}
	for i := range results {
		if results[i].ErrorKind == "not_run" {
			results[i].ErrorKind = skippedKind
			if ctx.Err() != nil {
				results[i].Error = ctx.Err().Error()
			}
		}
	}
	return results
}

func printProgress(done, total int64, started time.Time) {
	elapsed := time.Since(started)
	rate := float64(done) / elapsed.Seconds()
	eta := time.Duration(0)
	if rate > 0 && total > done {
		eta = time.Duration(float64(total-done)/rate) * time.Second
	}
	fmt.Fprintf(os.Stderr, "\r[%d/%d] %.1f/s eta=%s   ", done, total, rate, eta.Round(time.Second))
}
