package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kiramint/linux-lab-tui/internal/grade"
	"github.com/kiramint/linux-lab-tui/internal/progress"
	"github.com/kiramint/linux-lab-tui/internal/shell"
	"github.com/kiramint/linux-lab-tui/internal/tui"

	_ "embed"
)

//go:embed quests/chapter2.yaml
var questYAML []byte

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "-v", "--version":
			fmt.Println("linux-lab 0.1.1")
			return
		case "help", "-h", "--help":
			printHelp()
			return
		case "list":
			bank, err := grade.ParseBank(questYAML)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			for i, q := range bank.Questions {
				sudo := ""
				if q.NeedsSudo {
					sudo = " [sudo]"
				}
				fmt.Printf("%2d. %s — %s%s\n", i+1, q.ID, q.Title, sudo)
			}
			return
		case "test-blocklist":
			// quick manual check used in smoke tests
			samples := []string{"dd if=/dev/zero", "fdisk -l", "cfdisk", "parted", "rm -rf / --no-preserve-root", "ls"}
			for _, s := range samples {
				fmt.Printf("%q blocked=%v reason=%s\n", s, shell.IsBlocked(s), shell.BlockReason(s))
			}
			return
		}
	}

	bank, err := grade.ParseBank(questYAML)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load quests: %v\n", err)
		os.Exit(1)
	}

	state, err := progress.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load progress: %v\n", err)
		os.Exit(1)
	}
	sessionID := fmt.Sprintf("%d", time.Now().Unix())
	workspace := filepath.Join(bank.WorkspacePrefix, sessionID)
	if state != nil && state.Workspace != "" && !state.Submitted {
		// Resume same workspace if still present
		if st, err := os.Stat(state.Workspace); err == nil && st.IsDir() {
			workspace = state.Workspace
			sessionID = state.SessionID
		} else {
			state = nil
		}
	} else if state != nil && state.Submitted {
		state = nil // fresh after submit
	}
	if state == nil {
		state = progress.NewState(sessionID, workspace)
	} else {
		state.Workspace = workspace
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "workspace: %v\n", err)
		os.Exit(1)
	}
	_ = state.Save()

	sess, err := shell.Start(workspace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start shell: %v\n", err)
		os.Exit(1)
	}
	defer sess.Close()

	m := tui.New(bank, state, sess)
	p := tea.NewProgram(m, tea.WithAltScreen())
	m.SetProgram(p)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "tui: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\n进度文件: ~/.config/linux-lab/progress.json\n工作区: %s\n", workspace)
	if state.Submitted {
		fmt.Printf("交卷摘要: %s\n历史: %s\n", state.SubmitPath, state.HistoryPath)
	}
}

func printHelp() {
	fmt.Print(`linux-lab — Linux 终端操作实操测验（TUI）

用法:
  linux-lab              启动测验
  linux-lab list         列出题目
  linux-lab version
  linux-lab test-blocklist

快捷键:
  Shift+Tab    题目 / Shell 焦点切换
  Tab          Shell 焦点下交给 bash（补全）
  Ctrl+L       清屏（clear）
  PgUp/PgDn    Shell 回滚缓冲
  Ctrl+G       判当前题
  Ctrl+S       交卷（写入 IO history + JSON）
  n / p        下一题 / 上一题（题目焦点）
  Ctrl+Q       退出（未交卷会确认）
  Ctrl+C       在 Shell 焦点下转发给 bash

安全:
  禁止 dd / fdisk / cfdisk / parted / --no-preserve-root / rm -rf /

环境: Ubuntu 24.04 学生机；含 openssh-server 安装题（需 sudo）。
`)
}
