package generate

import (
	"os"
	"strings"
)

// detectModelImport returns "<module>/models" from ./go.mod when present.
func detectModelImport() string {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return "models"
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "module "); ok {
			mod := strings.TrimSpace(after)
			if mod != "" {
				return mod + "/models"
			}
		}
	}
	return "models"
}
