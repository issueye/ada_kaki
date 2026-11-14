package tools_test

import (
	"adakaki/internal/mcp"
	"adakaki/internal/tools"
)

// The tool layer must satisfy the MCP backend contract: the transport and the
// in-app agent call the same methods, so a signature drift is a compile error
// rather than a runtime surprise.
var _ mcp.Backend = (*tools.Backend)(nil)
