package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Result 是单个域名的探测结果。
type Result struct {
	Domain         string
	URL            string
	HTTPCode       int
	Success        bool
	Reachable      bool
	Scheme         string
	Attempts       string
	FinalURL       string
	Redirects      int
	ErrorKind      string
	Error          string
	LatencyMS      int64
	CheckedAt      time.Time
	DatasetCount   int64
	URLOccurrences int64
	DistinctURLs   int64
	UsageRecords   int64
	Line           int
}

type Prober struct {
	cfg    Config
	client *http.Client
}

type attemptOutcome struct {
	scheme    string
	url       string
	finalURL  string
	status    int
	redirects int
	responded bool
	kind      string
	err       error
}

func newProber(cfg Config) *Prober {
	dialTimeout := cfg.Timeout
	if dialTimeout > 10*time.Second {
		dialTimeout = 10 * time.Second
	}
	if dialTimeout <= 0 {
		dialTimeout = 10 * time.Second
	}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   dialTimeout,
			KeepAlive: -1,
		}).DialContext,
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS10, // 兼容老旧数据集站点
			InsecureSkipVerify: !cfg.TLSVerify,   //nolint:gosec // 默认忽略证书问题，只判断能否访问
		},
		TLSHandshakeTimeout: dialTimeout,
		DisableKeepAlives:   true,
		ForceAttemptHTTP2:   false,
	}
	return &Prober{
		cfg: cfg,
		client: &http.Client{
			Transport: transport,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (p *Prober) Probe(ctx context.Context, t Target) Result {
	started := time.Now()
	res := Result{
		Domain:         t.Raw,
		DatasetCount:   t.DatasetCount,
		URLOccurrences: t.URLOccurrences,
		DistinctURLs:   t.DistinctURLs,
		UsageRecords:   t.UsageRecords,
		Line:           t.Line,
	}

	u, err := normalize(t.Raw)
	if err != nil {
		res.ErrorKind = "invalid_url"
		res.Error = err.Error()
		res.LatencyMS = time.Since(started).Milliseconds()
		res.CheckedAt = time.Now().UTC()
		return res
	}

	schemes := []string{"https", "http"}
	if u.Scheme != "" {
		schemes = []string{u.Scheme}
	}

	res.URL = u.String(schemes[0])
	if p.cfg.SkipPrivate && isPrivateHost(u.Host) {
		res.ErrorKind = "skipped_private"
		res.Error = "私有/回环/链路本地地址，已跳过"
		res.Scheme = schemes[0]
		res.LatencyMS = time.Since(started).Milliseconds()
		res.CheckedAt = time.Now().UTC()
		return res
	}

	var attempted []string
	var last attemptOutcome
	for _, scheme := range schemes {
		attempted = append(attempted, scheme)
		last = p.attempt(ctx, u.String(scheme), scheme)
		res.URL = last.url
		res.Scheme = scheme
		res.FinalURL = last.finalURL
		res.HTTPCode = last.status
		res.Redirects = last.redirects
		res.Reachable = last.responded
		if last.responded {
			res.Success = last.status >= 200 && last.status < 400
			break
		}
		if ctx.Err() != nil {
			break
		}
		// 自动模式下 https 网络层失败（TLS/DNS/超时等）时，回退 http 再试一次。
		if u.Scheme != "" {
			break
		}
	}

	res.Attempts = strings.Join(attempted, ",")
	res.ErrorKind = last.kind
	if last.err != nil {
		res.Error = last.err.Error()
	} else if res.Reachable && !res.Success {
		switch {
		case last.status >= 500:
			res.ErrorKind = "http_5xx"
		case last.status >= 400:
			res.ErrorKind = "http_4xx"
		}
	}

	res.LatencyMS = time.Since(started).Milliseconds()
	res.CheckedAt = time.Now().UTC()
	return res
}

// attempt 对单个 scheme 发起请求并手动跟随重定向，整体受一次 timeout 预算约束。
func (p *Prober) attempt(parent context.Context, rawURL, scheme string) attemptOutcome {
	out := attemptOutcome{scheme: scheme, url: rawURL, finalURL: rawURL}
	cur := rawURL
	deadline := time.Now().Add(p.cfg.Timeout)

	for redirects := 0; ; redirects++ {
		var (
			status   int
			location string
			err      error
			kind     string
		)

		for try := 0; try <= p.cfg.Retries; try++ {
			remain := time.Until(deadline)
			if remain <= 0 {
				err = context.DeadlineExceeded
				kind = "timeout"
				break
			}
			reqCtx, cancel := context.WithTimeout(parent, remain)
			req, rerr := http.NewRequestWithContext(reqCtx, http.MethodGet, cur, nil)
			if rerr != nil {
				cancel()
				err, kind = rerr, "invalid_url"
				break
			}
			p.setHeaders(req)

			var resp *http.Response
			resp, err = p.client.Do(req)
			if err == nil {
				_, _ = io.CopyN(io.Discard, resp.Body, 2048)
				_ = resp.Body.Close()
				cancel()
				status = resp.StatusCode
				if status >= 300 && status < 400 {
					location = resp.Header.Get("Location")
				}
				break
			}
			cancel()
			if parent.Err() != nil {
				break
			}
			kind, _ = classifyError(err)
			if try == p.cfg.Retries || !retryableKind(kind) {
				break
			}
			select {
			case <-time.After(time.Duration(try+1) * 500 * time.Millisecond):
			case <-parent.Done():
			}
		}

		if err != nil {
			if kind == "" {
				kind = "other"
			}
			// 重定向目标不可达时，整条链路按失败处理，不能把上一跳的 3xx 当作成功。
			out.status = 0
			out.responded = false
			out.finalURL = cur
			out.redirects = redirects
			out.kind = kind
			out.err = err
			return out
		}

		out.status = status
		out.finalURL = cur
		out.responded = true

		if status >= 300 && status < 400 && location != "" {
			if redirects >= p.cfg.MaxRedirects {
				out.kind = "too_many_redirects"
				out.err = fmt.Errorf("超过最大重定向次数 %d", p.cfg.MaxRedirects)
				return out
			}
			base, perr := url.Parse(cur)
			if perr != nil {
				out.kind = "redirect_error"
				out.err = perr
				return out
			}
			next, perr := base.Parse(location)
			if perr != nil {
				out.kind = "redirect_error"
				out.err = perr
				return out
			}
			if next.Scheme != "http" && next.Scheme != "https" {
				out.kind = "redirect_error"
				out.err = fmt.Errorf("重定向到不支持的协议 %q", next.Scheme)
				return out
			}
			cur = next.String()
			continue
		}

		out.redirects = redirects
		return out
	}
}

func (p *Prober) setHeaders(req *http.Request) {
	req.Close = true
	req.Header.Set("User-Agent", p.cfg.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
}

func retryableKind(kind string) bool {
	switch kind {
	case "timeout", "conn_reset", "conn_closed", "other":
		return true
	}
	return false
}

func classifyError(err error) (string, string) {
	if err == nil {
		return "", ""
	}
	if errors.Is(err, context.Canceled) {
		return "aborted", err.Error()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout", err.Error()
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout", err.Error()
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "dns_error", err.Error()
	}
	var certErr *tls.CertificateVerificationError
	if errors.As(err, &certErr) {
		return "tls_error", err.Error()
	}
	var unknownAuth x509.UnknownAuthorityError
	if errors.As(err, &unknownAuth) {
		return "tls_error", err.Error()
	}
	var hostnameErr x509.HostnameError
	if errors.As(err, &hostnameErr) {
		return "tls_error", err.Error()
	}
	var invalidCert x509.CertificateInvalidError
	if errors.As(err, &invalidCert) {
		return "tls_error", err.Error()
	}
	var recordErr tls.RecordHeaderError
	if errors.As(err, &recordErr) {
		return "tls_error", err.Error()
	}

	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "connection refused"):
		return "conn_refused", msg
	case strings.Contains(lower, "connection reset"):
		return "conn_reset", msg
	case strings.Contains(lower, "no such host"), strings.Contains(lower, "server misbehaving"):
		return "dns_error", msg
	case strings.Contains(lower, "network is unreachable"), strings.Contains(lower, "no route to host"):
		return "network_unreachable", msg
	case strings.Contains(lower, "i/o timeout"), strings.Contains(lower, "handshake timeout"):
		return "timeout", msg
	case strings.Contains(lower, "tls:"), strings.Contains(lower, "x509:"):
		return "tls_error", msg
	case strings.Contains(lower, "eof"):
		return "conn_closed", msg
	}
	return "other", msg
}
