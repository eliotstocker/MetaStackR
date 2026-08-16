## How MetaStackr Works

An end-to-end automated workflow from local branches to production merge.

---
### Push & Open Stacked PRs
- **Atomic Commits**: Modify child submodules and parent meta-repo simultaneously. Commit pointers align automatically.
- **Unified Push & PRs**: Push branches and open Pull Requests across all modified repositories with a single `git meta create-pr` command.
---
### Automatic Dependency Locking
- **Webhook Ingestion**: MetaStackr detects PR creation and links root meta PRs with their child submodule PRs.
- **Prevent Premature Merges**: Attaches a status check (`meta-repo/sync`) to block parent merges while child PRs remain open.
---
### Real-Time Pointer Synchronization
- **Track Submodule Merges**: When child PRs are merged, MetaStackr captures the new target commit SHAs.
- **Auto-Update Parent**: MetaStackr automatically bumps root submodule pointers to the new SHAs, eliminating detached HEADs.
---
### Safe, Autonomous Root Merge
- **Topological Merge**: As dependencies clear, MetaStackr cascades merges across the tree in topological order.
- **Zero-Touch Completion**: Auto-merges the parent meta PR once all children are merged, or removes the blocker check for final review.
