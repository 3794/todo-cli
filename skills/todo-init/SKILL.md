---
name: todo-init
description: Turn on the TODO workflow for this project. Creates TODO.md if it's missing and appends a "## TODO workflow" section to CLAUDE.md if it has none. Safe to run again.
disable-model-invocation: true
allowed-tools: Bash(${CLAUDE_SKILL_DIR}/init.sh "${CLAUDE_PROJECT_DIR}")
---

# /todo-init

Turn on the TODO workflow for this project (the folder the session started in).

Run exactly this one command. Don't change the path or add `cd`.

```bash
${CLAUDE_SKILL_DIR}/init.sh "${CLAUDE_PROJECT_DIR}"
```

What the script does:

1. If `TODO.md` doesn't exist, it creates it with a single `# TODO` line. An existing `TODO.md` is left alone.
2. If `CLAUDE.md` has no `## TODO workflow` section, it appends the rules below to the end (creating the file if needed). Existing content is kept byte for byte. If the section is already there, it does nothing.

Rules for you:

- Don't change files any other way (Write, Edit, ...). Don't touch git.
- Report what was created and what was skipped in one or two short lines, based on the script's output.
- If the script fails, pass on its error message as is and stop. Don't try to do it by hand.

## The rules that get added

`init.sh` appends the content of the block below to `CLAUDE.md` as is. This block is the source of the rules.

```markdown
## TODO workflow

- When you get a work request (build, fix, or change something), check `TODO.md` before you start.
  - If the task is already listed, work on that item.
  - If not, add it as `- [ ] <task>` first, then start.
  - Don't add items for plain questions.
- When the work is done and verified, change the item to `[x]` and append ` done:YYYY-MM-DD`.
- `TODO.md` is also edited by the TUI (`todo`). Change only the lines you need with partial edits (Edit); never rewrite the whole file.
```
