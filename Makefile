.PHONY: install install-bin install-skill test

SKILL_SRC  := $(CURDIR)/skills/todo-init
SKILL_LINK := $(HOME)/.claude/skills/todo-init

# Install the binary to ~/.local/bin/todo and link the /todo-init skill.
install: install-bin install-skill

install-bin:
	go build -o $(HOME)/.local/bin/todo ./cmd/todo

# ~/.claude/skills/todo-init -> this repo. Leaves a correct link alone and
# refuses to replace anything else.
install-skill:
	@mkdir -p "$(dir $(SKILL_LINK))"
	@if [ -L "$(SKILL_LINK)" ] && [ "$$(readlink "$(SKILL_LINK)")" = "$(SKILL_SRC)" ]; then \
		echo "skill link already in place: $(SKILL_LINK) -> $(SKILL_SRC)"; \
	elif [ -e "$(SKILL_LINK)" ] || [ -L "$(SKILL_LINK)" ]; then \
		echo "error: $(SKILL_LINK) already exists and is not a link to $(SKILL_SRC); not touching it" >&2; \
		exit 1; \
	else \
		ln -s "$(SKILL_SRC)" "$(SKILL_LINK)" && echo "linked $(SKILL_LINK) -> $(SKILL_SRC)"; \
	fi

test:
	go vet ./...
	go test ./...
