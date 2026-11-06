package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testConfig() Config {
	return Config{
		Concurrency:  1,
		Timeout:      5 * time.Second,
		MaxRedirects: 3,
		UserAgent:    "dataset-url-probe-test",
		SkipPrivate:  false,
	}
}

func TestNormalize(t *testing.T) {
	cases := []struct {
		in     string
		scheme string
		host   string
		path   string
		wantOK bool
	}{
		{"github.com", "", "github.com", "/", true},
		{"HTTPS://GitHub.com/a/b?x=1", "https", "github.com", "/a/b", true},
		{"http://example.com:8080/x", "http", "example.com", "/x", true},
		{"example.com/path?q=1", "", "example.com", "/path", true},
		{"10.9.20.10:8080", "", "10.9.20.10", "/", true},
		{"fcon_1000.projects.nitrc.org", "", "fcon_1000.projects.nitrc.org", "/", true},
		{"  example.com  ", "", "example.com", "/", true},
		{"", "", "", "", false},
		{"ftp://example.com", "", "", "", false},
		{"a b.com", "", "", "", false},
	}
	for _, tc := range cases {
		u, err := normalize(tc.in)
		if tc.wantOK {
			if err != nil {
				t.Fatalf("normalize(%q) error: %v", tc.in, err)
			}
			if u.Scheme != tc.scheme || u.Host != tc.host || u.Path != tc.path {
				t.Fatalf("normalize(%q) = %+v, want scheme=%q host=%q path=%q", tc.in, u, tc.scheme, tc.host, tc.path)
			}
		} else if err == nil {
			t.Fatalf("normalize(%q) expected error, got %+v", tc.in, u)
		}
	}
}

func TestNormalizeIDN(t *testing.T) {
	u, err := normalize("doñasol.com")
	if err != nil {
		t.Fatalf("normalize IDN: %v", err)
	}
	if !strings.HasPrefix(u.Host, "xn--") {
		t.Fatalf("expected punycode host, got %q", u.Host)
	}
}

func TestIsPrivateHost(t *testing.T) {
	for _, host := range []string{"10.9.20.10", "127.0.0.1", "192.168.1.1", "::1", "169.254.1.1"} {
		if !isPrivateHost(host) {
			t.Errorf("%s should be private", host)
		}
	}
	for _, host := range []string{"202.118.75.18", "github.com", "8.8.8.8"} {
		if isPrivateHost(host) {
			t.Errorf("%s should not be private", host)
		}
	}
}

func TestProbeHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := newProber(testConfig())
	res := p.Probe(context.Background(), Target{Raw: srv.URL})
	if !res.Success || !res.Reachable || res.HTTPCode != 200 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.ErrorKind != "" || res.Error != "" {
		t.Fatalf("unexpected error: %+v", res)
	}
}

func TestProbeHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	p := newProber(testConfig())
	res := p.Probe(context.Background(), Target{Raw: srv.URL})
	if res.Success || !res.Reachable || res.HTTPCode != 403 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.ErrorKind != "http_4xx" {
		t.Fatalf("expected http_4xx, got %q", res.ErrorKind)
	}
}

func TestProbeFollowsRedirect(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/new", http.StatusFound)
	})
	mux.HandleFunc("/new", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newProber(testConfig())
	res := p.Probe(context.Background(), Target{Raw: srv.URL + "/old"})
	if !res.Success || res.Redirects != 1 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if !strings.HasSuffix(res.FinalURL, "/new") {
		t.Fatalf("unexpected final url: %s", res.FinalURL)
	}
}

func TestProbeFallbackToHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	raw := strings.TrimPrefix(srv.URL, "http://")
	p := newProber(testConfig())
	res := p.Probe(context.Background(), Target{Raw: raw})
	if !res.Success || res.Scheme != "http" {
		t.Fatalf("expected http fallback success, got %+v", res)
	}
	if res.Attempts != "https,http" {
		t.Fatalf("unexpected attempts: %q", res.Attempts)
	}
}

func TestProbeSelfSignedTLS(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := testConfig()
	cfg.TLSVerify = false
	res := newProber(cfg).Probe(context.Background(), Target{Raw: srv.URL})
	if !res.Success {
		t.Fatalf("self-signed cert should be accepted by default: %+v", res)
	}

	cfg.TLSVerify = true
	res = newProber(cfg).Probe(context.Background(), Target{Raw: srv.URL})
	if res.Success {
		t.Fatalf("strict TLS verification should fail: %+v", res)
	}
	if res.ErrorKind != "tls_error" {
		t.Fatalf("expected tls_error, got %q (%s)", res.ErrorKind, res.Error)
	}
}

func TestProbeSkipPrivate(t *testing.T) {
	cfg := testConfig()
	cfg.SkipPrivate = true
	res := newProber(cfg).Probe(context.Background(), Target{Raw: "127.0.0.1:8080"})
	if res.ErrorKind != "skipped_private" {
		t.Fatalf("expected skipped_private, got %+v", res)
	}
}

func TestLoadTargetsCSV(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "domains.csv")
	content := "\ufeffdomain,dataset_count,url_occurrences,distinct_urls,usage_records\n" +
		"github.com,55420,150463,62422,149803\n" +
		"# comment line\n" +
		"example.com,2,3,4,5\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	targets, err := loadTargets(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
	if targets[0].Raw != "github.com" || targets[0].DatasetCount != 55420 {
		t.Fatalf("unexpected first target: %+v", targets[0])
	}
}

func TestLoadTargetsPlainList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "domains.txt")
	if err := os.WriteFile(path, []byte("github.com\n\n# x\nexample.com\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	targets, err := loadTargets(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Raw != "github.com" {
		t.Fatalf("unexpected targets: %+v", targets)
	}
}
