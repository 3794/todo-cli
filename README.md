# todo

A terminal UI for a plain `TODO.md` file — built so that you and Claude Code can edit the same file at the same time.

- `todo` shows `./TODO.md` as a checklist you can add to, edit, check off and delete from.
- Claude Code reads and edits the same file directly. The `/todo-init` skill adds a short workflow to your project's `CLAUDE.md` so Claude keeps `TODO.md` up to date while it works.
- The TUI never overwrites what someone else wrote: every change touches only its own line and is applied to a fresh read of the file, and the screen refreshes when the file changes on disk.

## Install

### The `todo` command

With Go 1.26 or later:

```sh
go install github.com/3794/todo-cli/cmd/todo@latest
```

This puts `todo` in `$(go env GOPATH)/bin` (usually `~/go/bin`); make sure that directory is on your `PATH`.

### The Claude Code plugin (`/todo-init`)

From your shell:

```sh
claude plugin marketplace add 3794/todo-cli
claude plugin install todo@todo-cli
```

Or inside Claude Code:

```text
/plugin marketplace add 3794/todo-cli
/plugin install todo@todo-cli
```

The skill is then available as `/todo:todo-init`.

### From a clone

```sh
git clone https://github.com/3794/todo-cli.git
cd todo-cli
make install
```

`make install` builds `todo` into `~/.local/bin/todo` (make sure `~/.local/bin` is on your `PATH`) and links the skill into `~/.claude/skills/todo-init`, where it runs as `/todo-init`. It refuses to replace anything else that is already at that path. Use either the plugin or the link, not both, or you'll see the skill twice.

## Usage

```sh
todo              # ./TODO.md (created with a "# TODO" title if missing)
todo notes.md     # any file
todo -- -odd.md   # a path that starts with "-"
todo --version
todo --help
```

| Key | Action |
|---|---|
| `j` / `k`, `↓` / `↑` | Move the cursor (headings and items) |
| `a` | Add an item: below the current item and its sub-items with the same indent, below the current heading, or at the end of the file |
| `e`, `Enter` | Edit the item's text (indent and check state are kept) |
| `space` | Toggle done: `[x]` plus ` done:YYYY-MM-DD`, or back to `[ ]` |
| `d` | Delete the item and its sub-items (asks `y`/`n`) |
| `Esc` | Cancel input or confirmation |
| `q`, `Ctrl+C` | Quit |

## File format

```markdown
# TODO

## Auth
- [ ] Handle token expiry #auth due:2026-10-24
  - [ ] Call the refresh API
- [x] Login screen done:2026-10-22

## Deploy
- [ ] Set up CI cache
```

- A line starting with `## ` is a section heading.
- An item is a line matching `^(\s*)- \[(.)\] (.*)$`. `[ ]` is open, `[x]` or `[X]` is done, and any other mark (such as `[/]`) is shown as is and treated as open. Indented items are sub-items.
- `#tags` and `due:` dates are highlighted. Item text is shown literally, never interpreted as Markdown.
- Long headings and items wrap to the screen width, lined up under the start of their text.
- Every other line (titles, blank lines, notes, paragraphs) is hidden in the TUI and always kept exactly as it is.

## Using it with Claude Code

Run `/todo:todo-init` (or `/todo-init` if you installed from a clone) once in a project. It runs a small script that

1. creates `TODO.md` with a `# TODO` title if it doesn't exist, and
2. appends the section below to `CLAUDE.md` (creating the file if needed) unless it already has one. Existing content is kept byte for byte, and running it again changes nothing. It never touches git.

The section it adds:

```markdown
## TODO workflow

- When you get a work request (build, fix, or change something), check `TODO.md` before you start.
  - If the task is already listed, work on that item.
  - If not, add it as `- [ ] <task>` first, then start.
  - Don't add items for plain questions.
- When the work is done and verified, change the item to `[x]` and append ` done:YYYY-MM-DD`.
- `TODO.md` is also edited by the TUI (`todo`). Change only the lines you need with partial edits (Edit); never rewrite the whole file.
```

The skill only runs when you type it; Claude never invokes it on its own. Its one pre-approved command is that script, run on the project folder.

## Editing the same file from both sides

The TUI is written so that it never loses what another program wrote:

- **Line-level edits only.** The file is read as a list of lines and only the target line is changed, inserted or removed. Untouched lines stay byte for byte, including the final newline (or its absence) and CRLF line endings.
- **Re-read before every write.** Each change is applied to a fresh read of the file. The target is found by its content as it was when you started, not by line number. If it can't be found, nothing is written and you get a short notice.
- **Atomic writes.** The new content goes to a temporary file in the same directory, which is then renamed over the original. Right before the rename the file is read once more; if it changed in the meantime, the change is redone on the new content.
- **Live refresh.** The file's modification time and size are checked every 500 ms and the screen is redrawn when they change. The cursor stays on the same line content.
- **Your typing is never thrown away.** If the item you were adding below was checked off or removed elsewhere, the new item still gets added, near it or at the end of the file. If the item you were editing changed, nothing is saved, but your text stays in the input so you can save it again or cancel.

## Platforms

macOS and Linux (CI runs on both). Windows hasn't been tested.

## License

[MIT](LICENSE)
