package com.metastackr.plugin

import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.ui.Messages
import com.intellij.openapi.project.Project
import java.io.File

class MetaInitAction : AnAction("Initialize MetaStackr Repository", "Initialize repository with MetaStackr hooks and tracking", null) {
    override fun actionPerformed(e: AnActionEvent) {
        val project = e.project ?: return
        val output = MetaStackrUtil.runGitMeta(project, listOf("init"))
        
        try {
            MetaStackrUtil.execGit(project, listOf("config", "metastackr.initialized", "true"))
        } catch (ex: Exception) {}

        Messages.showInfoMessage(
            project,
            "MetaStackr initialized successfully!\n\n$output",
            "MetaStackr Initialized"
        )
    }

    override fun update(e: AnActionEvent) {
        val project = e.project
        e.presentation.isEnabledAndVisible = project != null
    }
}

class MetaCommitAction : AnAction("MetaStackr: Commit", "Atomic commit across all submodules or staged changes", null) {
    override fun actionPerformed(e: AnActionEvent) {
        val project = e.project ?: return
        if (!MetaStackrUtil.isInitialized(project)) {
            val init = Messages.showYesNoDialog(
                project,
                "This repository is not initialized with MetaStackr. Would you like to initialize now?",
                "MetaStackr Not Initialized",
                Messages.getQuestionIcon()
            )
            if (init == Messages.YES) {
                MetaInitAction().actionPerformed(e)
            }
            return
        }

        val (staged, unstaged) = MetaStackrUtil.getAllWorktreeChanges(project)
        if (staged.isEmpty() && unstaged.isEmpty()) {
            Messages.showInfoMessage(project, "No changes found across the worktree.", "MetaStackr Commit")
            return
        }

        val hasStaged = staged.isNotEmpty()
        val prompt = if (hasStaged) {
            "Enter commit message for ${staged.size} staged changes (submodule pointers will be auto-aligned):"
        } else {
            "Enter atomic commit message (all dirty submodules will be automatically staged):"
        }

        val message = Messages.showInputDialog(project, prompt, "MetaStackr Commit", Messages.getQuestionIcon())
        if (message.isNullOrBlank()) return

        val args = mutableListOf("commit", "-m", message)
        if (hasStaged) {
            args.add("--staged")
        }

        val output = MetaStackrUtil.runGitMeta(project, args)
        Messages.showInfoMessage(
            project,
            if (hasStaged) "✅ Staged commit completed and parent pointers updated!" else "✅ Atomic commit completed across all submodules!",
            "MetaStackr Success"
        )
    }

    override fun update(e: AnActionEvent) {
        val project = e.project
        e.presentation.isEnabled = project != null
    }
}

class MetaStageAllAction : AnAction("Stage All Changes", "Stage all changes across root and submodules with git add -A", null) {
    override fun actionPerformed(e: AnActionEvent) {
        val project = e.project ?: return
        val basePath = project.basePath ?: return

        MetaStackrUtil.execGit(project, listOf("add", "-A"), basePath)
        for (sub in MetaStackrUtil.getSubmodules(project)) {
            val subDir = File(basePath, sub)
            if (subDir.exists()) {
                MetaStackrUtil.execGit(project, listOf("add", "-A"), subDir.absolutePath)
            }
        }
        Messages.showInfoMessage(project, "Staged all changes across worktree.", "MetaStackr Staging")
    }
}

class MetaUnstageAllAction : AnAction("Unstage All Changes", "Unstage all changes across worktree", null) {
    override fun actionPerformed(e: AnActionEvent) {
        val project = e.project ?: return
        val basePath = project.basePath ?: return

        MetaStackrUtil.execGit(project, listOf("restore", "--staged", "."), basePath)
        for (sub in MetaStackrUtil.getSubmodules(project)) {
            val subDir = File(basePath, sub)
            if (subDir.exists()) {
                MetaStackrUtil.execGit(project, listOf("restore", "--staged", "."), subDir.absolutePath)
            }
        }
        Messages.showInfoMessage(project, "Unstaged all changes across worktree.", "MetaStackr Staging")
    }
}

class MetaPushAction : AnAction("MetaStackr: Push (Bottom-Up)", "Enforce bottom-up pushing across submodules and root", null) {
    override fun actionPerformed(e: AnActionEvent) {
        val project = e.project ?: return
        val output = MetaStackrUtil.runGitMeta(project, listOf("push"))
        Messages.showInfoMessage(project, "Bottom-up push completed!\n$output", "MetaStackr Push")
    }
}

class MetaSyncAction : AnAction("MetaStackr: Sync Submodules", "Fetch upstream and fast-forward submodules", null) {
    override fun actionPerformed(e: AnActionEvent) {
        val project = e.project ?: return
        val output = MetaStackrUtil.runGitMeta(project, listOf("sync"))
        Messages.showInfoMessage(project, "Submodules sync completed!\n$output", "MetaStackr Sync")
    }
}

class MetaCheckoutAction : AnAction("MetaStackr: Checkout Branch", "Switch or create branch system-wide across all submodules", null) {
    override fun actionPerformed(e: AnActionEvent) {
        val project = e.project ?: return
        val branch = Messages.showInputDialog(
            project,
            "Enter branch name to checkout system-wide:",
            "MetaStackr Checkout",
            Messages.getQuestionIcon()
        )
        if (branch.isNullOrBlank()) return

        val output = MetaStackrUtil.runGitMeta(project, listOf("checkout", branch))
        Messages.showInfoMessage(project, "Switched to branch '$branch' across all submodules!\n$output", "MetaStackr Checkout")
    }
}
