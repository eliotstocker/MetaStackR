# MetaStackr (git-meta)

**MetaStackr** is an event-driven meta-repository orchestration system built in **Go 1.25** with a **PostgreSQL** backend and a custom **CLI (`git-meta`)**.

It enables developers to work transparently across a root meta-repository containing multiple Git submodules. Developers operate locally using familiar Git workflows, while MetaStackr automatically creates, tracks, synchronizes, and atomically merges child pull requests across sub-repositories on GitHub.

---

## Key Features

- **Standardized Local Workflow (`git-meta`)**:
  - `git meta status`: Interrogates local submodule drift (uncommitted changes, unpushed commits) merged with live remote PR/CI status via terminal tables.
  - `git meta checkout <branch>`: Safely creates or switches branches across parent meta-repo and all submodules.
  - `git meta push`: Enforces bottom-up pushing (pushes submodule origins before parent commit pointers).
  - `git meta retry-merge`: Re-triggers cascade merges on partially failed PRs.
  - `git meta install-hooks`: Installs `post-checkout` and `pre-commit` hooks into `.git/hooks`.

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

### 1. Build & Install CLI

```bash
make build
make install # Installs git-meta to /usr/local/bin/git-meta
```

Now `git meta` works natively in your terminal!

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

## Documentation & Guides

- 📖 **[Detailed User Guide](USER_GUIDE.md)**: Comprehensive operational guide, CLI reference, and troubleshooting.
- 🤖 **[Agent Instructions (AGENTS.md)](AGENTS.md)**: Workspace rules and context for AI coding agents.

---

## License

MIT
