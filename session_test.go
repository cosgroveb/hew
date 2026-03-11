package hew

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNormalizeProjectPath(t *testing.T) {
	// Deterministic: same path always produces same hash
	got1, err := NormalizeProjectPath("/home/user/projects/myapp")
	if err != nil {
		t.Fatalf("NormalizeProjectPath: %v", err)
	}
	got2, err := NormalizeProjectPath("/home/user/projects/myapp")
	if err != nil {
		t.Fatalf("NormalizeProjectPath: %v", err)
	}
	if got1 != got2 {
		t.Errorf("same path produced different hashes: %q vs %q", got1, got2)
	}
	if len(got1) != 16 {
		t.Errorf("expected 16-char hash, got %d: %q", len(got1), got1)
	}

	// Different paths produce different hashes
	got3, err := NormalizeProjectPath("/tmp/foo")
	if err != nil {
		t.Fatalf("NormalizeProjectPath: %v", err)
	}
	if got1 == got3 {
		t.Errorf("different paths produced same hash: %q", got1)
	}

	// Relative path fails
	_, err = NormalizeProjectPath("relative/path")
	if err == nil {
		t.Error("NormalizeProjectPath(relative): expected error, got nil")
	}

	// Empty path fails
	_, err = NormalizeProjectPath("")
	if err == nil {
		t.Error("NormalizeProjectPath(empty): expected error, got nil")
	}
}

func TestSessionDir(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/tmp/test-state")

	dir, err := SessionDir("/home/user/projects/myapp")
	if err != nil {
		t.Fatalf("SessionDir: %v", err)
	}

	want := "/tmp/test-state/hew/projects/"
	if !contains(dir, want) {
		t.Errorf("SessionDir: got %q, want to contain %q", dir, want)
	}
}

func TestSessionDirDefaultXDG(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")

	dir, err := SessionDir("/home/user/projects/myapp")
	if err != nil {
		t.Fatalf("SessionDir: %v", err)
	}

	if !contains(dir, ".local/state/hew/projects/") {
		t.Errorf("SessionDir default: got %q, want to contain .local/state/hew/projects/", dir)
	}
}

func TestLoadLatestSessionEmpty(t *testing.T) {
	tmpdir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmpdir)

	msgs, err := LoadLatestSession("/tmp/test-project-nonexistent")
	if err != nil {
		t.Fatalf("LoadLatestSession (empty): %v", err)
	}
	if msgs != nil {
		t.Errorf("LoadLatestSession (empty): got %v, want nil", msgs)
	}
}

func TestSaveAndLoadSession(t *testing.T) {
	tmpdir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmpdir)

	pwd := "/tmp/test-project-save"
	msgs := []Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	}

	if err := SaveSession(pwd, msgs); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	// Verify file was created
	dir, _ := SessionDir(pwd)
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("SaveSession: expected 1 session file, got %d (err=%v)", len(files), err)
	}

	// Load it back
	loaded, err := LoadLatestSession(pwd)
	if err != nil {
		t.Fatalf("LoadLatestSession: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("LoadLatestSession: expected 2 messages, got %d", len(loaded))
	}
	if loaded[0].Role != "user" || loaded[0].Content != "hello" {
		t.Errorf("message 0: got %+v", loaded[0])
	}
	if loaded[1].Role != "assistant" || loaded[1].Content != "hi there" {
		t.Errorf("message 1: got %+v", loaded[1])
	}
}

func TestSaveMultipleAndLoadLatest(t *testing.T) {
	tmpdir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmpdir)

	pwd := "/tmp/test-project-multi"

	// Save first session
	if err := SaveSession(pwd, []Message{{Role: "user", Content: "first"}}); err != nil {
		t.Fatalf("SaveSession 1: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	// Save second session
	if err := SaveSession(pwd, []Message{{Role: "user", Content: "second"}}); err != nil {
		t.Fatalf("SaveSession 2: %v", err)
	}

	// Load latest should return "second"
	loaded, err := LoadLatestSession(pwd)
	if err != nil {
		t.Fatalf("LoadLatestSession: %v", err)
	}
	if len(loaded) != 1 || loaded[0].Content != "second" {
		t.Errorf("LoadLatestSession: expected 'second', got %+v", loaded)
	}
}

func TestListSessions(t *testing.T) {
	tmpdir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmpdir)

	pwd := "/tmp/test-project-list"

	// Save multiple sessions
	for i := 0; i < 3; i++ {
		if err := SaveSession(pwd, []Message{
			{Role: "user", Content: "msg"},
		}); err != nil {
			t.Fatalf("SaveSession %d: %v", i, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	sessions, err := ListSessions(pwd)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 3 {
		t.Errorf("ListSessions: got %d sessions, want 3", len(sessions))
	}

	// Should be sorted newest first
	if len(sessions) >= 2 && sessions[0].Created.Before(sessions[1].Created) {
		t.Errorf("ListSessions: not sorted newest first")
	}
}

func TestListSessionsEmpty(t *testing.T) {
	tmpdir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmpdir)

	sessions, err := ListSessions("/tmp/nonexistent-project")
	if err != nil {
		t.Fatalf("ListSessions (empty): %v", err)
	}
	if sessions != nil {
		t.Errorf("ListSessions (empty): got %v, want nil", sessions)
	}
}

func TestSessionIsolation(t *testing.T) {
	tmpdir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmpdir)

	// Save to project A
	if err := SaveSession("/tmp/project-a", []Message{{Role: "user", Content: "a"}}); err != nil {
		t.Fatalf("SaveSession A: %v", err)
	}

	// Project B should have no sessions
	msgs, err := LoadLatestSession("/tmp/project-b")
	if err != nil {
		t.Fatalf("LoadLatestSession B: %v", err)
	}
	if msgs != nil {
		t.Errorf("project B should have no sessions, got %+v", msgs)
	}
}

func TestSaveSessionCreatesDirectories(t *testing.T) {
	tmpdir := t.TempDir()
	// Use a nested path that doesn't exist yet
	xdg := filepath.Join(tmpdir, "deep", "nested", "state")
	t.Setenv("XDG_STATE_HOME", xdg)

	err := SaveSession("/tmp/test-project-dirs", []Message{{Role: "user", Content: "test"}})
	if err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	// Verify directory was created
	dir, _ := SessionDir("/tmp/test-project-dirs")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Errorf("session directory was not created: %s", dir)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsAt(s, substr)
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
