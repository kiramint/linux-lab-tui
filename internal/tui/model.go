package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

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

type ptyOutMsg struct{}
type tickMsg time.Time
type blinkMsg time.Time

type Model struct {
	bank         *grade.Bank
	state        *progress.State
	session      *shell.Session
	focus        focusPane
	width        int
	height       int
	leftVP       viewport.Model
	termW        int
	termH        int
	status       string
	quitting     bool
	confirmQ     bool
	lastGrade    *grade.QuestionResult
	prog         *tea.Program
	blinkOn      bool
	scrollOffset int // 0 = follow live screen; >0 looks into scrollback
	answering    bool
	answerBuf    string
}

func New(bank *grade.Bank, state *progress.State, session *shell.Session) *Model {
	state.EnsureMaps()
	m := &Model{
		bank:    bank,
		state:   state,
		session: session,
		focus:   focusShell,
		leftVP:  viewport.New(40, 20),
		blinkOn: true,
		status:  "Shift+Tab 切换焦点 · a/Ctrl+A 简答 · Ctrl+G 判题 · Ctrl+S 交卷 · Ctrl+Q 退出",
	}
	session.OnOutput = func() {
		if m.prog != nil {
			m.prog.Send(ptyOutMsg{})
		}
	}
	m.refreshLeft()
	return m
}

func (m *Model) SetProgram(p *tea.Program) { m.prog = p }

func (m *Model) Init() tea.Cmd {
	m.ensureInit()
	m.refreshLeft()
	return tea.Batch(tea.EnterAltScreen, autosaveTick(), blinkTick())
}

func autosaveTick() tea.Cmd {
	return tea.Tick(time.Second*2, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func blinkTick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg { return blinkMsg(t) })
}

func (m *Model) bonusIDs() map[string]bool {
	ids := map[string]bool{}
	for _, q := range m.bank.Questions {
		if q.Bonus {
			ids[q.ID] = true
		}
	}
	return ids
}

func (m *Model) formalPassed() int {
	return m.state.FormalPassedCount(m.bonusIDs())
}

