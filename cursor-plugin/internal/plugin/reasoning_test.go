package plugin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"cursorplugin/internal/cursorapi"
	"cursorplugin/internal/cursorproto"

	"github.com/stretchr/testify/require"
)

func TestHandlerRoutesReasoningForJSONAndSSE(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "sse"}[stream], func(t *testing.T) {
			client := &recordingCursorClient{steps: []cursorRunStep{{events: []cursorproto.ServerEvent{
				{Kind: cursorproto.EventThinking, Text: "separate thought"}, {Kind: cursorproto.EventText, Text: "answer"}, {Kind: cursorproto.EventDone},
			}}}}
			emitter := &captureEmitter{done: make(chan struct{})}
			handler := NewHandler(Dependencies{Cursor: client, Emitter: emitter})
			raw, err := json.Marshal(executorRequest{StreamID: "reasoning", StorageJSON: []byte(`{"type":"cursor","access_token":"test","refresh_token":"test"}`), Payload: []byte(`{"model":"cursor/auto","messages":[{"role":"user","content":"hello"}]}`)})
			require.NoError(t, err)
			method := "executor.execute"
			if stream {
				method = "executor.execute_stream"
			}
			response, ok := handler.CallWithStatus(context.Background(), method, raw)
			require.True(t, ok, string(response))
			if stream {
				select {
				case <-emitter.done:
				case <-time.After(time.Second):
					t.Fatal("stream did not finish")
				}
				require.NoError(t, emitter.closeError)
				var output strings.Builder
				for _, payload := range emitter.payloads {
					output.Write(payload)
				}
				require.Contains(t, output.String(), `"reasoning_content":"separate thought"`)
				require.Contains(t, output.String(), `"content":"answer"`)
			} else {
				var outer struct{ Result executorResponse }
				require.NoError(t, json.Unmarshal(response, &outer))
				require.Contains(t, string(outer.Result.Payload), `"reasoning_content":"separate thought"`)
			}
		})
	}
}

type drainingCursorClient struct {
	fakeCursorClient
	doneObserved, release chan struct{}
}

func (client *drainingCursorClient) Run(_ context.Context, _ cursorapi.RunInput, emit func(cursorproto.ServerEvent) error) (cursorapi.RunResult, error) {
	if err := emit(cursorproto.ServerEvent{Kind: cursorproto.EventText, Text: "answer"}); err != nil {
		return cursorapi.RunResult{}, err
	}
	if err := emit(cursorproto.ServerEvent{Kind: cursorproto.EventDone}); err != nil {
		return cursorapi.RunResult{}, err
	}
	close(client.doneObserved)
	<-client.release
	return cursorapi.RunResult{OutputExposed: true}, nil
}

func TestStreamWaitsForUpstreamDrainBeforeTerminalMarker(t *testing.T) {
	client := &drainingCursorClient{doneObserved: make(chan struct{}), release: make(chan struct{})}
	emitter := &captureEmitter{done: make(chan struct{})}
	handler := NewHandler(Dependencies{Cursor: client, Emitter: emitter})
	raw, err := json.Marshal(executorRequest{StreamID: "drain", StorageJSON: []byte(`{"type":"cursor","access_token":"test","refresh_token":"test"}`), Payload: []byte(`{"model":"cursor/auto","messages":[{"role":"user","content":"hello"}]}`)})
	require.NoError(t, err)
	_, ok := handler.CallWithStatus(context.Background(), "executor.execute_stream", raw)
	require.True(t, ok)
	select {
	case <-client.doneObserved:
	case <-time.After(time.Second):
		t.Fatal("upstream did not finish")
	}
	emitter.mu.Lock()
	count := len(emitter.payloads)
	emitter.mu.Unlock()
	close(client.release)
	require.Equal(t, 1, count, "terminal chunks must wait until upstream cleanup finishes")
	select {
	case <-emitter.done:
	case <-time.After(time.Second):
		t.Fatal("stream did not close")
	}
	require.NoError(t, emitter.closeError)
	require.Equal(t, "[DONE]", string(emitter.payloads[len(emitter.payloads)-1]))
}
