// Package workspace manages per-work state directories.
//
// One workspace = one work (a novel, an episode). Layout:
//
//	<workspace_dir>/<id>/
//	├── script/        agent-authored script JSONL files
//	├── casting.toml   character → voice model mapping
//	├── dict/          registered pronunciation dictionary record
//	├── wav/           per-line synthesized WAV files (<line_id>.wav)
//	├── cache/         synthesis cache index
//	└── master/        mastered outputs + credits + tmp artifacts
package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nlink-jp/voice-studio-mcp/internal/toolerr"
)

// Well-known subdirectories inside a workspace.
const (
	DirScript = "script"
	DirDict   = "dict"
	DirWav    = "wav"
	DirCache  = "cache"
	DirMaster = "master"
)

// Workspace is a validated, materialized per-work directory.
type Workspace struct {
	ID      string
	BaseDir string
}

// Path joins parts under the workspace base directory (no traversal check;
// use ResolveInside for agent-supplied relative paths).
func (w *Workspace) Path(parts ...string) string {
	return filepath.Join(append([]string{w.BaseDir}, parts...)...)
}

// ResolveInside resolves an agent-supplied relative path against the
// workspace base directory and rejects anything that escapes it
// (absolute paths, ".." traversal).
func (w *Workspace) ResolveInside(rel string) (string, error) {
	if rel == "" {
		return "", toolerr.New(toolerr.CodePathNotAllowed, "path must not be empty")
	}
	if filepath.IsAbs(rel) {
		return "", toolerr.Newf(toolerr.CodePathNotAllowed,
			"path %q must be relative to the workspace root", rel)
	}
	cleaned := filepath.Clean(filepath.Join(w.BaseDir, rel))
	if cleaned != w.BaseDir && !strings.HasPrefix(cleaned, w.BaseDir+string(filepath.Separator)) {
		return "", toolerr.Newf(toolerr.CodePathNotAllowed,
			"path %q escapes the workspace root", rel)
	}
	return cleaned, nil
}

// Manager creates, lists, and deletes workspaces under a root directory.
type Manager struct {
	root string
}

// NewManager returns a Manager rooted at dir.
func NewManager(dir string) *Manager {
	return &Manager{root: filepath.Clean(dir)}
}

// Root returns the workspace root directory.
func (m *Manager) Root() string { return m.root }

// Ensure validates id and creates the workspace directory tree (idempotent).
func (m *Manager) Ensure(id string) (*Workspace, error) {
	if err := ValidateID(id); err != nil {
		return nil, err
	}
	w := &Workspace{ID: id, BaseDir: filepath.Join(m.root, id)}
	for _, d := range []string{DirScript, DirDict, DirWav, DirCache, DirMaster} {
		if err := os.MkdirAll(w.Path(d), 0o755); err != nil {
			return nil, toolerr.Newf(toolerr.CodeWorkspaceFailed, "create workspace dir: %v", err)
		}
	}
	return w, nil
}

// List returns the IDs of existing workspaces (sorted).
func (m *Manager) List() ([]string, error) {
	entries, err := os.ReadDir(m.root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, toolerr.Newf(toolerr.CodeWorkspaceFailed, "list workspaces: %v", err)
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() && ValidateID(e.Name()) == nil {
			ids = append(ids, e.Name())
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// Delete removes a workspace and all of its state. Refuses to remove anything
// that is not a direct child of the workspace root (defense in depth on top
// of ValidateID).
func (m *Manager) Delete(id string) error {
	if err := ValidateID(id); err != nil {
		return err
	}
	cleaned := filepath.Clean(filepath.Join(m.root, id))
	if filepath.Dir(cleaned) != m.root {
		return fmt.Errorf("refused to delete: %s is not a direct child of %s", cleaned, m.root)
	}
	if err := os.RemoveAll(cleaned); err != nil {
		return toolerr.Newf(toolerr.CodeWorkspaceFailed, "remove workspace: %v", err)
	}
	return nil
}
