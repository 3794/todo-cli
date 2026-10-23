// Command todo shows a TODO.md file as a terminal UI.
//
//	todo           # ./TODO.md
//	todo <path>    # any file
//	todo -- <path> # a path that starts with "-"
package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"runtime/debug"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/3794/todo-cli/internal/tui"
)

const (
	defaultPath = "TODO.md"
	newFile     = "# TODO\n"
	usage       = `usage: todo [path]

Shows a TODO.md file (default: ./TODO.md) as a terminal UI.
The file is created with a "# TODO" title if it doesn't exist.

options:
  -h, --help   show this help
  --version    print the version
  --           treat the next argument as a path, even if it starts with "-"
`
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// action is what the command line asks for.
type action int

const (
	actionOpen action = iota
	actionHelp
	actionVersion
)

var errUsage = errors.New("usage")

// parseArgs interprets the arguments. Unknown options are an error, so that
// they never turn into a file name.
func parseArgs(args []string) (action, string, error) {
	var paths []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			paths = append(paths, args[i+1:]...)
			i = len(args)
		case a == "-h" || a == "--help":
			return actionHelp, "", nil
		case a == "--version":
			return actionVersion, "", nil
		case strings.HasPrefix(a, "-"):
			return 0, "", fmt.Errorf("%w: unknown option %q", errUsage, a)
		default:
			paths = append(paths, a)
		}
	}
	switch len(paths) {
	case 0:
		return actionOpen, defaultPath, nil
	case 1:
		if paths[0] == "" {
			return 0, "", fmt.Errorf("%w: empty path", errUsage)
		}
		return actionOpen, paths[0], nil
	default:
		return 0, "", fmt.Errorf("%w: too many arguments", errUsage)
	}
}

// version is the module version for `go install ...@version` builds and
// "(devel)" for local builds.
func version() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		return bi.Main.Version
	}
	return "(devel)"
}

func run(args []string, stdout, stderr io.Writer) int {
	act, path, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "todo: %s\n\n%s", strings.TrimPrefix(err.Error(), "usage: "), usage)
		return 2
	}
	switch act {
	case actionHelp:
		fmt.Fprint(stdout, usage)
		return 0
	case actionVersion:
		fmt.Fprintln(stdout, "todo", version())
		return 0
	}

	if err := ensureFile(path); err != nil {
		fmt.Fprintln(stderr, "todo:", err)
		return 1
	}
	m, err := tui.New(path)
	if err != nil {
		fmt.Fprintln(stderr, "todo:", err)
		return 1
	}
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Fprintln(stderr, "todo:", err)
		return 1
	}
	return 0
}

// ensureFile creates path with a "# TODO" title if it doesn't exist.
func ensureFile(path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := f.WriteString(newFile); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
