package install

import (
	"fmt"
	"strings"
)

// tomlSection renders the whole [mcp_servers.<key>] section, which is the
// snippet Codex CLI and Grok CLI print and what tomlUpsert writes.
func tomlSection(key string, en Entry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[mcp_servers.%s]\n", key)
	fmt.Fprintf(&b, "command = %s\n", tomlString(en.Command))
	args := make([]string, 0, len(en.Args))
	for _, a := range en.Args {
		args = append(args, tomlString(a))
	}
	fmt.Fprintf(&b, "args = [%s]\n", strings.Join(args, ", "))
	if len(en.Env) > 0 {
		pairs := make([]string, 0, len(en.Env))
		for _, pair := range EnvPairs(en.Env) {
			k, v, _ := strings.Cut(pair, "=")
			pairs = append(pairs, fmt.Sprintf("%s = %s", k, tomlString(v)))
		}
		fmt.Fprintf(&b, "env = { %s }\n", strings.Join(pairs, ", "))
	}
	return b.String()
}

// tomlString quotes a TOML basic string. Go's %q escapes are a valid subset.
func tomlString(s string) string { return fmt.Sprintf("%q", s) }

// tomlUpsert replaces the [table] section with section, or appends it. Every
// other line in the file is left exactly as it was.
func tomlUpsert(text, table, section string) string {
	lines := splitLines(text)
	start, end := findSection(lines, table)
	block := splitLines(strings.TrimRight(section, "\n"))
	if start < 0 {
		for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, block...)
		return joinLines(lines)
	}
	next := append([]string{}, lines[:start]...)
	next = append(next, block...)
	next = append(next, lines[end:]...)
	return joinLines(next)
}

// tomlDelete removes the [table] section and the blank line that separated it.
func tomlDelete(text, table string) string {
	lines := splitLines(text)
	start, end := findSection(lines, table)
	if start < 0 {
		return joinLines(lines)
	}
	next := append([]string{}, lines[:start]...)
	next = append(next, lines[end:]...)
	// Collapse the hole the section left behind.
	for i := 0; i+1 < len(next); i++ {
		if strings.TrimSpace(next[i]) == "" && strings.TrimSpace(next[i+1]) == "" {
			next = append(next[:i], next[i+1:]...)
			i--
		}
	}
	for len(next) > 0 && strings.TrimSpace(next[len(next)-1]) == "" {
		next = next[:len(next)-1]
	}
	return joinLines(next)
}

// findSection returns the [start,end) line range of a section, end exclusive.
func findSection(lines []string, table string) (int, int) {
	header := "[" + table + "]"
	for i, line := range lines {
		if strings.TrimSpace(line) != header {
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			if isTableHeader(lines[j]) {
				return i, j
			}
		}
		return i, len(lines)
	}
	return -1, -1
}

func isTableHeader(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "[") && !strings.HasPrefix(t, "[[") && !strings.HasPrefix(t, "#")
}

func splitLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if text == "" {
		return nil
	}
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return []string{""}
	}
	return strings.Split(text, "\n")
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}
