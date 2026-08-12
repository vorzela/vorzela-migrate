package generate

import (
	"os"
	"path"
	"path/filepath"
	"strings"
)

// detectModelImport returns the import path of the models directory, derived
// from the module path in ./go.mod. Without a go.mod the directory name is the
// best guess available.
func detectModelImport(modelDir string) string {
	rel := path.Clean(filepath.ToSlash(strings.TrimSpace(modelDir)))
	rel = strings.TrimPrefix(rel, "./")
	if rel == "" || rel == "." || strings.HasPrefix(rel, "..") || path.IsAbs(rel) {
		rel = DefaultModelDir
		rel = strings.TrimPrefix(path.Clean(rel), "./")
	}

	data, err := os.ReadFile("go.mod")
	if err != nil {
		return rel
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "module "); ok {
			if mod := strings.TrimSpace(after); mod != "" {
				return mod + "/" + rel
			}
		}
	}
	return rel
}
