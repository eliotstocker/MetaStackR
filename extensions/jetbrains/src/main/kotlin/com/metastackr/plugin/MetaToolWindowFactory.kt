package com.metastackr.plugin

import com.intellij.openapi.project.Project
import com.intellij.openapi.wm.ToolWindow
import com.intellij.openapi.wm.ToolWindowFactory
import com.intellij.ui.content.ContentFactory
import javax.swing.*
import java.awt.*

class MetaToolWindowFactory : ToolWindowFactory {
    override fun createToolWindowContent(project: Project, toolWindow: ToolWindow) {
        val panel = MetaToolWindowPanel(project)
        val contentFactory = ContentFactory.getInstance()
        val content = contentFactory.createContent(panel, "", false)
        toolWindow.contentManager.addContent(content)
    }
}

class MetaToolWindowPanel(private val project: Project) : JPanel(BorderLayout()) {
    private val stagedListModel = DefaultListModel<String>()
    private val stagedList = JList(stagedListModel)
    private val changesListModel = DefaultListModel<String>()
    private val changesList = JList(changesListModel)
    private val commitField = JTextField()
    private val statusLabel = JLabel("Ready")

    init {
        buildUI()
        refresh()
    }

    private fun buildUI() {
        removeAll()

        if (!MetaStackrUtil.isInitialized(project)) {
            // Uninitialized View
            val uninitPanel = JPanel(GridBagLayout())
            val gbc = GridBagConstraints()
            gbc.gridwidth = GridBagConstraints.REMAINDER
            gbc.insets = Insets(10, 10, 10, 10)

            val label = JLabel("<html><center><h3>MetaStackr Multi-Repo Orchestrator</h3><p>This repository is not initialized with MetaStackr.</p></center></html>")
            val initBtn = JButton("🚀 Initialize MetaStackr")
            initBtn.addActionListener {
                MetaStackrUtil.runGitMeta(project, listOf("init"))
                try {
                    MetaStackrUtil.execGit(project, listOf("config", "metastackr.initialized", "true"))
                } catch (e: Exception) {}
                buildUI()
                refresh()
            }

            uninitPanel.add(label, gbc)
            uninitPanel.add(initBtn, gbc)
            add(uninitPanel, BorderLayout.CENTER)
            revalidate()
            repaint()
            return
        }

        // Toolbar
        val toolbar = JToolBar()
        toolbar.isFloatable = false

        val stageAllBtn = JButton("+ Stage All")
        stageAllBtn.addActionListener {
            MetaStageAllAction().actionPerformed(null as? com.intellij.openapi.actionSystem.AnActionEvent ?: return@addActionListener)
            refresh()
        }

        val unstageAllBtn = JButton("- Unstage All")
        unstageAllBtn.addActionListener {
            MetaUnstageAllAction().actionPerformed(null as? com.intellij.openapi.actionSystem.AnActionEvent ?: return@addActionListener)
            refresh()
        }

        val pushBtn = JButton("⬆ Push")
        pushBtn.addActionListener {
            val out = MetaStackrUtil.runGitMeta(project, listOf("push"))
            JOptionPane.showMessageDialog(this, "Push output:\n$out", "MetaStackr Push", JOptionPane.INFORMATION_MESSAGE)
            refresh()
        }

        val syncBtn = JButton("🔄 Sync")
        syncBtn.addActionListener {
            val out = MetaStackrUtil.runGitMeta(project, listOf("sync"))
            JOptionPane.showMessageDialog(this, "Sync output:\n$out", "MetaStackr Sync", JOptionPane.INFORMATION_MESSAGE)
            refresh()
        }

        val refreshBtn = JButton("↻ Refresh")
        refreshBtn.addActionListener { refresh() }

        toolbar.add(stageAllBtn)
        toolbar.add(unstageAllBtn)
        toolbar.addSeparator()
        toolbar.add(pushBtn)
        toolbar.add(syncBtn)
        toolbar.add(refreshBtn)

        add(toolbar, BorderLayout.NORTH)

        // Center Split (Staged vs Changes)
        val centerPanel = JPanel(GridLayout(2, 1, 6, 6))

        val stagedPanel = JPanel(BorderLayout())
        stagedPanel.border = BorderFactory.createTitledBorder("Staged Changes")
        stagedPanel.add(JScrollPane(stagedList), BorderLayout.CENTER)

        val changesPanel = JPanel(BorderLayout())
        changesPanel.border = BorderFactory.createTitledBorder("Changes (Worktree & Submodules)")
        changesPanel.add(JScrollPane(changesList), BorderLayout.CENTER)

        centerPanel.add(stagedPanel)
        centerPanel.add(changesPanel)
        add(centerPanel, BorderLayout.CENTER)

        // Bottom Commit Area
        val bottomPanel = JPanel(BorderLayout(6, 6))
        bottomPanel.border = BorderFactory.createEmptyBorder(6, 6, 6, 6)

        val commitInputPanel = JPanel(BorderLayout(4, 4))
        commitInputPanel.add(JLabel("Commit Msg:"), BorderLayout.WEST)
        commitInputPanel.add(commitField, BorderLayout.CENTER)

        val commitBtn = JButton("✔ Commit (MetaStackr)")
        commitBtn.addActionListener {
            val msg = commitField.text.trim()
            if (msg.isEmpty()) {
                JOptionPane.showMessageDialog(this, "Please enter a commit message", "Error", JOptionPane.ERROR_MESSAGE)
                return@addActionListener
            }

            val (staged, _) = MetaStackrUtil.getAllWorktreeChanges(project)
            val args = mutableListOf("commit", "-m", msg)
            if (staged.isNotEmpty()) {
                args.add("--staged")
            }

            val out = MetaStackrUtil.runGitMeta(project, args)
            commitField.text = ""
            JOptionPane.showMessageDialog(
                this,
                if (staged.isNotEmpty()) "✅ Staged commit succeeded (submodule pointers aligned)!" else "✅ Atomic commit completed across all submodules!",
                "MetaStackr Success",
                JOptionPane.INFORMATION_MESSAGE
            )
            refresh()
        }
        commitInputPanel.add(commitBtn, BorderLayout.EAST)

        bottomPanel.add(commitInputPanel, BorderLayout.NORTH)
        bottomPanel.add(statusLabel, BorderLayout.SOUTH)

        add(bottomPanel, BorderLayout.SOUTH)
        revalidate()
        repaint()
    }

    private fun refresh() {
        if (!MetaStackrUtil.isInitialized(project)) {
            buildUI()
            return
        }

        stagedListModel.clear()
        changesListModel.clear()

        val (staged, unstaged) = MetaStackrUtil.getAllWorktreeChanges(project)

        staged.forEach { item ->
            val code = item.stagedCode ?: 'M'
            stagedListModel.addElement("[$code] ${item.path}")
        }

        unstaged.forEach { item ->
            val code = item.unstagedCode ?: 'M'
            changesListModel.addElement("[$code] ${item.path}")
        }

        statusLabel.text = "Status: ${staged.size} staged, ${unstaged.size} unstaged changes"
    }
}
