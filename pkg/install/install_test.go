package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func testEnv(t *testing.T) Env {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	app := filepath.Join(root, "appdata")
	for _, d := range []string{home, app} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return Env{GOOS: "linux", Home: home, AppData: app, LocalAppData: app}
}

func entry() Entry {
	return Entry{
		Command: "/opt/board/vb",
		Args:    []string{"mcp"},
		Env:     map[string]string{"VB_URL": "http://127.0.0.1:4100", "VB_CHANNEL": "default"},
	}
}

// marker makes a client look installed without touching its config file.
func marker(t *testing.T, c Client, e Env) {
	t.Helper()
	p := filepath.Dir(c.Path(e))
	if c.Markers != nil {
		p = c.Markers[0](e)
	}
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestClientsAreWellFormed(t *testing.T) {
	seen := map[string]bool{}
	formats := map[Format]bool{
		FormatMcpServers: true, FormatVSCode: true, FormatZed: true,
		FormatOpenCode: true, FormatOpenClaw: true, FormatTOML: true,
	}
	envs := []Env{
		{GOOS: "windows", Home: `C:\Users\a`, AppData: `C:\Users\a\AppData\Roaming`, LocalAppData: `C:\Users\a\AppData\Local`},
		{GOOS: "darwin", Home: "/Users/a", AppData: "/Users/a/.config", LocalAppData: "/Users/a/.config"},
		{GOOS: "linux", Home: "/home/a", AppData: "/home/a/.config", LocalAppData: "/home/a/.config"},
	}
	clients := Clients()
	if len(clients) < 12 {
		t.Fatalf("expected a broad client list, got %d", len(clients))
	}
	for _, c := range clients {
		if c.ID == "" || c.Name == "" {
			t.Fatalf("client missing id or name: %+v", c)
		}
		if seen[c.ID] {
			t.Fatalf("duplicate client id %q", c.ID)
		}
		seen[c.ID] = true
		if c.ID != strings.ToLower(c.ID) {
			t.Fatalf("id %q must be lowercase", c.ID)
		}
		if !formats[c.Format] {
			t.Fatalf("%s: unknown format %q", c.ID, c.Format)
		}
		for _, e := range envs {
			p := c.Path(e)
			if e.GOOS != runtime.GOOS {
				continue // filepath joins with host separators; only check placeholders
			}
			if !filepath.IsAbs(p) {
				t.Fatalf("%s on %s: path %q is not absolute", c.ID, e.GOOS, p)
			}
			if strings.Contains(p, "~") || strings.Contains(p, "%") {
				t.Fatalf("%s on %s: path %q still has a placeholder", c.ID, e.GOOS, p)
			}
		}
	}
	for _, id := range []string{"claude-desktop", "claude-code", "cursor", "vscode", "zed", "codex", "gemini", "antigravity", "opencode", "openclaw", "grok", "cline", "windsurf"} {
		if _, ok := Lookup(id); !ok {
			t.Fatalf("client %q is missing", id)
		}
	}
}

func TestApplyJSONShapesPreserveOtherKeys(t *testing.T) {
	cases := []struct {
		id     string
		nested []string
	}{
		{"claude-desktop", []string{"mcpServers"}},
		{"cursor", []string{"mcpServers"}},
		{"windsurf", []string{"mcpServers"}},
		{"gemini", []string{"mcpServers"}},
		{"antigravity", []string{"mcpServers"}},
		{"claude-code", []string{"mcpServers"}},
		{"cline", []string{"mcpServers"}},
		{"zed", []string{"context_servers"}},
		{"opencode", []string{"mcp"}},
		{"openclaw", []string{"mcp", "servers"}},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			e := testEnv(t)
			c, ok := Lookup(tc.id)
			if !ok {
				t.Fatalf("no client %q", tc.id)
			}
			marker(t, c, e)
			path := c.Path(e)
			if err := writeFile(path, `{"unrelated": {"keep": true}}`); err != nil {
				t.Fatal(err)
			}
			res := Apply(c, e, entry(), "open-board", false, false, false)
			if res.Status != "written" {
				t.Fatalf("%s: status %q detail %q", tc.id, res.Status, res.Detail)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var doc map[string]any
			if err := json.Unmarshal(raw, &doc); err != nil {
				t.Fatalf("%s: written file is not JSON: %v", tc.id, err)
			}
			if doc["unrelated"] == nil {
				t.Fatalf("%s: unrelated keys were dropped", tc.id)
			}
			obj := dig(doc, tc.nested...)
			if obj == nil || obj["open-board"] == nil {
				t.Fatalf("%s: entry missing at %v: %s", tc.id, tc.nested, raw)
			}
			server, _ := obj["open-board"].(map[string]any)
			if c.Format == FormatOpenCode {
				cmd, _ := server["command"].([]any)
				if len(cmd) != 2 || cmd[0] != "/opt/board/vb" || cmd[1] != "mcp" {
					t.Fatalf("%s: opencode command must be argv: %#v", tc.id, server["command"])
				}
				if server["type"] != "local" {
					t.Fatalf("%s: opencode entry needs type local", tc.id)
				}
			} else {
				if server["command"] != "/opt/board/vb" {
					t.Fatalf("%s: command = %#v", tc.id, server["command"])
				}
				args, _ := server["args"].([]any)
				if len(args) != 1 || args[0] != "mcp" {
					t.Fatalf("%s: args = %#v", tc.id, server["args"])
				}
			}
			if c.Format == FormatVSCode && server["type"] != "stdio" {
				t.Fatalf("vscode entry needs type stdio: %#v", server)
			}
			if c.Format == FormatZed && server["source"] != "custom" {
				t.Fatalf("zed entry needs source custom: %#v", server)
			}
			envKey := "env"
			if c.Format == FormatOpenCode {
				envKey = "environment"
			}
			env, _ := server[envKey].(map[string]any)
			if env["VB_CHANNEL"] != "default" {
				t.Fatalf("%s: %s = %#v", tc.id, envKey, server[envKey])
			}
			if _, err := os.Stat(path + ".bak"); err != nil {
				t.Fatalf("%s: no backup written: %v", tc.id, err)
			}
			// A second run is a no-op.
			if res := Apply(c, e, entry(), "open-board", false, false, false); res.Status != "unchanged" {
				t.Fatalf("%s: second run status %q", tc.id, res.Status)
			}
			// Removing leaves the unrelated keys and no entry.
			if res := Apply(c, e, entry(), "open-board", true, false, false); res.Status != "removed" {
				t.Fatalf("%s: remove status %q detail %q", tc.id, res.Status, res.Detail)
			}
			raw, _ = os.ReadFile(path)
			if strings.Contains(string(raw), "open-board") {
				t.Fatalf("%s: entry survived removal: %s", tc.id, raw)
			}
			if !strings.Contains(string(raw), "unrelated") {
				t.Fatalf("%s: removal dropped unrelated keys: %s", tc.id, raw)
			}
		})
	}
}

