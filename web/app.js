document.addEventListener('DOMContentLoaded', () => {
  initMarkdownLoader();
  initTerminal();
  initDAGSimulator();
});

/* Markdown File Loader for data-markdown elements */
async function initMarkdownLoader() {
  const elements = document.querySelectorAll('[data-markdown]');

  for (const el of elements) {
    const mdPath = el.getAttribute('data-markdown');
    if (!mdPath) continue;

    try {
      const response = await fetch(mdPath);
      if (response.ok) {
        const rawText = await response.text();
        // Remove frontmatter YAML header if present
        const cleanMarkdown = rawText.replace(/^---[\s\S]*?---\s*/, '');

        if (typeof marked !== 'undefined') {
          el.innerHTML = marked.parse(cleanMarkdown);
        } else {
          el.innerText = cleanMarkdown;
        }
      }
    } catch (err) {
      console.warn(`[markdown-loader] failed to fetch ${mdPath}:`, err);
    }
  }
}

/* Terminal Simulator */
const terminalData = {
  status: `⚡ MetaStackR Status
Meta Repo: org/metastackr-root | Branch: feature/v2.0-auth

 Submodule Path      |  Local Branch  |  Local Drift   |  Child PR  |  Review      |  CI State 
------------------------------------------------------------------------------------------
 sub/auth-service    | feature/v2.0-auth | CLEAN          | #42        | ✅ APPROVED  | ✅ SUCCESS
 sub/ui-app          | feature/v2.0-auth | CLEAN          | #18        | ✅ APPROVED  | ✅ SUCCESS
 sub/billing-api     | feature/v2.0-auth | DIRTY          | #07        | ⏳ PENDING   | ⏳ PENDING

Backend Meta PR Status: SYNCING (Lock Version: 4)`,

  checkout: `Checking out branch 'feature/payment-gateway' in root meta-repo...
Checking out branch 'feature/payment-gateway' in submodule 'sub/auth-service'...
Checking out branch 'feature/payment-gateway' in submodule 'sub/ui-app'...
Checking out branch 'feature/payment-gateway' in submodule 'sub/billing-api'...
✅ Switched to branch 'feature/payment-gateway' across 3 submodules!`,

  push: `Pushing submodule 'sub/auth-service' on branch 'feature/v2.0-auth'...
Pushing submodule 'sub/billing-api' on branch 'feature/v2.0-auth'...
All 2 dirty submodules pushed to origin.
Pushing root meta-repo commit pointers on branch 'feature/v2.0-auth'...
✅ Bottom-up push completed successfully! No dangling commit pointers created.`,

  retry: `POST http://localhost:8080/api/v1/prs/retry-merge
Payload: {"meta_repo": "org/metastackr-root", "pr_number": 42}
Response HTTP 200 OK:
{
  "status": "retry_initiated",
  "meta_id": "8f3b2a1c-99e4-4d82-b13c-0e7890123456"
}
🚀 Cascade merge retry initiated! Resuming execution from unmerged node 'sub/billing-api'.`
};

function initTerminal() {
  const body = document.getElementById('terminal-body');
  const tabs = document.querySelectorAll('.term-tab');
  if (!body || !tabs.length) return;

  function loadTab(command) {
    tabs.forEach(t => t.classList.remove('active'));
    const activeTab = Array.from(tabs).find(t => t.dataset.cmd === command);
    if (activeTab) activeTab.classList.add('active');

    const prompt = `<span class="prompt-line">dev@localhost ~/meta-repo $</span> git meta ${command}\n\n`;
    const text = terminalData[command] || 'Command output...';
    body.innerHTML = prompt + text;
  }

  tabs.forEach(tab => {
    tab.addEventListener('click', () => {
      loadTab(tab.dataset.cmd);
    });
  });

  loadTab('status');
}

