package config

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/vorzela/vorzela-migrate/v2/internal/output"
)

// canonicalConfigKeys maps a tolerant spelling of a config key (lowercase,
// without underscores or dashes) to its canonical uppercase key. This lets
// vm.yaml accept database_url, DATABASE_URL and databaseUrl while keeping one
// internal representation.
var canonicalConfigKeys = map[string]string{
	"databaseurl":       "DATABASE_URL",
	"migrationpath":     "MIGRATION_PATH",
	"sqlcsupport":       "SQLC_SUPPORT",
	"environment":       "ENVIRONMENT",
	"env":               "ENVIRONMENT",
	"enhanced":          "ENHANCED",
	"online":            "ONLINE",
	"verifychecksums":   "VERIFY_CHECKSUMS",
	"detectdrift":       "DETECT_DRIFT",
	"verbose":           "VERBOSE",
	"strict":            "STRICT",
	"autorunextensions": "AUTO_RUN_EXTENSIONS",
	"autorunfunctions":  "AUTO_RUN_FUNCTIONS",
	"autorunenums":      "AUTO_RUN_ENUMS",
	"drifthandling":     "DRIFT_HANDLING",
}

// canonicalConfigKey returns the canonical uppercase key for raw, if raw is a
// recognised key in any of the accepted spellings.
func canonicalConfigKey(raw string) (string, bool) {
	compact := strings.ToLower(strings.TrimSpace(raw))
	compact = strings.NewReplacer("_", "", "-", "").Replace(compact)
	key, ok := canonicalConfigKeys[compact]
	return key, ok
}

// preferredYAMLKey returns the snake_case spelling suggested to users.
func preferredYAMLKey(canonical string) string {
	return strings.ToLower(canonical)
}

// QuoteYAMLValue returns a single-line YAML scalar. Config values are DSNs,
// paths and enum names; quoting keeps characters such as # and : from changing
// the meaning of the file.
func QuoteYAMLValue(value string) string {
	if strings.ContainsAny(value, "\r\n\t") {
		replacer := strings.NewReplacer(
			`\`, `\\`,
			`"`, `\"`,
			"\n", `\n`,
			"\r", `\r`,
			"\t", `\t`,
		)
		return `"` + replacer.Replace(value) + `"`
	}
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// yamlEntry is one key/value pair from a YAML config file. Key is empty when
// the key is not recognised.
type yamlEntry struct {
	RawKey string
	Key    string
	Value  string
	Line   int
}

var dotenvLinePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\s*=`)

// looksLikeDotenv reports whether every meaningful line in content is a
// KEY=VALUE assignment. This detects v2.4.0-era vm.yaml files that used dotenv
// syntax despite the .yaml extension.
func looksLikeDotenv(content []byte) bool {
	lines := 0
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		lines++
		if !dotenvLinePattern.MatchString(trimmed) {
			return false
		}
	}
	return lines > 0
}

// parseYAMLConfig parses a flat YAML mapping into config entries. Comments are
// ignored by the YAML parser and line numbers refer to the key line.
func parseYAMLConfig(content []byte) ([]yamlEntry, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}

	root := doc.Content[0]
	if root.Kind == yaml.ScalarNode && root.Tag == "!!null" {
		return nil, nil
	}
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("top-level YAML must be a mapping of key: value pairs")
	}

	entries := make([]yamlEntry, 0, len(root.Content)/2)
	for i := 0; i+1 < len(root.Content); i += 2 {
		keyNode := root.Content[i]
		valueNode := root.Content[i+1]

		if keyNode.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("line %d: mapping keys must be plain names", keyNode.Line)
		}

		value, err := yamlScalarValue(valueNode)
		if err != nil {
			return nil, fmt.Errorf("line %d: %s: %w", keyNode.Line, keyNode.Value, err)
		}

		canonical, known := canonicalConfigKey(keyNode.Value)
		if !known {
			canonical = ""
		}
		entries = append(entries, yamlEntry{
			RawKey: keyNode.Value,
			Key:    canonical,
			Value:  value,
			Line:   keyNode.Line,
		})
	}
	return entries, nil
}

// yamlScalarValue flattens a scalar node to the string form used by the
// existing config setters. Lists, maps and other structures are rejected so a
// typo produces a clear error instead of being silently ignored.
func yamlScalarValue(node *yaml.Node) (string, error) {
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Tag == "!!null" {
			return "", nil
		}
		return node.Value, nil
	case yaml.AliasNode:
		return yamlScalarValue(node.Alias)
	default:
		return "", fmt.Errorf("value must be a string, number or boolean")
	}
}

// applyYAMLEntries applies parsed YAML entries and warns about unknown or
// duplicate keys. Unknown keys are skipped so a typo cannot change behaviour
// silently; vm lint reports them as errors.
func applyYAMLEntries(base string, entries []yamlEntry, cfg *Config) {
	seen := make(map[string]int)
	for _, entry := range entries {
		if entry.Key == "" {
			output.WarningStderr("%s line %d: unknown key '%s' — run 'vm lint' for details", base, entry.Line, entry.RawKey)
			continue
		}
		if firstLine, duplicate := seen[entry.Key]; duplicate {
			output.WarningStderr("%s line %d: duplicate key '%s' (first defined on line %d)", base, entry.Line, entry.RawKey, firstLine)
		} else {
			seen[entry.Key] = entry.Line
		}
		applyConfigValue(cfg, entry.Key, entry.Value)
	}
}
