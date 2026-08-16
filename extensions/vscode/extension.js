const vscode = require('vscode');
const { exec } = require('child_process');
const path = require('path');
const fs = require('fs');

let statusBarItem;
let scmProvider = null;
let stagedGroup = null;
let changesGroup = null;
let isRefreshing = false;
let refreshDebounceTimer = null;
let fileWatcher = null;

async function activate(context) {
    console.log('MetaStackr VS Code Extension is activating...');

    // 1. Initialize Status Bar Item (available for all states)
    statusBarItem = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 100);
    context.subscriptions.push(statusBarItem);

    // 2. Register Initialize Command (always available)
    context.subscriptions.push(
        vscode.commands.registerCommand('metastackr.init', async () => {
            const rootPath = getWorkspaceRoot();
            if (!rootPath) {
                vscode.window.showErrorMessage('No workspace folder open to initialize.');
                return;
            }

            vscode.window.withProgress({
                location: vscode.ProgressLocation.Notification,
                title: 'Initializing MetaStackr...',
                cancellable: false
            }, async () => {
                return new Promise((resolve) => {
                    runGitMetaCmd(['init'], async (err) => {
                        if (err) {
                            vscode.window.showErrorMessage(`MetaStackr initialization failed: ${err.message}`);
                        } else {
                            try {
                                await execGitAsync('git config metastackr.initialized true', rootPath);
                            } catch (e) {}

                            vscode.window.showInformationMessage('✅ MetaStackr initialized! Unified multi-repo source control is now active.');
                            await setupInitializedWorkspace(context);
                        }
                        resolve();
                    });
                });
            });
        })
    );

    // 3. Register All Other SCM & Multi-Repo Commands
    registerScmCommands(context);

    // 4. Check if workspace is initialized on startup
    await evaluateWorkspaceState(context);

    // Listen for workspace folder changes
    context.subscriptions.push(
        vscode.workspace.onDidChangeWorkspaceFolders(async () => {
            await evaluateWorkspaceState(context);
        })
    );
}

async function evaluateWorkspaceState(context) {
    const rootPath = getWorkspaceRoot();
    if (!rootPath) {
        setUninitializedState(false);
        return;
    }

    const initialized = await isRepoInitialized(rootPath);
    if (initialized) {
        await setupInitializedWorkspace(context);
    } else {
        const hasSubmodules = fs.existsSync(path.join(rootPath, '.gitmodules'));
        const hasGit = fs.existsSync(path.join(rootPath, '.git'));
        setUninitializedState(hasSubmodules || hasGit);
    }
}

async function isRepoInitialized(rootPath) {
    if (!rootPath) return false;

    // 1. Git config check
    try {
        const val = await execGitAsync('git config --get metastackr.initialized', rootPath);
        if (val.trim() === 'true') return true;
    } catch (e) {}

    // 2. AGENTS.md check
    try {
        const agentsPath = path.join(rootPath, 'AGENTS.md');
        if (fs.existsSync(agentsPath)) {
            const content = fs.readFileSync(agentsPath, 'utf8');
            if (content.includes('MetaStackr') || content.includes('git-meta') || content.includes('git meta')) {
                return true;
            }
        }
    } catch (e) {}

    // 3. .git/hooks/post-checkout check
    try {
        const hookPath = path.join(rootPath, '.git', 'hooks', 'post-checkout');
        if (fs.existsSync(hookPath)) {
            const content = fs.readFileSync(hookPath, 'utf8');
            if (content.includes('git-meta') || content.includes('git meta')) {
                return true;
            }
        }
    } catch (e) {}

    return false;
}

async function setupInitializedWorkspace(context) {
    await vscode.commands.executeCommand('setContext', 'metastackr.isInitialized', true);

    // Automatically disable built-in Git VCS for this workspace if configured
    await disableBuiltinGit();

    // Create SCM Provider if not already created
    if (!scmProvider) {
        scmProvider = vscode.scm.createSourceControl('metastackr', 'MetaStackr');
        scmProvider.inputBox.placeholder = 'Message (Press Enter or Click Checkmark to Commit)';
        scmProvider.acceptInputCommand = { command: 'metastackr.commit', title: 'Commit' };

        stagedGroup = scmProvider.createResourceGroup('staged', 'Staged Changes');
        changesGroup = scmProvider.createResourceGroup('changes', 'Changes');
        stagedGroup.hideWhenEmpty = true;
        changesGroup.hideWhenEmpty = true;

        context.subscriptions.push(scmProvider);
    }

    // Configure Status Bar for Active MetaStackr
    statusBarItem.command = 'metastackr.refresh';
    statusBarItem.tooltip = 'MetaStackr: Click to refresh status';
    statusBarItem.text = '$(git-branch) MetaStackr: Active';
    statusBarItem.show();

    // Setup file watcher
    if (!fileWatcher) {
        fileWatcher = vscode.workspace.createFileSystemWatcher('**/*');
        fileWatcher.onDidChange(() => triggerDebouncedRefresh());
        fileWatcher.onDidCreate(() => triggerDebouncedRefresh());
        fileWatcher.onDidDelete(() => triggerDebouncedRefresh());
        context.subscriptions.push(fileWatcher);
    }

    refreshStatus();
}

