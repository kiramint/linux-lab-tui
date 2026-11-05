package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kiramint/linux-lab-tui/internal/grade"
	"github.com/kiramint/linux-lab-tui/internal/progress"
	"github.com/kiramint/linux-lab-tui/internal/shell"
)

type focusPane int

const (
	focusQuestion focusPane = iota
	focusShell
)

type ptyOutMsg []byte
type tickMsg time.Time

type Model struct {
	bank      *grade.Bank
	state     *progress.State
	session   *shell.Session
	focus     focusPane
	width     int
	height    int
	leftVP    viewport.Model
	rightVP   viewport.Model
	shellView strings.Builder
	status    string
	quitting  bool
	confirmQ  bool
	lastGrade *grade.QuestionResult
	prog      *tea.Program // set after NewProgram for pty callbacks
}

func New(bank *grade.Bank, state *progress.State, session *shell.Session) *Model {
	m := &Model{
		bank:    bank,
		state:   state,
		session: session,
		focus:   focusShell,
		leftVP:  viewport.New(40, 20),
		rightVP: viewport.New(60, 20),
		status:  "Tab 切换焦点 · Ctrl+G 判题 · Ctrl+S 交卷 · Ctrl+Q 退出",
	}
	session.OnOutput = func(b []byte) {
		// Delivered via Program.Send once wired
		if m.prog != nil {
			m.prog.Send(ptyOutMsg(append([]byte(nil), b...)))
		}
	}
	m.refreshLeft()
	return m
}

func (m *Model) SetProgram(p *tea.Program) { m.prog = p }

func (m *Model) Init() tea.Cmd {
	return tea.Batch(tea.EnterAltScreen, tick())
}

