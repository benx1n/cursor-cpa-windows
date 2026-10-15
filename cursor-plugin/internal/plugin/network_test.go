package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"

	"cursorplugin/internal/cursorauth"

	"github.com/stretchr/testify/require"
)

// Exercise the actual default auth and model clients through a CONNECT proxy.
// The tunnel terminates at a trusted local TLS server, so no external requests
// or real login credentials are needed to catch missing proxy wiring.
func TestHandlerUsesHostProxyForLoginAndModels(t *testing.T) {
	var polls, models, connects atomic.Int32
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")
		if r.URL.Path == "/auth/poll" {
			polls.Add(1)
			if r.URL.Query().Get("uuid") == "" || r.URL.Query().Get("verifier") == "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusNotFound)
			return
		}
		models.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer upstream.Close()
	upstreamURL, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != "api2.cursor.sh:443" {
			http.Error(w, "unexpected proxy request", http.StatusBadRequest)
			return
		}
		connects.Add(1)
		remote, err := net.Dial("tcp", upstreamURL.Host)
		if err != nil {
			http.Error(w, "test upstream unavailable", http.StatusBadGateway)
			return
		}
		client, buffer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			remote.Close()
			return
		}
		defer client.Close()
		defer remote.Close()
		_, _ = fmt.Fprint(buffer, "HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = buffer.Flush()
		go func() {
			_, _ = io.Copy(remote, buffer)
			remote.Close()
		}()
		_, _ = io.Copy(client, remote)
	}))
	defer proxy.Close()
	previousTransport := http.DefaultTransport
	transport := previousTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = upstream.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	transport.TLSClientConfig.ServerName = upstreamURL.Hostname()
	http.DefaultTransport = transport
	defer func() { http.DefaultTransport = previousTransport }()

	handler := NewHandler(Dependencies{})
	raw, ok := handler.CallWithStatus(context.Background(), "auth.login.start", marshalNetworkRequest(t, map[string]any{
		"Host": map[string]string{"ProxyURL": proxy.URL},
	}))
	require.True(t, ok, string(raw))
	var response struct {
		Result struct{ State string }
	}
	require.NoError(t, json.Unmarshal(raw, &response))
	raw, ok = handler.CallWithStatus(context.Background(), "auth.login.poll", marshalNetworkRequest(t, map[string]any{
		"State": response.Result.State, "Host": map[string]string{"ProxyURL": proxy.URL},
	}))
	require.True(t, ok, string(raw))
	require.Contains(t, string(raw), `"Status":"pending"`)
	require.EqualValues(t, 1, polls.Load())

	credentials, err := cursorauth.MarshalCredentials(cursorauth.Credentials{Type: "cursor", AccessToken: "test-access", RefreshToken: "test-refresh"})
	require.NoError(t, err)
	// Host is intentionally omitted: executor/management calls also use the last
	// host snapshot, and must not fall back to the GUI process environment.
	raw, ok = handler.CallWithStatus(context.Background(), "model.for_auth", marshalNetworkRequest(t, map[string]any{"StorageJSON": credentials}))
	require.False(t, ok)
	require.Contains(t, string(raw), "401")
	require.EqualValues(t, 1, models.Load())
	require.EqualValues(t, 2, connects.Load())
}

func TestProxyContextsKeepAccountOverridesIsolated(t *testing.T) {
	settings := &networkSettings{}
	ctx, err := settings.context(context.Background(), "model.static", []byte(`{"Host":{"ProxyURL":"http://global.example:7890"}}`))
	require.NoError(t, err)
	require.Equal(t, "http://global.example:7890", ctx.Value(proxyContextKey{}))

	var workers sync.WaitGroup
	for index := range 24 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			expected := fmt.Sprintf("http://account-%d.example:7890", index)
			raw, marshalErr := json.Marshal(map[string]any{
				"StorageJSON":  []byte(`{"proxy_url":"http://stored.example:7890"}`),
				"AuthMetadata": map[string]string{"proxy_url": expected},
			})
			if marshalErr != nil {
				t.Error(marshalErr)
				return
			}
			ctx, contextErr := settings.context(context.Background(), "executor.execute", raw)
			if contextErr != nil {
				t.Error(contextErr)
				return
			}
			if got := ctx.Value(proxyContextKey{}); got != expected {
				t.Errorf("proxy = %v; want %s", got, expected)
			}
		}()
	}
	workers.Wait()
	require.Equal(t, "http://global.example:7890", settings.proxy())
	_, err = settings.context(context.Background(), "auth.login.poll", []byte(`{"Host":{"ProxyURL":""}}`))
	require.NoError(t, err)
	require.Empty(t, settings.proxy())
	// A later host update must not change an already running request.
	require.Equal(t, "http://global.example:7890", ctx.Value(proxyContextKey{}))
}

func TestProxyExplicitDirectAndInvalidSettings(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "https://api2.cursor.sh/auth/poll", nil)
	require.NoError(t, err)
	for _, value := range []string{"direct", "NONE", " direct "} {
		proxy, err := resolveProxy(request, value)
		require.NoError(t, err)
		require.Nil(t, proxy)
	}
	for _, value := range []string{"http://localhost:7890", "https://localhost:7890", "SOCKS5://localhost:1080", "socks5h://user:password@localhost:1080"} {
		proxy, err := resolveProxy(request, value)
		require.NoError(t, err)
		require.NotNil(t, proxy)
	}
	for _, value := range []string{"password@example.invalid", "ftp://secret@localhost", "http://user:secret@localhost/path", "http://localhost/?token=secret", "http://%secret"} {
		_, err := resolveProxy(request, value)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret")
	}
}

func marshalNetworkRequest(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return raw
}
