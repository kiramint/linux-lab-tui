package grade

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Bank struct {
	Version         int        `yaml:"version" json:"version"`
	Title           string     `yaml:"title" json:"title"`
	WorkspacePrefix string     `yaml:"workspace_prefix" json:"workspace_prefix"`
	Questions       []Question `yaml:"questions" json:"questions"`
}

type Question struct {
	ID           string   `yaml:"id" json:"id"`
	Title        string   `yaml:"title" json:"title"`
	Section      string   `yaml:"section" json:"section"`
	NeedsSudo    bool     `yaml:"needs_sudo" json:"needs_sudo"`
	Bonus        bool     `yaml:"bonus" json:"bonus"`
	Instructions string   `yaml:"instructions" json:"instructions"`
	Init         string   `yaml:"init" json:"init"`
	// InitOnce defaults to true when omitted (nil).
	InitOnce     *bool    `yaml:"init_once" json:"init_once"`
	AnswerPrompt string   `yaml:"answer_prompt" json:"answer_prompt"`
	Graders      []Grader `yaml:"graders" json:"graders"`
}

// ShouldInitOnce reports whether init should run at most once per question ID.
func (q Question) ShouldInitOnce() bool {
	if q.InitOnce == nil {
		return true
	}
	return *q.InitOnce
}

// HasShortAnswer is true when the question expects a typed answer.
func (q Question) HasShortAnswer() bool {
	if strings.TrimSpace(q.AnswerPrompt) != "" {
		return true
	}
	for _, g := range q.Graders {
		if strings.EqualFold(g.Type, "short_answer") {
			return true
		}
	}
	return false
}

// FormalCount returns the number of non-bonus questions.
func (b *Bank) FormalCount() int {
	n := 0
	for _, q := range b.Questions {
		if !q.Bonus {
			n++
		}
	}
	return n
}

// Grader is one of: file | command_regex | output | short_answer
type Grader struct {
	Type string `yaml:"type" json:"type"`

	// file
	Path         string `yaml:"path,omitempty" json:"path,omitempty"`
	Exists       *bool  `yaml:"exists,omitempty" json:"exists,omitempty"`
	IsDir        bool   `yaml:"is_dir,omitempty" json:"is_dir,omitempty"`
	ContentRegex string `yaml:"content_regex,omitempty" json:"content_regex,omitempty"`
	ModeOctal    string `yaml:"mode_octal,omitempty" json:"mode_octal,omitempty"`
	ModeRegex    string `yaml:"mode_regex,omitempty" json:"mode_regex,omitempty"`

	// command_regex / shared description
	Pattern     string `yaml:"pattern,omitempty" json:"pattern,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// output
	Command      string `yaml:"command,omitempty" json:"command,omitempty"`
	CwdWorkspace bool   `yaml:"cwd_workspace,omitempty" json:"cwd_workspace,omitempty"`
	StdoutRegex  string `yaml:"stdout_regex,omitempty" json:"stdout_regex,omitempty"`
	StdoutExact  string `yaml:"stdout_exact,omitempty" json:"stdout_exact,omitempty"`

	// short_answer
	ExpectExact   string `yaml:"expect_exact,omitempty" json:"expect_exact,omitempty"`
	ExpectRegex   string `yaml:"expect_regex,omitempty" json:"expect_regex,omitempty"`
	ExpectCommand string `yaml:"expect_command,omitempty" json:"expect_command,omitempty"`
	Normalize     string `yaml:"normalize,omitempty" json:"normalize,omitempty"`
}

func LoadBank(path string) (*Bank, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseBank(data)
}

func ParseBank(data []byte) (*Bank, error) {
	var b Bank
	if err := yaml.Unmarshal(data, &b); err != nil {
		return nil, err
	}
	if len(b.Questions) == 0 {
		return nil, fmt.Errorf("quest bank has no questions")
	}
	if b.WorkspacePrefix == "" {
		b.WorkspacePrefix = "/tmp/linux-lab"
	}
	return &b, nil
}
