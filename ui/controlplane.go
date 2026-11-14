package ui

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"adakaki/internal/audit"
	"adakaki/internal/guard"
	"adakaki/internal/mcp"
	"adakaki/internal/store"
)

// startControlPlane serves the MCP endpoint over HTTP on the loopback,
// for AI clients that speak HTTP, plus a health check. It runs until the
// app quits.
func (a *app) startControlPlane() {
	if !a.store.Settings.MCPHTTP {
		return
	}
	port := a.store.Settings.MCPHTTPPort
	if port == 0 {
		port = 8731
	}
	token := a.loadToken()
	// The backend lives on the app: MCP and the built-in agent share it
	// (host key policy and the agent mask are installed in newApp).
	backend := a.toolBackend
	if backend == nil {
		log.Printf("mcp http: no tool backend")
		return
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		log.Printf("mcp http: %v", err)
		a.pendingToast = fmt.Sprintf("MCP HTTP 端口 %d 被占用，HTTP 端点未启动", port)
		return
	}

	server := mcp.NewServer("adakaki", "0.1.0", backend)
	server.SetTransport("http")
	// The rails: every call lands in the audit trail, dangerous commands
	// ask a human first.
	server.SetHooks(mcp.Hooks{
		Guard: func(c mcp.Call) (string, string) {
			v, rule := a.guard.Check(guard.Request{
				Tool: c.Tool, Client: c.Client, Session: c.Session, Command: c.Command,
			})
			if rule != nil {
				return string(v), rule.Reason + "（" + rule.Source + "）"
			}
			return string(v), ""
		},
		Audit: func(e audit.Entry) { _ = a.auditLog.Record(e) },
	})
	// Every tool call masks the sessions it touches: the human watches,
	// the input locks, and the mask lifts when the call ends. The tracker
	// itself is installed once on the shared backend.
	mux := a.mcpMux(server, token)

	a.mcpUp = true
	go func() {
		log.Printf("mcp http: serving on http://%s/mcp", ln.Addr())
		if err := http.Serve(ln, mux); err != nil {
			log.Printf("mcp http: %v", err)
		}
	}()
}

// mcpMux builds the control plane's routes: /mcp for MCP over HTTP,
// /healthz for a quick check.
func (a *app) mcpMux(server *mcp.Server, token string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /mcp", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, `{"error":"unauthorized: the token is in `+tokenPath()+`"}`, http.StatusUnauthorized)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		resp := server.Handle(body)
		if resp == nil { // a notification: accepted, nothing to say
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(resp)
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"ok":true,"sessions":%d}`, a.mgr.Count())
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "POST /mcp is the MCP endpoint; GET /healthz", http.StatusNotFound)
	})
	return mux
}

// tokenPath is where the HTTP MCP token lives.
func tokenPath() string {
	dir, err := store.Dir()
	if err != nil {
		return "the config directory"
	}
	return filepath.Join(dir, "token")
}

// loadToken reads the Bearer token, generating one on first use.
func (a *app) loadToken() string {
	p := tokenPath()
	if data, err := os.ReadFile(p); err == nil && len(data) >= 32 {
		return string(data)
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		log.Printf("token: %v", err)
		return "adakaki-local"
	}
	token := hex.EncodeToString(buf)
	if dir, err := store.Dir(); err == nil {
		_ = os.MkdirAll(dir, 0o700)
	}
	if err := os.WriteFile(p, []byte(token), 0o600); err != nil {
		log.Printf("token: %v", err)
	}
	return token
}
