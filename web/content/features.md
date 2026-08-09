### Optimistic SQL Locking

Reconciliation loops enforce strict `lock_version` verification to eliminate race conditions on meta PR updates.

---

### Bottom-Up Pushes

CLI guarantees submodule changes are pushed to remote origins before updating parent pointer commits.

---

### DAG Cycle Detection

Builds a Directed Acyclic Graph of dependencies, aborting immediately with `ErrCycleDetected` if a loop occurs.

---

### Saga Cascade Merge

Executes merges topologically in parallel depth batches, halting on conflicts without force-reverting base branches.

---

### Single Checks Matrix

Maintains a single GitHub Check Run named `meta-repo/sync` with a clean markdown matrix table showing approvals and CI.

---

### Git Hooks Drift Check

Installs `post-checkout` and `pre-commit` hooks in `.git/hooks` to verify submodule alignment and detached HEAD states.

---

### Privacy-by-Default

Operates strictly on Git metadata (branches/SHAs) without pulling source code. Toggle `allowCodePull: true` to opt-in for dry-run merges and static analysis.

