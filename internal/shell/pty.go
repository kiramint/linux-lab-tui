package shell

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
)

// Session is an interactive bash PTY with IO history and Enter-time blocklist.
type Session struct {
	ptmx      *os.File
	cmd       *exec.Cmd
	Workspace string

	mu      sync.Mutex
	history strings.Builder
	lineBuf []byte

	OnOutput func([]byte)

	closed bool
}

func Start(workspace string) (*Session, error) {
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return nil, err
	}
	cmd := exec.Command("bash", "--noprofile", "--norc")
	cmd.Env = append(os.Environ(),
		"LINUX_LAB_WORKSPACE="+workspace,
		"TERM=xterm-256color",
		`PS1=\[\e[32m\]lab\[\e[0m\]:\w\$ `,
		"HOME="+os.Getenv("HOME"), // keep real home for ~/.config; cwd is workspace
	)
	cmd.Dir = workspace

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}
	_ = pty.Setsize(ptmx, &pty.Winsize{Rows: 24, Cols: 80})

	s := &Session{ptmx: ptmx, cmd: cmd, Workspace: workspace}
	s.appendHistory(fmt.Sprintf("=== linux-lab session start %s ===\nworkspace=%s\n",
		time.Now().Format(time.RFC3339), workspace))

	// Ensure cwd + friendly prompt (PS1 already set in env for non-interactive? bash --norc reads PS1 from env if set)
	_, _ = ptmx.Write([]byte("cd \"$LINUX_LAB_WORKSPACE\" && export PS1='\\[\\e[32m\\]lab\\[\\e[0m\\]:\\w\\$ '\n"))

	go s.readLoop()
	return s, nil
}

func (s *Session) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			s.appendHistory(string(chunk))
			if s.OnOutput != nil {
				s.OnOutput(chunk)
			}
		}
		if err != nil {
			if err != io.EOF {
				s.appendHistory(fmt.Sprintf("\n[pty read error: %v]\n", err))
			}
			return
		}
	}
}

func (s *Session) appendHistory(t string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history.WriteString(t)
}

func (s *Session) History() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.history.String()
}

func (s *Session) Resize(rows, cols uint16) {
	if s.ptmx == nil {
		return
	}
	if rows < 1 {
		rows = 1
	}
	if cols < 1 {
		cols = 1
	}
	_ = pty.Setsize(s.ptmx, &pty.Winsize{Rows: rows, Cols: cols})
}

// HandleInput processes raw input bytes from the TUI keyboard when shell pane is focused.
// Characters are forwarded to the PTY; on Enter the pending line is checked against the blocklist.
// If blocked, sends Ctrl+U to erase the line in the shell and returns a warning message.
func (s *Session) HandleInput(data []byte) (blockedMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.ptmx == nil {
		return ""
	}

	for _, b := range data {
		switch b {
		case 0x03: // Ctrl+C
			s.lineBuf = nil
			_, _ = s.ptmx.Write([]byte{0x03})
		case 0x04: // Ctrl+D
			s.lineBuf = nil
			_, _ = s.ptmx.Write([]byte{0x04})
		case 0x15: // Ctrl+U kill line
			s.lineBuf = nil
			_, _ = s.ptmx.Write([]byte{0x15})
		case 0x7f, 0x08: // Backspace
			if len(s.lineBuf) > 0 {
				s.lineBuf = s.lineBuf[:len(s.lineBuf)-1]
			}
			_, _ = s.ptmx.Write([]byte{b})
		case '\r', '\n':
			line := string(s.lineBuf)
			if reason := BlockReason(line); reason != "" {
				// Erase line in shell (Ctrl+U) and do not execute
				_, _ = s.ptmx.Write([]byte{0x15})
				s.history.WriteString(">>> " + line + "\n")
				s.history.WriteString("[BLOCKED] " + reason + "\n")
				warn := fmt.Sprintf("\r\n\x1b[31m[SAFETY] %s — command NOT executed\x1b[0m\r\n", reason)
				// Inject warning into history; UI also shows via OnOutput
				s.history.WriteString(warn)
				s.lineBuf = nil
				if s.OnOutput != nil {
					// release lock? call outside — schedule via copy
					msg := []byte(warn)
					go func() { s.OnOutput(msg) }()
				}
				blockedMsg = reason
				continue
			}
			s.history.WriteString(">>> " + line + "\n")
			s.lineBuf = nil
			_, _ = s.ptmx.Write([]byte{'\n'})
		default:
			// Escape sequences / arrows: forward and reset simple line tracking on ESC
			if b == 0x1b {
				s.lineBuf = nil // arrow/history nav invalidates simple buffer
				_, _ = s.ptmx.Write([]byte{b})
				continue
			}
			s.lineBuf = append(s.lineBuf, b)
			_, _ = s.ptmx.Write([]byte{b})
		}
	}
	return blockedMsg
}

func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.ptmx != nil {
		_ = s.ptmx.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
		_, _ = s.cmd.Process.Wait()
	}
	return nil
}
