# MetaStackr Detailed User Guide

Welcome to the **MetaStackr User Guide**. This document provides end-to-end instructions for developers, repository maintainers, and DevOps engineers managing multi-repo workflows with `git-meta` and the `metastackrd` daemon.

---

## Table of Contents

1. [Concepts & Terminology](#concepts--terminology)
2. [CLI Command Reference (`git-meta`)](#cli-command-reference-git-meta)
   - [`git meta status`](#git-meta-status)
   - [`git meta checkout`](#git-meta-checkout)
   - [`git meta commit`](#git-meta-commit)
   - [`git meta push`](#git-meta-push)
   - [`git meta sync`](#git-meta-sync)
   - [`git meta rebase`](#git-meta-rebase)
   - [`git meta retry-merge`](#git-meta-retry-merge)
   - [`git meta install-hooks`](#git-meta-install-hooks)
   - [`git meta init`](#git-meta-init)
   - [`git meta setup-webhook`](#git-meta-setup-webhook)
   - [`git meta agents`](#git-meta-agents)
   - [`git meta version`](#git-meta-version)
3. [Developer Workflow Scenario](#developer-workflow-scenario)
4. [Backend Administration (`metastackrd`)](#backend-administration-metastackrd)
   - [Database Schema & Optimistic Locking](#database-schema--optimistic-locking)
   - [GitHub Webhooks Setup](#github-webhooks-setup)
   - [Privacy-by-Default (Opt-In Code Access)](#privacy-by-default-opt-in-code-access)
5. [Extensions & IDE Plugins](#extensions--ide-plugins)
   - [Chrome Extension](#chrome-extension)
   - [VS Code Extension](#vs-code-extension)
   - [JetBrains Plugin](#jetbrains-plugin)
6. [Troubleshooting & Partial Failure Recovery](#troubleshooting--partial-failure-recovery)

---

## Concepts & Terminology

- **Meta-Repository (Meta-Repo)**: A parent Git repository containing multiple submodules configured via `.gitmodules`.
- **Submodule / Child Repo**: An independent Git repository tracked inside the meta-repo at a specific commit pointer.
- **Meta PR**: A GitHub pull request opened against the root meta-repository.
- **Child PR**: A GitHub pull request opened against a child repository matching the feature branch of the Meta PR.
- **Cascade Merge**: An automated, topological merge protocol that merges child PRs in parallel depth batches before bumping submodule pointers in dependent repositories and finally merging the Meta PR.

---

## CLI Command Reference (`git-meta`)

Make sure `git-meta` is in your system `PATH` (run `make install` to copy to `/usr/local/bin`).

### `git meta status`

Displays local submodule drift (uncommitted changes, unpushed commits) side-by-side with remote PR approval and CI statuses.

```bash
git meta status [--server <server-url>]
```

**Output Example**:

```
⚡ MetaStackR Status

Meta Repo: org/meta-repo | Branch: feature/auth-v2

 Submodule Path      |  Local Branch  |  Local Drift   |  Child PR  |  Review      |  CI State 
------------------------------------------------------------------------------------------
 sub/auth-service    | feature/auth-v2| DIRTY          | #42        | ✅ APPROVED  | ✅ SUCCESS
 sub/ui-app          | feature/auth-v2| CLEAN          | #18        | ⏳ PENDING   | ⏳ PENDING

Backend Meta PR Status: SYNCING (Lock Version: 3)
```

---

### `git meta checkout`

Safely creates or switches branches across the parent meta-repo and all tracked submodules.

```bash
# Switch to existing branch
git meta checkout feature/my-feature

# Create and switch to new branch across all submodules
git meta checkout -b feature/my-feature
```

---

### `git meta commit`

Creates coordinated atomic commits across all modified submodules and updates the parent meta-repo commit pointers in a single operation.

```bash
git meta commit -m "<commit-message>"
```

---

### `git meta push`

Enforces **bottom-up pushing**. It inspects all submodules, pushes dirty submodule commits to their respective remote origins first, and only updates parent meta-repo commit pointers once all submodules are pushed.

```bash
git meta push
```

> **Why Bottom-Up?** If you push a meta-repo commit pointer without pushing the submodule origin first, other developers will get broken/dangling commit references (`fatal: reference is not a tree`). `git meta push` eliminates this risk.

---

### `git meta sync`

Fetches `origin/main`, fast-forwards/rebases local submodules, and aligns root pointers to keep your multi-repo workspace synchronized.

```bash
git meta sync
```

---

### `git meta rebase`

Conducts a two-phase rebase: rebases child submodules first against the target upstream branch, then updates parent meta-repo references.

```bash
git meta rebase <upstream-branch>
```

---

### `git meta retry-merge`

Triggers a retry of the cascade merge engine on a Meta PR that entered `FAILED_PARTIAL` status (due to a transient network issue or resolved merge conflict).

```bash
git meta retry-merge --pr <pr-number> [--server <server-url>]
```

---

### `git meta install-hooks`

Installs `post-checkout` and `pre-commit` Git hooks into your workspace's `.git/hooks` directory to automatically warn developers about detached HEAD states or unaligned submodule branches.

```bash
git meta install-hooks
```

---

### `git meta init`

Initializes and onboards a repository to MetaStackr by registering with the backend server, installing local Git hooks, and setting up GitHub webhooks.

```bash
git meta init [--server <server-url>] [--url <webhook-url>] [--secret <secret>] [--allow-code-pull]
```

---

### `git meta setup-webhook`

Automates repository webhook registration with GitHub.

```bash
git meta setup-webhook [--url <webhook-url>] [--secret <secret>]
```

---

### `git meta agents`

Prints machine-readable guidelines and operation rules for AI coding agents operating in MetaStackr workspaces.

```bash
git meta agents [--json]
```

---

### `git meta version`

Prints the current semantic version of the `git-meta` CLI binary.

```bash
git meta version [--json]
# Or using the root flag:
git meta --version
```

---

## Developer Workflow Scenario

### Step 1: Create a Feature Branch
```bash
cd /path/to/meta-repo
git meta checkout -b feature/user-billing
```

### Step 2: Make Changes Across Submodules
Make changes inside submodules (`sub/billing-api`, `sub/frontend`) and commit locally within each submodule:

```bash
cd sub/billing-api
git commit -am "feat: add stripe integration"
cd ../frontend
git commit -am "feat: add checkout form"
cd ../..
```

### Step 3: Check Local Drift
```bash
git meta status
```

### Step 4: Perform Bottom-Up Push
```bash
git meta push
```

### Step 5: Open Meta PR & Child PRs
Open pull requests on GitHub for `sub/billing-api`, `sub/frontend`, and `meta-repo`. MetaStackr will synthesize the tree and maintain a single GitHub Check Run named `meta-repo/sync`.

---

## Backend Administration (`metastackrd`)

### Database Schema & Optimistic Locking

`metastackrd` requires PostgreSQL (v14+). The daemon executes embedded SQL migrations automatically on start up.

Key Database Tables:
- `meta_prs`: Meta PR status and `lock_version`.
- `child_prs`: Child PR states, review state, CI state, merged SHAs.
- `child_pr_dependencies`: DAG relationship edges.
- `merge_audit_logs`: Audit trail of merge events and saga operations.

### GitHub Webhooks Setup

1. In your GitHub organization or repository settings, navigate to **Webhooks** $\rightarrow$ **Add webhook**.
2. Set **Payload URL** to `https://your-domain.com/webhooks/github`.
3. Set **Content type** to `application/json`.
4. Enter your secret in **Secret** and set `WEBHOOK_SECRET` on `metastackrd`.
5. Select events:
   - `Pull requests`
   - `Pull request reviews`
   - `Check runs`
   - `Workflow runs`

### Privacy-by-Default (Opt-In Code Access)

By default, `metastackrd` operates on branch metadata and Git SHAs without caching, pulling, or cloning repository source code files. This guarantees that your source code remains completely private.

To authorize local code access features, toggle `allow_code_pull` to `true` when registering your tracked repository (via `git meta init --allow-code-pull` or in repository onboarding configuration):

```yaml
# repository configuration example
name: MetaStackrConfig
allow_code_pull: false  # Change to true to opt-in to code analysis
```

**What you get if you opt in:**
1. **Static Import Analysis**: Allows the server to inspect source files to automatically build and sort topological dependency DAGs based on code imports.
2. **Local Merge Dry-Runs**: Executes dry-run merges on the server to catch file conflicts early and flag `FAILED_DRIFT` before pushing updates to GitHub.
3. **Advanced Line-Level Diff Warnings**: Generates detailed warnings inside GitHub Check Runs highlighting exact code-level alignment mismatches.

---

## Extensions & IDE Plugins

MetaStackr includes browser extensions and IDE plugins located in the `extensions/` directory.

### Build All Extensions

You can package all extensions at once using the root Makefile target:

```bash
make build-extensions
```

---

### Chrome Extension

Located in `extensions/chrome/`. It integrates directly into GitHub Pull Request UI pages to show live submodule tree status and trigger cascade merges.

**Development / Unpacked Loading:**
1. Open Chrome and navigate to `chrome://extensions`.
2. Enable **Developer mode** in the top right toggle.
3. Click **Load unpacked** and select the `extensions/chrome` directory.

**Packaging for Distribution:**
```bash
make build-chrome
# Output: metastackr-chrome.zip
```

---

### VS Code Extension

Located in `extensions/vscode/`. It registers a unified Source Control Manager (SCM) provider, status bar item, and commands (`metastackr.commit`, `metastackr.checkout`, `metastackr.sync`).

**Build & Package (.vsix):**
```bash
make build-vscode
# Output: metastackr-vscode.vsix
```

**Install in VS Code:**
```bash
code --install-extension metastackr-vscode.vsix
```

---

### JetBrains Plugin

Located in `extensions/jetbrains/`. Built using Kotlin and Gradle for IntelliJ IDEA, PyCharm, WebStorm, and GoLand.

**Build Plugin Archive:**
```bash
make build-jetbrains
# Output: extensions/jetbrains/build/distributions/
```

---

## Troubleshooting & Partial Failure Recovery

If a child PR fails to merge (e.g. merge conflict):
1. MetaStackr halts the cascade merge immediately.
2. The Meta PR status is set to `FAILED_PARTIAL`. Previously merged child PRs on base branches are **NOT** force-reverted.
3. The exact error is logged to `merge_audit_logs`.
4. The developer resolves the conflict in the failing child PR and merges it manually or updates the PR branch.
5. The developer runs `git meta retry-merge --pr <PR_NUMBER>`. MetaStackr will detect already-merged child PRs, skip them, and resume the cascade merge from the unmerged node.