function setUninitializedState(showInitPrompt) {
    vscode.commands.executeCommand('setContext', 'metastackr.isInitialized', false);

    if (scmProvider) {
        scmProvider.dispose();
        scmProvider = null;
        stagedGroup = null;
        changesGroup = null;
    }

    if (fileWatcher) {
        fileWatcher.dispose();
        fileWatcher = null;
    }

    if (showInitPrompt) {
        statusBarItem.command = 'metastackr.init';
        statusBarItem.text = '$(plus) MetaStackr: Initialize';
        statusBarItem.tooltip = 'Click to initialize this multi-repo workspace with MetaStackr';
        statusBarItem.show();
    } else {
        statusBarItem.hide();
    }
}

async function disableBuiltinGit() {
    const config = vscode.workspace.getConfiguration('metastackr');
    const autoDisable = config.get('autoDisableBuiltinGit', true);
    if (!autoDisable) return;

    try {
        const gitConfig = vscode.workspace.getConfiguration('git');
        if (gitConfig.get('enabled') !== false) {
            await gitConfig.update('enabled', false, vscode.ConfigurationTarget.Workspace);
            console.log('Disabled built-in Git extension for this workspace (managed by MetaStackr)');
        }
    } catch (err) {
        console.warn('Could not update git.enabled setting:', err);
    }
}

