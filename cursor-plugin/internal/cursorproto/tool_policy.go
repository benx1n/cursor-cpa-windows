package cursorproto

import "strings"

// Client tools are exposed as MCP tools, while Cursor's built-in editor belongs
// to a different machine and protocol. Repeat this on suffix turns as well.
const clientToolPolicy = `<gateway_tool_policy>
This request runs through a gateway. The user's files and execution environment belong to the downstream client, not the gateway or Cursor's native workspace.
Use the client-provided tools from MCP server opencodex-responses for filesystem access, file creation/editing/deletion, and shell commands. When you need tool schemas, use GetDynamicTools for this server, then CallDynamicTool with the exact exposed tool name and schema.
Do not use Cursor-native editing, apply-patch, write, delete, read, grep, directory-listing, shell, or background-terminal tools for those operations. A native editor success does not create a file on the client. Do not simulate a tool call in text or claim a file was saved without a successful client tool result.
Choose only tools actually present in the client catalog. Respect a user's request not to use tools. If the catalog has no suitable tool, explain the limitation instead of falling back to a native editor.
</gateway_tool_policy>`

const noClientToolPolicy = `<gateway_tool_policy>
No client filesystem, shell, or web-search tools are exposed for this request. Answer ordinary conversational requests directly in text. Do not invoke Cursor-native editing, file reads/writes, shell commands, or web search as substitutes for missing client tools. Explain when a requested external operation is unavailable. Native image generation remains available when the user explicitly asks for an image.
</gateway_tool_policy>`

func toolPolicy(tools []ToolDefinition) string {
	if len(tools) == 0 {
		return noClientToolPolicy
	}
	policy := clientToolPolicy
	var preferred []string
	for _, name := range []string{"apply_patch", "exec_command", "shell_command", "write_stdin", "read_file", "write_file"} {
		if exposed := exposedTool(tools, name); exposed != "" {
			preferred = append(preferred, exposed)
		}
	}
	if len(preferred) > 0 {
		policy += "\nClient tools available for these operations: " + strings.Join(preferred, ", ") + "."
	}
	return policy
}

func exposedTool(tools []ToolDefinition, name string) string {
	for _, tool := range tools {
		if tool.Name == name {
			return tool.Name
		}
	}
	for _, tool := range tools {
		if strings.HasSuffix(tool.Name, "__"+name) || strings.HasSuffix(tool.Name, "."+name) {
			return tool.Name
		}
	}
	return ""
}
