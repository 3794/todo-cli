#!/bin/sh
# Turns on the TODO workflow in a project directory (default: current).
#
#   - TODO.md:   created as "# TODO" if missing, otherwise left alone.
#   - CLAUDE.md: the "## TODO workflow" section from SKILL.md is appended if
#                the file has no such section (the file is created if
#                missing). Existing bytes are never changed.
#
# Running it again changes nothing. It never touches git.
set -eu

skill_dir=$(cd "$(dirname "$0")" && pwd)
target=${1:-.}
heading='## TODO workflow'

cd "$target"

# The rules are the ```markdown block in SKILL.md.
rules=$(awk '/^```markdown[[:space:]]*$/ { inblock = 1; next }
             inblock && /^```[[:space:]]*$/ { exit }
             inblock { print }' "$skill_dir/SKILL.md")
first_line=$(printf '%s\n' "$rules" | head -n 1)
if [ "$first_line" != "$heading" ]; then
	echo "todo-init: rules block not found in $skill_dir/SKILL.md" >&2
	exit 1
fi

# 1. TODO.md
if [ -e TODO.md ] || [ -L TODO.md ]; then
	echo "skipped TODO.md (already exists)"
else
	(set -C; printf '# TODO\n' > TODO.md)
	echo "created TODO.md"
fi

# 2. CLAUDE.md
has_heading() {
	grep -Eq "^$1[[:space:]]*\$" CLAUDE.md
}
if [ -e CLAUDE.md ] && has_heading "$heading"; then
	echo "skipped CLAUDE.md (\"$heading\" section already present)"
elif [ -s CLAUDE.md ]; then
	# Separate from existing content by exactly one blank line.
	case $(tail -c 2 CLAUDE.md | od -An -c | tr -d ' ') in
	'\n\n') sep='' ;;
	*'\n') sep='\n' ;;
	*) sep='\n\n' ;;
	esac
	printf "$sep%s\n" "$rules" >> CLAUDE.md
	echo "appended \"$heading\" section to CLAUDE.md"
else
	existed=no
	[ -e CLAUDE.md ] && existed=yes
	printf '%s\n' "$rules" >> CLAUDE.md
	if [ $existed = yes ]; then
		echo "added \"$heading\" section to empty CLAUDE.md"
	else
		echo "created CLAUDE.md with \"$heading\" section"
	fi
fi
