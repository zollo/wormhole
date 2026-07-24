package proxy

import (
	"bufio"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// newProxyServer starts the proxy on a random port and returns its base URL.
func newProxyServer(t *testing.T, opts Options) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(New(opts))
	t.Cleanup(srv.Close)
	return srv
}

// clientThrough returns an http.Client that routes all traffic through proxyURL.
func clientThrough(t *testing.T, proxyURL string) *http.Client {
	t.Helper()
	u, err := url.Parse(proxyURL)
	if err != nil {
		t.Fatalf("parse proxy url: %v", err)
	}
	return &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(u)}}
}

func TestForwardsHTTP(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Origin", "yes")
		io.WriteString(w, "hello from origin")
	}))
	t.Cleanup(origin.Close)

	proxySrv := newProxyServer(t, Options{})
	client := clientThrough(t, proxySrv.URL)

	resp, err := client.Get(origin.URL)
	if err != nil {
		t.Fatalf("request through proxy: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Origin"); got != "yes" {
		t.Errorf("X-Origin = %q, want yes", got)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello from origin" {
		t.Errorf("body = %q", body)
	}
}

func TestAddsForwardedForHeader(t *testing.T) {
	var gotXFF string
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotXFF = r.Header.Get("X-Forwarded-For")
	}))
	t.Cleanup(origin.Close)

	proxySrv := newProxyServer(t, Options{})
	client := clientThrough(t, proxySrv.URL)

	resp, err := client.Get(origin.URL)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()

	if gotXFF == "" {
		t.Error("expected X-Forwarded-For to be set")
	}
}

func TestRejectsNonAbsoluteURL(t *testing.T) {
	// A direct (non-proxied) request has a relative RequestURI.
	proxySrv := newProxyServer(t, Options{})
	resp, err := http.Get(proxySrv.URL + "/")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestBasicAuth(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ok")
	}))
	t.Cleanup(origin.Close)

	proxySrv := newProxyServer(t, Options{Username: "alice", Password: "s3cret"})

	t.Run("missing credentials rejected", func(t *testing.T) {
		client := clientThrough(t, proxySrv.URL)
		resp, err := client.Get(origin.URL)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusProxyAuthRequired {
			t.Fatalf("status = %d, want 407", resp.StatusCode)
		}
	})

	t.Run("valid credentials accepted", func(t *testing.T) {
		u, _ := url.Parse(proxySrv.URL)
		u.User = url.UserPassword("alice", "s3cret")
		client := clientThrough(t, u.String())
		resp, err := client.Get(origin.URL)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
	})

	t.Run("wrong password rejected", func(t *testing.T) {
		u, _ := url.Parse(proxySrv.URL)
		u.User = url.UserPassword("alice", "wrong")
		client := clientThrough(t, u.String())
		resp, err := client.Get(origin.URL)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusProxyAuthRequired {
			t.Fatalf("status = %d, want 407", resp.StatusCode)
		}
	})
}

// TestConnectTunnel verifies the CONNECT path by tunnelling a raw request to a
// plain TCP echo-ish HTTP server.
func TestConnectTunnel(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "tunneled")
	}))
	t.Cleanup(origin.Close)
	originHost := strings.TrimPrefix(origin.URL, "http://")

	proxySrv := newProxyServer(t, Options{})
	proxyHost := strings.TrimPrefix(proxySrv.URL, "http://")

	conn, err := net.Dial("tcp", proxyHost)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer conn.Close()

	// Ask the proxy to open a tunnel to the origin.
	if _, err := io.WriteString(conn, "CONNECT "+originHost+" HTTP/1.1\r\nHost: "+originHost+"\r\n\r\n"); err != nil {
		t.Fatalf("write CONNECT: %v", err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: "CONNECT"})
	if err != nil {
		t.Fatalf("read CONNECT response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CONNECT status = %d, want 200", resp.StatusCode)
	}

	// Now speak plain HTTP through the tunnel.
	if _, err := io.WriteString(conn, "GET / HTTP/1.1\r\nHost: "+originHost+"\r\nConnection: close\r\n\r\n"); err != nil {
		t.Fatalf("write tunneled request: %v", err)
	}
	body, _ := io.ReadAll(br)
	if !strings.Contains(string(body), "tunneled") {
		t.Errorf("tunneled response = %q, want to contain %q", body, "tunneled")
	}
}

func TestParseProxyBasicAuth(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		wantUser string
		wantPass string
		wantOK   bool
	}{
		{"valid", "Basic " + base64.StdEncoding.EncodeToString([]byte("u:p")), "u", "p", true},
		{"case insensitive scheme", "basic " + base64.StdEncoding.EncodeToString([]byte("u:p")), "u", "p", true},
		{"empty", "", "", "", false},
		{"wrong scheme", "Bearer xyz", "", "", false},
		{"bad base64", "Basic !!!!", "", "", false},
		{"no colon", "Basic " + base64.StdEncoding.EncodeToString([]byte("nocolon")), "", "", false},
		{"password with colon", "Basic " + base64.StdEncoding.EncodeToString([]byte("u:p:q")), "u", "p:q", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, pass, ok := parseProxyBasicAuth(tt.header)
			if ok != tt.wantOK || user != tt.wantUser || pass != tt.wantPass {
				t.Errorf("got (%q,%q,%v), want (%q,%q,%v)",
					user, pass, ok, tt.wantUser, tt.wantPass, tt.wantOK)
			}
		})
	}
}

func TestRemoveHopByHopHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("Connection", "X-Custom-Hop")
	h.Set("X-Custom-Hop", "should-be-removed")
	h.Set("Proxy-Connection", "keep-alive")
	h.Set("X-Keep", "keep-me")

	removeHopByHopHeaders(h)

	if h.Get("X-Custom-Hop") != "" {
		t.Error("connection-listed header not removed")
	}
	if h.Get("Proxy-Connection") != "" {
		t.Error("standard hop-by-hop header not removed")
	}
	if h.Get("X-Keep") != "keep-me" {
		t.Error("end-to-end header wrongly removed")
	}
}
