package shell

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/creack/pty"
	"github.com/hinshun/vt10x"
)

const maxScrollback = 2000

// Session is an interactive bash PTY with VT emulation, IO history, and Enter-time blocklist.
type Session struct {
	ptmx      *os.File
	cmd       *exec.Cmd
	Workspace string

	mu       sync.Mutex
	history  strings.Builder
	lineBuf  []byte
	scroll   []string // ANSI-stripped completed lines for PageUp scrollback
	partial  []byte   // incomplete scrollback line
	cols     int
	rows     int
	closed   bool

	term vt10x.Terminal // own Lock; never hold mu while locking term

	OnOutput func()
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
		"HOME="+os.Getenv("HOME"),
	)
	cmd.Dir = workspace

	cols, rows := 80, 24
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
	if err != nil {
		return nil, err
	}

	term := vt10x.New(vt10x.WithSize(cols, rows))
	s := &Session{
		ptmx:      ptmx,
		cmd:       cmd,
		Workspace: workspace,
		term:      term,
		cols:      cols,
		rows:      rows,
	}
	s.appendHistory(fmt.Sprintf("=== linux-lab session start %s ===\nworkspace=%s\n",
		time.Now().Format(time.RFC3339), workspace))

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
			s.noteScrollback(chunk)
			s.feedVT(chunk)
			if s.OnOutput != nil {
				s.OnOutput()
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

func (s *Session) feedVT(p []byte) {
	if s.term == nil {
		return
	}
	_, _ = s.term.Write(p)
}

// noteScrollback appends ANSI-stripped complete lines for PageUp history.
func (s *Session) noteScrollback(p []byte) {
	plain := stripANSI(p)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, b := range plain {
		if b == '\n' {
			line := string(s.partial)
			s.partial = nil
			// drop lone CR leftovers
			line = strings.TrimSuffix(line, "\r")
			s.scroll = append(s.scroll, line)
			if len(s.scroll) > maxScrollback {
				s.scroll = s.scroll[len(s.scroll)-maxScrollback:]
			}
			continue
		}
		if b == '\r' {
			continue
		}
		s.partial = append(s.partial, b)
	}
}

func stripANSI(p []byte) []byte {
	var out []byte
	i := 0
	for i < len(p) {
		if p[i] == 0x1b {
			i++
			if i >= len(p) {
				break
			}
			switch p[i] {
			case '[': // CSI
				i++
				for i < len(p) {
					c := p[i]
					i++
					if c >= 0x40 && c <= 0x7e {
						break
					}
				}
			case ']': // OSC
				i++
				for i < len(p) {
					if p[i] == 0x07 {
						i++
						break
					}
					if p[i] == 0x1b && i+1 < len(p) && p[i+1] == '\\' {
						i += 2
						break
					}
					i++
				}
			case '(':
				fallthrough
			case ')':
				i++ // charset designation + one more
				if i < len(p) {
					i++
				}
			default:
				i++
			}
			continue
		}
		if p[i] == 0x08 { // backspace — approximate for scrollback text
			if len(out) > 0 {
				_, size := utf8.DecodeLastRune(out)
				out = out[:len(out)-size]
			}
			i++
			continue
		}
		out = append(out, p[i])
		i++
	}
	return out
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

func (s *Session) Term() vt10x.Terminal { return s.term }

func (s *Session) ScrollbackLen() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.scroll)
}

func (s *Session) ScrollbackLine(i int) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i < 0 || i >= len(s.scroll) {
		return ""
	}
	return s.scroll[i]
}

func (s *Session) ScrollbackSnapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.scroll))
	copy(out, s.scroll)
	return out
}

func (s *Session) Size() (cols, rows int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cols, s.rows
}

func (s *Session) Resize(rows, cols uint16) {
	if rows < 1 {
		rows = 1
	}
	if cols < 1 {
		cols = 1
	}
	s.mu.Lock()
	s.rows = int(rows)
	s.cols = int(cols)
	ptmx := s.ptmx
	term := s.term
	s.mu.Unlock()

	if term != nil {
		term.Resize(int(cols), int(rows))
	}
	if ptmx != nil {
		_ = pty.Setsize(ptmx, &pty.Winsize{Rows: rows, Cols: cols})
	}
}

func (s *Session) InjectOutput(data []byte) {
	s.appendHistory(string(data))
	s.noteScrollback(data)
	s.feedVT(data)
	if s.OnOutput != nil {
		s.OnOutput()
	}
}

func (s *Session) HandleInput(data []byte) (blockedMsg string) {
	s.mu.Lock()
	if s.closed || s.ptmx == nil {
		s.mu.Unlock()
		return ""
	}
	ptmx := s.ptmx
	s.mu.Unlock()

	var warn []byte
	for _, b := range data {
		s.mu.Lock()
		switch b {
		case 0x03:
			s.lineBuf = nil
			s.mu.Unlock()
			_, _ = ptmx.Write([]byte{0x03})
		case 0x04:
			s.lineBuf = nil
			s.mu.Unlock()
			_, _ = ptmx.Write([]byte{0x04})
		case 0x0c:
			s.mu.Unlock()
			_, _ = ptmx.Write([]byte{0x0c})
		case 0x15:
			s.lineBuf = nil
			s.mu.Unlock()
			_, _ = ptmx.Write([]byte{0x15})
		case 0x7f, 0x08:
			if len(s.lineBuf) > 0 {
				s.lineBuf = s.lineBuf[:len(s.lineBuf)-1]
			}
			s.mu.Unlock()
			_, _ = ptmx.Write([]byte{b})
		case '\t':
			s.lineBuf = append(s.lineBuf, '\t')
			s.mu.Unlock()
			_, _ = ptmx.Write([]byte{'\t'})
		case '\r', '\n':
			line := string(s.lineBuf)
			if reason := BlockReason(line); reason != "" {
				s.history.WriteString(">>> " + line + "\n")
				s.history.WriteString("[BLOCKED] " + reason + "\n")
				w := fmt.Sprintf("\r\n\x1b[31m[SAFETY] %s — command NOT executed\x1b[0m\r\n", reason)
				s.history.WriteString(w)
				s.lineBuf = nil
				s.mu.Unlock()
				_, _ = ptmx.Write([]byte{0x15})
				warn = []byte(w)
				blockedMsg = reason
				continue
			}
			s.history.WriteString(">>> " + line + "\n")
			s.lineBuf = nil
			s.mu.Unlock()
			_, _ = ptmx.Write([]byte{'\n'})
		default:
			if b == 0x1b {
				s.lineBuf = nil
				s.mu.Unlock()
				_, _ = ptmx.Write([]byte{b})
				continue
			}
			s.lineBuf = append(s.lineBuf, b)
			s.mu.Unlock()
			_, _ = ptmx.Write([]byte{b})
		}
	}
	if len(warn) > 0 {
		s.noteScrollback(warn)
		s.feedVT(warn)
		if s.OnOutput != nil {
			s.OnOutput()
		}
	}
	return blockedMsg
}

func (s *Session) Close() error {
	s.mu.Lock()
	s.closed = true
	ptmx := s.ptmx
	cmd := s.cmd
	s.mu.Unlock()
	if ptmx != nil {
		_ = ptmx.Close()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}
	return nil
}
