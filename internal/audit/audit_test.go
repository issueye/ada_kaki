package audit

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestRecordAndTail(t *testing.T) {
	path := t.TempDir() + "/audit.log"
	l, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close() // TempDir's cleanup needs the file closed
	for i := 0; i < 5; i++ {
		err := l.Record(Entry{
			Time: time.Now(), Transport: "http", Client: "claude",
			Tool: "run_command", Session: "local_0001", Command: "echo hi",
			OK: true, DurationMS: int64(i),
		})
		if err != nil {
			t.Fatalf("Record: %v", err)
		}
	}
	l.Record(Entry{Time: time.Now(), Tool: "close_session", OK: false, Err: "no session x"})

	got, err := l.Tail(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 6 {
		t.Fatalf("%d entries; want 6", len(got))
	}
	if got[0].Tool != "run_command" || got[5].Tool != "close_session" {
		t.Errorf("tail order wrong: first=%s last=%s", got[0].Tool, got[5].Tool)
	}
	if !strings.Contains(got[5].Err, "no session") {
		t.Errorf("error not recorded: %+v", got[5])
	}
}

func TestRotation(t *testing.T) {
	path := t.TempDir() + "/audit.log"
	l, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	l.max = 1000 // small files, so entries roll over
	l.keep = 2
	for i := 0; i < 40; i++ {
		if err := l.Record(Entry{Time: time.Now(), Tool: "t", Command: strings.Repeat("x", 40)}); err != nil {
			t.Fatalf("Record %d: %v", i, err)
		}
	}
	// With 3 files of ~3 entries each, the tail spans them all.
	got, err := l.Tail(5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("tail = %d entries; want 5", len(got))
	}
	// The rotations exist and hold what rotated off.
	for _, p := range []string{path + ".1", path + ".2"} {
		data, err := os.ReadFile(p)
		if err != nil || len(data) == 0 {
			t.Errorf("rotation %s missing: %v", p, err)
		}
	}
}
