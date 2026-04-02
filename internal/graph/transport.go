package graph

import (
	"context"
	"net"
	"net/http"
	"time"
)

// NewIPv4Transport returns an http.Transport that only resolves IPv4 addresses.
// Required for Azure NZ North which has broken IPv6 egress.
func NewIPv4Transport() *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			// Force tcp4 regardless of what the caller requests
			d := &net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}
			return d.DialContext(ctx, "tcp4", addr)
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:  10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

// NewIPv4HTTPClient returns an *http.Client with IPv4-only transport.
// This satisfies azcore's policy.Transporter interface (has Do method)
// and can be used as azcore.ClientOptions.Transport.
func NewIPv4HTTPClient() *http.Client {
	return &http.Client{
		Transport: NewIPv4Transport(),
	}
}

