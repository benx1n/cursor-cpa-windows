package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

type proxyContextKey struct{}

// The native ABI includes Host for auth/model calls, but not executor or
// management calls. Retain the most recent host setting for those calls and
// capture it in each request context so concurrent account overrides are isolated.
type networkSettings struct {
	hostProxy atomic.Pointer[string]
}

func (settings *networkSettings) client(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ForceAttemptHTTP2 = true
	transport.Proxy = func(request *http.Request) (*url.URL, error) {
		proxy, ok := request.Context().Value(proxyContextKey{}).(string)
		if !ok {
			proxy = settings.proxy()
		}
		return resolveProxy(request, proxy)
	}
	return &http.Client{Transport: transport, Timeout: timeout}
}

func (settings *networkSettings) proxy() string {
	if value := settings.hostProxy.Load(); value != nil {
		return *value
	}
	return ""
}

func resolveProxy(request *http.Request, value string) (*url.URL, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return http.ProxyFromEnvironment(request)
	}
	if strings.EqualFold(value, "direct") || strings.EqualFold(value, "none") {
		return nil, nil
	}
	proxy, err := url.Parse(value)
	if err != nil || proxy.Hostname() == "" || proxy.RawQuery != "" || proxy.Fragment != "" || proxy.Opaque != "" || (proxy.Path != "" && proxy.Path != "/") {
		return nil, errors.New("invalid Cursor upstream proxy URL")
	}
	proxy.Scheme = strings.ToLower(proxy.Scheme)
	switch proxy.Scheme {
	case "http", "https", "socks5", "socks5h":
		return proxy, nil
	default:
		return nil, errors.New("Cursor upstream proxy requires http, https, socks5 or socks5h")
	}
}

func (settings *networkSettings) context(ctx context.Context, method string, raw []byte) (context.Context, error) {
	switch method {
	case "auth.parse", "auth.login.start", "auth.login.poll", "auth.refresh", "model.static", "model.for_auth", "executor.execute", "executor.execute_stream":
	default:
		return context.WithValue(ctx, proxyContextKey{}, settings.proxy()), nil
	}
	var request struct {
		Host     *struct{ ProxyURL string }
		Metadata struct {
			ProxyURL string `json:"proxy_url"`
		}
		AuthMetadata struct {
			ProxyURL string `json:"proxy_url"`
		}
		StorageJSON []byte
	}
	if len(raw) != 0 {
		if err := json.Unmarshal(raw, &request); err != nil {
			return nil, err
		}
	}
	proxy := settings.proxy()
	if request.Host != nil {
		proxy = strings.TrimSpace(request.Host.ProxyURL)
		settings.hostProxy.Store(&proxy)
	}
	if method == "auth.refresh" || method == "model.for_auth" || strings.HasPrefix(method, "executor.") {
		var storage struct {
			ProxyURL string `json:"proxy_url"`
		}
		// Credential validation remains in the auth/executor handlers.
		_ = json.Unmarshal(request.StorageJSON, &storage)
		for _, value := range []string{storage.ProxyURL, request.AuthMetadata.ProxyURL, request.Metadata.ProxyURL} {
			if strings.TrimSpace(value) != "" {
				proxy = strings.TrimSpace(value)
			}
		}
	}
	return context.WithValue(ctx, proxyContextKey{}, proxy), nil
}

func accountProxyContext(ctx context.Context, proxy string) context.Context {
	if proxy = strings.TrimSpace(proxy); proxy != "" {
		return context.WithValue(ctx, proxyContextKey{}, proxy)
	}
	return ctx
}
