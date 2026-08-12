package migrate

import "strings"

// ExtractUp returns the Up half of a migration file: everything between the Up
// marker and the Down marker, with comment-only lines removed. An empty string
// means the file has no Up section.
func ExtractUp(content string) string {
	return extractSection(content, true)
}

// ExtractDown returns the Down half of a migration file: everything from the
// Down marker to the end of the file, with comment-only lines removed.
func ExtractDown(content string) string {
	return extractSection(content, false)
}

func extractSection(content string, up bool) string {
	var (
		inSection bool
		body      []string
	)
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if up {
			if isUpMarker(trimmed) {
				inSection = true
				continue
			}
			if inSection && isDownMarker(trimmed) {
				break
			}
		} else {
			if isDownMarker(trimmed) {
				inSection = true
				continue
			}
		}
		if inSection && !strings.HasPrefix(trimmed, "--") {
			body = append(body, line)
		}
	}
	return strings.TrimSpace(strings.Join(body, "\n"))
}

// isUpMarker recognises the Up header in every layout vm accepts: the arrow
// style it scaffolds, goose, golang-migrate, and a plain "-- Up" line.
func isUpMarker(line string) bool {
	return strings.Contains(line, "⬆") ||
		strings.Contains(line, "+goose Up") ||
		strings.Contains(line, "migrate:up") ||
		isBareMarker(line, "up")
}

// isDownMarker is isUpMarker for the rollback half.
func isDownMarker(line string) bool {
	return strings.Contains(line, "⬇") ||
		strings.Contains(line, "+goose Down") ||
		strings.Contains(line, "migrate:down") ||
		isBareMarker(line, "down")
}

// isBareMarker matches a line that is nothing but the word, optionally behind a
// single SQL comment prefix: "-- Up", "--Up", "UP".
func isBareMarker(line, word string) bool {
	stripped := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "--"))
	return strings.EqualFold(stripped, word)
}
