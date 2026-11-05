package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/hinshun/vt10x"
	"github.com/kiramint/linux-lab-tui/internal/shell"
)

var ansi16 = []string{
	"0", "1", "2", "3", "4", "5", "6", "7",
	"8", "9", "10", "11", "12", "13", "14", "15",
}

const (
	attrReverse   = 1 << 0
	attrUnderline = 1 << 1
	attrBold      = 1 << 2
)

func colorStyle(fg, bg vt10x.Color, bold, reverse, underline bool) lipgloss.Style {
	st := lipgloss.NewStyle()
	if reverse {
		fg, bg = bg, fg
		if fg == vt10x.DefaultFG {
			fg = vt10x.Black
		}
		if bg == vt10x.DefaultBG {
			bg = vt10x.White
		}
	}
	if fg != vt10x.DefaultFG && fg != vt10x.DefaultCursor {
		if fg < 16 {
			st = st.Foreground(lipgloss.Color(ansi16[fg]))
		} else if fg < 256 {
			st = st.Foreground(lipgloss.Color(itoaColor(int(fg))))
		}
	}
	if bg != vt10x.DefaultBG {
		if bg < 16 {
			st = st.Background(lipgloss.Color(ansi16[bg]))
		} else if bg < 256 {
			st = st.Background(lipgloss.Color(itoaColor(int(bg))))
		}
	}
	if bold {
		st = st.Bold(true)
	}
	if underline {
		st = st.Underline(true)
	}
	return st
}

func itoaColor(n int) string {
	if n <= 0 {
		return "0"
	}
	var b [3]byte
	i := len(b)
	for n > 0 && i > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

type cellSnap struct {
	ch                rune
	fg, bg            vt10x.Color
	bold, ul, reverse bool
}

func renderShellPane(sess *shell.Session, width, height int, showCursor, blinkOn bool, scrollOffset int) string {
	if sess == nil || sess.Term() == nil || width < 1 || height < 1 {
		if width < 1 {
			width = 1
		}
		return strings.Repeat(" ", width)
	}

	if scrollOffset < 0 {
		scrollOffset = 0
	}

	// Scrolled view: plain ANSI-stripped line history (stable PageUp).
	if scrollOffset > 0 {
		scroll := sess.ScrollbackSnapshot()
		sbLen := len(scroll)
		if scrollOffset > sbLen {
			scrollOffset = sbLen
		}
		end := sbLen - scrollOffset
		if end < 0 {
			end = 0
		}
		start := end - height
		if start < 0 {
			start = 0
		}
		var b strings.Builder
		for row := 0; row < height; row++ {
			idx := start + row
			line := ""
			if idx < end {
				line = scroll[idx]
			}
			runes := []rune(line)
			for x := 0; x < width; x++ {
				ch := ' '
				if x < len(runes) {
					ch = runes[x]
				}
				b.WriteRune(ch)
			}
			if row < height-1 {
				b.WriteByte('\n')
			}
		}
		return b.String()
	}

	// Live view: VT cell grid + blinking cursor.
	term := sess.Term()
	term.Lock()
	cols, rows := term.Size()
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	cur := term.Cursor()
	curVis := term.CursorVisible()
	grid := make([][]cellSnap, rows)
	for y := 0; y < rows; y++ {
		grid[y] = make([]cellSnap, cols)
		for x := 0; x < cols; x++ {
			g := term.Cell(x, y)
			ch := g.Char
			if ch == 0 {
				ch = ' '
			}
			grid[y][x] = cellSnap{
				ch:      ch,
				fg:      g.FG,
				bg:      g.BG,
				bold:    g.Mode&attrBold != 0,
				ul:      g.Mode&attrUnderline != 0,
				reverse: g.Mode&attrReverse != 0,
			}
		}
	}
	term.Unlock()

	var b strings.Builder
	b.Grow(height * (width + 16))
	for row := 0; row < height; row++ {
		if row >= rows {
			b.WriteString(strings.Repeat(" ", width))
		} else {
			for x := 0; x < width; x++ {
				if x >= cols {
					b.WriteByte(' ')
					continue
				}
				c := grid[row][x]
				ch := c.ch
				rev := c.reverse
				if showCursor && blinkOn && curVis && x == cur.X && row == cur.Y {
					rev = !rev
					if ch == ' ' {
						ch = '█'
					}
				}
				st := colorStyle(c.fg, c.bg, c.bold, rev, c.ul)
				b.WriteString(st.Render(string(ch)))
			}
		}
		if row < height-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
