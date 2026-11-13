package mcp

// toolDefs is the catalog tools/list answers with, in display order.
func toolDefs() []toolDef {
	return []toolDef{
		{
			Name:        "list_shells",
			Description: "List the local shells available for create_session (cmd, PowerShell, Git Bash, ...).",
			InputSchema: schema{Type: "object"},
		},
		{
			Name:        "list_sessions",
			Description: "List all terminal sessions with their id, kind, title, state and exit code.",
			InputSchema: schema{Type: "object"},
		},
		{
			Name:        "create_session",
			Description: "Create a local shell session. Use list_shells to see what is available; use shell \"custom\" with command to run any command line.",
			InputSchema: schema{
				Type: "object",
				Properties: map[string]schemaProp{
					"shell":   {Type: "string", Description: "A shell id from list_shells, or \"custom\""},
					"cwd":     {Type: "string", Description: "Working directory (default: the user's home)"},
					"command": {Type: "string", Description: "For shell \"custom\": the command line to run"},
				},
				Required: []string{"shell"},
			},
		},
		{
			Name:        "ssh_connect",
			Description: "Open an interactive SSH shell session. Give a saved profile, or the connection details. First meetings with a host follow the known_hosts policy (the key is stored after confirmation); a host whose key CHANGED is refused.",
			InputSchema: schema{
				Type: "object",
				Properties: map[string]schemaProp{
					"profile":    {Type: "string", Description: "A saved profile name from list_ssh_profiles"},
					"host":       {Type: "string", Description: "Hostname or IP"},
					"port":       {Type: "integer", Description: "Port (default 22)"},
					"user":       {Type: "string", Description: "User name"},
					"auth_kind":  {Type: "string", Enum: []string{"password", "key", "agent"}, Description: "How to authenticate (default password; agent = the OpenSSH agent)"},
					"password":   {Type: "string", Description: "Password for auth_kind password"},
					"key_path":   {Type: "string", Description: "Private key path for auth_kind key"},
					"passphrase": {Type: "string", Description: "Key passphrase"},
				},
			},
		},
		{
			Name:        "list_ssh_profiles",
			Description: "List saved SSH profiles (no credentials).",
			InputSchema: schema{Type: "object"},
		},
		{
			Name:        "write_session",
			Description: "Write to a session as if typed. Include \\r to press Enter. Prefer run_command for one-shot commands.",
			InputSchema: schema{
				Type: "object",
				Properties: map[string]schemaProp{
					"id":   {Type: "string", Description: "The session id"},
					"data": {Type: "string", Description: "Bytes to type, escapes included"},
				},
				Required: []string{"id", "data"},
			},
		},
		{
			Name:        "read_session",
			Description: "Read a session's terminal text incrementally: what appeared since the last read. On a screen reset (a full-screen program repainting) the whole text comes back.",
			InputSchema: schema{
				Type: "object",
				Properties: map[string]schemaProp{
					"id":        {Type: "string", Description: "The session id"},
					"wait_ms":   {Type: "integer", Description: "Block up to this long for new output (default 0)"},
					"max_chars": {Type: "integer", Description: "Return at most this many characters (default 20000)"},
				},
				Required: []string{"id"},
			},
		},
		{
			Name:        "screen_session",
			Description: "Read what a session's screen shows right now (the last rows of the terminal, without scrollback) - the way to see full-screen programs.",
			InputSchema: schema{
				Type:       "object",
				Properties: map[string]schemaProp{"id": {Type: "string", Description: "The session id"}},
				Required:   []string{"id"},
			},
		},
		{
			Name:        "resize_session",
			Description: "Resize a session's terminal in character cells.",
			InputSchema: schema{
				Type: "object",
				Properties: map[string]schemaProp{
					"id":   {Type: "string", Description: "The session id"},
					"cols": {Type: "integer", Description: "Columns"},
					"rows": {Type: "integer", Description: "Rows"},
				},
				Required: []string{"id", "cols", "rows"},
			},
		},
		{
			Name:        "close_session",
			Description: "Close a session (its process or SSH connection ends).",
			InputSchema: schema{
				Type:       "object",
				Properties: map[string]schemaProp{"id": {Type: "string", Description: "The session id"}},
				Required:   []string{"id"},
			},
		},
		{
			Name:        "reconnect_session",
			Description: "Restart an exited session: same id, same pane. An SSH session dials again; a local one starts its shell or command again.",
			InputSchema: schema{
				Type:       "object",
				Properties: map[string]schemaProp{"id": {Type: "string", Description: "The exited session id"}},
				Required:   []string{"id"},
			},
		},
		{
			Name:        "run_command",
			Description: "Write a command to a session (creating one if id is empty) and collect its output until the terminal goes quiet or the timeout hits. The AI workhorse.",
			InputSchema: schema{
				Type: "object",
				Properties: map[string]schemaProp{
					"id":         {Type: "string", Description: "An existing session; empty creates one from shell"},
					"shell":      {Type: "string", Description: "When id is empty: the shell to create (default the first available)"},
					"cwd":        {Type: "string", Description: "Working directory of the session to create"},
					"command":    {Type: "string", Description: "The command line to run"},
					"timeout_ms": {Type: "integer", Description: "Hard limit (default 10000)"},
					"quiet_ms":   {Type: "integer", Description: "Return this long after the last output (default 800); a command that prints nothing waits 3x before giving up"},
				},
				Required: []string{"command"},
			},
		},
	}
}
