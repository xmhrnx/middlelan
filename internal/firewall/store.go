package firewall

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// fileName is the rules file inside the data directory.
const fileName = "firewall.json"

// state is what firewall.json holds.
type state struct {
	// Enabled is true when the user has turned the firewall on.
	Enabled bool `json:"enabled"`
	// Installed is true while rules may exist on the system: it is set
	// before the first apply and cleared only after a successful clear, so
	// a crash in between is cleaned up on the next Refresh.
	Installed bool `json:"installed"`
	// Rules are the user's rules.
	Rules []Rule `json:"rules"`
}

// clone returns a deep copy of s.
func (s state) clone() state {
	s.Rules = append([]Rule(nil), s.Rules...)
	return s
}

// loadState reads path. A missing file is a fresh, disabled firewall.
func loadState(path string) (state, error) {
	var s state
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return state{}, err
	}
	return s, nil
}

// saveState writes s to path atomically.
func saveState(path string, s state) error {
	if s.Rules == nil {
		s.Rules = []Rule{}
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
