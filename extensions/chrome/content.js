(function() {
  'use strict';

  // GitHub uses Turbo Drive (pjax) to swap page content without full reloads
  document.addEventListener('turbo:load', initialize);
  // Fallback for direct page loads
  if (document.readyState === 'complete' || document.readyState === 'interactive') {
    initialize();
  } else {
    document.addEventListener('DOMContentLoaded', initialize);
  }

  function initialize() {
    const prMatch = window.location.pathname.match(/^\/([^\/]+)\/([^\/]+)\/pull\/(\d+)/);
    if (!prMatch) return;

    const owner = prMatch[1];
    const repo = prMatch[2];
    const prNumber = parseInt(prMatch[3], 10);
    const repoFullName = `${owner}/${repo}`;

    // Get current branch from the GitHub DOM if possible
    const headBranchEl = document.querySelector('.head-ref');
    const branchName = headBranchEl ? headBranchEl.innerText.trim() : '';

    if (!branchName) return;

    fetchPRStatus(repoFullName, branchName, (metaPR) => {
      if (metaPR) {
        injectHeaderBanner(metaPR);
        injectSubmodulesTab(metaPR);
        injectSidebarCard(metaPR, repoFullName, prNumber);
      }
    });
  }

  function fetchPRStatus(repo, branch, callback) {
    const url = `http://localhost:8080/api/v1/prs/status?repo=${encodeURIComponent(repo)}&branch=${encodeURIComponent(branch)}`;
    fetch(url)
      .then(res => {
        if (!res.ok) throw new Error('Unreachable server');
        return res.json();
      })
      .then(data => {
        if (data && data.meta_pr) {
          callback(data.meta_pr);
        } else {
          callback(null);
        }
      })
      .catch(err => {
        console.log('[MetaStackr] Standalone backend not active or repo untracked.');
        callback(null);
      });
  }

  function injectHeaderBanner(metaPR) {
    if (document.getElementById('metastackr-header-banner')) return;

    const targetHeader = document.querySelector('.gh-header-show');
    if (!targetHeader) return;

    const banner = document.createElement('div');
    banner.id = 'metastackr-header-banner';
    banner.className = 'metastackr-banner';
    banner.innerHTML = `
      <div class="metastackr-banner-inner">
        <span class="metastackr-logo">⚡ metastackr</span>
        <span class="metastackr-status">Meta PR Status: <strong>${metaPR.status}</strong></span>
        <span class="metastackr-meta">Lock Version: ${metaPR.lock_version}</span>
      </div>
    `;

    targetHeader.parentNode.insertBefore(banner, targetHeader);
  }

  function injectSubmodulesTab(metaPR) {
    if (document.getElementById('metastackr-submodules-tab')) return;

    const tabList = document.querySelector('.tabnav-tabs');
    if (!tabList) return;

    const subTab = document.createElement('a');
    subTab.id = 'metastackr-submodules-tab';
    subTab.href = '#';
    subTab.className = 'tabnav-tab js-pjax-history-navigate';
    subTab.innerHTML = `
      Submodules
      <span class="Counter">${metaPR.child_prs ? metaPR.child_prs.length : 0}</span>
    `;

    subTab.addEventListener('click', (e) => {
      e.preventDefault();
      // Highlight selected tab
      tabList.querySelectorAll('.tabnav-tab').forEach(t => t.classList.remove('selected'));
      subTab.classList.add('selected');

      // Inject custom panel
      showSubmodulesGrid(metaPR);
    });

    tabList.appendChild(subTab);
  }

  function showSubmodulesGrid(metaPR) {
    let container = document.getElementById('metastackr-submodules-panel');
    if (!container) {
      container = document.createElement('div');
      container.id = 'metastackr-submodules-panel';
      container.className = 'metastackr-panel';
      
      const prContainer = document.querySelector('.js-discussion');
      if (prContainer) {
        prContainer.style.display = 'none';
        prContainer.parentNode.insertBefore(container, prContainer);
      }
    }

    let rowsHtml = '';
    (metaPR.child_prs || []).forEach(child => {
      rowsHtml += `
        <div class="metastackr-grid-row">
          <div class="col-path"><code>${child.submodule_path}</code></div>
          <div class="col-repo">${child.repo_full_name}</div>
          <div class="col-pr"><a href="https://github.com/${child.repo_full_name}/pull/${child.pr_number}">#${child.pr_number}</a></div>
          <div class="col-sha"><code>${child.head_sha.substring(0, 7)}</code></div>
          <div class="col-status"><span class="status-badge state-${child.status.toLowerCase()}">${child.status}</span></div>
        </div>
      `;
    });

    container.innerHTML = `
      <div class="metastackr-panel-header">
        <h3>Submodule Sync Status</h3>
        <button id="close-metastackr-panel" class="btn btn-sm">Return to Conversation</button>
      </div>
      <div class="metastackr-grid">
        <div class="metastackr-grid-header">
          <div class="col-path">Submodule Path</div>
          <div class="col-repo">Submodule Repo</div>
          <div class="col-pr">PR #</div>
          <div class="col-sha">Commit SHA</div>
          <div class="col-status">Merge Status</div>
        </div>
        ${rowsHtml || '<div class="metastackr-empty">No child submodules tracked for this branch</div>'}
      </div>
    `;

    document.getElementById('close-metastackr-panel').addEventListener('click', () => {
      container.remove();
      const prContainer = document.querySelector('.js-discussion');
      if (prContainer) {
        prContainer.style.display = 'block';
      }
      const subTab = document.getElementById('metastackr-submodules-tab');
      if (subTab) subTab.classList.remove('selected');
      const convTab = document.querySelector('.tabnav-tab'); // First tab (Conversation)
      if (convTab) convTab.classList.add('selected');
    });
  }

  function injectSidebarCard(metaPR, repoFullName, prNumber) {
    if (document.getElementById('metastackr-sidebar-card')) return;

    const sidebar = document.querySelector('.discussion-sidebar');
    if (!sidebar) return;

    const card = document.createElement('div');
    card.id = 'metastackr-sidebar-card';
    card.className = 'discussion-sidebar-item';
    card.innerHTML = `
      <div class="metastackr-sidebar-header">
        <span class="metastackr-sidebar-logo">⚡ metastackr</span>
      </div>
      <div class="metastackr-sidebar-body">
        <p>Cascade merge states topologically across submodules.</p>
        <button id="metastackr-merge-btn" class="btn btn-sm btn-primary btn-block" ${metaPR.status !== 'FAILED' && metaPR.status !== 'FAILED_PARTIAL' ? 'disabled' : ''}>
          Trigger Cascade Merge
        </button>
      </div>
    `;

    sidebar.appendChild(card);

    const mergeBtn = document.getElementById('metastackr-merge-btn');
    if (mergeBtn) {
      mergeBtn.addEventListener('click', () => {
        mergeBtn.disabled = true;
        mergeBtn.innerText = 'Retrying...';

        fetch(`http://localhost:8080/api/v1/prs/retry-merge`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json'
          },
          body: JSON.stringify({
            meta_repo: repoFullName,
            pr_number: prNumber
          })
        })
        .then(res => {
          if (res.ok) {
            mergeBtn.innerText = 'Merge Queued';
          } else {
            mergeBtn.innerText = 'Retry Failed';
            mergeBtn.disabled = false;
          }
        })
        .catch(() => {
          mergeBtn.innerText = 'Error Connection';
          mergeBtn.disabled = false;
        });
      });
    }
  }
})();