func TestApplyTOMLKeepsOtherSections(t *testing.T) {
	e := testEnv(t)
	c, _ := Lookup("codex")
	marker(t, c, e)
	path := c.Path(e)
	original := "model = \"gpt-5-codex\"\n\n[profiles.fast]\nmodel = \"o4-mini\"\n\n[mcp_servers.other]\ncommand = \"other\"\n"
	if err := writeFile(path, original); err != nil {
		t.Fatal(err)
	}
	res := Apply(c, e, entry(), "open-board", false, false, false)
	if res.Status != "written" {
		t.Fatalf("status %q detail %q", res.Status, res.Detail)
	}
	raw, _ := os.ReadFile(path)
	text := string(raw)
	for _, want := range []string{
		`model = "gpt-5-codex"`,
		"[profiles.fast]",
		"[mcp_servers.other]",
		"[mcp_servers.open-board]",
		`command = "/opt/board/vb"`,
		`args = ["mcp"]`,
		"VB_URL",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	if res := Apply(c, e, entry(), "open-board", false, false, false); res.Status != "unchanged" {
		t.Fatalf("second run status %q", res.Status)
	}
	if res := Apply(c, e, entry(), "open-board", true, false, false); res.Status != "removed" {
		t.Fatalf("remove status %q detail %q", res.Status, res.Detail)
	}
	raw, _ = os.ReadFile(path)
	if strings.Contains(string(raw), "open-board") {
		t.Fatalf("section survived removal:\n%s", raw)
	}
	if !strings.Contains(string(raw), "[mcp_servers.other]") || !strings.Contains(string(raw), "[profiles.fast]") {
		t.Fatalf("removal damaged other sections:\n%s", raw)
	}
}

func TestDryRunAndUndetectedClients(t *testing.T) {
	e := testEnv(t)
	c, _ := Lookup("cursor")
	res := Apply(c, e, entry(), "open-board", false, false, false)
	if res.Status != "not-installed" {
		t.Fatalf("undetected client status %q", res.Status)
	}
	if _, err := os.Stat(c.Path(e)); err == nil {
		t.Fatal("a file was written for an undetected client")
	}
	// An explicit selection forces the write even when nothing is installed.
	if res := Apply(c, e, entry(), "open-board", false, false, true); res.Status != "written" {
		t.Fatalf("forced install status %q detail %q", res.Status, res.Detail)
	}

	c, _ = Lookup("windsurf")
	marker(t, c, e)
	res = Apply(c, e, entry(), "open-board", false, true, false)
	if res.Status != "dry-run" || !strings.Contains(res.Snippet, "/opt/board/vb") {
		t.Fatalf("dry-run gave %q snippet %q", res.Status, res.Snippet)
	}
	if _, err := os.Stat(c.Path(e)); err == nil {
		t.Fatal("dry-run wrote a file")
	}
}

func TestUnparsableConfigIsLeftAlone(t *testing.T) {
	e := testEnv(t)
	c, _ := Lookup("openclaw")
	marker(t, c, e)
	path := c.Path(e)
	// JSON5 with comments is valid for OpenClaw but not for encoding/json.
	original := "{\n  // my gateway\n  mcp: { servers: {} },\n}\n"
	if err := writeFile(path, original); err != nil {
		t.Fatal(err)
	}
	res := Apply(c, e, entry(), "open-board", false, false, false)
	if res.Status != "manual" {
		t.Fatalf("status %q, want manual", res.Status)
	}
	if res.Snippet == "" {
		t.Fatal("manual result must still show the snippet")
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != original {
		t.Fatalf("file changed on parse failure:\n%s", raw)
	}
}

func TestBOMConfigIsParsedAndRewritten(t *testing.T) {
	e := testEnv(t)
	c, _ := Lookup("claude-desktop")
	marker(t, c, e)
	path := c.Path(e)
	if err := writeFile(path, "\ufeff"+`{"mcpServers": {"other": {"command": "x"}}}`); err != nil {
		t.Fatal(err)
	}
	if res := Apply(c, e, entry(), "open-board", false, false, false); res.Status != "written" {
		t.Fatalf("status %q detail %q", res.Status, res.Detail)
	}
	raw, _ := os.ReadFile(path)
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("file is not clean JSON after rewrite: %v", err)
	}
	servers, _ := doc["mcpServers"].(map[string]any)
	if servers["other"] == nil || servers["open-board"] == nil {
		t.Fatalf("entries lost: %s", raw)
	}
}

func TestServerNameIsSafe(t *testing.T) {
	cases := map[string]string{
		"":              "open-board",
		"open board":    "open-board",
		"my.server":     "my-server",
		"Board_2":       "Board_2",
		`bad"quote key`: "bad-quote-key",
	}
	for in, want := range cases {
		if got := ServerName(in); got != want {
			t.Fatalf("ServerName(%q) = %q, want %q", in, got, want)
		}
	}
}
