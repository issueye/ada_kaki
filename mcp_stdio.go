package main

import (
	"fmt"
	"log"
	"os"

	"adakaki/internal/audit"
	"adakaki/internal/engine"
	"adakaki/internal/guard"
	"adakaki/internal/mcp"
	"adakaki/internal/store"
)

// runMCPStdio runs the MCP server over stdio for AI agents: a headless
// session engine with no GUI. stdout carries only JSON-RPC; logs go to
// stderr.
func runMCPStdio(args []string) {
	for _, a := range args {
		fmt.Fprintf(os.Stderr, "adakaki mcp: unknown flag %q ignored\n", a)
	}
	cfg, err := store.Load()
	if err != nil {
		cfg = &store.Config{Settings: store.Settings{MCPHTTP: true}}
		fmt.Fprintln(os.Stderr, "adakaki mcp: config:", err)
	}
	mgr := engine.NewManager()
	defer mgr.CloseAll()

	backend := mcp.NewEngineBackend(mgr, cfg)
	server := mcp.NewServer("adakaki", "0.1.0", backend)
	server.SetTransport("stdio")
	// Headless rails: no human to ask, so Confirm follows the setting
	// (deny by default); every call still lands in the audit trail.
	guardPolicy := guard.FromSettings(cfg.Settings.GuardDisabled, cfg.Settings.GuardHeadless,
		cfg.Settings.GuardDeny, cfg.Settings.GuardConfirm)
	auditLog, err := audit.Open(mustStdioAuditPath())
	if err != nil {
		fmt.Fprintln(os.Stderr, "adakaki mcp: audit:", err)
	}
	server.SetHooks(mcp.Hooks{
		Guard: func(c mcp.Call) (string, string) {
			v, rule := guardPolicy.Check(guard.Request{
				Tool: c.Tool, Client: c.Client, Session: c.Session, Command: c.Command,
			})
			if rule != nil {
				return string(v), rule.Reason + "（" + rule.Source + "）"
			}
			return string(v), ""
		},
		Audit: func(e audit.Entry) {
			if auditLog != nil {
				_ = auditLog.Record(e)
			}
		},
	})
	log.SetOutput(os.Stderr)
	if err := server.ServeStdio(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "adakaki mcp:", err)
		os.Exit(1)
	}
}

func mustStdioAuditPath() string {
	p, err := store.AuditLogPath()
	if err != nil {
		return "audit.log"
	}
	return p
}
