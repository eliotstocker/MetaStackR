---
tag: // Developer CLI Tooling
title: git-meta Interactive Terminal
---

## `git-meta` Interactive Terminal

Operate natively using standard Git commands. `git-meta` automatically handles submodule branch alignment, bottom-up pushing, and cascade retries.

### Key CLI Subcommands:
- `git meta status`: Fetch local Git submodule drift and remote backend PR/CI status in Lipgloss tables.
- `git meta checkout <branch>`: Safely switch or create feature branches across root and all submodules.
- `git meta push`: Enforce bottom-up pushing (push submodules before parent commit pointers).
- `git meta retry-merge`: Re-trigger cascade merge on partially failed PRs.
- `git meta install-hooks`: Install `post-checkout` and `pre-commit` hooks into `.git/hooks`.