func tick() tea.Cmd {
	return tea.Tick(time.Second*2, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		return m, nil

	case ptyOutMsg:
		m.shellView.Write(msg)
		m.rightVP.SetContent(sanitizeTerminal(m.shellView.String()))
		m.rightVP.GotoBottom()
		return m, nil

	case tickMsg:
		_ = m.state.Save()
		return m, tick()

	case tea.KeyMsg:
		if m.confirmQ {
			switch msg.String() {
			case "y", "Y":
				m.quitting = true
				return m, tea.Quit
			default:
				m.confirmQ = false
				m.status = "已取消退出"
				return m, nil
			}
		}

		// Global keys
		switch msg.String() {
		case "ctrl+q", "ctrl+c":
			if msg.String() == "ctrl+c" && m.focus == focusShell {
				m.session.HandleInput([]byte{0x03})
				return m, nil
			}
			if m.state.Submitted {
				m.quitting = true
				return m, tea.Quit
			}
			m.confirmQ = true
			m.status = "确认退出？未交卷进度已自动保存。按 y 退出，其他键取消"
			return m, nil
		case "tab":
			if m.focus == focusQuestion {
				m.focus = focusShell
			} else {
				m.focus = focusQuestion
			}
			m.refreshLeft()
			return m, nil
		case "ctrl+g":
			m.checkCurrent()
			m.refreshLeft()
			return m, nil
		case "ctrl+s":
			path, err := m.submit()
			if err != nil {
				m.status = "交卷失败: " + err.Error()
			} else {
				m.status = "已交卷: " + path
			}
			m.refreshLeft()
			return m, nil
		}

		if m.focus == focusShell {
			m.feedKey(msg)
			return m, nil
		}

		// Question pane keys
		switch msg.String() {
		case "enter", "c":
			m.checkCurrent()
		case "n", "right", "l":
			if m.state.CurrentIndex < len(m.bank.Questions)-1 {
				m.state.CurrentIndex++
				_ = m.state.Save()
			}
		case "p", "left", "h":
			if m.state.CurrentIndex > 0 {
				m.state.CurrentIndex--
				_ = m.state.Save()
			}
		case "s":
			path, err := m.submit()
			if err != nil {
				m.status = "交卷失败: " + err.Error()
			} else {
				m.status = "已交卷: " + path
			}
		}
		m.refreshLeft()
		var cmd tea.Cmd
		m.leftVP, cmd = m.leftVP.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) currentQ() grade.Question {
	i := m.state.CurrentIndex
	if i < 0 {
		i = 0
	}
	if i >= len(m.bank.Questions) {
		i = len(m.bank.Questions) - 1
	}
	return m.bank.Questions[i]
}

func (m *Model) checkCurrent() {
	q := m.currentQ()
	hist := m.session.History()
	res := grade.GradeQuestion(q, m.state.Workspace, hist)
	m.lastGrade = &res
	details := make([]string, 0, len(res.Checks))
	for _, c := range res.Checks {
		mark := "FAIL"
		if c.OK {
			mark = "PASS"
		}
		details = append(details, mark+": "+c.Message)
	}
	m.state.Results[q.ID] = progress.QuestionResult{
		ID:        q.ID,
		Passed:    res.Passed,
		CheckedAt: time.Now(),
		Details:   details,
	}
	_ = m.state.Save()
	if res.Passed {
		m.status = fmt.Sprintf("✓ %s 通过", q.ID)
	} else {
		m.status = fmt.Sprintf("✗ %s 未通过 — 查看左侧详情", q.ID)
	}
}

func (m *Model) submit() (string, error) {
	// Auto-check all for summary freshness on current history
	hist := m.session.History()
	for _, q := range m.bank.Questions {
		if _, ok := m.state.Results[q.ID]; !ok {
			res := grade.GradeQuestion(q, m.state.Workspace, hist)
			details := []string{}
			for _, c := range res.Checks {
				mark := "FAIL"
				if c.OK {
					mark = "PASS"
				}
				details = append(details, mark+": "+c.Message)
			}
			m.state.Results[q.ID] = progress.QuestionResult{
				ID: q.ID, Passed: res.Passed, CheckedAt: time.Now(), Details: details,
			}
		}
	}
	subDir, err := progress.SubmissionsPath()
	if err != nil {
		return "", err
	}
	ts := time.Now().Format("20060102-150405")
	histPath := filepath.Join(subDir, ts+"-history.txt")
	sumPath := filepath.Join(subDir, ts+"-summary.json")
	if err := os.WriteFile(histPath, []byte(hist), 0o644); err != nil {
		return "", err
	}
	type summary struct {
		Title     string                           `json:"title"`
		SessionID string                           `json:"session_id"`
		Workspace string                           `json:"workspace"`
		Submitted time.Time                        `json:"submitted_at"`
		Passed    int                              `json:"passed"`
		Total     int                              `json:"total"`
		Results   map[string]progress.QuestionResult `json:"results"`
		History   string                           `json:"history_file"`
	}
	sum := summary{
		Title:     m.bank.Title,
		SessionID: m.state.SessionID,
		Workspace: m.state.Workspace,
		Submitted: time.Now(),
		Passed:    m.state.PassedCount(),
		Total:     len(m.bank.Questions),
		Results:   m.state.Results,
		History:   histPath,
	}
	data, _ := json.MarshalIndent(sum, "", "  ")
	if err := os.WriteFile(sumPath, data, 0o644); err != nil {
		return "", err
	}
	m.state.Submitted = true
	m.state.SubmitPath = sumPath
	m.state.HistoryPath = histPath
	_ = m.state.Save()
	return sumPath + " (+ " + histPath + ")", nil
}

func (m *Model) layout() {
	if m.width < 20 || m.height < 8 {
		return
	}
	innerH := m.height - 3 // title + status
	leftW := m.width * 40 / 100
	if leftW < 30 {
		leftW = 30
	}
	rightW := m.width - leftW - 3
	if rightW < 20 {
		rightW = 20
		leftW = m.width - rightW - 3
	}
	m.leftVP.Width = leftW - 2
	m.leftVP.Height = innerH - 2
	m.rightVP.Width = rightW - 2
	m.rightVP.Height = innerH - 2
	m.session.Resize(uint16(m.rightVP.Height), uint16(m.rightVP.Width))
	m.refreshLeft()
	m.rightVP.SetContent(sanitizeTerminal(m.shellView.String()))
}

func (m *Model) refreshLeft() {
	q := m.currentQ()
	var b strings.Builder
	passN := m.state.PassedCount()
	total := len(m.bank.Questions)
	fmt.Fprintf(&b, "进度 %d/%d 已通过 · 题 %d/%d\n", passN, total, m.state.CurrentIndex+1, total)
	fmt.Fprintf(&b, "────────────────────────────────\n")
	fmt.Fprintf(&b, "[%s] %s\n", q.ID, q.Title)
	fmt.Fprintf(&b, "章节: %s\n", q.Section)
	if q.NeedsSudo {
		fmt.Fprintf(&b, "⚠ 需要 sudo（Ubuntu 24.04 学生机）\n")
	}
	fmt.Fprintf(&b, "\n%s\n", strings.TrimSpace(q.Instructions))
	fmt.Fprintf(&b, "\n── 判题器提示 ──\n")
	for i, g := range q.Graders {
		switch g.Type {
		case "file":
			ex := true
			if g.Exists != nil {
				ex = *g.Exists
			}
			fmt.Fprintf(&b, "%d. file %s exists=%v\n", i+1, g.Path, ex)
		case "command_regex":
			desc := g.Description
			if desc == "" {
				desc = "命令历史匹配"
			}
			fmt.Fprintf(&b, "%d. command: %s\n", i+1, desc)
		case "output":
			desc := g.Description
			if desc == "" {
				desc = g.Command
			}
			fmt.Fprintf(&b, "%d. output: %s\n", i+1, desc)
		}
	}
	if r, ok := m.state.Results[q.ID]; ok {
		fmt.Fprintf(&b, "\n── 上次结果: ")
		if r.Passed {
			fmt.Fprintf(&b, "PASS ──\n")
		} else {
			fmt.Fprintf(&b, "FAIL ──\n")
		}
		for _, d := range r.Details {
			fmt.Fprintf(&b, "  %s\n", d)
		}
	}
	fmt.Fprintf(&b, "\n键: Enter/c 判题 · n/p 下一题/上一题 · s 交卷\n")
	fmt.Fprintf(&b, "焦点: %s\n", map[focusPane]string{focusQuestion: "题目", focusShell: "Shell"}[m.focus])
	m.leftVP.SetContent(b.String())
}

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	borderFocus = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("12"))
	borderBlur  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240"))
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	dangerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
)

