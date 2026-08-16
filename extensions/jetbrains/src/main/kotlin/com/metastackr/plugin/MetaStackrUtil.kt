package com.metastackr.plugin

import com.intellij.openapi.project.Project
import java.io.BufferedReader
import java.io.File
import java.io.InputStreamReader

data class FileStatusItem(
    val path: String,
    val fullPath: String,
    val submodule: String?,
    val stagedCode: Char?,
    val unstagedCode: Char?,
    val isStaged: Boolean
)

object MetaStackrUtil {

    fun isInitialized(project: Project): Boolean {
        val basePath = project.basePath ?: return false

        // 1. Check git config metastackr.initialized
        try {
            val out = execGit(project, listOf("config", "--get", "metastackr.initialized"), basePath)
            if (out.trim() == "true") return true
        } catch (e: Exception) {}

        // 2. Check AGENTS.md
        val agentsFile = File(basePath, "AGENTS.md")
        if (agentsFile.exists()) {
            val content = agentsFile.readText()
            if (content.contains("MetaStackr") || content.contains("git-meta") || content.contains("git meta")) {
                return true
            }
        }

        // 3. Check post-checkout hook
        val hookFile = File(basePath, ".git/hooks/post-checkout")
        if (hookFile.exists()) {
            val content = hookFile.readText()
            if (content.contains("git-meta") || content.contains("git meta")) {
                return true
            }
        }

        return false
    }

    fun hasSubmodules(project: Project): Boolean {
        val basePath = project.basePath ?: return false
        val gitmodules = File(basePath, ".gitmodules")
        return gitmodules.exists()
    }

    fun getSubmodules(project: Project): List<String> {
        val basePath = project.basePath ?: return emptyList()
        val gitmodules = File(basePath, ".gitmodules")
        if (!gitmodules.exists()) return emptyList()

        val paths = mutableListOf<String>()
        val regex = Regex("""path\s*=\s*(.+)""")
        gitmodules.readLines().forEach { line ->
            val match = regex.find(line.trim())
            if (match != null) {
                paths.add(match.groupValues[1].trim())
            }
        }
        return paths
    }

    fun execGit(project: Project, args: List<String>, workingDir: String? = null): String {
        val cwd = workingDir ?: project.basePath ?: return ""
        val cmd = mutableListOf("git")
        cmd.addAll(args)

        val process = ProcessBuilder(cmd)
            .directory(File(cwd))
            .redirectErrorStream(true)
            .start()

        val reader = BufferedReader(InputStreamReader(process.inputStream))
        val output = reader.readText()
        process.waitFor()
        return output
    }

    fun runGitMeta(project: Project, args: List<String>): String {
        val basePath = project.basePath ?: return ""
        val cmd = mutableListOf("git", "meta")
        cmd.addAll(args)
        cmd.add("--json")

        return try {
            val process = ProcessBuilder(cmd)
                .directory(File(basePath))
                .redirectErrorStream(true)
                .start()

            val reader = BufferedReader(InputStreamReader(process.inputStream))
            val output = reader.readText()
            process.waitFor()
            output
        } catch (e: Exception) {
            // Fallback to git-meta binary
            val fallbackCmd = mutableListOf("git-meta")
            fallbackCmd.addAll(args)
            fallbackCmd.add("--json")
            val process = ProcessBuilder(fallbackCmd)
                .directory(File(basePath))
                .redirectErrorStream(true)
                .start()
            val reader = BufferedReader(InputStreamReader(process.inputStream))
            val output = reader.readText()
            process.waitFor()
            output
        }
    }

    fun getAllWorktreeChanges(project: Project): Pair<List<FileStatusItem>, List<FileStatusItem>> {
        val basePath = project.basePath ?: return Pair(emptyList(), emptyList())
        val staged = mutableListOf<FileStatusItem>()
        val unstaged = mutableListOf<FileStatusItem>()

        val submodules = getSubmodules(project)

        // 1. Root changes
        parsePorcelain(basePath, null).forEach { item ->
            if (item.stagedCode != null) staged.add(item)
            if (item.unstagedCode != null) unstaged.add(item)
        }

        // 2. Submodule changes
        for (sub in submodules) {
            val subDir = File(basePath, sub)
            if (subDir.exists()) {
                parsePorcelain(subDir.absolutePath, sub).forEach { item ->
                    if (item.stagedCode != null) staged.add(item)
                    if (item.unstagedCode != null) unstaged.add(item)
                }
            }
        }

        return Pair(staged, unstaged)
    }

    private fun parsePorcelain(cwd: String, submodule: String?): List<FileStatusItem> {
        val items = mutableListOf<FileStatusItem>()
        val output = try {
            val process = ProcessBuilder("git", "status", "--porcelain=v1", "-u")
                .directory(File(cwd))
                .start()
            val reader = BufferedReader(InputStreamReader(process.inputStream))
            val text = reader.readText()
            process.waitFor()
            text
        } catch (e: Exception) {
            return emptyList()
        }

        output.lines().forEach { line ->
            if (line.length >= 3) {
                val stagedChar = line[0]
                val unstagedChar = line[1]
                var rawPath = line.substring(3).trim()
                if (rawPath.contains(" -> ")) {
                    rawPath = rawPath.split(" -> ")[1].trim()
                }
                if (rawPath.startsWith("\"") && rawPath.endsWith("\"")) {
                    rawPath = rawPath.substring(1, rawPath.length - 1)
                }

                val fullPath = File(cwd, rawPath).absolutePath
                val displayPath = if (submodule != null) "[$submodule] $rawPath" else rawPath

                val hasStaged = stagedChar != ' ' && stagedChar != '?'
                val hasUnstaged = unstagedChar != ' ' || stagedChar == '?'

                if (hasStaged) {
                    items.add(
                        FileStatusItem(
                            path = displayPath,
                            fullPath = fullPath,
                            submodule = submodule,
                            stagedCode = stagedChar,
                            unstagedCode = null,
                            isStaged = true
                        )
                    )
                }
                if (hasUnstaged) {
                    items.add(
                        FileStatusItem(
                            path = displayPath,
                            fullPath = fullPath,
                            submodule = submodule,
                            stagedCode = null,
                            unstagedCode = if (stagedChar == '?') '?' else unstagedChar,
                            isStaged = false
                        )
                    )
                }
            }
        }

        return items
    }
}
