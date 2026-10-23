package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		args []string
		act  action
		path string
		err  bool
	}{
		{nil, actionOpen, "TODO.md", false},
		{[]string{"notes.md"}, actionOpen, "notes.md", false},
		{[]string{"-h"}, actionHelp, "", false},
		{[]string{"--help"}, actionHelp, "", false},
		{[]string{"notes.md", "--help"}, actionHelp, "", false},
		{[]string{"--version"}, actionVersion, "", false},
		{[]string{"--", "-dash.md"}, actionOpen, "-dash.md", false},
		{[]string{"--", "--version"}, actionOpen, "--version", false},
		{[]string{"--", "a.md", "b.md"}, 0, "", true},
		{[]string{"--"}, actionOpen, "TODO.md", false},
		{[]string{"--bogus"}, 0, "", true},
		{[]string{"-v"}, 0, "", true},
		{[]string{"-"}, 0, "", true},
		{[]string{"a.md", "b.md"}, 0, "", true},
		{[]string{""}, 0, "", true},
	}
	for _, tt := range tests {
		act, path, err := parseArgs(tt.args)
		if (err != nil) != tt.err || (!tt.err && (act != tt.act || path != tt.path)) {
			t.Errorf("parseArgs(%q) = %v, %q, %v", tt.args, act, path, err)
		}
	}
}

// Options never create files, whatever they are.
func TestRunOptionsCreateNoFiles(t *testing.T) {
	dir := t.TempDir()
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(wd) })

	tests := []struct {
		args        []string
		code        int
		out, errOut string
	}{
		{[]string{"--version"}, 0, "todo ", ""},
		{[]string{"--help"}, 0, "usage: todo [path]", ""},
		{[]string{"-h"}, 0, "usage: todo [path]", ""},
		{[]string{"--bogus"}, 2, "", `unknown option "--bogus"`},
		{[]string{"-x", "file.md"}, 2, "", `unknown option "-x"`},
		{[]string{"a.md", "b.md"}, 2, "", "too many arguments"},
	}
	for _, tt := range tests {
		var out, errOut bytes.Buffer
		code := run(tt.args, &out, &errOut)
		if code != tt.code || !strings.Contains(out.String(), tt.out) || !strings.Contains(errOut.String(), tt.errOut) {
			t.Errorf("run(%q) = %d\nstdout: %s\nstderr: %s", tt.args, code, out.String(), errOut.String())
		}
		if code == 2 && !strings.Contains(errOut.String(), "usage: todo [path]") {
			t.Errorf("run(%q): no usage on stderr", tt.args)
		}
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		t.Fatalf("files created: %v", ents)
	}
}

func TestVersionIsDevelInTests(t *testing.T) {
	if v := version(); v == "" {
		t.Fatal("empty version")
	}
}

func TestEnsureFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "TODO.md")
	if err := ensureFile(path); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "# TODO\n" {
		t.Fatalf("new file = %q", b)
	}
	os.WriteFile(path, []byte("keep"), 0o644)
	if err := ensureFile(path); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "keep" {
		t.Fatalf("existing file changed: %q", b)
	}
}
