package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Entry is the stdio server line every client gets: this binary, the `mcp`
// subcommand, and the broker it should talk to.
type Entry struct {
	Command string
	Args    []string
	Env     map[string]string
}

// Result is one client's outcome.
type Result struct {
	Client string `json:"client"`
	Name   string `json:"name"`
	Path   string `json:"path"`
	Format string `json:"format"`
	// Status is one of: written, unchanged, removed, absent, not-installed,
	// dry-run, manual.
	Status  string `json:"status"`
	Detail  string `json:"detail,omitempty"`
	Snippet string `json:"snippet,omitempty"`
}

// Failed reports whether this result is something the user must look at.
func (r Result) Failed() bool { return r.Status == "manual" }

// Apply writes, refreshes or removes the entry for one client. force writes the
// file even when the client was not detected, which is what an explicit
// --client selection means.
func Apply(c Client, e Env, entry Entry, name string, remove, dryRun, force bool) Result {
	path := c.Path(e)
	res := Result{Client: c.ID, Name: c.Name, Path: path, Format: string(c.Format)}
	installed := c.Installed(e)
	if !installed && !fileExists(path) && !force {
		res.Status = "not-installed"
		return res
	}
	raw, exists, err := readFile(path)
	if err != nil {
		res.Status, res.Detail = "manual", err.Error()
		return res
	}
	if exists && strings.TrimSpace(raw) == "" {
		raw = "{}"
	}
	key := ServerName(name)
	snippet, err := render(c.Format, key, entry)
	if err != nil {
		res.Status, res.Detail = "manual", err.Error()
		return res
	}
	res.Snippet = snippet
	if dryRun {
		res.Status = "dry-run"
		return res
	}
	if remove && !exists {
		res.Status = "absent"
		return res
	}
	next, err := transform(c.Format, raw, key, snippet, remove)
	if err != nil {
		res.Status, res.Detail = "manual", err.Error()
		return res
	}
	if exists && next == raw {
		res.Status = "unchanged"
		return res
	}
	if !remove && exists {
		backup := path + ".bak"
		if err := os.WriteFile(backup, []byte(raw), 0o644); err != nil {
			res.Status, res.Detail = "manual", "backup failed: "+err.Error()
			return res
		}
		res.Detail = "backup " + backup
	}
	if err := writeFile(path, next); err != nil {
		res.Status, res.Detail = "manual", err.Error()
		return res
	}
	got, _, err := readFile(path)
	if err != nil || hasEntry(c.Format, got, key) == remove {
		res.Status = "manual"
		res.Detail = "wrote " + path + " but the read-back check failed"
		return res
	}
	if remove {
		res.Status = "removed"
	} else {
		res.Status = "written"
	}
	return res
}

// ServerName is the config key used for the server: the flag, sanitized so it
// is a valid TOML bare key, defaulting to "open-board".
func ServerName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "open-board"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

