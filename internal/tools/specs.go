// Tool specifications as a model sees them: the same twelve tools the MCP
// transport publishes, in the same order, so the in-app agent and external
// clients drive one identical surface (docs/AI-功能规划与设计.md §F3).
package tools

import "encoding/json"

// Spec is one tool's declaration.
type Spec struct {
	Name        string
	Description string
	Schema      json.RawMessage // a JSON Schema object for the arguments
}

// Specs returns the tool catalog in display order. The MCP catalog test in
// internal/mcp pins this list against toolDefs(), so a change here without a
// matching transport change fails the build.
func Specs() []Spec { return catalog }

var catalog = []Spec{
	{
		Name:        "list_shells",
		Description: "List the local shells available for create_session (cmd, PowerShell, Git Bash, ...).",
		Schema:      json.RawMessage(`{"type":"object"}`),
	},
	{
		Name:        "list_sessions",
		Description: "List all terminal sessions with their id, kind, title, state and exit code.",
		Schema:      json.RawMessage(`{"type":"object"}`),
	},
	{
		Name:        "create_session",
		Description: "Create a local shell session. Use list_shells to see what is available; use shell \"custom\" with command to run any command line.",
		Schema: json.RawMessage(`{"type":"object","properties":{
			"shell":{"type":"string","description":"A shell id from list_shells, or \"custom\""},
			"cwd":{"type":"string","description":"Working directory (default: the user's home)"},
			"command":{"type":"string","description":"For shell \"custom\": the command line to run"}},
			"required":["shell"]}`),
	},
	{
		Name:        "ssh_connect",
		Description: "Open an interactive SSH shell session. Give a saved profile, or the connection details. First meetings follow the known_hosts policy; a host whose key CHANGED is refused.",
		Schema: json.RawMessage(`{"type":"object","properties":{
			"profile":{"type":"string","description":"A saved profile name from list_ssh_profiles"},
			"host":{"type":"string","description":"Hostname or IP"},
			"port":{"type":"integer","description":"Port (default 22)"},
			"user":{"type":"string","description":"User name"},
			"auth_kind":{"type":"string","enum":["password","key","agent"],"description":"How to authenticate (default password; agent = the OpenSSH agent)"},
			"password":{"type":"string","description":"Password for auth_kind password"},
			"key_path":{"type":"string","description":"Private key path for auth_kind key"},
			"passphrase":{"type":"string","description":"Key passphrase"}}}`),
	},
	{
		Name:        "list_ssh_profiles",
		Description: "List saved SSH profiles (no credentials).",
		Schema:      json.RawMessage(`{"type":"object"}`),
	},
	{
		Name:        "write_session",
		Description: "Write to a session as if typed. Include \\r to press Enter. Prefer run_command for one-shot commands.",
		Schema: json.RawMessage(`{"type":"object","properties":{
			"id":{"type":"string","description":"The session id"},
			"data":{"type":"string","description":"Bytes to type, escapes included"}},
			"required":["id","data"]}`),
	},
	{
		Name:        "read_session",
		Description: "Read a session's terminal text incrementally: what appeared since the last read. On a screen reset the whole text comes back.",
		Schema: json.RawMessage(`{"type":"object","properties":{
			"id":{"type":"string","description":"The session id"},
			"wait_ms":{"type":"integer","description":"Block up to this long for new output (default 0)"},
			"max_chars":{"type":"integer","description":"Return at most this many characters (default 20000)"}},
			"required":["id"]}`),
	},
	{
		Name:        "screen_session",
		Description: "Read what a session's screen shows right now (the last rows, without scrollback) - the way to see full-screen programs.",
		Schema: json.RawMessage(`{"type":"object","properties":{
			"id":{"type":"string","description":"The session id"}},
			"required":["id"]}`),
	},
	{
		Name:        "resize_session",
		Description: "Resize a session's terminal in character cells.",
		Schema: json.RawMessage(`{"type":"object","properties":{
			"id":{"type":"string","description":"The session id"},
			"cols":{"type":"integer","description":"Columns"},
			"rows":{"type":"integer","description":"Rows"}},
			"required":["id","cols","rows"]}`),
	},
	{
		Name:        "close_session",
		Description: "Close a session (its process or SSH connection ends).",
		Schema: json.RawMessage(`{"type":"object","properties":{
			"id":{"type":"string","description":"The session id"}},
			"required":["id"]}`),
	},
	{
		Name:        "reconnect_session",
		Description: "Restart an exited session: same id, same pane. An SSH session dials again; a local one starts its shell or command again.",
		Schema: json.RawMessage(`{"type":"object","properties":{
			"id":{"type":"string","description":"The exited session id"}},
			"required":["id"]}`),
	},
	{
		Name:        "run_command",
		Description: "Write a command to a session (creating one if id is empty) and collect its output until the terminal goes quiet or the timeout hits. The AI workhorse.",
		Schema: json.RawMessage(`{"type":"object","properties":{
			"id":{"type":"string","description":"An existing session; empty creates one from shell"},
			"shell":{"type":"string","description":"When id is empty: the shell to create (default the first available)"},
			"cwd":{"type":"string","description":"Working directory of the session to create"},
			"command":{"type":"string","description":"The command line to run"},
			"timeout_ms":{"type":"integer","description":"Hard limit (default 10000)"},
			"quiet_ms":{"type":"integer","description":"Return this long after the last output (default 800); a command that prints nothing waits 3x before giving up"}},
			"required":["command"]}`),
	},
}
