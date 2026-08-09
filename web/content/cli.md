---
tag: // Developer CLI Tooling
title: git-meta Interactive Terminal
---

## `git-meta` Interactive Terminal

Operate natively using standard Git commands. `git-meta` automatically handles submodule branch alignment, bottom-up pushing, and cascade retries.

### Key CLI Subcommands:
- `git meta status`: Fetch local Git submodule drift and remote backend PR/CI status in Lipgloss tables.
- `git meta checkout [-b] <branch>`: Safely switch or create feature branches across root and all submodules.
- `git meta commit -m "<msg>"`: Create atomic commits in modified submodules and update parent pointers.
- `git meta push`: Enforce bottom-up pushing (push submodules before parent commit pointers).
- `git meta sync`: Fetch origin/main, fast-forward/rebase local submodules, and align root pointers.
- `git meta rebase <upstream>`: Conduct two-phase rebase (child submodules first, then parent meta-repo).
- `git meta retry-merge --pr <pr>`: Re-trigger cascade merge on partially failed PRs.
- `git meta install-hooks`: Install `post-checkout` and `pre-commit` hooks into `.git/hooks`.
- `git meta init`: Onboard repository with backend registration, Git hooks, and GitHub webhooks.
- `git meta setup-webhook`: Automate repository webhook registration with GitHub.
- `git meta agents`: Output guidelines and machine-readable instructions for AI agents.
- `git meta version`: Print the semantic version (`git meta version` or `git meta --version`).