// render returns the exact config fragment this client needs, which is what
// `--dry-run` prints and what transform writes.
func render(f Format, key string, en Entry) (string, error) {
	switch f {
	case FormatTOML:
		return tomlSection(key, en), nil
	case FormatMcpServers, FormatVSCode, FormatZed, FormatOpenCode, FormatOpenClaw:
		obj, err := entryObject(f, en)
		if err != nil {
			return "", err
		}
		var wrapped map[string]any
		if f == FormatOpenClaw {
			// openclaw.json nests servers one level deeper: mcp.servers.
			wrapped = map[string]any{"mcp": map[string]any{"servers": map[string]any{key: obj}}}
		} else {
			wrapped = map[string]any{configKey(f): map[string]any{key: obj}}
		}
		data, err := marshal(wrapped)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	return "", fmt.Errorf("unknown config format %q", f)
}

func entryObject(f Format, en Entry) (map[string]any, error) {
	env := en.Env
	if len(env) == 0 {
		env = nil
	}
	switch f {
	case FormatMcpServers:
		return withEnv(map[string]any{"command": en.Command, "args": en.Args}, env), nil
	case FormatVSCode:
		return withEnv(map[string]any{"type": "stdio", "command": en.Command, "args": en.Args}, env), nil
	case FormatZed:
		return withEnv(map[string]any{"source": "custom", "command": en.Command, "args": en.Args}, env), nil
	case FormatOpenCode:
		cmd := append([]string{en.Command}, en.Args...)
		obj := map[string]any{"type": "local", "command": cmd, "enabled": true}
		if env != nil {
			obj["environment"] = env
		}
		return obj, nil
	case FormatOpenClaw:
		return withEnv(map[string]any{"command": en.Command, "args": en.Args, "transport": "stdio"}, env), nil
	}
	return nil, fmt.Errorf("unknown config format %q", f)
}

func withEnv(obj map[string]any, env map[string]string) map[string]any {
	if len(env) > 0 {
		obj["env"] = env
	}
	return obj
}

func configKey(f Format) string {
	switch f {
	case FormatVSCode:
		return "servers"
	case FormatZed:
		return "context_servers"
	case FormatOpenCode, FormatOpenClaw:
		return "mcp"
	}
	return "mcpServers"
}

// transform merges or removes the entry in one client's file. It never
// rewrites unrelated keys and never touches a file it cannot parse.
func transform(f Format, raw, key, snippet string, remove bool) (string, error) {
	raw = strings.TrimPrefix(raw, "\ufeff") // editors on Windows like to add a BOM
	if f == FormatTOML {
		text := raw
		if strings.TrimSpace(text) == "" {
			text = ""
		}
		if remove {
			return tomlDelete(text, "mcp_servers."+key), nil
		}
		return tomlUpsert(text, "mcp_servers."+key, snippet), nil
	}

	var doc map[string]any
	if strings.TrimSpace(raw) == "" {
		doc = map[string]any{}
	} else if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return "", fmt.Errorf("cannot parse %s: %v (add the snippet manually)", "config", err)
	}
	if doc == nil {
		doc = map[string]any{}
	}
	path := []string{configKey(f)}
	if f == FormatOpenClaw {
		path = []string{"mcp", "servers"}
	}
	if remove {
		container := dig(doc, path...)
		if _, ok := container[key]; !ok {
			return raw, nil // nothing of ours to remove
		}
		delete(container, key)
	} else {
		snippetDoc := map[string]any{}
		if err := json.Unmarshal([]byte(snippet), &snippetDoc); err != nil {
			return "", err
		}
		snippetContainer := dig(snippetDoc, path...)
		if snippetContainer == nil || snippetContainer[key] == nil {
			return "", errors.New("internal: snippet shape mismatch")
		}
		ensureMap(doc, path...)[key] = snippetContainer[key]
	}
	data, err := marshal(doc)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func ensureMap(doc map[string]any, path ...string) map[string]any {
	cur := doc
	for _, p := range path {
		child, ok := cur[p].(map[string]any)
		if !ok {
			child = map[string]any{}
			cur[p] = child
		}
		cur = child
	}
	return cur
}

func dig(doc map[string]any, path ...string) map[string]any {
	cur := doc
	for _, p := range path {
		next, ok := cur[p].(map[string]any)
		if !ok {
			return nil
		}
		cur = next
	}
	return cur
}

// hasEntry reads back a written file and confirms the server key exists.
func hasEntry(f Format, raw, key string) bool {
	if strings.TrimSpace(raw) == "" {
		return false
	}
	if f == FormatTOML {
		return strings.Contains(raw, "[mcp_servers."+key+"]")
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return false
	}
	path := []string{configKey(f)}
	if f == FormatOpenClaw {
		path = []string{"mcp", "servers"}
	}
	return dig(doc, path...) != nil && dig(doc, path...)[key] != nil
}

func marshal(v any) ([]byte, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func readFile(path string) (string, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return string(data), true, nil
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// EnvPairs renders an env map as sorted KEY=VALUE strings, for terminal output.
func EnvPairs(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+env[k])
	}
	return out
}
