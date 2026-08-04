# Repository Agent Guidelines

This repository is a **Meta-Repo** managed by **MetaStackr** (`git-meta`).

## About MetaStackr
MetaStackr orchestrates development across multi-repository meta-repos with Git submodules. It coordinates local workflows, tracks submodule drift, synchronizes PRs, and handles atomic cascade merges on GitHub.

## Rules for AI Agents
- **Do NOT run raw `git checkout` or `git commit` commands directly inside nested submodule directories.**
  - Running raw git commands directly inside submodules breaks pointer alignment and creates state drift.
- **Use `git-meta` for all multi-repo operations.**
  - Use `git-meta` commands to manage branches, commits, pushes, and synchronization across the meta-repo and submodules.
- **Always supply `--json` to `git-meta` CLI commands for deterministic state parsing.**
  - All `git-meta` subcommands accept `--json` to return structured JSON payloads.

## Key Operations

- **Inspect State & Submodule Drift**:
  `git meta status --json`
  Returns local submodule drift (uncommitted/unpushed changes) merged with remote Meta PR status.

- **Switch/Create Branches System-Wide**:
  `git meta checkout -b <branch-name> --json`
  Safely creates or switches branches across the parent meta-repo and all submodules.

- **Atomic Commits Across Submodules**:
  `git meta commit -m "<message>" --json`
  Creates coordinated commits in all modified submodules and updates parent commit pointers.

- **Push Changes (Bottom-Up Enforcement)**:
  `git meta push --json`
  Pushes submodule commits to remote origin before pushing parent commit pointer updates.

- **Sync Upstream Changes**:
  `git meta sync --json`
  Fetches upstream, fast-forwards/rebases local submodules, and aligns root pointers.

- **Two-Phase Rebase**:
  `git meta rebase <upstream-branch> --json`
  Rebases child submodules first, then parent meta-repo references.

- **Retry Cascade Merges**:
  `git meta retry-merge --pr <pr-number> --json`
  Re-triggers cascade merges on partially failed PRs.

