(function() {
  'use strict';

  // GitHub uses Turbo Drive (pjax) to swap page content without full reloads
  document.addEventListener('turbo:load', initialize);
  document.addEventListener('turbo:render', initialize);
  
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
    const headBranchEl = document.querySelector('.head-ref, span.commit-ref, a.commit-ref, [data-hovercard-type="commit"], .branch-name');
    const branchName = headBranchEl ? headBranchEl.innerText.trim() : 'head';

    fetchPRStatus(repoFullName, prNumber, branchName, (metaPR) => {
      const activeMetaPR = metaPR || {
        status: 'OPEN',
        lock_version: 1,
        child_prs: []
      };
      injectSubmodulesTab(activeMetaPR, repoFullName, prNumber);
      injectSidebarCard(activeMetaPR, repoFullName, prNumber);
    });
  }

  let activeServerURL = 'https://api.metastac.kr';

  function fetchPRStatus(repo, prNumber, branch, callback) {
    if (typeof chrome !== 'undefined' && chrome.runtime && chrome.runtime.sendMessage) {
      chrome.runtime.sendMessage({ action: 'fetchPRStatus', repo, prNumber, branch }, (response) => {
        if (!chrome.runtime.lastError && response && response.metaPR) {
          if (response.serverURL) activeServerURL = response.serverURL;
          callback(response.metaPR);
          return;
        }
        directFetchPRStatus(repo, prNumber, branch, callback);
      });
    } else {
      directFetchPRStatus(repo, prNumber, branch, callback);
    }
  }

  function directFetchPRStatus(repo, prNumber, branch, callback) {
    const tryFetch = (serverURL, fallbackURL) => {
      const url = `${serverURL}/api/v1/prs/status?repo=${encodeURIComponent(repo)}&pr=${prNumber}&branch=${encodeURIComponent(branch)}`;
      fetch(url)
        .then(res => {
          if (!res.ok) throw new Error('Unreachable server');
          return res.json();
        })
        .then(data => {
          if (data && data.meta_pr) {
            activeServerURL = serverURL;
            callback(data.meta_pr);
          } else if (fallbackURL) {
            tryFetch(fallbackURL, null);
          } else {
            callback({
              status: 'OPEN',
              lock_version: 1,
              child_prs: []
            });
          }
        })
        .catch(err => {
          if (fallbackURL) {
            tryFetch(fallbackURL, null);
          } else {
            console.log('[MetaStackr] Standalone backend not active or repo untracked.');
            callback({
              status: 'OPEN',
              lock_version: 1,
              child_prs: []
            });
          }
        });
    };

    tryFetch('https://api.metastac.kr', 'http://localhost:8080');
  }

  document.addEventListener('turbo:before-visit', cleanupMetaStackrPanel);
  window.addEventListener('popstate', cleanupMetaStackrPanel);

  function cleanupMetaStackrPanel() {
    const container = document.getElementById('metastackr-submodules-panel');
    if (container) {
      container.style.display = 'none';
    }
    const buckets = document.querySelectorAll(
      '#discussion_bucket, .js-discussion, #files, .js-diff-container, #commits_bucket, #checks_bucket, turbo-frame#files-tab-frame'
    );
    buckets.forEach(b => {
      b.style.display = '';
    });
    const subTab = document.getElementById('metastackr-submodules-tab');
    if (subTab) {
      subTab.classList.remove('selected');
      subTab.classList.remove('PullRequestHeaderTabNav-module__selected__g5kH0');
      subTab.removeAttribute('aria-current');
    }
  }

  function injectSubmodulesTab(metaPR, repoFullName, prNumber) {
    if (document.getElementById('metastackr-submodules-tab')) {
      const existingCounter = document.querySelector('#metastackr-submodules-tab [data-component="CounterLabel"]');
      if (existingCounter && metaPR && metaPR.child_prs) {
        existingCounter.innerText = metaPR.child_prs.length;
      }
      return;
    }

    const tabList = document.querySelector('[class*="TabNavList"], nav[aria-label*="Pull request navigation"] div, nav[aria-label*="Pull request navigation"] ul, .tabnav-tabs, ul.tabnav-tabs');
    if (!tabList) return;

    const childPRs = (metaPR && metaPR.child_prs) ? metaPR.child_prs : [];

    const subTab = document.createElement('a');
    subTab.id = 'metastackr-submodules-tab';
    subTab.href = '#';
    subTab.className = 'tabnav-tab js-pjax-history-navigate PullRequestHeaderTabNav-module__TabNavLink__JCc1O position-relative flex-shrink-0 text-normal PullRequestHeaderNavigation-module__overrideLineHeight__TeEsl';
    subTab.innerHTML = `
      <svg data-component="Octicon" aria-hidden="true" focusable="false" class="octicon octicon-zap fg-muted mr-2 d-none d-sm-inline-block" viewBox="0 0 16 16" width="16" height="16" fill="currentColor" display="inline-block" overflow="visible" style="vertical-align: text-bottom;">
        <path d="M8.22 1.754a.75.75 0 0 0-1.44 0L4.537 7.033A.75.75 0 0 0 5.228 8.1h2.522l-1.97 6.142a.75.75 0 0 0 1.44 0l2.243-5.279A.75.75 0 0 0 8.772 7.9H6.25l1.97-6.146Z"></path>
      </svg>MetaStackr<span aria-hidden="true" data-variant="secondary" data-component="CounterLabel" class="ml-2 prc-CounterLabel-CounterLabel-X-kRU">${childPRs.length}</span>
    `;

    subTab.addEventListener('click', (e) => {
      e.preventDefault();
      // Highlight selected tab
      tabList.querySelectorAll('a').forEach(t => {
        t.classList.remove('selected');
        t.classList.remove('PullRequestHeaderTabNav-module__selected__g5kH0');
        t.removeAttribute('aria-current');
      });
      subTab.classList.add('selected');
      subTab.classList.add('PullRequestHeaderTabNav-module__selected__g5kH0');
      subTab.setAttribute('aria-current', 'page');

      // Inject custom panel
      showSubmodulesGrid(metaPR, childPRs);
    });

    // Attach cleanup handlers to sibling native tabs
    tabList.querySelectorAll('a:not(#metastackr-submodules-tab)').forEach(nativeTab => {
      nativeTab.addEventListener('click', () => {
        cleanupMetaStackrPanel();
      });
    });

    tabList.appendChild(subTab);
  }

  function showSubmodulesGrid(metaPR, childPRs) {
    let container = document.getElementById('metastackr-submodules-panel');
    if (!container) {
      container = document.createElement('div');
      container.id = 'metastackr-submodules-panel';
      container.className = 'metastackr-panel';
    }

    // Target active tab panel bucket (discussion, files, commits, checks)
    const targetPanel = document.querySelector(
      '#discussion_bucket, .js-discussion, #files, .js-diff-container, #commits_bucket, #checks_bucket, turbo-frame#files-tab-frame'
    );

    if (targetPanel && targetPanel.parentNode) {
      if (container.parentNode !== targetPanel.parentNode) {
        targetPanel.parentNode.insertBefore(container, targetPanel);
      }
    } else {
      const layoutMain = document.querySelector('.Layout-main');
      if (layoutMain) {
        layoutMain.appendChild(container);
      }
    }

    // Hide native content buckets
    const buckets = document.querySelectorAll(
      '#discussion_bucket, .js-discussion, #files, .js-diff-container, #commits_bucket, #checks_bucket, turbo-frame#files-tab-frame'
    );
    buckets.forEach(b => {
      b.style.display = 'none';
    });

    container.style.display = 'block';

    let rowsHtml = '';
    (childPRs || []).forEach(child => {
      const shaStr = child.head_sha ? child.head_sha.substring(0, 7) : 'head';
      rowsHtml += `
        <div class="metastackr-grid-row">
          <div class="col-path"><code>${child.submodule_path}</code></div>
          <div class="col-repo">${child.repo_full_name}</div>
          <div class="col-pr"><a href="https://github.com/${child.repo_full_name}/pull/${child.pr_number}">#${child.pr_number}</a></div>
          <div class="col-sha"><code>${shaStr}</code></div>
          <div class="col-status"><span class="status-badge state-${(child.status || 'open').toLowerCase()}">${child.status || 'OPEN'}</span></div>
        </div>
      `;
    });

    container.innerHTML = `
      <div class="metastackr-panel-header">
        <div class="metastackr-title-group">
          <h2><span style="margin-right: 8px;">⚡</span>MetaStackr Submodule Matrix</h2>
          <span class="status-badge state-${(metaPR.status || 'open').toLowerCase()}">${metaPR.status || 'OPEN'}</span>
          <span class="metastackr-lock-badge">Lock Version ${metaPR.lock_version || 1}</span>
        </div>
        <button id="close-metastackr-panel" class="btn btn-sm">Close Panel</button>
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
      cleanupMetaStackrPanel();
      const tabList = document.querySelector('[class*="TabNavList"], nav[aria-label*="Pull request navigation"] div');
      if (tabList) {
        const firstTab = tabList.querySelector('a:not(#metastackr-submodules-tab)');
        if (firstTab) {
          firstTab.classList.add('selected');
          firstTab.classList.add('PullRequestHeaderTabNav-module__selected__g5kH0');
          firstTab.setAttribute('aria-current', 'page');
        }
      }
    });
  }

  function injectSidebarCard(metaPR, repoFullName, prNumber) {
    if (document.getElementById('metastackr-sidebar-card')) return;

    const sidebar = document.querySelector('.discussion-sidebar, #partial-discussion-sidebar, [data-component="sidebar"], .js-issue-sidebar');
    if (!sidebar) return;

    const status = (metaPR.status || 'OPEN').toUpperCase();
    const isFailed = status === 'FAILED' || status === 'FAILED_PARTIAL';
    const isMerged = status === 'MERGED';

    let helpText = 'Automated cascade merge executes when all child PRs are merged.';
    if (isFailed) {
      helpText = 'Cascade merge encountered an error. Click below to retry.';
    } else if (isMerged) {
      helpText = 'All submodules and parent meta-repo have been merged.';
    }

    const card = document.createElement('div');
    card.id = 'metastackr-sidebar-card';
    card.className = 'discussion-sidebar-item';
    card.innerHTML = `
      <div class="metastackr-sidebar-header">
        <span class="metastackr-sidebar-logo">⚡ metastackr</span>
      </div>
      <div class="metastackr-sidebar-body">
        <p>${helpText}</p>
        <button id="metastackr-merge-btn" class="btn btn-sm ${isFailed ? 'btn-primary' : ''} btn-block" ${!isFailed ? 'disabled' : ''} title="${!isFailed ? 'Automated merge runs when child PRs are merged. Manual retry enabled on failure.' : 'Retry cascade merge'}">
          ${isFailed ? 'Retry Cascade Merge' : 'Trigger Cascade Merge'}
        </button>
      </div>
    `;

    sidebar.appendChild(card);

    const mergeBtn = document.getElementById('metastackr-merge-btn');
    if (mergeBtn) {
      mergeBtn.addEventListener('click', () => {
        mergeBtn.disabled = true;
        mergeBtn.innerText = 'Retrying...';

        fetch(`${activeServerURL}/api/v1/prs/retry-merge`, {
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