function registerScmCommands(context) {
    // Open File
    context.subscriptions.push(
        vscode.commands.registerCommand('metastackr.openFile', async (resourceUri) => {
            if (resourceUri) {
                try {
                    await vscode.commands.executeCommand('vscode.open', resourceUri);
                } catch (e) {
                    console.error('Failed to open file:', e);
                }
            }
        })
    );

    // Refresh Status
    context.subscriptions.push(
        vscode.commands.registerCommand('metastackr.refresh', () => {
            refreshStatus();
        })
    );

    // Stage Single File
    context.subscriptions.push(
        vscode.commands.registerCommand('metastackr.stage', async (resourceState) => {
            const uri = resourceState?.resourceUri || (resourceState instanceof vscode.Uri ? resourceState : null);
            if (!uri) return;

            const rootPath = getWorkspaceRoot();
            if (!rootPath) return;

            const targetInfo = getRepoForPath(uri.fsPath, rootPath);
            try {
                await execGitAsync(`git add -- "${targetInfo.relPath}"`, targetInfo.repoDir);
                refreshStatus();
            } catch (err) {
                vscode.window.showErrorMessage(`Failed to stage ${targetInfo.relPath}: ${err.message}`);
            }
        })
    );

    // Unstage Single File
    context.subscriptions.push(
        vscode.commands.registerCommand('metastackr.unstage', async (resourceState) => {
            const uri = resourceState?.resourceUri || (resourceState instanceof vscode.Uri ? resourceState : null);
            if (!uri) return;

            const rootPath = getWorkspaceRoot();
            if (!rootPath) return;

            const targetInfo = getRepoForPath(uri.fsPath, rootPath);
            try {
                await execGitAsync(`git restore --staged -- "${targetInfo.relPath}" || git reset HEAD -- "${targetInfo.relPath}"`, targetInfo.repoDir);
                refreshStatus();
            } catch (err) {
                vscode.window.showErrorMessage(`Failed to unstage ${targetInfo.relPath}: ${err.message}`);
            }
        })
    );

    // Stage All Changes
    context.subscriptions.push(
        vscode.commands.registerCommand('metastackr.stageAll', async () => {
            const rootPath = getWorkspaceRoot();
            if (!rootPath) return;

            try {
                const submodules = await getSubmodulePaths(rootPath);
                await execGitAsync('git add -A', rootPath);
                for (const subPath of submodules) {
                    const subDir = path.join(rootPath, subPath);
                    if (fs.existsSync(subDir)) {
                        await execGitAsync('git add -A', subDir);
                    }
                }
                vscode.window.showInformationMessage('Staged all changes across worktree.');
                refreshStatus();
            } catch (err) {
                vscode.window.showErrorMessage(`Failed to stage all changes: ${err.message}`);
            }
        })
    );

    // Unstage All Changes
    context.subscriptions.push(
        vscode.commands.registerCommand('metastackr.unstageAll', async () => {
            const rootPath = getWorkspaceRoot();
            if (!rootPath) return;

            try {
                const submodules = await getSubmodulePaths(rootPath);
                await execGitAsync('git restore --staged . || git reset HEAD', rootPath);
                for (const subPath of submodules) {
                    const subDir = path.join(rootPath, subPath);
                    if (fs.existsSync(subDir)) {
                        await execGitAsync('git restore --staged . || git reset HEAD', subDir);
                    }
                }
                vscode.window.showInformationMessage('Unstaged all changes across worktree.');
                refreshStatus();
            } catch (err) {
                vscode.window.showErrorMessage(`Failed to unstage all changes: ${err.message}`);
            }
        })
    );

    // Commit (Handles staged vs all automatically)
    context.subscriptions.push(
        vscode.commands.registerCommand('metastackr.commit', async () => {
            if (!scmProvider) return;

            let msg = scmProvider.inputBox.value.trim();
            if (!msg) {
                msg = await vscode.window.showInputBox({
                    prompt: 'Enter commit message',
                    placeHolder: 'e.g. feat: implement user authentication'
                });
                if (!msg) return;
            }

            const hasStaged = (stagedGroup?.resourceStates && stagedGroup.resourceStates.length > 0);
            const hasChanges = (changesGroup?.resourceStates && changesGroup.resourceStates.length > 0);

            if (!hasStaged && !hasChanges) {
                vscode.window.showInformationMessage('No changes found to commit.');
                return;
            }

            const args = ['commit', '-m', msg];
            if (hasStaged) {
                args.push('--staged');
            }

            vscode.window.withProgress({
                location: vscode.ProgressLocation.Notification,
                title: hasStaged ? 'Committing staged changes with MetaStackr...' : 'Creating atomic commit across submodules...',
                cancellable: false
            }, async () => {
                return new Promise((resolve) => {
                    runGitMetaCmd(args, (err) => {
                        if (err) {
                            vscode.window.showErrorMessage(`Commit failed: ${err.message}`);
                        } else {
                            scmProvider.inputBox.value = '';
                            vscode.window.showInformationMessage(hasStaged ? '✅ Staged commit succeeded (submodule pointers aligned)!' : '✅ Atomic commit succeeded across all submodules!');
                            refreshStatus();
                        }
                        resolve();
                    });
                });
            });
        })
    );

    // Commit Staged Only
    context.subscriptions.push(
        vscode.commands.registerCommand('metastackr.commitStaged', async () => {
            let msg = scmProvider ? scmProvider.inputBox.value.trim() : '';
            if (!msg) {
                msg = await vscode.window.showInputBox({ prompt: 'Enter commit message for staged changes' });
                if (!msg) return;
            }
            runGitMetaCmd(['commit', '-m', msg, '--staged'], (err) => {
                if (err) {
                    vscode.window.showErrorMessage(`Commit staged failed: ${err.message}`);
                } else {
                    if (scmProvider) scmProvider.inputBox.value = '';
                    vscode.window.showInformationMessage('✅ Committed staged changes and updated parent pointers!');
                    refreshStatus();
                }
            });
        })
    );

    // Commit All Atomic
    context.subscriptions.push(
        vscode.commands.registerCommand('metastackr.commitAll', async () => {
            let msg = scmProvider ? scmProvider.inputBox.value.trim() : '';
            if (!msg) {
                msg = await vscode.window.showInputBox({ prompt: 'Enter atomic commit message' });
                if (!msg) return;
            }
            runGitMetaCmd(['commit', '-m', msg], (err) => {
                if (err) {
                    vscode.window.showErrorMessage(`Atomic commit failed: ${err.message}`);
                } else {
                    if (scmProvider) scmProvider.inputBox.value = '';
                    vscode.window.showInformationMessage('✅ Atomic commit completed across all submodules!');
                    refreshStatus();
                }
            });
        })
    );

    // Checkout Branch
    context.subscriptions.push(
        vscode.commands.registerCommand('metastackr.checkout', async () => {
            const branch = await vscode.window.showInputBox({ prompt: 'Enter branch name to checkout system-wide' });
            if (!branch) return;

            vscode.window.withProgress({
                location: vscode.ProgressLocation.Notification,
                title: `Switching to branch '${branch}'...`,
                cancellable: false
            }, async () => {
                return new Promise((resolve) => {
                    runGitMetaCmd(['checkout', branch], (err) => {
                        if (err) {
                            vscode.window.showErrorMessage(`Checkout failed: ${err.message}`);
                        } else {
                            vscode.window.showInformationMessage(`Switched to branch '${branch}' across all submodules!`);
                            refreshStatus();
                        }
                        resolve();
                    });
                });
            });
        })
    );

    // Push (Bottom-Up)
    context.subscriptions.push(
        vscode.commands.registerCommand('metastackr.push', async () => {
            vscode.window.withProgress({
                location: vscode.ProgressLocation.Notification,
                title: 'Pushing submodules and parent repo (bottom-up enforcement)...',
                cancellable: false
            }, async () => {
                return new Promise((resolve) => {
                    runGitMetaCmd(['push'], (err) => {
                        if (err) {
                            vscode.window.showErrorMessage(`Push failed: ${err.message}`);
                        } else {
                            vscode.window.showInformationMessage('✅ Bottom-up push succeeded across all submodules & root!');
                            refreshStatus();
                        }
                        resolve();
                    });
                });
            });
        })
    );

    // Sync Submodules
    context.subscriptions.push(
        vscode.commands.registerCommand('metastackr.sync', async () => {
            vscode.window.withProgress({
                location: vscode.ProgressLocation.Notification,
                title: 'Syncing and fast-forwarding submodules...',
                cancellable: false
            }, async () => {
                return new Promise((resolve) => {
                    runGitMetaCmd(['sync'], (err) => {
                        if (err) {
                            vscode.window.showErrorMessage(`Sync failed: ${err.message}`);
                        } else {
                            vscode.window.showInformationMessage('✅ Submodules synced with upstream origin!');
                            refreshStatus();
                        }
                        resolve();
                    });
                });
            });
        })
    );
}

