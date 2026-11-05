package progress

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	ConfigDirName   = "linux-lab"
	ProgressFile    = "progress.json"
	SubmissionsDir  = "submissions"
)

type QuestionResult struct {
	ID        string    `json:"id"`
	Passed    bool      `json:"passed"`
	CheckedAt time.Time `json:"checked_at,omitempty"`
	Details   []string  `json:"details,omitempty"`
}

type State struct {
	SessionID     string                    `json:"session_id"`
	Workspace     string                    `json:"workspace"`
	CurrentIndex  int                       `json:"current_index"`
	Results       map[string]QuestionResult `json:"results"`
	StartedAt     time.Time                 `json:"started_at"`
	UpdatedAt     time.Time                 `json:"updated_at"`
	Submitted     bool                      `json:"submitted"`
	SubmitPath    string                    `json:"submit_path,omitempty"`
	HistoryPath   string                    `json:"history_path,omitempty"`
}

func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".config", ConfigDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func ProgressPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ProgressFile), nil
}

func SubmissionsPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	sub := filepath.Join(dir, SubmissionsDir)
	if err := os.MkdirAll(sub, 0o755); err != nil {
		return "", err
	}
	return sub, nil
}

func Load() (*State, error) {
	path, err := ProgressPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s.Results == nil {
		s.Results = map[string]QuestionResult{}
	}
	return &s, nil
}

func (s *State) Save() error {
	path, err := ProgressPath()
	if err != nil {
		return err
	}
	s.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func NewState(sessionID, workspace string) *State {
	now := time.Now()
	return &State{
		SessionID:    sessionID,
		Workspace:    workspace,
		CurrentIndex: 0,
		Results:      map[string]QuestionResult{},
		StartedAt:    now,
		UpdatedAt:    now,
	}
}

func (s *State) PassedCount() int {
	n := 0
	for _, r := range s.Results {
		if r.Passed {
			n++
		}
	}
	return n
}
