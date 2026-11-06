package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"
)

const (
	toolName       = "dataset-url-probe"
	defaultUA      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	versionDefault = "dev"
)

// version 由构建参数 -ldflags "-X main.version=..." 注入。
var version = versionDefault

// Config 是探活程序的运行参数。
type Config struct {
	Input        string
	OutDir       string
	Concurrency  int
	Timeout      time.Duration
	Retries      int
	MaxRedirects int
	UserAgent    string
	MaxDuration  time.Duration
	TLSVerify    bool
	SkipPrivate  bool
	Dated        bool
	Limit        int
	Quiet        bool
}

func parseConfig(args []string) (Config, bool, error) {
	var (
		cfg         Config
		showVersion bool
	)

	fs := flag.NewFlagSet(toolName, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&cfg.Input, "input", "", "输入清单（CSV/TXT，第一列或每行为域名/URL）")
	fs.StringVar(&cfg.OutDir, "outdir", "out", "输出目录")
	fs.IntVar(&cfg.Concurrency, "concurrency", 200, "并发探测数")
	fs.DurationVar(&cfg.Timeout, "timeout", 12*time.Second, "单个域名单次尝试的总超时（含重定向）")
	fs.IntVar(&cfg.Retries, "retries", 0, "网络层错误重试次数（不含 HTTP 4xx/5xx）")
	fs.IntVar(&cfg.MaxRedirects, "max-redirects", 5, "最多跟随的重定向次数")
	fs.StringVar(&cfg.UserAgent, "user-agent", defaultUA, "请求 User-Agent")
	fs.DurationVar(&cfg.MaxDuration, "max-duration", 0, "整轮探活的最长耗时，0 表示不限制")
	fs.BoolVar(&cfg.TLSVerify, "tls-verify", false, "严格校验 TLS 证书（默认关闭：证书过期/自签仍视为可访问）")
	fs.BoolVar(&cfg.SkipPrivate, "skip-private", true, "跳过私有/回环/链路本地等字面 IP")
	fs.BoolVar(&cfg.Dated, "dated", false, "输出文件名带日期后缀（便于服务器保留历史）")
	fs.IntVar(&cfg.Limit, "limit", 0, "只探测前 N 条（0 表示全部，便于试跑）")
	fs.BoolVar(&cfg.Quiet, "quiet", false, "不打印进度")
	fs.BoolVar(&showVersion, "version", false, "打印版本后退出")

	if err := fs.Parse(args); err != nil {
		return cfg, false, err
	}
	if showVersion {
		fmt.Printf("%s %s (%s/%s, %s)\n", toolName, version, runtime.GOOS, runtime.GOARCH, runtime.Version())
		return cfg, true, nil
	}
	if cfg.Input == "" {
		return cfg, false, fmt.Errorf("-input 不能为空")
	}
	if cfg.Concurrency < 1 {
		return cfg, false, fmt.Errorf("-concurrency 必须 >= 1")
	}
	if cfg.Timeout <= 0 {
		return cfg, false, fmt.Errorf("-timeout 必须 > 0")
	}
	if cfg.Retries < 0 {
		return cfg, false, fmt.Errorf("-retries 不能为负数")
	}
	if cfg.MaxRedirects < 0 {
		return cfg, false, fmt.Errorf("-max-redirects 不能为负数")
	}
	if cfg.Limit < 0 {
		return cfg, false, fmt.Errorf("-limit 不能为负数")
	}
	return cfg, false, nil
}