func (m *Model) View() string {
	if m.width == 0 {
		return "loading..."
	}
	title := titleStyle.Render(m.bank.Title)
	passN := m.state.PassedCount()
	head := fmt.Sprintf("%s  ·  %d/%d PASS  ·  ws: %s", title, passN, len(m.bank.Questions), m.state.Workspace)

	leftBorder := borderBlur
	rightBorder := borderBlur
	if m.focus == focusQuestion {
		leftBorder = borderFocus
	} else {
		rightBorder = borderFocus
	}
	left := leftBorder.Width(m.leftVP.Width + 2).Height(m.leftVP.Height + 2).Render(m.leftVP.View())
	right := rightBorder.Width(m.rightVP.Width + 2).Height(m.rightVP.Height + 2).Render(m.rightVP.View())
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	status := m.status
	if m.confirmQ {
		status = dangerStyle.Render(status)
	} else {
		status = statusStyle.Render(status)
	}
	return head + "\n" + body + "\n" + status
}

// sanitizeTerminal strips most CSI sequences for viewport display (keep simple).
func sanitizeTerminal(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == 0x1b && i+1 < len(s) {
			// ESC [
			if s[i+1] == '[' {
				i += 2
				for i < len(s) {
					c := s[i]
					i++
					if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
						break
					}
				}
				continue
			}
			// ESC ]
			if s[i+1] == ']' {
				i += 2
				for i < len(s) && s[i] != 0x07 && s[i] != '\a' {
					if s[i] == 0x1b {
						break
					}
					i++
				}
				if i < len(s) {
					i++
				}
				continue
			}
			i += 2
			continue
		}
		if s[i] == '\r' {
			i++
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
