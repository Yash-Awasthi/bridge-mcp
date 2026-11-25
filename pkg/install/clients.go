// Package install registers the board's stdio MCP server in every MCP-capable
// editor and agent CLI on this machine: one entry, one name, one config shape
// per client, with a backup before anything is written.
package install

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Format is how a client spells a stdio MCP server in its config file.
type Format string

const (
	// FormatMcpServers is {"mcpServers": {"<name>": {"command", "args", "env"}}}.
	// Claude Desktop, Claude Code, Cursor, Windsurf, Gemini CLI, Qwen, Cline, Antigravity.
	FormatMcpServers Format = "mcpServers"
	// FormatVSCode is {"servers": {"<name>": {"type": "stdio", "command", ...}}}.
	FormatVSCode Format = "vscode"
	// FormatZed is {"context_servers": {"<name>": {"source": "custom", ...}}}.
	FormatZed Format = "zed"
	// FormatOpenCode is {"mcp": {"<name>": {"type": "local", "command": [...]}}}.
	FormatOpenCode Format = "opencode"
	// FormatOpenClaw is {"mcp": {"servers": {"<name>": {"command", ...}}}} (JSON5).
	FormatOpenClaw Format = "openclaw"
	// FormatTOML is [mcp_servers.<name>] (Codex CLI, Grok CLI).
	FormatTOML Format = "toml"
)

// Env is the machine an install targets. Tests set it to a temp directory;
// the CLI fills it from the real environment.
type Env struct {
	GOOS         string // runtime.GOOS
	Home         string // user home directory
	AppData      string // Windows %APPDATA%, elsewhere $XDG_CONFIG_HOME or ~/.config
	LocalAppData string // Windows %LOCALAPPDATA%, elsewhere the same as AppData
}

// SystemEnv reads the current machine.
func SystemEnv() Env {
	home, _ := os.UserHomeDir()
	goos := runtime.GOOS
	appData := os.Getenv("XDG_CONFIG_HOME")
	if goos == "windows" {
		appData = os.Getenv("APPDATA")
	} else if appData == "" || !filepath.IsAbs(appData) {
		appData = filepath.Join(home, ".config")
	}
	local := os.Getenv("LOCALAPPDATA")
	if goos != "windows" || local == "" {
		local = appData
	}
	return Env{GOOS: goos, Home: home, AppData: appData, LocalAppData: local}
}

// Client is one MCP-capable app and the file it reads servers from.
type Client struct {
	ID     string
	Name   string
	Docs   string
	Format Format
	// Path is the user-scope config file for this client.
	Path func(Env) string
	// Markers, when set, are paths whose existence means the client is
	// installed. When nil the config file's directory is used.
	Markers []func(Env) string
	// Note is a short caveat shown by `vb install --list`.
	Note string
}

// Installed reports whether the client appears to be on this machine.
func (c Client) Installed(e Env) bool {
	if _, err := os.Stat(c.Path(e)); err == nil {
		return true
	}
	markers := c.Markers
	if markers == nil {
		markers = []func(Env) string{func(e Env) string { return filepath.Dir(c.Path(e)) }}
	}
	for _, m := range markers {
		if p := m(e); p != "" {
			if _, err := os.Stat(p); err == nil {
				return true
			}
		}
	}
	return false
}

// appPath builds a path per OS: winRel under %APPDATA%, macRel under $HOME,
// linuxRel under the XDG config dir.
func appPath(winRel, macRel, linuxRel string) func(Env) string {
	return func(e Env) string {
		switch e.GOOS {
		case "windows":
			return filepath.Join(e.AppData, filepath.FromSlash(winRel))
		case "darwin":
			return filepath.Join(e.Home, filepath.FromSlash(macRel))
		default:
			return filepath.Join(e.AppData, filepath.FromSlash(linuxRel))
		}
	}
}

func homePath(rel ...string) func(Env) string {
	return func(e Env) string { return filepath.Join(append([]string{e.Home}, rel...)...) }
}

func localPath(rel string) func(Env) string {
	return func(e Env) string { return filepath.Join(e.LocalAppData, filepath.FromSlash(rel)) }
}

func appDataDir(name string) func(Env) string {
	return func(e Env) string { return filepath.Join(e.AppData, name) }
}

