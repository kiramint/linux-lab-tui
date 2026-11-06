package grade

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

type CheckResult struct {
	OK      bool
	Message string
}

type QuestionResult struct {
	QuestionID string
	Passed     bool
	Checks     []CheckResult
}

// ExtractTypedCommands pulls command lines the student typed from IO history.
func ExtractTypedCommands(history string) []string {
	var cmds []string
	lines := strings.Split(history, "\n")
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, ">>> ") {
			cmds = append(cmds, strings.TrimPrefix(line, ">>> "))
			continue
		}
		if idx := strings.LastIndex(line, "$ "); idx >= 0 && idx+2 < len(line) {
			rest := line[idx+2:]
			if rest != "" && !strings.HasPrefix(rest, ">>>") {
				cmds = append(cmds, rest)
			}
		}
	}
	return cmds
}

// RunInit executes the question's init script in workspace via bash -lc.
// Init is NOT subject to the student command blocklist (dd allowed here).
func RunInit(q Question, workspace string) error {
	script := strings.TrimSpace(q.Init)
	if script == "" {
		return nil
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return err
	}
	cmd := exec.Command("bash", "-lc", script)
	cmd.Dir = workspace
	cmd.Env = append(os.Environ(), "LINUX_LAB_WORKSPACE="+workspace)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return fmt.Errorf("%v", err)
		}
		return fmt.Errorf("%v: %s", err, truncate(msg, 200))
	}
	return nil
}

// GradeQuestion runs all graders. studentAnswer is used by short_answer graders.
func GradeQuestion(q Question, workspace, history, studentAnswer string) QuestionResult {
	res := QuestionResult{QuestionID: q.ID}
	for _, g := range q.Graders {
		cr := runGrader(g, workspace, history, studentAnswer)
		res.Checks = append(res.Checks, cr)
	}
	res.Passed = true
	if len(res.Checks) == 0 {
		res.Passed = false
		res.Checks = append(res.Checks, CheckResult{OK: false, Message: "no graders configured"})
		return res
	}
	for _, c := range res.Checks {
		if !c.OK {
			res.Passed = false
			break
		}
	}
	return res
}

func runGrader(g Grader, workspace, history, studentAnswer string) CheckResult {
	switch strings.ToLower(g.Type) {
	case "file":
		return gradeFile(g, workspace)
	case "command_regex":
		return gradeCommandRegex(g, history)
	case "output":
		return gradeOutput(g, workspace)
	case "short_answer":
		return gradeShortAnswer(g, workspace, studentAnswer)
	default:
		return CheckResult{OK: false, Message: fmt.Sprintf("unknown grader type %q", g.Type)}
	}
}

func normalizeAnswer(s, mode string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "\n")
	s = strings.TrimSpace(s)
	switch strings.ToLower(mode) {
	case "", "trim":
		return s
	default:
		return s
	}
}

func gradeShortAnswer(g Grader, workspace, studentAnswer string) CheckResult {
	desc := g.Description
	if desc == "" {
		desc = "简答"
	}
	ans := normalizeAnswer(studentAnswer, g.Normalize)
	if ans == "" {
		return CheckResult{OK: false, Message: "未作答: " + desc}
	}

	if g.ExpectCommand != "" {
		cmd := exec.Command("bash", "-lc", g.ExpectCommand)
		// short_answer expect_command defaults to workspace cwd
		cmd.Dir = workspace
		cmd.Env = append(os.Environ(), "LINUX_LAB_WORKSPACE="+workspace)
		out, err := cmd.CombinedOutput()
		expected := normalizeAnswer(string(out), g.Normalize)
		if err != nil && expected == "" {
			return CheckResult{OK: false, Message: fmt.Sprintf("expect_command failed: %v", err)}
		}
		if ans == expected {
			return CheckResult{OK: true, Message: "简答正确: " + desc}
		}
		return CheckResult{OK: false, Message: fmt.Sprintf("简答不符: %s (期望 %q 得到 %q)", desc, expected, ans)}
	}

	if g.ExpectExact != "" {
		want := normalizeAnswer(g.ExpectExact, g.Normalize)
		if ans == want {
			return CheckResult{OK: true, Message: "简答正确: " + desc}
		}
		return CheckResult{OK: false, Message: fmt.Sprintf("简答不符: %s (期望 %q)", desc, want)}
	}

	if g.ExpectRegex != "" {
		re, err := regexp.Compile(g.ExpectRegex)
		if err != nil {
			return CheckResult{OK: false, Message: fmt.Sprintf("bad expect_regex: %v", err)}
		}
		if re.MatchString(ans) {
			return CheckResult{OK: true, Message: "简答正确: " + desc}
		}
		return CheckResult{OK: false, Message: "简答不符正则: " + desc}
	}

	return CheckResult{OK: false, Message: "short_answer 未配置期望值: " + desc}
}

