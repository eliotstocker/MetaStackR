# MetaStackr Configuration Details

MetaStackr coordinates multi-repo submodules using a config file. Below is the configuration structure and edge definitions.

## Schema Specifications

- **name**: Name of the parent meta-repository.
- **allowCodePull**: Boolean flag (default `false`) indicating if the orchestration server is authorized to pull/clone repository source code.
- **submodules**: List of tracked submodules.
  - **path**: The filesystem path relative to the root meta-repository.
  - **repo**: The remote GitHub repository identifier (e.g. `org/sub-repo`).
  - **dependsOn**: Array of submodule paths that must be merged/processed BEFORE this submodule.

## Opt-In Code Access Benefits

When `allowCodePull` is enabled (`true`), the backend gains permission to cache and inspect files, unlocking the following features:
1. **Static Import Dependency Analysis**: Auto-builds topological DAGs by scanning file imports directly.
2. **Local Dry-Run Reconciliations**: Simulates merges locally to detect conflicts and raise warnings before writing pointer updates back to remote origins.
3. **Check Run Diff Integration**: Formats detailed warnings in the check run status showing exact line differences in mismatching submodule files.

