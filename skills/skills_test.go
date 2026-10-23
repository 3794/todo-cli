// Package skills_test checks the /todo-init skill: its frontmatter, its
// rules text, init.sh and the Makefile's install-skill target.
package skills_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const heading = "## TODO workflow"

func skillDir(t *testing.T) string {
	t.Helper()
	d, err := filepath.Abs("todo-init")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// rulesFromSkill returns the ```markdown block of SKILL.md.
func rulesFromSkill(t *testing.T) string {
	t.Helper()
	m := regexp.MustCompile("(?s)\n```markdown\n(.*?)\n```\n").FindStringSubmatch(read(t, filepath.Join(skillDir(t), "SKILL.md")))
	if m == nil {
		t.Fatal("no ```markdown block in SKILL.md")
	}
	return m[1] + "\n"
}

func runInit(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command(filepath.Join(skillDir(t), "init.sh"), dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("init.sh: %v\n%s", err, out)
	}
	return string(out)
}

func TestFrontmatter(t *testing.T) {
	s := read(t, filepath.Join(skillDir(t), "SKILL.md"))
	fm, _, ok := strings.Cut(strings.TrimPrefix(s, "---\n"), "\n---\n")
	if !strings.HasPrefix(s, "---\n") || !ok {
		t.Fatal("no frontmatter")
	}
	for _, want := range []string{
		"name: todo-init\n",
		"disable-model-invocation: true\n",
		`allowed-tools: Bash(${CLAUDE_SKILL_DIR}/init.sh "${CLAUDE_PROJECT_DIR}")` + "\n",
	} {
		if !strings.Contains(fm+"\n", want) {
			t.Errorf("frontmatter lacks %q", want)
		}
	}
	// The body must tell Claude to run exactly the pre-approved command.
	if !strings.Contains(s, "\n${CLAUDE_SKILL_DIR}/init.sh \"${CLAUDE_PROJECT_DIR}\"\n") {
		t.Error("body does not show the allowed command verbatim")
	}
	if fi, err := os.Stat(filepath.Join(skillDir(t), "init.sh")); err != nil || fi.Mode()&0o111 == 0 {
		t.Errorf("init.sh not executable: %v", err)
	}
}

// The README quotes the rules that /todo-init adds; they must match SKILL.md
// word for word.
func TestRulesMatchReadme(t *testing.T) {
	readme := read(t, "../README.md")
	m := regexp.MustCompile("(?s)\n```markdown\n(" + regexp.QuoteMeta(heading) + "\n.*?)\n```\n").FindStringSubmatch(readme)
	if m == nil {
		t.Fatal("README.md doesn't quote the rules block")
	}
	got := rulesFromSkill(t)
	if want := m[1] + "\n"; got != want {
		t.Fatalf("SKILL.md rules differ from README.md:\n got %q\nwant %q", got, want)
	}
	if !strings.HasPrefix(got, heading+"\n") {
		t.Fatalf("rules don't start with %q", heading)
	}
	for _, r := range got {
		if r > 0x7f {
			t.Fatalf("rules contain non-ASCII %q", r)
		}
	}
}

func TestInit(t *testing.T) {
	rules := rulesFromSkill(t)
	tests := []struct {
		name         string
		todo, claude *string // nil: file absent
		wantTodo     string
		wantClaude   string
	}{
		{"(a) empty folder", nil, nil, "# TODO\n", rules},
		{"(b) CLAUDE.md with content", nil, ptr("# Project\n\nExisting rules.\n"), "# TODO\n", "# Project\n\nExisting rules.\n\n" + rules},
		{"CLAUDE.md without final newline", nil, ptr("# P\nend"), "# TODO\n", "# P\nend\n\n" + rules},
		// Ends in a multi-byte UTF-8 character (Hangul, 3 bytes) with no
		// newline: init.sh inspects the last raw bytes to pick the separator.
		{"ends in a multi-byte character", nil, ptr("# P\n끝"), "# TODO\n", "# P\n끝\n\n" + rules},
		{"CLAUDE.md ending in a blank line", nil, ptr("# P\n\n"), "# TODO\n", "# P\n\n" + rules},
		{"empty CLAUDE.md", nil, ptr(""), "# TODO\n", rules},
		{"CRLF CLAUDE.md keeps its bytes", nil, ptr("# P\r\nrules\r\n"), "# TODO\n", "# P\r\nrules\r\n\n" + rules},
		{"section already present", nil, ptr("# P\n\n" + heading + "\n\n- my version\n"), "# TODO\n", "# P\n\n" + heading + "\n\n- my version\n"},
		{"section present with CRLF", nil, ptr(heading + "\r\n- x\r\n"), "# TODO\n", heading + "\r\n- x\r\n"},
		{"heading text inside a line doesn't count", nil, ptr("see ## TODO workflow\n"), "# TODO\n", "see ## TODO workflow\n\n" + rules},
		{"deeper heading doesn't count", nil, ptr("### TODO workflow\n"), "# TODO\n", "### TODO workflow\n\n" + rules},
		{"(d) existing TODO.md", ptr("# Tasks\n- [ ] already here"), nil, "# Tasks\n- [ ] already here", rules},
		{"empty TODO.md stays empty", ptr(""), nil, "", rules},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.todo != nil {
				os.WriteFile(filepath.Join(dir, "TODO.md"), []byte(*tt.todo), 0o644)
			}
			if tt.claude != nil {
				os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(*tt.claude), 0o644)
			}
			out1 := runInit(t, dir)
			check := func() {
				t.Helper()
				if got := read(t, filepath.Join(dir, "TODO.md")); got != tt.wantTodo {
					t.Fatalf("TODO.md = %q, want %q", got, tt.wantTodo)
				}
				if got := read(t, filepath.Join(dir, "CLAUDE.md")); got != tt.wantClaude {
					t.Fatalf("CLAUDE.md = %q, want %q", got, tt.wantClaude)
				}
				if ents, _ := os.ReadDir(dir); len(ents) != 2 {
					t.Fatalf("unexpected files: %v", ents)
				}
			}
			check()
			// (c) running again changes nothing and says so.
			out2 := runInit(t, dir)
			check()
			if !strings.Contains(out2, "skipped TODO.md") || !strings.Contains(out2, "skipped CLAUDE.md") {
				t.Fatalf("second run output:\n%s", out2)
			}
			if tt.todo == nil && !strings.Contains(out1, "created TODO.md") {
				t.Fatalf("first run output:\n%s", out1)
			}
		})
	}
}

