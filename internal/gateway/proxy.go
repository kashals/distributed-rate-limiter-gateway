package gateway

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

// NewReverseProxy constructs a reverse proxy targeting backendURL.
// Returns an error if the URL is unparseable.
func NewReverseProxy(backendURL string) (*httputil.ReverseProxy, error) {
	target, err := url.Parse(backendURL)
	if err != nil {
		return nil, fmt.Errorf("parse backend url: %w", err)
	}

	proxy := &httputil.ReverseProxy{
		Director:     buildDirector(target),
		ErrorHandler: proxyErrorHandler,
		Transport:    buildTransport(),
	}

	return proxy, nil
}

// buildDirector returns the Director func that mutates each outbound request.
func buildDirector(target *url.URL) func(*http.Request) {
	return func(req *http.Request) {
		// route to backend
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host

		// inject verified user identity; backend consumes X-User-ID, not the raw JWT
		if userID, ok := UserIDFromContext(req.Context()); ok {
			req.Header.Set("X-User-ID", userID)
		}

		// strip authorization header before forwarding
		req.Header.Del("Authorization")

		// set real client ip
		if ip, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
			req.Header.Set("X-Real-IP", ip)
		}

		// append to X-Forwarded-For chain
		if prior := req.Header.Get("X-Forwarded-For"); prior != "" {
			req.Header.Set("X-Forwarded-For", prior+", "+req.RemoteAddr)
		} else {
			req.Header.Set("X-Forwarded-For", req.RemoteAddr)
		}
	}
}

// proxyErrorHandler handles upstream dial failures and transport errors.
func proxyErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	// client disconnected before response; nothing to write
	if errors.Is(err, context.Canceled) {
		return
	}
	w.WriteHeader(http.StatusBadGateway)
}

// buildTransport returns a tuned http.Transport for upstream connections.
func buildTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   50,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}
