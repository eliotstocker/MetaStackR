package com.metastackr.plugin

import com.intellij.openapi.project.Project
import com.intellij.openapi.startup.StartupActivity
import com.intellij.openapi.vcs.AbstractVcs
import com.intellij.openapi.vcs.ProjectLevelVcsManager
import com.intellij.openapi.vcs.VcsDirectoryMapping
import com.intellij.openapi.vcs.VcsException
import com.intellij.openapi.vcs.VcsKey
import com.intellij.openapi.vcs.FilePath
import com.intellij.openapi.vcs.changes.*
import com.intellij.openapi.vcs.checkin.CheckinEnvironment
import com.intellij.openapi.vfs.LocalFileSystem
import com.intellij.openapi.vfs.VirtualFile
import com.intellij.openapi.progress.ProgressIndicator
import com.intellij.vcsUtil.VcsUtil
import java.io.File

class MetaStackrVcs(project: Project) : AbstractVcs(project, NAME) {
    companion object {
        const val NAME = "MetaStackr"
        const val DISPLAY_NAME = "MetaStackr"
        val KEY = VcsKey(NAME)

        fun getInstance(project: Project): MetaStackrVcs? {
            return ProjectLevelVcsManager.getInstance(project).findVcsByName(NAME) as? MetaStackrVcs
        }

        fun switchToMetaStackr(project: Project) {
            val vcsManager = ProjectLevelVcsManager.getInstance(project)
            val basePath = project.basePath ?: return

            // Map project root to MetaStackr, taking over from Git
            val newMappings = listOf(VcsDirectoryMapping(basePath, NAME))
            vcsManager.directoryMappings = newMappings
            println("[MetaStackr] Switched Project VCS from Git to MetaStackr (active)")
        }

        fun restoreGit(project: Project) {
            val vcsManager = ProjectLevelVcsManager.getInstance(project)
            val basePath = project.basePath ?: return
            val newMappings = listOf(VcsDirectoryMapping(basePath, "Git"))
            vcsManager.directoryMappings = newMappings
            println("[MetaStackr] Restored Project VCS to native Git")
        }
    }

    private val changeProvider = MetaChangeProvider(project)
    private val checkinEnvironment = MetaCheckinEnvironment(project)

    override fun getName(): String = NAME
    override fun getDisplayName(): String = DISPLAY_NAME
    override fun getChangeProvider(): ChangeProvider = changeProvider
    override fun getCheckinEnvironment(): CheckinEnvironment = checkinEnvironment

    override fun isVersionedDirectory(dir: VirtualFile?): Boolean {
        if (dir == null) return false
        return File(dir.path, ".git").exists() || File(dir.path, ".gitmodules").exists()
    }
}

class MetaChangeProvider(private val project: Project) : ChangeProvider {
    override fun getChanges(
        holder: ChangelistBuilder,
        progress: ProgressIndicator,
        changesBeforeUpdate: ChangeListManagerGate
    ) {
        val (staged, unstaged) = MetaStackrUtil.getAllWorktreeChanges(project)

        for (item in staged + unstaged) {
            val file = File(item.fullPath)
            val filePath = VcsUtil.getFilePath(file)

            val beforeRevision = if (item.stagedCode == 'A' || item.unstagedCode == '?') null else CurrentContentRevision(filePath)
            val afterRevision = if (item.stagedCode == 'D' || item.unstagedCode == 'D') null else CurrentContentRevision(filePath)

            val change = Change(beforeRevision, afterRevision)
            holder.processChange(change, MetaStackrVcs.KEY)
        }
    }

    override fun isModifiedDocumentTrackingRequired(): Boolean = false
    override fun doCleanup(files: MutableList<VirtualFile>?) {}
}

class MetaCheckinEnvironment(private val project: Project) : CheckinEnvironment {
    override fun commit(
        changes: MutableList<Change>,
        commitMessage: String,
        commitContext: CommitContext,
        feedback: MutableSet<String>?
    ): MutableList<VcsException>? {
        val (staged, _) = MetaStackrUtil.getAllWorktreeChanges(project)
        val args = mutableListOf("commit", "-m", commitMessage)
        if (staged.isNotEmpty()) {
            args.add("--staged")
        }

        try {
            MetaStackrUtil.runGitMeta(project, args)
        } catch (e: Exception) {
            return mutableListOf(VcsException("MetaStackr commit failed: ${e.message}"))
        }

        return null
    }

    override fun scheduleMissingFileForDeletion(files: MutableList<FilePath>?): MutableList<VcsException>? = null
    override fun scheduleUnversionedFilesForAddition(files: MutableList<VirtualFile>?): MutableList<VcsException>? = null
    override fun isCheckinPrepared(changes: MutableList<Change>?): Boolean = true
    override fun getHelpId(): String? = null
}

class MetaStartupActivity : StartupActivity {
    override fun runActivity(project: Project) {
        if (MetaStackrUtil.isInitialized(project)) {
            MetaStackrVcs.switchToMetaStackr(project)
        }
    }
}