// Clients is every client the installer knows. Keep it ordered: people read
// the table, and the order is stable across runs and OSes.
func Clients() []Client {
	return []Client{
		{
			ID: "claude-desktop", Name: "Claude Desktop", Docs: "https://claude.ai/download",
			Format: FormatMcpServers,
			Path: appPath("Claude/claude_desktop_config.json",
				"Library/Application Support/Claude/claude_desktop_config.json",
				"Claude/claude_desktop_config.json"),
			Markers: []func(Env) string{
				appDataDir("Claude"),
				appPath("Claude/claude_desktop_config.json",
					"Library/Application Support/Claude/claude_desktop_config.json",
					"Claude/claude_desktop_config.json"),
			},
		},
		{
			ID: "claude-code", Name: "Claude Code", Docs: "https://claude.com/claude-code",
			Format: FormatMcpServers, Path: homePath(".claude.json"),
			Markers: []func(Env) string{homePath(".claude"), homePath(".claude.json")},
		},
		{
			ID: "antigravity", Name: "Antigravity (App, IDE, CLI)", Docs: "https://antigravity.google/docs/mcp",
			Format: FormatMcpServers, Path: homePath(".gemini", "config", "mcp_config.json"),
			Markers: []func(Env) string{homePath(".gemini", "config"), homePath(".gemini", "antigravity")},
			Note:    "global ~/.gemini/config/mcp_config.json; workspace file is .agents/mcp_config.json",
		},
		{
			ID: "cursor", Name: "Cursor", Docs: "https://cursor.com/docs/context/mcp",
			Format: FormatMcpServers, Path: homePath(".cursor", "mcp.json"),
			Markers: []func(Env) string{homePath(".cursor"), localPath("Programs/cursor")},
		},
		{
			ID: "windsurf", Name: "Windsurf", Docs: "https://windsurf.com",
			Format: FormatMcpServers, Path: homePath(".codeium", "windsurf", "mcp_config.json"),
			Markers: []func(Env) string{homePath(".codeium", "windsurf")},
		},
		{
			ID: "vscode", Name: "VS Code (Copilot)", Docs: "https://code.visualstudio.com/docs/copilot/customization/mcp-servers",
			Format: FormatVSCode,
			Path:   appPath("Code/User/mcp.json", "Library/Application Support/Code/User/mcp.json", "Code/User/mcp.json"),
			Markers: []func(Env) string{
				appPath("Code/User", "Library/Application Support/Code", "Code"),
				homePath(".vscode"),
			},
			Note: "user mcp.json; a repo can also use .vscode/mcp.json",
		},
		{
			ID: "vscode-insiders", Name: "VS Code Insiders", Docs: "https://code.visualstudio.com/insiders",
			Format: FormatVSCode,
			Path:   appPath("Code - Insiders/User/mcp.json", "Library/Application Support/Code - Insiders/User/mcp.json", "Code - Insiders/User/mcp.json"),
			Markers: []func(Env) string{
				appPath("Code - Insiders/User", "Library/Application Support/Code - Insiders", "Code - Insiders"),
			},
		},
		{
			ID: "cline", Name: "Cline (VS Code)", Docs: "https://docs.cline.bot/mcp/configuring-mcp-servers",
			Format: FormatMcpServers,
			Path: appPath("Code/User/globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json",
				"Library/Application Support/Code/User/globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json",
				"Code/User/globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json"),
			Markers: []func(Env) string{
				appPath("Code/User/globalStorage/saoudrizwan.claude-dev",
					"Library/Application Support/Code/User/globalStorage/saoudrizwan.claude-dev",
					"Code/User/globalStorage/saoudrizwan.claude-dev"),
			},
			Note: "installed as a VS Code extension, not a separate app",
		},
		{
			ID: "roo", Name: "Roo Code (VS Code)", Docs: "https://docs.roocode.com/features/mcp/using-mcp-in-roo",
			Format: FormatMcpServers,
			Path: appPath("Code/User/globalStorage/rooveterinaryinc.roo-cline/settings/cline_mcp_settings.json",
				"Library/Application Support/Code/User/globalStorage/rooveterinaryinc.roo-cline/settings/cline_mcp_settings.json",
				"Code/User/globalStorage/rooveterinaryinc.roo-cline/settings/cline_mcp_settings.json"),
			Markers: []func(Env) string{
				appPath("Code/User/globalStorage/rooveterinaryinc.roo-cline",
					"Library/Application Support/Code/User/globalStorage/rooveterinaryinc.roo-cline",
					"Code/User/globalStorage/rooveterinaryinc.roo-cline"),
			},
		},
		{
			ID: "zed", Name: "Zed", Docs: "https://zed.dev/docs/ai/mcp",
			Format: FormatZed,
			Path:   appPath("Zed/settings.json", "Library/Application Support/Zed/settings.json", "zed/settings.json"),
			Markers: []func(Env) string{
				appPath("Zed", "Library/Application Support/Zed", "zed"),
				localPath("Programs/Zed"),
			},
		},
		{
			ID: "codex", Name: "Codex CLI", Docs: "https://developers.openai.com/codex/mcp",
			Format: FormatTOML, Path: homePath(".codex", "config.toml"),
			Markers: []func(Env) string{homePath(".codex")},
		},
		{
			ID: "grok", Name: "Grok CLI", Docs: "https://docs.x.ai/build/features/mcp-servers",
			Format: FormatTOML, Path: homePath(".grok", "config.toml"),
			Markers: []func(Env) string{homePath(".grok")},
		},
		{
			ID: "gemini", Name: "Gemini CLI", Docs: "https://github.com/google-gemini/gemini-cli/blob/main/docs/tools/mcp-server.md",
			Format: FormatMcpServers, Path: homePath(".gemini", "settings.json"),
			Markers: []func(Env) string{homePath(".gemini")},
		},
		{
			ID: "qwen", Name: "Qwen Code CLI", Docs: "https://github.com/QwenLM/qwen-code",
			Format: FormatMcpServers, Path: homePath(".qwen", "settings.json"),
			Markers: []func(Env) string{homePath(".qwen")},
		},
		{
			ID: "opencode", Name: "OpenCode", Docs: "https://opencode.ai/docs/mcp-servers/",
			Format: FormatOpenCode, Path: homePath(".config", "opencode", "opencode.json"),
			Markers: []func(Env) string{
				homePath(".config", "opencode"),
				homePath(".local", "share", "opencode"),
				appPath("opencode", ".config/opencode", "opencode"),
			},
			Note: "global config is ~/.config/opencode/opencode.json on every OS",
		},
		{
			ID: "openclaw", Name: "OpenClaw", Docs: "https://docs.openclaw.ai/tools/mcp",
			Format: FormatOpenClaw, Path: homePath(".openclaw", "openclaw.json"),
			Markers: []func(Env) string{homePath(".openclaw")},
			Note:    "reads mcp.servers from openclaw.json (JSON5)",
		},
	}
}

// Lookup finds a client by id, case-insensitively.
func Lookup(id string) (Client, bool) {
	want := strings.ToLower(strings.TrimSpace(id))
	for _, c := range Clients() {
		if strings.ToLower(c.ID) == want {
			return c, true
		}
	}
	return Client{}, false
}
