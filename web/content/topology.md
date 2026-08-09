---
tag: // Topological Saga Merge Engine
title: Interactive DAG Cascade Simulator
---

## Interactive DAG Cascade Simulator

Watch how MetaStackr parses submodule dependencies into a Directed Acyclic Graph, executes parallel depth-batch merges, and handles partial conflicts without force-reverting base branches.

### Execution Phases:
1. **State Locking**: Lock Meta PR and Child PR states using optimistic `lock_version` updates.
2. **Topological Batching**: Calculate parallel depth levels ($Depth_0, Depth_1, \dots$).
3. **Saga Merge Execution**: Merge child PRs in parallel per depth level.
4. **Submodule Pointer Bumping**: Automatically update submodule commit pointers in meta-repo.
5. **Root PR Completion**: Complete the root Meta PR merge.
