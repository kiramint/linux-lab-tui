package shell

import (
	"strings"
	"testing"

	"github.com/hinshun/vt10x"
)

func TestVTClearScreen(t *testing.T) {
	term := vt10x.New(vt10x.WithSize(40, 10))
	_, _ = term.Write([]byte("hello world\r\nline2\r\n"))
	before := term.String()
	if !strings.Contains(before, "hello") {
		t.Fatalf("expected content before clear, got %q", before)
	}
	_, _ = term.Write([]byte("\x1b[H\x1b[2J"))
	after := term.String()
	if strings.Contains(after, "hello") {
		t.Fatalf("clear should remove content, got %q", after)
	}
}

func TestVTWrapAndScroll(t *testing.T) {
	term := vt10x.New(vt10x.WithSize(10, 4))
	_, _ = term.Write([]byte("abcdefghijABCDEFGHIJ"))
	s := term.String()
	lines := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected wrapped lines, got %q", s)
	}
	if strings.TrimRight(lines[0], " ") != "abcdefghij" {
		t.Fatalf("line0=%q", lines[0])
	}
	if strings.TrimRight(lines[1], " ") != "ABCDEFGHIJ" {
		t.Fatalf("line1=%q", lines[1])
	}
	_, _ = term.Write([]byte("\r\n1111111111\r\n2222222222\r\n3333333333\r\n"))
	s2 := term.String()
	if strings.Contains(s2, "abcdefghij") {
		t.Fatalf("first line should have scrolled off visible screen: %q", s2)
	}
}

func TestScrollbackFromOutput(t *testing.T) {
	s := &Session{}
	s.noteScrollback([]byte("one\r\ntwo\r\n"))
	s.noteScrollback([]byte("\x1b[31mred\x1b[0m\r\n"))
	if s.ScrollbackLen() != 3 {
		t.Fatalf("len=%d lines=%v", s.ScrollbackLen(), s.ScrollbackSnapshot())
	}
	if s.ScrollbackLine(2) != "red" {
		t.Fatalf("ansi strip failed: %q", s.ScrollbackLine(2))
	}
}

func TestStripANSI(t *testing.T) {
	got := string(stripANSI([]byte("a\x1b[1;32mb\x1b[0mc")))
	if got != "abc" {
		t.Fatalf("got %q", got)
	}
}

func TestBlocklistStillWorks(t *testing.T) {
	if !IsBlocked("dd if=/dev/zero of=/dev/sda") {
		t.Fatal("dd should be blocked")
	}
	if !IsBlocked("sudo fdisk -l") {
		t.Fatal("fdisk should be blocked")
	}
	if !IsBlocked("parted /dev/sda") {
		t.Fatal("parted should be blocked")
	}
	if !IsBlocked("rm -rf / --no-preserve-root") {
		t.Fatal("no-preserve-root should be blocked")
	}
	if IsBlocked("ls -la") {
		t.Fatal("ls should be allowed")
	}
}
