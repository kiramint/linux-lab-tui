package grade

import (
	"os"
	"path/filepath"
	"testing"
)

func boolPtr(b bool) *bool { return &b }

func TestFileGrader(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "practice/docs"), 0o755)
	os.WriteFile(filepath.Join(dir, "practice/docs/readme.txt"), []byte("hi"), 0o644)

	q := Question{
		ID: "t",
		Graders: []Grader{
			{Type: "file", Path: "practice/docs/readme.txt", Exists: boolPtr(true), ContentRegex: "hi"},
			{Type: "file", Path: "missing.txt", Exists: boolPtr(false)},
		},
	}
	res := GradeQuestion(q, dir, "")
	if !res.Passed {
		t.Fatalf("expected pass: %+v", res.Checks)
	}
}

func TestCommandRegex(t *testing.T) {
	hist := "welcome\n>>> sudo apt install -y openssh-server\nReading package lists...\n"
	g := Grader{Type: "command_regex", Pattern: `(?i)sudo\s+apt(-get)?\s+install\s+.*openssh-server`, Description: "apt"}
	cr := gradeCommandRegex(g, hist)
	if !cr.OK {
		t.Fatalf("expected match: %s", cr.Message)
	}
}

func TestModeOctal(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "scripts"), 0o755)
	p := filepath.Join(dir, "scripts/hello.sh")
	os.WriteFile(p, []byte("#!/bin/bash\necho hi\n"), 0o700)
	g := Grader{Type: "file", Path: "scripts/hello.sh", Exists: boolPtr(true), ModeOctal: "0700", ContentRegex: `(?i)echo.*hi`}
	cr := gradeFile(g, dir)
	if !cr.OK {
		t.Fatalf("mode check failed: %s", cr.Message)
	}
}

func TestParseBank(t *testing.T) {
	data, err := os.ReadFile("../../quests/chapter2.yaml")
	if err != nil {
		t.Skip(err)
	}
	b, err := ParseBank(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Questions) < 10 {
		t.Fatalf("expected >=10 questions, got %d", len(b.Questions))
	}
}