/* Interactive DAG Visualizer & Saga Simulator */
function initDAGSimulator() {
  const nodeAuth = document.getElementById('node-auth');
  const nodeUI = document.getElementById('node-ui');
  const nodeBilling = document.getElementById('node-billing');
  const nodeMeta = document.getElementById('node-meta');
  const logContainer = document.getElementById('sim-log');

  const btnSimulate = document.getElementById('btn-sim-merge');
  const btnConflict = document.getElementById('btn-sim-conflict');
  const btnReset = document.getElementById('btn-sim-reset');

  if (!nodeAuth || !nodeUI || !nodeBilling || !nodeMeta) return;

  function appendLog(msg, colorClass = '') {
    if (!logContainer) return;
    const time = new Date().toLocaleTimeString();
    const line = document.createElement('div');
    line.className = colorClass;
    line.innerHTML = `<span style="color: #5E7366;">[${time}]</span> ${msg}`;
    logContainer.appendChild(line);
    logContainer.scrollTop = logContainer.scrollHeight;
  }

  function resetNodes() {
    [nodeAuth, nodeUI, nodeBilling, nodeMeta].forEach(n => {
      n.className = 'dag-node';
      const badge = n.querySelector('.node-badge');
      if (badge) {
        badge.textContent = 'OPEN';
        badge.style.background = 'var(--bg-dark)';
        badge.style.color = 'var(--text-muted)';
      }
    });
    if (logContainer) logContainer.innerHTML = '<div>System ready. Click a simulation button below.</div>';
  }

  if (btnReset) btnReset.addEventListener('click', resetNodes);

  if (btnSimulate) {
    btnSimulate.addEventListener('click', async () => {
      resetNodes();
      appendLog('🚀 Initiating Saga Cascade Merge for Meta PR #42...', 'text-yellow');
      appendLog('Step 1/5: Optimistic locking meta_prs (lock_version = 1 -> 2)');

      // Batch 1: Submodule Auth & Submodule UI
      await sleep(1000);
      appendLog('Step 2/5: Executing Batch 1 (Depth 0) in parallel: sub/auth-service & sub/ui-app');
      setNodeState(nodeAuth, 'active-merging', 'MERGING');
      setNodeState(nodeUI, 'active-merging', 'MERGING');

      await sleep(1500);
      setNodeState(nodeAuth, 'merged', 'MERGED');
      setNodeState(nodeUI, 'merged', 'MERGED');
      appendLog('✅ Batch 1 complete! Merged SHAs: sub/auth-service@a8f10c, sub/ui-app@99d2e1', 'text-green');

      // Batch 2: Submodule Billing
      await sleep(1000);
      appendLog('Step 3/5: Executing Batch 2 (Depth 1): sub/billing-api');
      setNodeState(nodeBilling, 'active-merging', 'MERGING');

      await sleep(1500);
      setNodeState(nodeBilling, 'merged', 'MERGED');
      appendLog('✅ Batch 2 complete! Merged SHA: sub/billing-api@f3c990', 'text-green');

      // Step 4 & 5: Root Meta PR
      await sleep(1000);
      appendLog('Step 4/5: Bumping submodule commit pointers in meta-repo root...');
      appendLog('Step 5/5: Merging Root Meta PR #42...');
      setNodeState(nodeMeta, 'active-merging', 'MERGING');

      await sleep(1200);
      setNodeState(nodeMeta, 'merged', 'MERGED');
      appendLog('🎉 CASCADE MERGE COMPLETED SUCCESSFULLY! GitHub Check Run updated to SUCCESS.', 'text-green');
    });
  }

  if (btnConflict) {
    btnConflict.addEventListener('click', async () => {
      resetNodes();
      appendLog('🚀 Initiating Cascade Merge for Meta PR #42...', 'text-yellow');
      
      await sleep(800);
      setNodeState(nodeAuth, 'active-merging', 'MERGING');
      setNodeState(nodeUI, 'active-merging', 'MERGING');

      await sleep(1200);
      setNodeState(nodeAuth, 'merged', 'MERGED');
      setNodeState(nodeUI, 'failed', 'FAILED');
      appendLog('❌ MERGE CONFLICT DETECTED in sub/ui-app PR #18!', 'text-red');
      appendLog('⚠️ Saga Protocol Activated: Halting cascade merge at FAILED_PARTIAL.', 'text-yellow');
      appendLog('📌 Submodule sub/auth-service remains merged. NO force-reverts executed on base branches.', 'text-yellow');
      appendLog('💡 Run "git meta retry-merge --pr 42" after resolving conflict.', 'text-green');
    });
  }

  function setNodeState(node, stateClass, text) {
    node.className = `dag-node ${stateClass}`;
    const badge = node.querySelector('.node-badge');
    if (badge) {
      badge.textContent = text;
      if (stateClass === 'merged') {
        badge.style.background = 'var(--green-dim)';
        badge.style.color = 'var(--green-neon)';
      } else if (stateClass === 'failed') {
        badge.style.background = 'rgba(255, 70, 114, 0.2)';
        badge.style.color = 'var(--red-accent)';
      } else {
        badge.style.background = 'var(--yellow-dim)';
        badge.style.color = 'var(--yellow-primary)';
      }
    }
  }

  function sleep(ms) {
    return new Promise(resolve => setTimeout(resolve, ms));
  }
}
