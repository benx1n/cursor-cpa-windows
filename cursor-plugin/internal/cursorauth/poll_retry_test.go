package cursorauth

import (
	"context"
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type handshakeTimeout struct{}

func (handshakeTimeout) Error() string   { return "net/http: TLS handshake timeout" }
func (handshakeTimeout) Timeout() bool   { return true }
func (handshakeTimeout) Temporary() bool { return true }

func TestPollLoginRetriesHandshakeTimeoutAndKeepsSession(t *testing.T) {
	calls := 0
	service := NewService(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return nil, handshakeTimeout{}
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header),
			Body: io.NopCloser(strings.NewReader(`{"accessToken":"access","refreshToken":"refresh"}`))}, nil
	})}, DefaultEndpoints())
	start, err := service.StartLogin(context.Background())
	require.NoError(t, err)

	result, err := service.PollLogin(context.Background(), start.State)
	require.NoError(t, err)
	require.Equal(t, LoginPending, result.Status)
	result, err = service.PollLogin(context.Background(), start.State)
	require.NoError(t, err)
	require.Equal(t, LoginSuccess, result.Status)
	require.Equal(t, "access", result.Credentials.AccessToken)
	require.NotContains(t, service.loginStates, start.State)
}

func TestPollLoginBoundsFailuresAndRedactsPKCE(t *testing.T) {
	service := NewService(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, handshakeTimeout{}
	})}, DefaultEndpoints())
	start, err := service.StartLogin(context.Background())
	require.NoError(t, err)
	state := service.loginStates[start.State]
	for range maxPollRetries {
		result, err := service.PollLogin(context.Background(), start.State)
		require.NoError(t, err)
		require.Equal(t, LoginPending, result.Status)
	}
	_, err = service.PollLogin(context.Background(), start.State)
	require.ErrorContains(t, err, "TLS handshake timeout")
	require.NotContains(t, err.Error(), state.Verifier)
	require.NotContains(t, err.Error(), state.UUID)
	require.NotContains(t, err.Error(), "https://")
	var timeout handshakeTimeout
	require.ErrorAs(t, err, &timeout)
	require.NotContains(t, service.loginStates, start.State)
}

func TestPollLoginHealthyPendingResetsFailures(t *testing.T) {
	fail := true
	service := NewService(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		if fail {
			return nil, io.ErrUnexpectedEOF
		}
		return responseWithStatus(http.StatusNotFound), nil
	})}, DefaultEndpoints())
	start, err := service.StartLogin(context.Background())
	require.NoError(t, err)
	for range 2 {
		fail = true
		for range maxPollRetries {
			result, err := service.PollLogin(context.Background(), start.State)
			require.NoError(t, err)
			require.Equal(t, LoginPending, result.Status)
		}
		fail = false
		_, err = service.PollLogin(context.Background(), start.State)
		require.NoError(t, err)
		require.Zero(t, service.loginStates[start.State].Failures)
	}
}

func TestPollLoginRetriesOnlyTemporaryHTTPStatuses(t *testing.T) {
	for _, status := range []int{http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusUnauthorized, http.StatusForbidden, http.StatusBadRequest} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			service := NewService(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return responseWithStatus(status), nil
			})}, DefaultEndpoints())
			start, err := service.StartLogin(context.Background())
			require.NoError(t, err)
			result, err := service.PollLogin(context.Background(), start.State)
			if status == 408 || status == 429 || status >= 500 {
				require.NoError(t, err)
				require.Equal(t, LoginPending, result.Status)
			} else {
				require.ErrorContains(t, err, "HTTP")
			}
		})
	}
}

func TestPollLoginDoesNotRetryCertificateFailureOrCancellation(t *testing.T) {
	for _, cause := range []error{x509.UnknownAuthorityError{}, context.Canceled, errors.New("permanent transport failure")} {
		t.Run(cause.Error(), func(t *testing.T) {
			service := NewService(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, cause
			})}, DefaultEndpoints())
			start, err := service.StartLogin(context.Background())
			require.NoError(t, err)
			_, err = service.PollLogin(context.Background(), start.State)
			require.ErrorIs(t, err, cause)
		})
	}
}

func TestPollLoginChecksExpirationBeforeRequest(t *testing.T) {
	service := NewService(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("expired login must not make a network request")
		return nil, nil
	})}, DefaultEndpoints())
	now := time.Now()
	service.now = func() time.Time { return now }
	start, err := service.StartLogin(context.Background())
	require.NoError(t, err)
	now = now.Add(16 * time.Minute)
	_, err = service.PollLogin(context.Background(), start.State)
	require.ErrorContains(t, err, "expired")
	require.NotContains(t, service.loginStates, start.State)
}
