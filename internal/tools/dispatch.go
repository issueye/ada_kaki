// Call dispatches one tool by name with its JSON arguments, the entry the
// in-app agent uses. It mirrors the MCP transport's argument mapping exactly
// (the transports share the backend), so a tool behaves the same whichever
// entry point reaches it.
package tools

import (
	"encoding/json"
	"fmt"
)

// Call runs one tool. An unknown name is an error rather than a silent
// success, so the agent can report it back to the model.
func (b *Backend) Call(name, rawArgs string) (string, error) {
	args := map[string]any{}
	if rawArgs != "" {
		if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
			return "", fmt.Errorf("%s: invalid arguments: %w", name, err)
		}
	}

	switch name {
	case "list_shells":
		return b.ListShells()
	case "list_sessions":
		return b.ListSessions()
	case "create_session":
		return b.CreateSession(argString(args, "shell"), argString(args, "cwd"),
			argString(args, "command"))
	case "ssh_connect":
		if profile := argString(args, "profile"); profile != "" {
			return b.SSHConnectProfile(profile)
		}
		return b.SSHConnect(argString(args, "host"), argInt(args, "port"),
			argString(args, "user"), argString(args, "auth_kind"),
			argString(args, "password"), argString(args, "key_path"),
			argString(args, "passphrase"))
	case "list_ssh_profiles":
		return b.ListSSHProfiles()
	case "write_session":
		return b.WriteSession(argString(args, "id"), argString(args, "data"))
	case "read_session":
		return b.ReadSession(argString(args, "id"), argInt(args, "wait_ms"),
			argInt(args, "max_chars"))
	case "screen_session":
		return b.ScreenSession(argString(args, "id"))
	case "resize_session":
		return b.ResizeSession(argString(args, "id"), argInt(args, "cols"),
			argInt(args, "rows"))
	case "close_session":
		return b.CloseSession(argString(args, "id"))
	case "reconnect_session":
		return b.ReconnectSession(argString(args, "id"))
	case "run_command":
		return b.RunCommand(argString(args, "id"), argString(args, "shell"),
			argString(args, "cwd"), argString(args, "command"),
			argInt(args, "timeout_ms"), argInt(args, "quiet_ms"))
	default:
		return "", fmt.Errorf("unknown tool %q", name)
	}
}

// CommandOf returns the command a tool call would run, for the guard and
// the audit trail; "" for tools that do not run one.
func CommandOf(name, rawArgs string) string {
	args := map[string]any{}
	if rawArgs != "" {
		_ = json.Unmarshal([]byte(rawArgs), &args)
	}
	switch name {
	case "run_command":
		return argString(args, "command")
	case "write_session":
		// Typing data runs whatever the shell makes of it; the guard judges
		// the typed line.
		return argString(args, "data")
	default:
		return ""
	}
}

func argString(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return v
}

func argInt(args map[string]any, key string) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	default:
		return 0
	}
}