func gradeFile(g Grader, workspace string) CheckResult {
	rel := g.Path
	if rel == "" {
		return CheckResult{OK: false, Message: "file grader missing path"}
	}
	full := rel
	if !filepath.IsAbs(rel) {
		full = filepath.Join(workspace, rel)
	}
	info, err := os.Lstat(full)
	exists := err == nil
	wantExists := true
	if g.Exists != nil {
		wantExists = *g.Exists
	}

	label := rel
	if !wantExists {
		if exists {
			return CheckResult{OK: false, Message: fmt.Sprintf("file should not exist: %s", label)}
		}
		return CheckResult{OK: true, Message: fmt.Sprintf("absent as required: %s", label)}
	}
	if !exists {
		return CheckResult{OK: false, Message: fmt.Sprintf("missing: %s", label)}
	}
	if g.IsDir && !info.IsDir() {
		return CheckResult{OK: false, Message: fmt.Sprintf("not a directory: %s", label)}
	}
	if g.ContentRegex != "" {
		if info.IsDir() {
			return CheckResult{OK: false, Message: fmt.Sprintf("cannot content-check directory: %s", label)}
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return CheckResult{OK: false, Message: fmt.Sprintf("read %s: %v", label, err)}
		}
		re, err := regexp.Compile(g.ContentRegex)
		if err != nil {
			return CheckResult{OK: false, Message: fmt.Sprintf("bad content_regex: %v", err)}
		}
		if !re.Match(data) {
			return CheckResult{OK: false, Message: fmt.Sprintf("content mismatch: %s", label)}
		}
	}
	if g.ModeOctal != "" {
		stat, ok := info.Sys().(*syscall.Stat_t)
		mode := info.Mode().Perm()
		if ok {
			_ = stat
		}
		want := strings.TrimPrefix(g.ModeOctal, "0")
		got := fmt.Sprintf("%o", uint32(mode))
		if !modeEqual(got, want) {
			return CheckResult{OK: false, Message: fmt.Sprintf("mode %s want %s got %s", label, g.ModeOctal, got)}
		}
	}
	return CheckResult{OK: true, Message: fmt.Sprintf("ok: %s", label)}
}

func modeEqual(got, want string) bool {
	for len(got) < 3 {
		got = "0" + got
	}
	for len(want) < 3 {
		want = "0" + want
	}
	if len(got) > 3 {
		got = got[len(got)-3:]
	}
	if len(want) > 3 {
		want = want[len(want)-3:]
	}
	return got == want
}

func gradeCommandRegex(g Grader, history string) CheckResult {
	if g.Pattern == "" {
		return CheckResult{OK: false, Message: "command_regex missing pattern"}
	}
	re, err := regexp.Compile(g.Pattern)
	if err != nil {
		return CheckResult{OK: false, Message: fmt.Sprintf("bad pattern: %v", err)}
	}
	desc := g.Description
	if desc == "" {
		desc = g.Pattern
	}
	cmds := ExtractTypedCommands(history)
	for _, c := range cmds {
		if re.MatchString(c) {
			return CheckResult{OK: true, Message: "matched: " + desc}
		}
	}
	if re.MatchString(history) {
		return CheckResult{OK: true, Message: "matched in history: " + desc}
	}
	return CheckResult{OK: false, Message: "no match: " + desc}
}

func gradeOutput(g Grader, workspace string) CheckResult {
	if g.Command == "" {
		return CheckResult{OK: false, Message: "output grader missing command"}
	}
	cmd := exec.Command("bash", "-lc", g.Command)
	if g.CwdWorkspace {
		cmd.Dir = workspace
	}
	out, err := cmd.CombinedOutput()
	text := string(out)
	desc := g.Description
	if desc == "" {
		desc = g.Command
	}
	if g.StdoutExact != "" {
		if strings.TrimSpace(text) == strings.TrimSpace(g.StdoutExact) {
			return CheckResult{OK: true, Message: "output exact: " + desc}
		}
		if err != nil {
			return CheckResult{OK: false, Message: fmt.Sprintf("output mismatch (%v): %s", err, desc)}
		}
		return CheckResult{OK: false, Message: "output mismatch: " + desc}
	}
	if g.StdoutRegex != "" {
		re, errRe := regexp.Compile(g.StdoutRegex)
		if errRe != nil {
			return CheckResult{OK: false, Message: fmt.Sprintf("bad stdout_regex: %v", errRe)}
		}
		if re.MatchString(text) {
			return CheckResult{OK: true, Message: "output ok: " + desc}
		}
		return CheckResult{OK: false, Message: fmt.Sprintf("output regex fail: %s (got %q)", desc, truncate(text, 120))}
	}
	if err != nil {
		return CheckResult{OK: false, Message: fmt.Sprintf("command failed: %s: %v", desc, err)}
	}
	return CheckResult{OK: true, Message: "command ran: " + desc}
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
