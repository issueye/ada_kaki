package mcp

import "testing"

// The tool catalog is a public contract: an external AI client sees these
// names, order and schemas. A regression here breaks every existing client,
// so the list is pinned rather than read from the catalog itself.
func TestToolCatalogContract(t *testing.T) {
	want := []string{
		"list_shells", "list_sessions", "create_session", "ssh_connect",
		"list_ssh_profiles", "write_session", "read_session", "screen_session",
		"resize_session", "close_session", "reconnect_session", "run_command",
	}
	defs := toolDefs()
	if len(defs) != len(want) {
		t.Fatalf("tool count = %d, want %d (adding or removing a tool is a contract change)",
			len(defs), len(want))
	}
	for i, name := range want {
		if defs[i].Name != name {
			t.Fatalf("tool %d = %q, want %q (order is the catalog's display order)",
				i, defs[i].Name, name)
		}
	}
	for _, d := range defs {
		if d.Description == "" {
			t.Fatalf("tool %q has no description", d.Name)
		}
		if d.InputSchema.Type != "object" {
			t.Fatalf("tool %q schema type = %q, want object", d.Name, d.InputSchema.Type)
		}
	}
}

// run_command is the AI workhorse; its argument names, required set and
// defaults-in-description are the part callers depend on most.
func TestRunCommandSchemaContract(t *testing.T) {
	var def toolDef
	for _, d := range toolDefs() {
		if d.Name == "run_command" {
			def = d
		}
	}
	if def.Name == "" {
		t.Fatal("run_command is missing from the catalog")
	}
	for _, key := range []string{"id", "shell", "cwd", "command", "timeout_ms", "quiet_ms"} {
		if _, ok := def.InputSchema.Properties[key]; !ok {
			t.Fatalf("run_command lost the %q argument", key)
		}
	}
	if len(def.InputSchema.Required) != 1 || def.InputSchema.Required[0] != "command" {
		t.Fatalf("run_command required = %v, want [command]", def.InputSchema.Required)
	}
	if def.InputSchema.Properties["command"].Type != "string" {
		t.Fatal("run_command's command argument must stay a string")
	}
}