function triggerDebouncedRefresh() {
    if (refreshDebounceTimer) clearTimeout(refreshDebounceTimer);
    refreshDebounceTimer = setTimeout(() => {
        refreshStatus();
    }, 600);
}

function getWorkspaceRoot() {
    return vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
}

// Map a file absolute path to its containing submodule or root repo
function getRepoForPath(filePath, rootPath) {
    const submodules = getSubmodulePathsSync(rootPath);
    for (const sub of submodules) {
        const subAbsDir = path.join(rootPath, sub);
        if (filePath.startsWith(subAbsDir + path.sep) || filePath === subAbsDir) {
            const relPath = path.relative(subAbsDir, filePath);
            return { repoDir: subAbsDir, relPath, isSubmodule: true, subPath: sub };
        }
    }
    const relPath = path.relative(rootPath, filePath);
    return { repoDir: rootPath, relPath, isSubmodule: false, subPath: '' };
}

function getSubmodulePathsSync(rootPath) {
    try {
        const gitmodulesPath = path.join(rootPath, '.gitmodules');
        if (!fs.existsSync(gitmodulesPath)) return [];
        const content = fs.readFileSync(gitmodulesPath, 'utf8');
        const matches = [];
        const regex = /path\s*=\s*(.+)/g;
        let match;
        while ((match = regex.exec(content)) !== null) {
            matches.push(match[1].trim());
        }
        return matches;
    } catch (e) {
        return [];
    }
}

async function getSubmodulePaths(rootPath) {
    return getSubmodulePathsSync(rootPath);
}

function execGitAsync(cmd, cwd) {
    return new Promise((resolve, reject) => {
        exec(cmd, { cwd }, (err, stdout, stderr) => {
            if (err) {
                reject(new Error(stderr || stdout || err.message));
            } else {
                resolve(stdout);
            }
        });
    });
}

// Parse git porcelain output into structured entries
async function parseGitPorcelain(cwd) {
    try {
        const stdout = await execGitAsync('git status --porcelain=v1 -u', cwd);
        const entries = [];
        const lines = stdout.split('\n');
        for (const line of lines) {
            if (!line || line.length < 3) continue;
            const stagedCode = line[0];
            const unstagedCode = line[1];
            let rawPath = line.substring(3).trim();
            if (rawPath.includes(' -> ')) {
                rawPath = rawPath.split(' -> ')[1].trim();
            }
            if (rawPath.startsWith('"') && rawPath.endsWith('"')) {
                rawPath = rawPath.slice(1, -1);
            }

            entries.push({
                stagedCode: stagedCode !== ' ' && stagedCode !== '?' ? stagedCode : null,
                unstagedCode: unstagedCode !== ' ' || stagedCode === '?' ? (stagedCode === '?' ? '?' : unstagedCode) : null,
                relPath: rawPath,
                absPath: path.join(cwd, rawPath)
            });
        }
        return entries;
    } catch (e) {
        return [];
    }
}

