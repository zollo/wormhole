package proxy

import (
	"encoding/base64"
	"net"
	"net/http"
	"strings"
)

// copyHeader copies all header values from src into dst.
func copyHeader(dst, src http.Header) {
	for key, values := range src {
		for _, v := range values {
			dst.Add(key, v)
		}
	}
}

// removeHopByHopHeaders strips connection-specific headers that must not be
// forwarded, including any listed in the request's own Connection header.
func removeHopByHopHeaders(h http.Header) {
	for _, name := range strings.Split(h.Get("Connection"), ",") {
		if name = strings.TrimSpace(name); name != "" {
			h.Del(name)
		}
	}
	for _, name := range hopByHopHeaders {
		h.Del(name)
	}
}

// appendForwardedFor records the immediate client's IP in the X-Forwarded-For
// chain so origin servers can see the request path.
func appendForwardedFor(r *http.Request, remoteAddr string) {
	ip := clientIP(remoteAddr)
	if ip == "" {
		return
	}
	if prior := r.Header.Get("X-Forwarded-For"); prior != "" {
		ip = prior + ", " + ip
	}
	r.Header.Set("X-Forwarded-For", ip)
}

// clientIP extracts the host portion from a "host:port" remote address.
func clientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

// parseProxyBasicAuth decodes a "Basic <base64>" Proxy-Authorization value.
func parseProxyBasicAuth(header string) (username, password string, ok bool) {
	const prefix = "Basic "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", "", false
	}
	decoded, err := base64.StdEncoding.DecodeString(header[len(prefix):])
	if err != nil {
		return "", "", false
	}
	user, pass, found := strings.Cut(string(decoded), ":")
	if !found {
		return "", "", false
	}
	return user, pass, true
}
