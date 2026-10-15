package cursorproto

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestNativeFilesRefuseAndRedirectWithoutTouchingGateway(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.txt")
	require.NoError(t, os.WriteFile(path, []byte("preserve me"), 0600))
	for _, operation := range []protoreflect.Name{"write_args", "delete_args", "ls_args"} {
		t.Run(string(operation), func(t *testing.T) {
			raw := nativeFileRequest(t, operation, path, false)
			reply, handled, err := ReplyNativeFileExec(raw, []ToolDefinition{{Name: "functions__exec_command"}})
			require.NoError(t, err)
			require.True(t, handled)
			client, err := newMessage("AgentClientMessage")
			require.NoError(t, err)
			require.NoError(t, proto.Unmarshal(reply, client))
			exec := client.Get(field(client, "exec_client_message")).Message()
			require.EqualValues(t, 42, exec.Get(field(exec, "id")).Uint())
			require.Equal(t, "native-file-42", exec.Get(field(exec, "exec_id")).String())
			result := exec.Get(exec.WhichOneof(exec.Descriptor().Oneofs().ByName("message"))).Message()
			failure := result.Get(field(result, "error")).Message()
			require.Equal(t, path, failure.Get(field(failure, "path")).String())
			require.Contains(t, failure.Get(field(failure, "error")).String(), "`functions__exec_command`")
			content, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, "preserve me", string(content))
		})
	}
}

func TestNativeFileRefusalPreservesImageWritePath(t *testing.T) {
	_, handled, err := ReplyNativeFileExec(nativeFileRequest(t, "write_args", "image.png", true), nil)
	require.NoError(t, err)
	require.False(t, handled)
}

func nativeFileRequest(t *testing.T, operation protoreflect.Name, path string, binary bool) []byte {
	t.Helper()
	server, err := newMessage("AgentServerMessage")
	require.NoError(t, err)
	exec, err := nestedMessage(server, "exec_server_message")
	require.NoError(t, err)
	require.NoError(t, setUint32(exec, "id", 42))
	require.NoError(t, setString(exec, "exec_id", "native-file-42"))
	args, err := nestedMessage(exec, operation)
	require.NoError(t, err)
	require.NoError(t, setString(args, "path", path))
	if operation == "write_args" {
		if binary {
			require.NoError(t, setBytes(args, "file_bytes", []byte{1, 2, 3}))
		} else {
			require.NoError(t, setString(args, "file_text", "must not be written"))
		}
	}
	require.NoError(t, setMessage(exec, operation, args))
	require.NoError(t, setMessage(server, "exec_server_message", exec))
	raw, err := proto.Marshal(server)
	require.NoError(t, err)
	return raw
}

func TestToolPolicyUsesOnlyExposedFileTools(t *testing.T) {
	policy := toolPolicy([]ToolDefinition{{Name: "exec_command"}, {Name: "write_stdin"}})
	require.Contains(t, policy, "MCP server opencodex-responses")
	require.Contains(t, policy, "Do not use Cursor-native editing")
	require.Contains(t, policy, "Client tools available for these operations: exec_command, write_stdin.")
	require.NotContains(t, policy, "apply_patch,")
	require.Contains(t, toolPolicy(nil), "Native image generation remains available")
	require.NotContains(t, nativeExecPolicy("delete", []ToolDefinition{{Name: "write_file"}}), "`write_file`")
	for _, operation := range []string{"read", "grep", "shell", "write", "delete", "ls"} {
		require.Contains(t, nativeExecPolicy(operation, []ToolDefinition{{Name: "exec_command"}}), "`exec_command`")
	}
}
