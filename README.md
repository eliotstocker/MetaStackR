# MetaStackr (git-meta)

**MetaStackr** is an event-driven meta-repository orchestration system built in **Go 1.25** with a **PostgreSQL** backend and a custom **CLI (`git-meta`)**.

It enables developers to work transparently across a root meta-repository containing multiple Git submodules. Developers operate locally using familiar Git workflows, while MetaStackr automatically creates, tracks, synchronizes, and atomically merges child pull requests across sub-repositories on GitHub.

---

## Key Features

- **Standardized Local Workflow (`git-meta`)**:
  - `git meta status`: Interrogates local submodule drift (uncommitted changes, unpushed commits) merged with live remote PR/CI status via terminal tables.
  - `git meta checkout [-b] <branch>`: Safely creates or switches branches across parent meta-repo and all submodules.
  - `git meta commit -m "<msg>"`: Creates coordinated atomic commits across all modified submodules and updates parent commit pointers.
  - `git meta push`: Enforces bottom-up pushing (pushes submodule origins before parent commit pointers).
  - `git meta create-pr`: Opens or creates GitHub Pull Requests across all modified submodules and meta-repo.
  - `git meta sync`: Fetches `origin/main`, fast-forwards/rebases local submodules, and aligns root pointers.
  - `git meta rebase <upstream>`: Conducts a two-phase rebase: rebases child submodules first, then parent meta-repo references.
  - `git meta retry-merge --pr <pr-number>`: Re-triggers cascade merges on partially failed PRs.
  - `git meta install-hooks`: Installs `post-checkout` and `pre-commit` hooks into `.git/hooks`.
  - `git meta init`: Onboards repository with backend registration, Git hooks installation, and GitHub webhooks setup.
  - `git meta setup-webhook`: Automates repository webhook registration with GitHub.
  - `git meta agents`: Displays guidelines and machine-readable instructions for AI agents.
  - `git meta version`: Displays semantic version information.

- **Event-Driven Orchestration Daemon (`metastackrd`)**:
  - **Webhook Ingestion**: HMAC-SHA256 signature verification (`X-Hub-Signature-256`) processing `pull_request`, `pull_request_review`, `check_run`, and `workflow_run` events.
  - **Single GitHub Check Run (`meta-repo/sync`)**: Posts and continuously updates a markdown matrix table on the Meta PR without creating comment noise.
  - **Topological DAG Engine & Cycle Detection**: Analyzes submodule dependencies into a Directed Acyclic Graph (DAG) and aborts if circular dependencies ($A \rightarrow B \rightarrow A$) are detected.
  - **Saga Cascade Merge & Optimistic Locking**: Uses SQL `lock_version` to prevent worker race conditions. Merges child PRs topologically in parallel batches, updates submodule pointers, and handles partial failures gracefully with retry resumption.

---

## Privacy-by-Default (Opt-In Code Access)

By default, the MetaStackr server is **data-blind**. It does not pull, clone, or cache your repository source code, orchestrating merges exclusively via branch metadata, commit SHAs, and GitHub APIs.

### Opting In to Code Access
To enable server-side code access, toggle `allow_code_pull` to `true` in your repository setup.

**What you get if you opt in:**
1. **Static Dependency Analysis**: Allows the server to inspect source files to automatically build and sort topological dependency DAGs based on code imports.
2. **Local Merge Dry-Runs**: Executes dry-run merges on the server to catch file conflicts early and flag `FAILED_DRIFT` before pushing updates to GitHub.
3. **Advanced Line-Level Diff Warnings**: Generates detailed warnings inside GitHub Check Runs highlighting exact code-level alignment mismatches.

---

## Quick Start

### 1. Install CLI (`git-meta`)

**One-Line Installation Script (macOS / Linux):**

```bash
curl -fsSL https://raw.githubusercontent.com/eliotstocker/MetaStackR/main/install.sh | bash
```

**Or Build & Install from Source:**

```bash
make build
make install # Installs git-meta to /usr/local/bin/git-meta
make build-extensions # (Optional) Package Chrome, VS Code, & JetBrains extensions
```

Now `git meta` works natively in your terminal!

**Shell Autocompletion:**

Enable tab-completion for `git meta` and `git-meta` by adding one of the following to your shell profile:

```bash
# Zsh (~/.zshrc)
source <(git meta completion zsh)

# Bash (~/.bashrc)
source <(git meta completion bash)
```

### 2. Run Backend Daemon (`metastackrd`)

Set your database and GitHub credentials:

```bash
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/metastackr?sslmode=disable"
export WEBHOOK_SECRET="your-github-webhook-secret"
export GH_TOKEN="ghp_your_github_token"

./metastackrd
```

The daemon will automatically execute embedded database migrations on startup and start listening on port `:8080`.

---

## Architecture Overview

```text
┌─────────────────────────────────────────────────────────────────────────┐
│                           LOCAL WORKSPACE                               │
│  - git-meta CLI (Thin client querying backend / local Git state)        │
│  - Git Hooks (post-checkout, pre-commit) for local submodule drift      │
└────────────────────────────────────┬────────────────────────────────────┘
                                     │ HTTP Status Query / Git Push
                                     v
┌─────────────────────────────────────────────────────────────────────────┐
│                       ORCHESTRATION BACKEND (GO)                        │
│                                                                         │
│  ┌───────────────────────┐             ┌─────────────────────────────┐  │
│  │ Webhook Ingestion     ├────────────>│ State & Reconciliation      │  │
│  │ Normalize GitHub Data │             │ DAG Engine (PostgreSQL)     │  │
│  └───────────────────────┘             └──────────────┬──────────────┘  │
│                                                       │                 │
│                                                       v                 │
│                                        ┌─────────────────────────────┐  │
│                                        │ Execution Engine            │  │
│                                        │ Cascade Merges / Check Runs │  │
│                                        └─────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Multi-Component Semantic Release

MetaStackr uses independent per-component Semantic Release workflows based on conventional commit scopes:

| Component | Scope Format | Tag Format | Artifacts |
|---|---|---|---|
| **Core (`git-meta` & `metastackrd`)** | `feat(core): ...`, `fix(cli): ...`, `fix(daemon): ...` (or unscoped) | `v1.x.x` | `git-meta`, `metastackrd` |
| **VS Code Extension** | `feat(vscode): ...`, `fix(vscode): ...` | `vscode-v1.x.x` | `metastackr-vscode.vsix` |
| **Chrome Extension** | `feat(chrome): ...`, `fix(chrome): ...` | `chrome-v1.x.x` | `metastackr-chrome.zip` |
| **JetBrains Plugin** | `feat(jetbrains): ...`, `fix(jetbrains): ...` | `jetbrains-v1.x.x` | `metastackr-jetbrains.zip` |

To trigger a release for a specific component, prefix your commit message with the corresponding scope.

---

## Documentation & Guides

- 📖 **[Detailed User Guide](USER_GUIDE.md)**: Comprehensive operational guide, CLI reference, and troubleshooting.
- 🤖 **[Agent Instructions (AGENTS.md)](AGENTS.md)**: Workspace rules and context for AI coding agents.

---

## License

MIT
