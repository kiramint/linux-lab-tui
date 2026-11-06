package grade

import (
	"os"
	"path/filepath"
	"strings"
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
	res := GradeQuestion(q, dir, "", "")
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
	if len(b.Questions) < 17 {
		t.Fatalf("expected >=17 questions, got %d", len(b.Questions))
	}
	var bonus, hasInit, hasShort int
	ids := map[string]bool{}
	for _, q := range b.Questions {
		if ids[q.ID] {
			t.Fatalf("duplicate id %s", q.ID)
		}
		ids[q.ID] = true
		if q.Bonus {
			bonus++
		}
		if strings.TrimSpace(q.Init) != "" {
			hasInit++
		}
		if q.HasShortAnswer() {
			hasShort++
		}
	}
	if bonus < 1 {
		t.Fatal("expected at least one bonus question")
	}
	if hasInit < 5 {
		t.Fatalf("expected several init scripts, got %d", hasInit)
	}
	if hasShort < 2 {
		t.Fatalf("expected short_answer questions, got %d", hasShort)
	}
	if !ids["Q01-paths-files"] || ids["Q07-whoami"] {
		t.Fatal("outline mismatch: expect merged paths-files and no whoami")
	}
	if b.FormalCount() != len(b.Questions)-bonus {
		t.Fatalf("FormalCount mismatch")
	}
}

func TestRunInit(t *testing.T) {
	dir := t.TempDir()
	q := Question{
		ID: "init-t",
		Init: `
mkdir -p demo/sub
echo hello > demo/sub/a.txt
`,
	}
	if err := RunInit(q, dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "demo/sub/a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "hello" {
		t.Fatalf("got %q", data)
	}
	// empty init ok
	if err := RunInit(Question{ID: "empty"}, dir); err != nil {
		t.Fatal(err)
	}
}

func TestRunInitFailure(t *testing.T) {
	dir := t.TempDir()
	err := RunInit(Question{ID: "bad", Init: "exit 42"}, dir)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestShortAnswerExact(t *testing.T) {
	g := Grader{Type: "short_answer", ExpectExact: "3.0M", Normalize: "trim", Description: "size"}
	cr := gradeShortAnswer(g, t.TempDir(), "  3.0M\n")
	if !cr.OK {
		t.Fatalf("exact: %s", cr.Message)
	}
	cr = gradeShortAnswer(g, t.TempDir(), "")
	if cr.OK || !strings.Contains(cr.Message, "未作答") {
		t.Fatalf("empty: %s", cr.Message)
	}
}

func TestShortAnswerRegex(t *testing.T) {
	g := Grader{Type: "short_answer", ExpectRegex: `^[0-9.]+[KMGT]?$`, Description: "size"}
	cr := gradeShortAnswer(g, t.TempDir(), "2.0M")
	if !cr.OK {
		t.Fatalf("regex: %s", cr.Message)
	}
	cr = gradeShortAnswer(g, t.TempDir(), "nope")
	if cr.OK {
		t.Fatal("expected fail")
	}
}

func TestShortAnswerExpectCommand(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "bigdir"), 0o755)
	os.WriteFile(filepath.Join(dir, "bigdir/x"), []byte("abc"), 0o644)
	g := Grader{
		Type:          "short_answer",
		ExpectCommand: "du -hs bigdir | awk '{print $1}'",
		Normalize:     "trim",
		Description:   "du",
	}
	// compute expected
	wantCR := gradeShortAnswer(g, dir, "___wrong___")
	if wantCR.OK {
		t.Fatal("wrong should fail")
	}
	if err := RunInit(Question{Init: "du -hs bigdir | awk '{print $1}' > .size"}, dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".size"))
	if err != nil {
		t.Fatal(err)
	}
	ans := strings.TrimSpace(string(data))
	cr := gradeShortAnswer(g, dir, ans)
	if !cr.OK {
		t.Fatalf("expect_command: %s (ans=%q)", cr.Message, ans)
	}
	res := GradeQuestion(Question{ID: "sa", Graders: []Grader{g}}, dir, "", ans)
	if !res.Passed {
		t.Fatalf("GradeQuestion: %+v", res.Checks)
	}
}

func TestShouldInitOnceDefault(t *testing.T) {
	q := Question{}
	if !q.ShouldInitOnce() {
		t.Fatal("default true")
	}
	f := false
	q.InitOnce = &f
	if q.ShouldInitOnce() {
		t.Fatal("explicit false")
	}
}

func TestHasShortAnswer(t *testing.T) {
	q := Question{AnswerPrompt: "x"}
	if !q.HasShortAnswer() {
		t.Fatal("prompt")
	}
	q2 := Question{Graders: []Grader{{Type: "short_answer"}}}
	if !q2.HasShortAnswer() {
		t.Fatal("grader")
	}
	if (Question{}).HasShortAnswer() {
		t.Fatal("none")
	}
}
