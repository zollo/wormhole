// Package proxy implements a small forward HTTP/HTTPS proxy.
//
// It handles plain HTTP requests by forwarding them to the origin server and
// HTTPS (or any TLS) traffic by tunnelling the connection with the CONNECT
// method. The whole thing is built on the Go standard library so it compiles
// to a single dependency-free binary.
package proxy

import (
	"crypto/subtle"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// hopByHopHeaders are headers that apply to a single transport-level
// connection and must not be forwarded by a proxy (RFC 7230 §6.1).
var hopByHopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Proxy-Connection",
	"Te",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

// Proxy is an http.Handler that forwards requests to their origin server.
//
// The zero value is not usable; construct one with New.
type Proxy struct {
	username string
	password string
	authOn   bool

	dialer    *net.Dialer
	transport *http.Transport
	logger    *slog.Logger
}

// Options configures a Proxy.
type Options struct {
	// Username and Password enable proxy Basic authentication. When both are
	// empty the proxy is open and accepts any client.
	Username string
	Password string

	// DialTimeout bounds how long establishing a connection to an origin
	// server may take. Defaults to 10s when zero.
	DialTimeout time.Duration

	// Logger receives structured request logs. Defaults to slog.Default.
	Logger *slog.Logger
}

// New builds a Proxy from the given options.
func New(opts Options) *Proxy {
	if opts.DialTimeout <= 0 {
		opts.DialTimeout = 10 * time.Second
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	dialer := &net.Dialer{
		Timeout:   opts.DialTimeout,
		KeepAlive: 30 * time.Second,
	}

	return &Proxy{
		username: opts.Username,
		password: opts.Password,
		authOn:   opts.Username != "" || opts.Password != "",
		dialer:   dialer,
		logger:   opts.Logger,
		transport: &http.Transport{
			DialContext:           dialer.DialContext,
			ForceAttemptHTTP2:     false,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}

// ServeHTTP dispatches a proxied request. HTTPS traffic arrives as a CONNECT
// request and is tunnelled; everything else is forwarded as a plain HTTP
// request.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !p.authorized(r) {
		w.Header().Set("Proxy-Authenticate", `Basic realm="wormhole"`)
		http.Error(w, "proxy authentication required", http.StatusProxyAuthRequired)
		return
	}

	if r.Method == http.MethodConnect {
		p.handleConnect(w, r)
		return
	}
	p.handleHTTP(w, r)
}

// authorized reports whether the request carries valid proxy credentials. When
// authentication is disabled every request is authorized.
func (p *Proxy) authorized(r *http.Request) bool {
	if !p.authOn {
		return true
	}
	user, pass, ok := parseProxyBasicAuth(r.Header.Get("Proxy-Authorization"))
	if !ok {
		return false
	}
	// Constant-time comparison to avoid leaking credentials via timing.
	userOK := subtle.ConstantTimeCompare([]byte(user), []byte(p.username)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(pass), []byte(p.password)) == 1
	return userOK && passOK
}

// handleHTTP forwards a plain (non-CONNECT) HTTP request to its origin and
// streams the response back to the client.
func (p *Proxy) handleHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	if !r.URL.IsAbs() {
		http.Error(w, "wormhole is a forward proxy; requests must use an absolute URL", http.StatusBadRequest)
		return
	}

	outReq := r.Clone(r.Context())
	outReq.RequestURI = ""
	removeHopByHopHeaders(outReq.Header)
	appendForwardedFor(outReq, r.RemoteAddr)

	resp, err := p.transport.RoundTrip(outReq)
	if err != nil {
		p.logger.Warn("upstream request failed",
			"method", r.Method, "url", r.URL.String(), "error", err)
		http.Error(w, "bad gateway: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	removeHopByHopHeaders(resp.Header)
	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)

	p.logger.Info("http",
		"method", r.Method,
		"url", r.URL.String(),
		"status", resp.StatusCode,
		"client", clientIP(r.RemoteAddr),
		"duration", time.Since(start).String(),
	)
}

// handleConnect establishes a raw TCP tunnel between the client and the
// requested host, used for HTTPS and other TLS-wrapped protocols.
func (p *Proxy) handleConnect(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	upstream, err := p.dialer.Dial("tcp", r.Host)
	if err != nil {
		p.logger.Warn("connect dial failed", "host", r.Host, "error", err)
		http.Error(w, "bad gateway: "+err.Error(), http.StatusBadGateway)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		upstream.Close()
		http.Error(w, "connection hijacking unsupported", http.StatusInternalServerError)
		return
	}

	client, _, err := hijacker.Hijack()
	if err != nil {
		upstream.Close()
		p.logger.Warn("hijack failed", "host", r.Host, "error", err)
		return
	}

	// Tell the client the tunnel is ready. Write directly to the raw
	// connection since the ResponseWriter is no longer usable after hijack.
	if _, err := client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		client.Close()
		upstream.Close()
		return
	}

	tunnel(client, upstream)

	p.logger.Info("connect",
		"host", r.Host,
		"client", clientIP(r.RemoteAddr),
		"duration", time.Since(start).String(),
	)
}

// tunnel wires two connections together, copying bytes in both directions
// until either side closes, then tears both down.
func tunnel(a, b net.Conn) {
	done := make(chan struct{}, 2)
	cp := func(dst, src net.Conn) {
		io.Copy(dst, src)
		// Unblock the peer copy by closing; a half-close would be nicer but
		// net.Conn does not expose it portably.
		dst.Close()
		src.Close()
		done <- struct{}{}
	}
	go cp(a, b)
	go cp(b, a)
	<-done
	<-done
}
