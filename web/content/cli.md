---
tag: // Developer CLI Tooling
title: git meta Interactive Terminal
---

## `git meta` Interactive Terminal

Operate natively using standard Git commands. `git meta` coordinates your multi-repo workspace, keeping branches aligned, enforcing bottom-up pushes, and preventing dangling pointers.

### Key Commands:
- `git meta status`: Inspect local submodule drift alongside remote PR and cascade merge status.
- `git meta checkout [-b] <branch>`: Safely switch or create feature branches across the parent meta-repo and all submodules.
- `git meta commit -m "<msg>"`: Create coordinated atomic commits across modified submodules and update parent commit pointers.
- `git meta push`: Enforce bottom-up pushes to remote origins before updating parent pointer references.
- `git meta create-pr`: Open Pull Requests across all modified repositories and the parent meta-repo in one step.
- `git meta sync`: Fetch upstream changes, fast-forward submodules, and align root pointers to main.
- `git meta rebase <upstream>`: Perform two-phase rebasing across child submodules and parent references safely.
- `git meta retry-merge --pr <id>`: Manually re-trigger cascade merge execution from unmerged dependency nodes.
- `git meta agents`: Output standardized machine-readable guidelines and operation rules for AI coding agents.
- `git meta init`: Onboard your workspace with backend tracking, automated Git hooks, and webhook integration.
