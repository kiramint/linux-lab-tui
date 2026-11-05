package grade

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Bank struct {
	Version          int        `yaml:"version" json:"version"`
	Title            string     `yaml:"title" json:"title"`
	WorkspacePrefix  string     `yaml:"workspace_prefix" json:"workspace_prefix"`
	Questions        []Question `yaml:"questions" json:"questions"`
}

type Question struct {
	ID           string   `yaml:"id" json:"id"`
	Title        string   `yaml:"title" json:"title"`
	Section      string   `yaml:"section" json:"section"`
	NeedsSudo    bool     `yaml:"needs_sudo" json:"needs_sudo"`
	Instructions string   `yaml:"instructions" json:"instructions"`
	Graders      []Grader `yaml:"graders" json:"graders"`
}

// Grader is one of: file | command_regex | output
type Grader struct {
	Type string `yaml:"type" json:"type"`

	// file
	Path          string `yaml:"path,omitempty" json:"path,omitempty"`
	Exists        *bool  `yaml:"exists,omitempty" json:"exists,omitempty"`
	IsDir         bool   `yaml:"is_dir,omitempty" json:"is_dir,omitempty"`
	ContentRegex  string `yaml:"content_regex,omitempty" json:"content_regex,omitempty"`
	ModeOctal     string `yaml:"mode_octal,omitempty" json:"mode_octal,omitempty"`
	ModeRegex     string `yaml:"mode_regex,omitempty" json:"mode_regex,omitempty"`

	// command_regex
	Pattern     string `yaml:"pattern,omitempty" json:"pattern,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// output
	Command       string `yaml:"command,omitempty" json:"command,omitempty"`
	CwdWorkspace  bool   `yaml:"cwd_workspace,omitempty" json:"cwd_workspace,omitempty"`
	StdoutRegex   string `yaml:"stdout_regex,omitempty" json:"stdout_regex,omitempty"`
	StdoutExact   string `yaml:"stdout_exact,omitempty" json:"stdout_exact,omitempty"`
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
