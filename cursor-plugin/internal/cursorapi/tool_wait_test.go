package cursorapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cursorplugin/internal/cursorproto"

	"github.com/stretchr/testify/require"
)

func TestToolWaitSurvivesSilentArgumentGenerationButPlainChatTimesOut(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := readConnectPayloadRaw(r.Body); err != nil {
			return
		}
		w.Header().Set("Content-Type", "application/connect+proto")
		for range 8 {
			if _, err := w.Write(connectFrame([]byte{0x0a, 0x02, 0x6a, 0x00})); err != nil {
				return
			}
			if err := http.NewResponseController(w).Flush(); err != nil {
				return
			}
			select {
			case <-r.Context().Done():
				return
			case <-time.After(10 * time.Millisecond):
			}
		}
		_, _ = w.Write(connectFrame(mcpExecServerMessage(3, "call-long-args", "exec_command", "cmd", "echo ok")))
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, HTTPClient: server.Client(),
		ProgressTimeout: 40 * time.Millisecond, ToolProgressTimeout: 300 * time.Millisecond,
		FirstDataTimeout: time.Second, FrameSilenceTimeout: time.Second, OverallTimeout: 2 * time.Second})
	require.NoError(t, err)
	input := validRunInput()
	_, err = client.Run(context.Background(), input, func(cursorproto.ServerEvent) error { return nil })
	require.ErrorIs(t, err, ErrProgressTimeout)
	input.Tools = []cursorproto.ToolDefinition{{Name: "exec_command", Parameters: []byte(`{"type":"object"}`)}}
	result, err := client.Run(context.Background(), input, func(cursorproto.ServerEvent) error { return nil })
	require.NoError(t, err)
	require.True(t, result.ToolExposed)
}

func TestNativeWriteRefusalLetsAgentSwitchToClientTool(t *testing.T) {
	replies := make(chan []byte, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := readConnectPayloadRaw(r.Body); err != nil {
			return
		}
		// write_args(path, file_text), with a correlated exec id.
		args := appendWireString(appendWireString(nil, 1, "/client/smoke.html"), 2, "must not be written on gateway")
		exec := appendWireString(appendWireBytes([]byte{8, 42}, 3, args), 15, "write-42")
		w.Header().Set("Content-Type", "application/connect+proto")
		if _, err := w.Write(connectFrame(appendWireBytes(nil, 2, exec))); err != nil {
			return
		}
		if err := http.NewResponseController(w).Flush(); err != nil {
			return
		}
		reply, err := readConnectPayloadRaw(r.Body)
		if err != nil {
			return
		}
		replies <- reply
		_, _ = w.Write(connectFrame(mcpExecServerMessage(43, "call-write", "apply_patch", "input", "patch on client")))
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, HTTPClient: server.Client(), ProgressTimeout: time.Second})
	require.NoError(t, err)
	input := validRunInput()
	input.Tools = []cursorproto.ToolDefinition{{Name: "apply_patch", Parameters: []byte(`{"type":"object"}`)}}
	var tools []string
	result, err := client.Run(context.Background(), input, func(event cursorproto.ServerEvent) error {
		if event.Kind == cursorproto.EventToolCall {
			tools = append(tools, event.Name)
		}
		return nil
	})
	require.NoError(t, err)
	require.True(t, result.InteractionResponded)
	require.Equal(t, []string{"apply_patch"}, tools)
	select {
	case reply := <-replies:
		exec := requireWireBytes(t, reply, 2)
		require.EqualValues(t, 42, requireWireVarint(t, exec, 1))
		require.Contains(t, string(reply), "`apply_patch`")
	case <-time.After(time.Second):
		t.Fatal("native write was not answered")
	}
}
