package cursorproto

import "fmt"

func nativeExecPolicy(operation string, tools []ToolDefinition) string {
	policy := fmt.Sprintf("Cursor native %s cannot run on the gateway host. Do not retry a native operation; ", operation)
	var candidates []string
	switch operation {
	case "shell":
		candidates = []string{"exec_command", "shell_command", "bash", "shell", "exec"}
	case "read":
		candidates = []string{"read", "read_file", "exec_command", "shell_command"}
	case "grep":
		candidates = []string{"grep", "grep_search", "exec_command", "shell_command"}
	case "write":
		candidates = []string{"apply_patch", "write_file", "exec_command", "shell_command"}
	case "delete":
		candidates = []string{"apply_patch", "delete_file", "exec_command", "shell_command"}
	case "ls":
		candidates = []string{"list_directory", "exec_command", "shell_command"}
	}
	for _, candidate := range candidates {
		if name := exposedTool(tools, candidate); name != "" {
			return policy + fmt.Sprintf("use the exposed client tool `%s` through MCP server opencodex-responses instead, following its schema, or report that the operation is unavailable and stop", name)
		}
	}
	return policy + "no matching client tool is exposed. Report that the operation is unavailable and stop"
}
