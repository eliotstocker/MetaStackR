## Why tho?

The difference between traditional manual Git submodule pain and MetaStackr automation.

| Problem | Without MetaStackr | With MetaStackr |
| :--- | :--- | :--- |
| **Submodule Drift** | Detached HEAD states and forgotten pointer updates | Coordinated multi-repo checkouts and automated pointer bumping |
| **Merge Order & Safety** | Root PR merged before dependencies $\rightarrow$ broken CI | Topologically locked status checks & automated cascade merging |
| **Context Switching** | Manually opening, tracking & merging 4+ separate PRs | Single-command PR creation and unified multi-repo tracking |
| **AI Agent Friction** | Coding agents break submodule pointers with raw Git commands | Machine-readable agent rules (`git meta agents`) and deterministic `--json` CLI |
