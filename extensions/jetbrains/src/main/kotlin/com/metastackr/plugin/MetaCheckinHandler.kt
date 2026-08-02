package com.metastackr.plugin

import com.intellij.openapi.project.Project
import com.intellij.openapi.vcs.CheckinProjectPanel
import com.intellij.openapi.vcs.changes.CommitContext
import com.intellij.openapi.vcs.checkin.CheckinHandler
import com.intellij.openapi.vcs.checkin.CheckinHandlerFactory
import com.intellij.openapi.vcs.ui.RefreshableOnComponent
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
        // Warn developers attempting direct uncoordinated commits
        val response = Messages.showOkCancelDialog(
            project,
            "Direct commit detected. MetaStackr recommends using coordinated atomic commits via 'git-meta commit' to prevent branch pointer drift.",
            "MetaStackr Coordinated Commit Recommendation",
            "Commit Anyway",
            "Cancel Commit",
            Messages.getWarningIcon()
        )

        return if (response == Messages.OK) {
            ReturnResult.COMMIT
        } else {
            ReturnResult.CANCEL
        }
    }
}

class MetaVCSListener : GitRepositoryChangeListener {
    override fun repositoryChanged(repository: GitRepository) {
        val project = repository.project
        val branch = repository.currentBranchName ?: return
        
        // Notify developer if branches drift between submodules
        println("[MetaStackr] Submodule repository updated: ${repository.root.path} -> Branch: $branch")
    }
}
