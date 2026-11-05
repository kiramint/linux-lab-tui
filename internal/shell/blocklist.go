package shell

import (
	"regexp"
	"strings"
)

// Dangerous patterns blocked before a line is sent to the PTY.
// Matching is case-insensitive. Patterns also catch the tokens as arguments.
var blockPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bdd\b`),
	regexp.MustCompile(`(?i)--no-preserve-root`),
	regexp.MustCompile(`(?i)\bfdisk\b`),
	regexp.MustCompile(`(?i)\bcfdisk\b`),
	regexp.MustCompile(`(?i)\bparted\b`),
	// rm -rf / or rm -rf /* with optional flags; also combo with no-preserve-root already covered
	regexp.MustCompile(`(?i)\brm\s+(-[a-zA-Z]*r[a-zA-Z]*f[a-zA-Z]*|-[a-zA-Z]*f[a-zA-Z]*r[a-zA-Z]*)\s+/\s*($|\*)`),
	regexp.MustCompile(`(?i)\brm\s+(-[a-zA-Z]+\s+)*/\s*($|\*)`),
}

// BlockReason returns a human-readable reason if cmdLine should be blocked.
// Returns empty string if the line is allowed.
func BlockReason(cmdLine string) string {
	line := strings.TrimSpace(cmdLine)
	if line == "" {
		return ""
	}
	// Ignore pure comments
	if strings.HasPrefix(line, "#") {
		return ""
	}
	for _, re := range blockPatterns {
		if re.MatchString(line) {
			return "blocked dangerous command: " + summarizeMatch(line, re)
		}
	}
	return ""
}

func summarizeMatch(line string, re *regexp.Regexp) string {
	loc := re.FindStringIndex(line)
	if loc == nil {
		return "safety policy"
	}
	return strings.TrimSpace(line[loc[0]:loc[1]])
}

// IsBlocked reports whether the command line is forbidden.
func IsBlocked(cmdLine string) bool {
	return BlockReason(cmdLine) != ""
}