func (m *Model) ensureInit() {
	q := m.currentQ()
	if strings.TrimSpace(q.Init) == "" {
		return
	}
	m.state.EnsureMaps()
	if q.ShouldInitOnce() && m.state.Inited[q.ID] {
		return
	}
	if err := grade.RunInit(q, m.state.Workspace); err != nil {
		m.status = "初始化失败: " + err.Error()
		return
	}
	m.state.Inited[q.ID] = true
	_ = m.state.Save()
	m.status = "已初始化环境 · " + q.ID
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		return m, nil

	case ptyOutMsg:
		m.scrollOffset = 0
		return m, nil

	case tickMsg:
		_ = m.state.Save()
		return m, autosaveTick()

	case blinkMsg:
		if m.focus == focusShell {
			m.blinkOn = !m.blinkOn
		} else {
			m.blinkOn = true
		}
		return m, blinkTick()

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

		// Short-answer edit mode
		if m.answering {
			return m, m.handleAnswerKey(msg)
		}

		// Global keys (before pane routing)
		switch msg.String() {
		case "ctrl+q":
			if m.state.Submitted {
				m.quitting = true
				return m, tea.Quit
			}
			m.confirmQ = true
			m.status = "确认退出？未交卷进度已自动保存。按 y 退出，其他键取消"
			return m, nil
		case "ctrl+c":
			if m.focus == focusShell {
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
		case "shift+tab":
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
		case "ctrl+a":
			if m.currentQ().HasShortAnswer() {
				m.beginAnswer()
				return m, nil
			}
			m.status = "本题无需简答"
			return m, nil
		}

		if m.focus == focusShell {
			switch msg.String() {
			case "pgup", "ctrl+b":
				max := m.session.ScrollbackLen()
				m.scrollOffset += m.termH
				if m.scrollOffset > max {
					m.scrollOffset = max
				}
				return m, nil
			case "pgdown", "ctrl+f":
				m.scrollOffset -= m.termH
				if m.scrollOffset < 0 {
					m.scrollOffset = 0
				}
				return m, nil
			}
			m.feedKey(msg)
			return m, nil
		}

		// Question pane keys
		switch msg.String() {
		case "enter", "c":
			m.checkCurrent()
		case "a":
			if m.currentQ().HasShortAnswer() {
				m.beginAnswer()
				return m, nil
			}
			m.status = "本题无需简答"
		case "n", "right", "l":
			if m.state.CurrentIndex < len(m.bank.Questions)-1 {
				m.state.CurrentIndex++
				_ = m.state.Save()
				m.ensureInit()
			}
		case "p", "left", "h":
			if m.state.CurrentIndex > 0 {
				m.state.CurrentIndex--
				_ = m.state.Save()
				m.ensureInit()
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

func (m *Model) beginAnswer() {
	m.state.EnsureMaps()
	q := m.currentQ()
	m.answering = true
	m.answerBuf = m.state.Answers[q.ID]
	m.focus = focusQuestion
	prompt := q.AnswerPrompt
	if prompt == "" {
		prompt = "请输入简答"
	}
	m.status = "答案> " + m.answerBuf
	_ = prompt
	m.refreshLeft()
}

func (m *Model) handleAnswerKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc", "ctrl+[":
		m.answering = false
		m.answerBuf = ""
		m.status = "已取消简答"
		m.refreshLeft()
		return nil
	case "enter":
		q := m.currentQ()
		m.state.EnsureMaps()
		m.state.Answers[q.ID] = strings.TrimSpace(m.answerBuf)
		_ = m.state.Save()
		m.answering = false
		m.status = "已保存简答: " + m.state.Answers[q.ID]
		m.answerBuf = ""
		m.refreshLeft()
		return nil
	case "backspace", "ctrl+h":
		if m.answerBuf != "" {
			_, size := utf8.DecodeLastRuneInString(m.answerBuf)
			m.answerBuf = m.answerBuf[:len(m.answerBuf)-size]
		}
		m.status = "答案> " + m.answerBuf
		return nil
	case "ctrl+u":
		m.answerBuf = ""
		m.status = "答案> "
		return nil
	default:
		if msg.Type == tea.KeyRunes {
			m.answerBuf += string(msg.Runes)
			m.status = "答案> " + m.answerBuf
			return nil
		}
		if len(msg.String()) == 1 {
			m.answerBuf += msg.String()
			m.status = "答案> " + m.answerBuf
		}
		return nil
	}
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

func (m *Model) studentAnswer() string {
	m.state.EnsureMaps()
	return m.state.Answers[m.currentQ().ID]
}

func (m *Model) checkCurrent() {
	q := m.currentQ()
	hist := m.session.History()
	ans := m.studentAnswer()
	res := grade.GradeQuestion(q, m.state.Workspace, hist, ans)
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
	hist := m.session.History()
	m.state.EnsureMaps()
	for _, q := range m.bank.Questions {
		if _, ok := m.state.Results[q.ID]; !ok {
			ans := m.state.Answers[q.ID]
			res := grade.GradeQuestion(q, m.state.Workspace, hist, ans)
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
		Title        string                             `json:"title"`
		SessionID    string                             `json:"session_id"`
		Workspace    string                             `json:"workspace"`
		Submitted    time.Time                          `json:"submitted_at"`
		Passed       int                                `json:"passed"`
		FormalPassed int                                `json:"formal_passed"`
		FormalTotal  int                                `json:"formal_total"`
		Total        int                                `json:"total"`
		Results      map[string]progress.QuestionResult `json:"results"`
		Answers      map[string]string                  `json:"answers,omitempty"`
		History      string                             `json:"history_file"`
	}
	sum := summary{
		Title:        m.bank.Title,
		SessionID:    m.state.SessionID,
		Workspace:    m.state.Workspace,
		Submitted:    time.Now(),
		Passed:       m.state.PassedCount(),
		FormalPassed: m.formalPassed(),
		FormalTotal:  m.bank.FormalCount(),
		Total:        len(m.bank.Questions),
		Results:      m.state.Results,
		Answers:      m.state.Answers,
		History:      histPath,
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
	innerH := m.height - 3
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
	m.termW = rightW - 2
	m.termH = innerH - 2
	if m.termW < 1 {
		m.termW = 1
	}
	if m.termH < 1 {
		m.termH = 1
	}
	m.session.Resize(uint16(m.termH), uint16(m.termW))
	m.refreshLeft()
}

func (m *Model) refreshLeft() {
	q := m.currentQ()
	var b strings.Builder
	formalPass := m.formalPassed()
	formalTotal := m.bank.FormalCount()
	total := len(m.bank.Questions)
	fmt.Fprintf(&b, "正式 %d/%d 通过 · 题 %d/%d（含附加）\n", formalPass, formalTotal, m.state.CurrentIndex+1, total)
	fmt.Fprintf(&b, "────────────────────────────────\n")
	bonusTag := ""
	if q.Bonus {
		bonusTag = " [附加]"
	}
	fmt.Fprintf(&b, "[%s] %s%s\n", q.ID, q.Title, bonusTag)
	fmt.Fprintf(&b, "章节: %s\n", q.Section)
	if q.NeedsSudo {
		fmt.Fprintf(&b, "⚠ 需要 sudo（Ubuntu 24.04 学生机）\n")
	}
	if strings.TrimSpace(q.Init) != "" {
		if m.state.Inited[q.ID] {
			fmt.Fprintf(&b, "✓ 环境已初始化\n")
		} else {
			fmt.Fprintf(&b, "· 进入本题时自动初始化\n")
		}
	}
	fmt.Fprintf(&b, "\n%s\n", strings.TrimSpace(q.Instructions))
	if q.HasShortAnswer() {
		fmt.Fprintf(&b, "\n── 简答 ──\n")
		prompt := q.AnswerPrompt
		if prompt == "" {
			prompt = "请输入答案"
		}
		fmt.Fprintf(&b, "%s\n", prompt)
		cur := m.state.Answers[q.ID]
		if cur == "" {
			fmt.Fprintf(&b, "当前答案: （未作答，按 a 或 Ctrl+A 输入）\n")
		} else {
			fmt.Fprintf(&b, "当前答案: %s\n", cur)
		}
	}
	fmt.Fprintf(&b, "\n── 判题器提示 ──\n")
	for i, g := range q.Graders {
		switch strings.ToLower(g.Type) {
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
		case "short_answer":
			desc := g.Description
			if desc == "" {
				desc = "简答"
			}
			fmt.Fprintf(&b, "%d. short_answer: %s\n", i+1, desc)
		default:
			fmt.Fprintf(&b, "%d. %s\n", i+1, g.Type)
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
	fmt.Fprintf(&b, "\n键: Enter/c 判题 · a 简答 · n/p 下一题/上一题 · s 交卷\n")
	fmt.Fprintf(&b, "焦点: %s（Shift+Tab 切换）\n", map[focusPane]string{focusQuestion: "题目", focusShell: "Shell"}[m.focus])
	m.leftVP.SetContent(b.String())
}

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	borderFocus  = lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(lipgloss.Color("14"))
	borderBlur   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240"))
	statusStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	dangerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	shellHiStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14")).Background(lipgloss.Color("236"))
)

func (m *Model) View() string {
	if m.width == 0 {
		return "loading..."
	}
	title := titleStyle.Render(m.bank.Title)
	formalPass := m.formalPassed()
	formalTotal := m.bank.FormalCount()
	head := fmt.Sprintf("%s  ·  正式 %d/%d PASS  ·  ws: %s", title, formalPass, formalTotal, m.state.Workspace)

	leftBorder := borderBlur
	rightBorder := borderBlur
	shellLabel := " Shell "
	if m.focus == focusQuestion {
		leftBorder = borderFocus
	} else {
		rightBorder = borderFocus
		shellLabel = shellHiStyle.Render(" ● Shell ")
	}

	leftBody := m.leftVP.View()
	rightBody := renderShellPane(m.session, m.termW, m.termH, m.focus == focusShell, m.blinkOn, m.scrollOffset)

	left := leftBorder.Width(m.leftVP.Width + 2).Height(m.leftVP.Height + 2).Render(leftBody)
	_ = shellLabel
	right := rightBorder.Width(m.termW + 2).Height(m.termH + 2).Render(rightBody)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	status := m.status
	if m.answering {
		prompt := m.currentQ().AnswerPrompt
		if prompt == "" {
			prompt = "简答"
		}
		status = fmt.Sprintf("[%s] 答案> %s  (Enter 保存 · Esc 取消)", prompt, m.answerBuf)
	} else if m.scrollOffset > 0 {
		status = fmt.Sprintf("[scrollback +%d] PgUp/PgDn · ", m.scrollOffset) + status
	}
	if m.confirmQ {
		status = dangerStyle.Render(status)
	} else {
		status = statusStyle.Render(status)
	}
	return head + "\n" + body + "\n" + status
}
