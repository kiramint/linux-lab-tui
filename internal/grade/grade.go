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

// HistoryExtractor pulls command lines the student typed from IO history.
// We look for lines after shell prompts or raw input records prefixed with "$ ".
func ExtractTypedCommands(history string) []string {
	var cmds []string
	lines := strings.Split(history, "\n")
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		// Our shell pane prefixes sent commands as: >>> CMD
		if strings.HasPrefix(line, ">>> ") {
			cmds = append(cmds, strings.TrimPrefix(line, ">>> "))
			continue
		}
		// Also accept classic PS1 capture: user@host:path$ cmd
		if idx := strings.LastIndex(line, "$ "); idx >= 0 && idx+2 < len(line) {
			rest := line[idx+2:]
			if rest != "" && !strings.HasPrefix(rest, ">>>") {
				cmds = append(cmds, rest)
			}
		}
	}
	return cmds
}

func GradeQuestion(q Question, workspace, history string) QuestionResult {
	res := QuestionResult{QuestionID: q.ID}
	for _, g := range q.Graders {
		cr := runGrader(g, workspace, history)
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

func runGrader(g Grader, workspace, history string) CheckResult {
	switch strings.ToLower(g.Type) {
	case "file":
		return gradeFile(g, workspace)
	case "command_regex":
		return gradeCommandRegex(g, history)
	case "output":
		return gradeOutput(g, workspace)
	default:
		return CheckResult{OK: false, Message: fmt.Sprintf("unknown grader type %q", g.Type)}
	}
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
			// Use permission bits only
			_ = stat
		}
		want := strings.TrimPrefix(g.ModeOctal, "0")
		got := fmt.Sprintf("%o", uint32(mode))
		// Normalize to 3 or 4 digits
		if !modeEqual(got, want) {
			return CheckResult{OK: false, Message: fmt.Sprintf("mode %s want %s got %s", label, g.ModeOctal, got)}
		}
	}
	return CheckResult{OK: true, Message: fmt.Sprintf("ok: %s", label)}
}

func modeEqual(got, want string) bool {
	// Compare last 3 octal digits
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
	// Fallback: search whole history (prompts may wrap oddly)
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
