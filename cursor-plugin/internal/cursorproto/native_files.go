package cursorproto

import (
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Return a correlated failure so the agent can retry with a client tool. Never
// perform native text edits, deletes, or directory reads on the gateway host.
// Binary writes remain with the separately scoped image-generation executor.
func ReplyNativeFileExec(raw []byte, tools []ToolDefinition) ([]byte, bool, error) {
	server, err := newMessage("AgentServerMessage")
	if err != nil {
		return nil, false, err
	}
	if err := proto.Unmarshal(raw, server); err != nil {
		return nil, false, fmt.Errorf("decode Cursor native file exec: %w", err)
	}
	active := server.WhichOneof(server.Descriptor().Oneofs().ByName("message"))
	if active == nil || active.Name() != "exec_server_message" {
		return nil, false, nil
	}
	exec := server.Get(active).Message()
	operation := exec.WhichOneof(exec.Descriptor().Oneofs().ByName("message"))
	if operation == nil {
		return nil, false, nil
	}
	args := exec.Get(operation).Message()
	var resultName protoreflect.Name
	var policyOperation string
	switch operation.Name() {
	case "write_args":
		if args.Get(field(args, "file_text")).String() == "" && len(args.Get(field(args, "file_bytes")).Bytes()) > 0 {
			return nil, false, nil
		}
		resultName, policyOperation = "write_result", "write"
	case "delete_args":
		resultName, policyOperation = "delete_result", "delete"
	case "ls_args":
		resultName, policyOperation = "ls_result", "ls"
	default:
		return nil, false, nil
	}
	result, err := newExecResult(exec, resultName)
	if err != nil {
		return nil, true, err
	}
	failure, err := nestedMessage(result, "error")
	if err != nil {
		return nil, true, err
	}
	if err := setString(failure, "path", args.Get(field(args, "path")).String()); err != nil {
		return nil, true, err
	}
	if err := setString(failure, "error", nativeExecPolicy(policyOperation, tools)); err != nil {
		return nil, true, err
	}
	if err := setMessage(result, "error", failure); err != nil {
		return nil, true, err
	}
	reply, err := encodeNativeExecReply(exec, resultName, result)
	return reply, true, err
}
