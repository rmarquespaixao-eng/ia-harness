package openai_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmarquespaixao-eng/ia-harness/adapters/provider/openai"
	"github.com/rmarquespaixao-eng/ia-harness/harness"
)

const retryOKSSE = "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"

func TestChat_RetryEmRateLimitRecupera(t *testing.T) {
	// Arrange — 429 nas duas primeiras tentativas, 200 na terceira.
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"slow down"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(retryOKSSE))
	}))
	t.Cleanup(srv.Close)
	prov := openai.New(openai.Config{BaseURL: srv.URL, MaxAttempts: 3, RetryBaseDelay: time.Millisecond}, openai.Deps{})
	t.Cleanup(func() { _ = prov.Close() })

	// Act
	resp, err := prov.Chat(context.Background(), harness.ChatRequest{Model: "m"}, nil)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, int32(3), atomic.LoadInt32(&calls))
	require.Len(t, resp.Message.Parts, 1)
	assert.Equal(t, "ok", resp.Message.Parts[0].Text)
}

func TestChat_NaoRepeteErroPermanente(t *testing.T) {
	// Arrange — 400 é permanente: não deve ser repetido.
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad"}}`))
	}))
	t.Cleanup(srv.Close)
	prov := openai.New(openai.Config{BaseURL: srv.URL, MaxAttempts: 3, RetryBaseDelay: time.Millisecond}, openai.Deps{})
	t.Cleanup(func() { _ = prov.Close() })

	// Act
	_, err := prov.Chat(context.Background(), harness.ChatRequest{Model: "m"}, nil)

	// Assert
	require.Error(t, err)
	var provErr *harness.ProviderError
	require.ErrorAs(t, err, &provErr)
	assert.Equal(t, "provider_http", provErr.Code)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls), "4xx não é repetido")
}

func TestChat_MaxAttemptsUmNaoRepete(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"message":"down"}}`))
	}))
	t.Cleanup(srv.Close)
	prov := openai.New(openai.Config{BaseURL: srv.URL, MaxAttempts: 1}, openai.Deps{})
	t.Cleanup(func() { _ = prov.Close() })

	_, err := prov.Chat(context.Background(), harness.ChatRequest{Model: "m"}, nil)

	require.Error(t, err)
	var provErr *harness.ProviderError
	require.ErrorAs(t, err, &provErr)
	assert.Equal(t, "provider_unavailable", provErr.Code)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
}