func TestInitUsesCurrentDirByDefault(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command(filepath.Join(skillDir(t), "init.sh"))
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err != nil {
		t.Fatal(err)
	}
}

func TestInitDirWithSpaces(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my project")
	os.Mkdir(dir, 0o755)
	runInit(t, dir)
	if got := read(t, filepath.Join(dir, "TODO.md")); got != "# TODO\n" {
		t.Fatalf("TODO.md = %q", got)
	}
}

func ptr(s string) *string { return &s }

// (e) make install-skill twice: the link points at this repo; something else
// in the way is left alone and reported as an error.
func TestMakeInstallSkill(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make not found")
	}
	repo, _ := filepath.Abs("..")
	home := t.TempDir()
	link := filepath.Join(home, ".claude", "skills", "todo-init")
	run := func() (string, error) {
		cmd := exec.Command("make", "-s", "install-skill", "HOME="+home)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	for i := range 2 {
		out, err := run()
		if err != nil {
			t.Fatalf("run %d: %v\n%s", i+1, err, out)
		}
		target, err := os.Readlink(link)
		if err != nil || target != filepath.Join(repo, "skills", "todo-init") {
			t.Fatalf("run %d: link -> %q (%v)", i+1, target, err)
		}
		if _, err := os.Stat(filepath.Join(link, "SKILL.md")); err != nil {
			t.Fatalf("SKILL.md not reachable through link: %v", err)
		}
	}

	// Something else at the link path: refuse and leave it alone.
	os.Remove(link)
	os.Mkdir(link, 0o755)
	os.WriteFile(filepath.Join(link, "SKILL.md"), []byte("mine"), 0o644)
	if out, err := run(); err == nil || !strings.Contains(out, "not touching it") {
		t.Fatalf("expected refusal, got err=%v\n%s", err, out)
	}
	if got := read(t, filepath.Join(link, "SKILL.md")); got != "mine" {
		t.Fatalf("existing skill overwritten: %q", got)
	}
	os.RemoveAll(link)
	os.Symlink("/somewhere/else", link)
	if _, err := run(); err == nil {
		t.Fatal("replaced a link pointing elsewhere")
	}
	if target, _ := os.Readlink(link); target != "/somewhere/else" {
		t.Fatalf("link changed to %q", target)
	}
}
