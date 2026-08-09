const vscode = require('vscode');
const { exec } = require('child_process');
const path = require('path');

let statusBarItem;
let scmProvider;
let activeGroup;

function activate(context) {
    console.log('MetaStackr VS Code Extension is active');

    // 1. Create SCM Provider
    scmProvider = vscode.scm.createSourceControl('metastackr', 'MetaStackr SCM');
    activeGroup = scmProvider.createResourceGroup('changes', 'Unified Changes');
    context.subscriptions.push(scmProvider);

    // 2. Create Status Bar Item
    statusBarItem = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 100);
    statusBarItem.command = 'metastackr.status';
    statusBarItem.text = '$(git-branch) MetaStackr: Active';
    statusBarItem.show();
    context.subscriptions.push(statusBarItem);

    // 3. Register Commands
    context.subscriptions.push(
        vscode.commands.registerCommand('metastackr.commit', async () => {
            const msg = await vscode.window.showInputBox({ prompt: 'Enter coordinated atomic commit message' });
            if (!msg) return;

            runGitMetaCmd(['commit', '-m', msg], (err, stdout) => {
                if (err) {
                    vscode.window.showErrorMessage(`Atomic commit failed: ${err.message}`);
                } else {
                    vscode.window.showInformationMessage('Atomic commit succeeded across submodules!');
                    refreshStatus();
                }
            });
        })
    );

    context.subscriptions.push(
        vscode.commands.registerCommand('metastackr.checkout', async () => {
            const branch = await vscode.window.showInputBox({ prompt: 'Enter branch name to checkout' });
            if (!branch) return;

            runGitMetaCmd(['checkout', branch], (err, stdout) => {
                if (err) {
                    vscode.window.showErrorMessage(`Checkout failed: ${err.message}`);
                } else {
                    vscode.window.showInformationMessage(`Switched to branch '${branch}' across submodules!`);
                    refreshStatus();
                }
            });
        })
    );

    context.subscriptions.push(
        vscode.commands.registerCommand('metastackr.sync', () => {
            vscode.window.withProgress({
                location: vscode.ProgressLocation.Notification,
                title: "Syncing MetaStackr submodules...",
                cancellable: false
            }, async () => {
                return new Promise((resolve) => {
                    runGitMetaCmd(['sync'], (err) => {
                        if (err) {
                            vscode.window.showErrorMessage(`Sync failed: ${err.message}`);
                        } else {
                            vscode.window.showInformationMessage('Workspace submodules sync complete!');
                            refreshStatus();
                        }
                        resolve();
                    });
                });
            });
        })
    );

    // Start status loop
    refreshStatus();
    setInterval(refreshStatus, 15000);
}

function refreshStatus() {
    const rootPath = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
    if (!rootPath) return;

    // Execute git-meta status --json
    exec('git-meta status --json', { cwd: rootPath }, (err, stdout) => {
        if (err) {
            statusBarItem.text = '$(warning) MetaStackr: CLI Error';
            return;
        }

        try {
            const res = JSON.parse(stdout);
            if (res.success && res.data) {
                const local = res.data.local_status;
                statusBarItem.text = `$(git-branch) MetaStackr: ${local.meta_branch}`;
                
                // Collect and render resources
                const resources = [];
                if (local.submodules) {
                    local.submodules.forEach(sub => {
                        if (sub.has_uncommitted) {
                            const fileUri = vscode.Uri.file(path.join(rootPath, sub.Path));
                            resources.push({
                                resourceUri: fileUri,
                                decorations: {
                                    tooltip: `Submodule dirty: ${sub.Path}`,
                                    strikeThrough: false
                                }
                            });
                        }
                    });
                }
                activeGroup.resourceStates = resources;
            }
        } catch (e) {
            // Standalone fallback or unparseable JSON
        }
    });
}

function runGitMetaCmd(args, callback) {
    const rootPath = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
    if (!rootPath) {
        callback(new Error('No workspace open'));
        return;
    }
    const cmdStr = `git-meta ${args.join(' ')} --json`;
    exec(cmdStr, { cwd: rootPath }, (err, stdout) => {
        if (err) return callback(err);
        try {
            const res = JSON.parse(stdout);
            if (!res.success) {
                return callback(new Error(res.message || 'CLI error'));
            }
            callback(null, res.message);
        } catch (e) {
            callback(null, stdout);
        }
    });
}

function deactivate() {}

module.exports = {
    activate,
    deactivate
};
