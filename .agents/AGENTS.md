# Repository Agent Guidelines

This repository is a **Meta-Repo** managed by `git-meta`.

## Rules for AI Agents
- Do NOT run raw `git checkout` or `git commit` commands directly inside nested submodule directories.
- Always supply `--json` to `git-meta` CLI commands for deterministic state parsing.

## Key Operations
- Inspect state: `git meta status --json`
- Switch/create branches: `git meta checkout -b <branch-name> --json`
- Commit changes across system: `git meta commit -m "<msg>" --json`
- Sync upstream changes: `git meta sync --json`