async function refreshStatus() {
    if (isRefreshing || !scmProvider) return;
    isRefreshing = true;

    const rootPath = getWorkspaceRoot();
    if (!rootPath) {
        isRefreshing = false;
        return;
    }

    try {
        const submodules = await getSubmodulePaths(rootPath);

        // 1. Fetch Root Branch
        let rootBranch = 'main';
        try {
            const branchOut = await execGitAsync('git symbolic-ref --short HEAD || git rev-parse --short HEAD', rootPath);
            rootBranch = branchOut.trim();
        } catch (e) {}

        // 2. Collect porcelain entries from root + all submodules
        const stagedList = [];
        const changesList = [];

        // Root porcelain
        const rootEntries = await parseGitPorcelain(rootPath);
        for (const entry of rootEntries) {
            const isSubmodulePointer = submodules.includes(entry.relPath);
            const prefix = isSubmodulePointer ? '[submodule pointer] ' : '';

            if (entry.stagedCode) {
                stagedList.push(createResourceState(entry.absPath, entry.stagedCode, true, `${prefix}${entry.relPath}`));
            }
            if (entry.unstagedCode) {
                changesList.push(createResourceState(entry.absPath, entry.unstagedCode, false, `${prefix}${entry.relPath}`));
            }
        }

        // Submodule porcelains
        for (const sub of submodules) {
            const subDir = path.join(rootPath, sub);
            if (!fs.existsSync(subDir)) continue;

            const subEntries = await parseGitPorcelain(subDir);
            for (const entry of subEntries) {
                const displayLabel = `[${sub}] ${entry.relPath}`;
                if (entry.stagedCode) {
                    stagedList.push(createResourceState(entry.absPath, entry.stagedCode, true, displayLabel));
                }
                if (entry.unstagedCode) {
                    changesList.push(createResourceState(entry.absPath, entry.unstagedCode, false, displayLabel));
                }
            }
        }

        if (stagedGroup) stagedGroup.resourceStates = stagedList;
        if (changesGroup) changesGroup.resourceStates = changesList;

        // Update status bar
        const stagedCount = stagedList.length;
        const changesCount = changesList.length;
        if (stagedCount === 0 && changesCount === 0) {
            statusBarItem.text = `$(git-branch) Meta: ${rootBranch} $(check)`;
        } else {
            statusBarItem.text = `$(git-branch) Meta: ${rootBranch} | $(diff-added) ${stagedCount} staged, $(diff-modified) ${changesCount} changes`;
        }
    } catch (err) {
        console.error('Error refreshing MetaStackr status:', err);
    } finally {
        isRefreshing = false;
    }
}

function createResourceState(absPath, statusCode, isStaged, displayLabel) {
    const uri = vscode.Uri.file(absPath);
    const isDeleted = statusCode === 'D';
    const isUntracked = statusCode === '?';

    let statusText = 'Modified';
    if (statusCode === 'A') statusText = 'Added';
    else if (statusCode === 'D') statusText = 'Deleted';
    else if (statusCode === 'R') statusText = 'Renamed';
    else if (statusCode === '?') statusText = 'Untracked';

    return {
        resourceUri: uri,
        command: {
            command: 'metastackr.openFile',
            title: 'Open File',
            arguments: [uri]
        },
        decorations: {
            tooltip: `${displayLabel} — ${statusText} (${isStaged ? 'Staged' : 'Unstaged'})`,
            strikeThrough: isDeleted,
            faded: isUntracked
        },
        contextValue: isStaged ? 'stagedResource' : 'changeResource'
    };
}

function runGitMetaCmd(args, callback) {
    const rootPath = getWorkspaceRoot();
    if (!rootPath) {
        callback(new Error('No workspace folder open'));
        return;
    }

    const cmdStr = `git meta ${args.join(' ')} --json || git-meta ${args.join(' ')} --json`;
    exec(cmdStr, { cwd: rootPath }, (err, stdout, stderr) => {
        if (err && !stdout) {
            return callback(new Error(stderr || err.message));
        }
        try {
            const res = JSON.parse(stdout);
            if (!res.success) {
                return callback(new Error(res.message || 'MetaStackr CLI error'));
            }
            callback(null, res.message);
        } catch (e) {
            callback(null, stdout || 'Command executed');
        }
    });
}

function deactivate() {}

module.exports = {
    activate,
    deactivate
};
