package com.metastackr.plugin

import com.intellij.openapi.project.Project
import com.intellij.openapi.vcs.CheckinProjectPanel
import com.intellij.openapi.vcs.changes.CommitContext
import com.intellij.openapi.vcs.checkin.CheckinHandler
import com.intellij.openapi.vcs.checkin.CheckinHandlerFactory
import com.intellij.openapi.ui.Messages
import git4idea.repo.GitRepository
import git4idea.repo.GitRepositoryChangeListener

class MetaCheckinHandlerFactory : CheckinHandlerFactory() {
    override fun createHandler(panel: CheckinProjectPanel, commitContext: CommitContext): CheckinHandler {
        return MetaCheckinHandler(panel.project)
    }
}

class MetaCheckinHandler(private val project: Project) : CheckinHandler() {
    override fun beforeCheckin(): ReturnResult {
        // Only intercept if the project IS initialized with MetaStackr
        if (!MetaStackrUtil.isInitialized(project)) {
            return ReturnResult.COMMIT
        }

        // Warn developers attempting uncoordinated commits in MetaStackr workspaces
        val response = Messages.showOkCancelDialog(
            project,
            "Direct uncoordinated Git commit detected in a MetaStackr workspace.\n\nMetaStackr recommends using 'git meta commit' (or 'git meta commit --staged') to prevent submodule pointer drift and broken refs.",
            "MetaStackr Coordinated Commit",
            "Commit via MetaStackr",
            "Cancel Commit",
            Messages.getWarningIcon()
        )

        return if (response == Messages.OK) {
            val (staged, _) = MetaStackrUtil.getAllWorktreeChanges(project)
            val msg = Messages.showInputDialog(
                project,
                "Enter commit message for MetaStackr:",
                "MetaStackr Coordinated Commit",
                Messages.getQuestionIcon()
            )
            if (!msg.isNullOrBlank()) {
                val args = mutableListOf("commit", "-m", msg)
                if (staged.isNotEmpty()) {
                    args.add("--staged")
                }
                MetaStackrUtil.runGitMeta(project, args)
                Messages.showInfoMessage(
                    project,
                    if (staged.isNotEmpty()) "✅ Staged commit completed and parent pointers updated!" else "✅ Atomic commit completed across all submodules!",
                    "MetaStackr Success"
                )
            }
            ReturnResult.CANCEL
        } else {
            ReturnResult.CANCEL
        }
    }
}

class MetaVCSListener : GitRepositoryChangeListener {
    override fun repositoryChanged(repository: GitRepository) {
        val project = repository.project
        if (!MetaStackrUtil.isInitialized(project)) return
        val branch = repository.currentBranchName ?: return
        
        println("[MetaStackr] Repository updated: ${repository.root.path} -> Branch: $branch")
    }
}
