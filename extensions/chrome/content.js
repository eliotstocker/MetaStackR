(function() {
  'use strict';

  // GitHub uses Turbo Drive (pjax) to swap page content without full reloads
  document.addEventListener('turbo:load', initialize);
  document.addEventListener('turbo:render', initialize);
  document.addEventListener('pjax:end', initialize);
  
  // Fallback for direct page loads
  if (document.readyState === 'complete' || document.readyState === 'interactive') {
    initialize();
  } else {
    document.addEventListener('DOMContentLoaded', initialize);
  }

  // Observe DOM changes to re-inject component if GitHub dynamic navigation wipes it out
  const observer = new MutationObserver(() => {
    const prMatch = window.location.pathname.match(/^\/([^\/]+)\/([^\/]+)\/pull\/(\d+)/);
    if (prMatch) {
      if (!document.getElementById('metastackr-submodules-tab') && !document.getElementById('metastackr-child-pr-banner')) {
        initialize();
      }
    }
  });

  if (document.body) {
    observer.observe(document.body, { childList: true, subtree: true });
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
      if (!metaPR) return;

      const isParentRepo = metaPR.meta_repo_full_name
        ? (metaPR.meta_repo_full_name.toLowerCase() === repoFullName.toLowerCase())
        : true;

      if (isParentRepo) {
        // Parent Meta Repo PR -> Inject Submodules Tab & Cascade Merge Sidebar
        injectSubmodulesTab(metaPR, repoFullName, prNumber);
        injectParentSidebarCard(metaPR, repoFullName, prNumber);
      } else {
        // Child Submodule Repo PR -> Inject Link Banner to Parent Meta PR & Child Sidebar Card
        injectParentMetaPRBanner(metaPR);
        injectChildSidebarCard(metaPR);
      }
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
            callback(null);
          }
        })
        .catch(() => {
          if (fallbackURL) {
            tryFetch(fallbackURL, null);
          } else {
            console.log('[MetaStackr] Standalone backend not active or repo untracked.');
            callback(null);
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

    const sidebar = document.querySelector('.discussion-sidebar, #partial-discussion-sidebar, [data-component="sidebar"], .js-issue-sidebar, [class*="PageLayout-Sidebar"]');
    if (sidebar) {
      sidebar.style.display = '';
    }

    const contentArea = document.querySelector(
      '[class*="PageLayoutContent"], [class*="PageLayout-Content"], .diff-view, .js-diff-container, #files, #discussion_bucket, .js-pull-discussion-timeline, #commits_bucket, #checks_bucket, .Layout-main'
    );
    if (contentArea) {
      contentArea.style.maxWidth = '';
      contentArea.style.width = '';
      Array.from(contentArea.children).forEach(child => {
        if (child.id !== 'metastackr-submodules-panel') {
          child.style.display = '';
        }
      });
    }

    const subTab = document.getElementById('metastackr-submodules-tab');
    if (subTab) {
      subTab.classList.remove('selected');
      subTab.classList.remove('PullRequestHeaderTabNav-module__selected__g5kH0');
      subTab.removeAttribute('aria-current');
    }
  }

  // --- CHILD REPO: Banner & Sidebar Link back to Root Meta PR ---

  function injectParentMetaPRBanner(metaPR) {
    if (document.getElementById('metastackr-child-pr-banner')) return;

    const parentUrl = `https://github.com/${metaPR.meta_repo_full_name}/pull/${metaPR.pr_number}`;
    const header = document.querySelector('header[class*="PageLayout-Header"], #repository-container-header, .gh-header');
    if (!header) return;

    const banner = document.createElement('div');
    banner.id = 'metastackr-child-pr-banner';
    banner.className = 'metastackr-banner-container';
    banner.innerHTML = `
      <div class="metastackr-banner-inner">
        <span class="metastackr-logo">
          <svg aria-hidden="true" focusable="false" class="octicon octicon-zap" viewBox="0 0 16 16" width="16" height="16" fill="currentColor" style="vertical-align: text-bottom;">
            <path d="M8.22 1.754a.75.75 0 0 0-1.44 0L4.537 7.033A.75.75 0 0 0 5.228 8.1h2.522l-1.97 6.142a.75.75 0 0 0 1.44 0l2.243-5.279A.75.75 0 0 0 8.772 7.9H6.25l1.97-6.146Z"></path>
          </svg>
          MetaStackr
        </span>
        <div class="metastackr-status">
          Child PR linked to Parent Meta PR: <strong>${metaPR.meta_repo_full_name}#${metaPR.pr_number}</strong>
          <span class="metastackr-meta">(${metaPR.branch_name || 'branch'})</span>
        </div>
        <a href="${parentUrl}" class="btn btn-sm btn-primary">
          View Root Meta PR #${metaPR.pr_number} ↗
        </a>
      </div>
    `;

    header.parentNode.insertBefore(banner, header.nextSibling);
  }

  function injectChildSidebarCard(metaPR) {
    if (document.getElementById('metastackr-sidebar-card')) return;

    const sidebar = document.querySelector('.discussion-sidebar, #partial-discussion-sidebar, [data-component="sidebar"], .js-issue-sidebar');
    if (!sidebar) return;

    const parentUrl = `https://github.com/${metaPR.meta_repo_full_name}/pull/${metaPR.pr_number}`;

    const card = document.createElement('div');
    card.id = 'metastackr-sidebar-card';
    card.className = 'discussion-sidebar-item';
    card.innerHTML = `
      <div class="metastackr-sidebar-header">
        <span class="metastackr-sidebar-logo">⚡ metastackr</span>
      </div>
      <div class="metastackr-sidebar-body">
        <p>This submodule PR is orchestrated by parent meta-repo PR <strong>#${metaPR.pr_number}</strong>.</p>
        <a href="${parentUrl}" class="btn btn-sm btn-block">
          Open Root PR #${metaPR.pr_number} ↗
        </a>
      </div>
    `;

    sidebar.appendChild(card);
  }

  // --- PARENT REPO: Submodules Tab & Cascade Merge Sidebar ---

  function injectSubmodulesTab(metaPR, repoFullName, prNumber) {
    if (document.getElementById('metastackr-submodules-tab')) {
      const existingCounter = document.querySelector('#metastackr-submodules-tab [data-component="CounterLabel"], #metastackr-submodules-tab .Counter');
      if (existingCounter && metaPR && metaPR.child_prs) {
        existingCounter.innerText = metaPR.child_prs.length;
      }
      return;
    }

    const tabList = document.querySelector(
      '[class*="TabNavList"], nav[aria-label*="Pull request"] > div, nav[aria-label*="Pull request"] > ul, nav[aria-label*="Pull request navigation"] > div, nav[aria-label*="Pull request navigation"] > ul, .tabnav-tabs'
    );
    if (!tabList) return;

    const childPRs = (metaPR && metaPR.child_prs) ? metaPR.child_prs : [];

    const subTab = document.createElement('a');
    subTab.id = 'metastackr-submodules-tab';
    subTab.href = '#';
    subTab.className = 'tabnav-tab js-pjax-history-navigate PullRequestHeaderTabNav-module__TabNavLink__JCc1O position-relative flex-shrink-0 text-normal PullRequestHeaderNavigation-module__overrideLineHeight__TeEsl';
    subTab.innerHTML = `
      <svg data-component="Octicon" aria-hidden="true" focusable="false" class="octicon octicon-zap fg-muted mr-2 d-none d-sm-inline-block" viewBox="0 0 16 16" width="16" height="16" fill="currentColor" display="inline-block" overflow="visible" style="vertical-align: text-bottom;">
        <path d="M8.22 1.754a.75.75 0 0 0-1.44 0L4.537 7.033A.75.75 0 0 0 5.228 8.1h2.522l-1.97 6.142a.75.75 0 0 0 1.44 0l2.243-5.279A.75.75 0 0 0 8.772 7.9H6.25l1.97-6.146Z"></path>
      </svg>MetaStackr<span aria-hidden="true" data-variant="secondary" data-component="CounterLabel" class="ml-2 prc-CounterLabel-CounterLabel-X-kRU Counter">${childPRs.length}</span>
    `;

    subTab.addEventListener('click', (e) => {
      e.preventDefault();
      tabList.querySelectorAll('a').forEach(t => {
        t.classList.remove('selected');
        t.classList.remove('PullRequestHeaderTabNav-module__selected__g5kH0');
        t.removeAttribute('aria-current');
      });
      subTab.classList.add('selected');
      subTab.classList.add('PullRequestHeaderTabNav-module__selected__g5kH0');
      subTab.setAttribute('aria-current', 'page');

      showSubmodulesGrid(metaPR, childPRs);
    });

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

    const prHeader = document.querySelector('header[class*="PageLayout-Header"], #repository-container-header, .gh-header');
    if (prHeader) prHeader.style.display = '';

    // Hide sidebar on conversation view so matrix takes 100% width
    const sidebar = document.querySelector('.discussion-sidebar, #partial-discussion-sidebar, [data-component="sidebar"], .js-issue-sidebar, [class*="PageLayout-Sidebar"]');
    if (sidebar) sidebar.style.display = 'none';

    const contentArea = document.querySelector(
      '[class*="PageLayoutContent"], [class*="PageLayout-Content"], .diff-view, .js-diff-container, #files, #discussion_bucket, .js-pull-discussion-timeline, #commits_bucket, #checks_bucket, .Layout-main'
    );

    if (contentArea) {
      contentArea.style.maxWidth = '100%';
      contentArea.style.width = '100%';
      Array.from(contentArea.children).forEach(child => {
        if (child.id !== 'metastackr-submodules-panel') {
          child.style.display = 'none';
        }
      });
      if (container.parentNode !== contentArea) {
        contentArea.appendChild(container);
      }
    } else {
      const fallbackContainer = document.querySelector('#repo-content-turbo-frame, #repo-content-pjax-container, .repository-content');
      if (fallbackContainer && container.parentNode !== fallbackContainer) {
        fallbackContainer.appendChild(container);
      }
    }

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
          <h2>
            <svg aria-hidden="true" focusable="false" class="octicon octicon-zap" viewBox="0 0 16 16" width="16" height="16" fill="currentColor" style="vertical-align: text-bottom; margin-right: 8px;">
              <path d="M8.22 1.754a.75.75 0 0 0-1.44 0L4.537 7.033A.75.75 0 0 0 5.228 8.1h2.522l-1.97 6.142a.75.75 0 0 0 1.44 0l2.243-5.279A.75.75 0 0 0 8.772 7.9H6.25l1.97-6.146Z"></path>
            </svg>Submodule Synchronization Matrix
          </h2>
          <span class="status-badge state-${(metaPR.status || 'open').toLowerCase()}">${metaPR.status || 'OPEN'}</span>
          <span class="metastackr-lock-badge">Lock Version ${metaPR.lock_version || 1}</span>
        </div>
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

    container.style.display = 'block';
  }

  function injectParentSidebarCard(metaPR, repoFullName, prNumber) {
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
        <span class="metastackr-sidebar-logo" style="display: flex; align-items: center; gap: 6px;">
          <svg aria-hidden="true" focusable="false" class="octicon octicon-zap" viewBox="0 0 16 16" width="16" height="16" fill="currentColor">
            <path d="M8.22 1.754a.75.75 0 0 0-1.44 0L4.537 7.033A.75.75 0 0 0 5.228 8.1h2.522l-1.97 6.142a.75.75 0 0 0 1.44 0l2.243-5.279A.75.75 0 0 0 8.772 7.9H6.25l1.97-6.146Z"></path>
          </svg>metastackr
        </span>
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
