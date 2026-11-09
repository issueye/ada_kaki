package store

import (
	"path/filepath"
	"testing"
)

func TestConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AppData", dir) // os.UserConfigDir reads %AppData% on Windows

	cfg := &Config{Settings: Settings{MCPHTTP: true, MCPHTTPPort: 8731}}
	if err := cfg.PutProfile(Profile{
		Name: "prod", Host: "10.0.0.1", Port: 2222, User: "ops",
		AuthKind: "password",
	}); err != nil {
		t.Fatalf("PutProfile: %v", err)
	}
	if err := cfg.PutProfile(Profile{Name: "box", Host: "box.local", User: "u", AuthKind: "key", KeyPath: `C:\keys\id_ed25519`}); err != nil {
		t.Fatal(err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Profiles) != 2 {
		t.Fatalf("%d profiles; want 2", len(got.Profiles))
	}
	p, ok := got.Profile("prod")
	if !ok || p.Host != "10.0.0.1" || p.Port != 2222 {
		t.Errorf("prod profile = %+v", p)
	}
	if !got.Settings.MCPHTTP || got.Settings.MCPHTTPPort != 8731 {
		t.Errorf("settings = %+v", got.Settings)
	}
	if err := got.DeleteProfile("prod"); err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Profile("prod"); ok {
		t.Error("profile was not deleted")
	}
}

func TestSecretDPAPIRoundTrip(t *testing.T) {
	// DPAPI is per-user and per-machine; the round trip is all we check.
	plain := "s3cret-密码"
	cipher, err := Protect(plain)
	if err != nil {
		t.Fatalf("Protect: %v", err)
	}
	if cipher == "" || cipher == plain {
		t.Fatalf("cipher = %q", cipher)
	}
	got, err := Unprotect(cipher)
	if err != nil {
		t.Fatalf("Unprotect: %v", err)
	}
	if got != plain {
		t.Errorf("Unprotect = %q; want %q", got, plain)
	}
	if s, err := Protect(""); err != nil || s != "" {
		t.Errorf("Protect empty = %q, %v", s, err)
	}
}

func TestFilePath(t *testing.T) {
	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p) != "config.json" {
		t.Errorf("path = %s", p)
	}
}

func TestRestoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AppData", dir)

	cfg := &Config{}
	cfg.Restore = []RestoreSpec{
		{Kind: "local", Shell: "cmd", Cwd: `C:\work`},
		{Kind: "ssh", Profile: "prod"},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Restore) != 2 ||
		got.Restore[0].Kind != "local" || got.Restore[0].Cwd != `C:\work` ||
		got.Restore[1].Kind != "ssh" || got.Restore[1].Profile != "prod" {
		t.Errorf("restore = %+v", got.Restore)
	}
}
