package grade

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInitScriptsFromBank(t *testing.T) {
	data, err := os.ReadFile("../../quests/chapter2.yaml")
	if err != nil {
		t.Skip(err)
	}
	b, err := ParseBank(data)
	if err != nil {
		t.Fatal(err)
	}
	ws := t.TempDir()
	ids := []string{"Q04-permissions-chmod", "Q08-processes", "Q13-scp", "Q14-du-hs", "Q16-lsof"}
	for _, id := range ids {
		var q Question
		for _, qq := range b.Questions {
			if qq.ID == id {
				q = qq
				break
			}
		}
		if q.ID == "" {
			t.Fatalf("missing %s", id)
		}
		done := make(chan error, 1)
		go func(q Question) { done <- RunInit(q, ws) }(q)
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("%s: %v", id, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: RunInit hung (use setsid for background helpers)", id)
		}
	}
	// re-init Q08 idempotent
	for _, qq := range b.Questions {
		if qq.ID == "Q08-processes" {
			if err := RunInit(qq, ws); err != nil {
				t.Fatalf("reinit q08: %v", err)
			}
		}
	}
	cmd := exec.Command("bash", "-lc", "du -hs bigdir | awk '{print $1}'")
	cmd.Dir = ws
	out, err := cmd.CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		t.Fatalf("du: %v %q", err, out)
	}
	for _, p := range []string{".lab/q08.pid", ".lab/q16.pid"} {
		b, err := os.ReadFile(filepath.Join(ws, p))
		if err != nil {
			t.Fatal(err)
		}
		pid := strings.TrimSpace(string(b))
		_ = exec.Command("kill", pid).Run()
	}
}

func TestBackgroundInitNeedsSetsid(t *testing.T) {
	ws := t.TempDir()
	q := Question{ID: "bg", Init: `
mkdir -p .lab
setsid bash -c 'exec -a lab-test-proc sleep 3600' </dev/null >/dev/null 2>&1 &
echo $! > .lab/pid
`}
	done := make(chan error, 1)
	go func() { done <- RunInit(q, ws) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("hung")
	}
	b, _ := os.ReadFile(filepath.Join(ws, ".lab/pid"))
	_ = exec.Command("kill", strings.TrimSpace(string(b))).Run()
}
