package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"open-board/pkg/install"
)

// homeEnv is a fake machine whose detected clients are exactly the ones whose
// marker directories the test creates.
func homeEnv(t *testing.T, markers ...string) install.Env {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	app := filepath.Join(root, "appdata")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(app, 0o755); err != nil {
		t.Fatal(err)
	}
	env := install.Env{GOOS: runtime.GOOS, Home: home, AppData: app, LocalAppData: app}
	for _, m := range markers {
		if err := os.MkdirAll(filepath.Join(home, m), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return env
}

func TestInstallTargetsDefaultsToDetected(t *testing.T) {
	env := homeEnv(t, ".cursor", ".codex")
	targets, code := installTargets(nil, false, env)
	if code != 0 {
		t.Fatalf("code %d", code)
	}
	ids := make([]string, 0, len(targets))
	for _, c := range targets {
		ids = append(ids, c.ID)
	}
	want := map[string]bool{"cursor": true, "codex": true}
	for _, id := range ids {
		if !want[id] {
			t.Fatalf("unexpected detected client %q", id)
		}
		delete(want, id)
	}
	if len(want) != 0 {
		t.Fatalf("detected clients missing: %v (got %v)", want, ids)
	}
}

func TestInstallTargetsExplicitAndAll(t *testing.T) {
	env := homeEnv(t)
	targets, code := installTargets([]string{"zed", "grok"}, false, env)
	if code != 0 || len(targets) != 2 || targets[0].ID != "zed" || targets[1].ID != "grok" {
		t.Fatalf("explicit targets = %v, code %d", targets, code)
	}
	if _, code := installTargets([]string{"nope"}, false, env); code != 2 {
		t.Fatalf("unknown client code = %d, want 2", code)
	}
	all, code := installTargets(nil, true, env)
	if code != 0 || len(all) != len(install.Clients()) {
		t.Fatalf("--all gave %d clients", len(all))
	}
}

func TestInstallWritesOnlyTheRequestedClient(t *testing.T) {
	env := homeEnv(t, ".cursor", ".codex")
	cursor, _ := install.Lookup("cursor")
	res := install.Apply(cursor, env, install.Entry{
		Command: "/usr/local/bin/vb", Args: []string{"mcp"},
		Env: map[string]string{"VB_URL": "http://127.0.0.1:4100", "VB_CHANNEL": "default"},
	}, "open-board", false, false, false)
	if res.Status != "written" {
		t.Fatalf("status %q detail %q", res.Status, res.Detail)
	}
	if _, err := os.Stat(cursor.Path(env)); err != nil {
		t.Fatalf("cursor config missing: %v", err)
	}
	if codex, _ := install.Lookup("codex"); func() bool { _, err := os.Stat(codex.Path(env)); return err == nil }() {
		t.Fatal("codex config was written without being targeted")
	}
}

func TestDisplayPathShortensHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home directory")
	}
	got := displayPath(filepath.Join(home, "sub", "file.json"))
	if !strings.HasPrefix(got, "~") {
		t.Fatalf("displayPath = %q, want a ~ prefix", got)
	}
	// A path under the home directory is shortened; one outside it is not.
	outside := filepath.Join(filepath.Dir(home), "outside-board-test")
	if strings.HasPrefix(outside, home) {
		t.Skip("home directory has no parent to test against")
	}
	if got := displayPath(outside); got != outside {
		t.Fatalf("displayPath changed an unrelated path: %q", got)
	}
}
