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

  function getPRUrl(repoFullName, prNumber) {
    if (window.location.hostname.includes('gitlab.com')) {
      return `https://gitlab.com/${repoFullName}/-/merge_requests/${prNumber}`;
    }
    return `https://github.com/${repoFullName}/pull/${prNumber}`;
  }

  function getPRInfoFromURL() {
    const ghMatch = window.location.pathname.match(/^\/([^\/]+)\/([^\/]+)\/pull\/(\d+)/);
    if (ghMatch) {
      return { owner: ghMatch[1], repo: ghMatch[2], prNumber: parseInt(ghMatch[3], 10), repoFullName: `${ghMatch[1]}/${ghMatch[2]}` };
    }
    const glMatch = window.location.pathname.match(/^\/([^\/]+)\/([^\/]+)\/(?:-\/)?merge_requests\/(\d+)/);
    if (glMatch) {
      return { owner: glMatch[1], repo: glMatch[2], prNumber: parseInt(glMatch[3], 10), repoFullName: `${glMatch[1]}/${glMatch[2]}` };
    }
    return null;
  }

  // Observe DOM changes to re-inject component if dynamic navigation wipes it out
  const observer = new MutationObserver(() => {
    const prInfo = getPRInfoFromURL();
    if (prInfo) {
      if (!document.getElementById('metastackr-submodules-tab') && !document.getElementById('metastackr-child-pr-banner')) {
        initialize();
      }
    }
  });

  if (document.body) {
    observer.observe(document.body, { childList: true, subtree: true });
  }

  function initialize() {
    const prInfo = getPRInfoFromURL();
    if (!prInfo) return;

    if (window.location.hostname.includes('gitlab.com')) {
      document.body.classList.add('metastackr-gitlab');
      document.body.classList.remove('metastackr-github');
    } else {
      document.body.classList.add('metastackr-github');
      document.body.classList.remove('metastackr-gitlab');
    }

    const owner = prInfo.owner;
    const repo = prInfo.repo;
    const prNumber = prInfo.prNumber;
    const repoFullName = prInfo.repoFullName;

    // Get current branch from GitHub or GitLab DOM
    const headBranchEl = document.querySelector(
      '.head-ref, span.commit-ref, a.commit-ref, [data-hovercard-type="commit"], .branch-name, ' +
      '.ref-name, .source-branch, [data-testid="ref-name"], a[href*="/-/commits/"], .mr-source-branch, span.gl-font-monospace'
    );
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
  window.addEventListener('hashchange', () => {
    const subTab = document.getElementById('metastackr-submodules-tab');
    if (subTab && (subTab.classList.contains('selected') || subTab.classList.contains('active'))) {
      if (!window.location.hash.includes('metastackr')) {
        cleanupMetaStackrPanel();
      }
    }
  });

  // SPA Route Watcher for non-hash location changes
  let lastObservedUrl = window.location.href;
  setInterval(() => {
    if (window.location.href !== lastObservedUrl) {
      lastObservedUrl = window.location.href;
      const subTab = document.getElementById('metastackr-submodules-tab');
      if (subTab && (subTab.classList.contains('selected') || subTab.classList.contains('active'))) {
        if (!window.location.hash.includes('metastackr')) {
          cleanupMetaStackrPanel();
        }
      }
    }
  }, 300);

  // Global capture-phase click listener to intercept native tab clicks before framework stopPropagation
  document.addEventListener('click', (e) => {
    const subTab = document.getElementById('metastackr-submodules-tab');
    if (!subTab) return;

    const isMetaActive = subTab.classList.contains('active') || subTab.classList.contains('selected') || document.body.classList.contains('metastackr-active');
    if (!isMetaActive) return;

    const clickedEl = e.target.closest('a, button, li, [role="tab"]');
    if (!clickedEl) return;

    if (clickedEl.id === 'metastackr-submodules-tab' || clickedEl.id === 'metastackr-submodules-tab-li' || clickedEl.querySelector('#metastackr-submodules-tab')) {
      return; // Clicked MetaStackr tab itself
    }

    const isNavTab = clickedEl.closest(
      '.notes-tab, .commits-tab, .pipelines-tab, .diffs-tab, .merge-request-tabs, .mr-tabs, .nav-tabs, .tabs-wrapper, .tabnav-tabs, .nav-item, .nav-link, [role="tab"], .gl-tab-nav-item'
    );
    if (isNavTab) {
      stopTabPolling();
      cleanupMetaStackrPanel();
    }
  }, true);

  function cleanupMetaStackrPanel() {
    document.body.classList.remove('metastackr-active');

    const container = document.getElementById('metastackr-submodules-panel');
    if (container) {
      container.style.display = 'none';
    }

    const sidebar = document.querySelector(
      '.discussion-sidebar, #partial-discussion-sidebar, [data-component="sidebar"], .js-issue-sidebar, [class*="PageLayout-Sidebar"], ' +
      'aside.right-sidebar, .issuable-sidebar, [data-testid="issuable-sidebar"], .sidebar-wrapper-inner'
    );
    if (sidebar) {
      sidebar.style.display = '';
    }

    const contentArea = document.querySelector(
      '[class*="PageLayoutContent"], [class*="PageLayout-Content"], .diff-view, .js-diff-container, #files, #discussion_bucket, .js-pull-discussion-timeline, #commits_bucket, #checks_bucket, .Layout-main, ' +
      '.merge-request-details, .issuable-details, #notes, .tab-content, .mr-state-widget'
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
      subTab.classList.remove('active');
      subTab.classList.remove('PullRequestHeaderTabNav-module__selected__g5kH0');
      subTab.removeAttribute('aria-current');
      if (subTab.parentNode && subTab.parentNode.classList.contains('nav-item')) {
        subTab.parentNode.classList.remove('active');
      }
    }
  }

  // --- CHILD REPO: Banner & Sidebar Link back to Root Meta PR ---

  function injectParentMetaPRBanner(metaPR) {
    if (document.getElementById('metastackr-child-pr-banner')) return;

    const parentUrl = getPRUrl(metaPR.meta_repo_full_name, metaPR.pr_number);
    const header = document.querySelector('header[class*="PageLayout-Header"], #repository-container-header, .gh-header, .detail-page-header, .detail-page-description, .mr-title, [data-testid="mr-title"]');
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

    const sidebar = document.querySelector('.discussion-sidebar, #partial-discussion-sidebar, [data-component="sidebar"], .js-issue-sidebar, aside.right-sidebar, .issuable-sidebar, [data-testid="issuable-sidebar"], .sidebar-wrapper-inner');
    if (!sidebar) return;

    const parentUrl = getPRUrl(metaPR.meta_repo_full_name, metaPR.pr_number);

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
      const existingCounter = document.querySelector('#metastackr-submodules-tab [data-component="CounterLabel"], #metastackr-submodules-tab .Counter, #metastackr-submodules-tab .badge');
      if (existingCounter && metaPR && metaPR.child_prs) {
        existingCounter.innerText = metaPR.child_prs.length;
      }
      return;
    }

    const tabList = document.querySelector(
      '[class*="TabNavList"], nav[aria-label*="Pull request"] > div, nav[aria-label*="Pull request"] > ul, nav[aria-label*="Pull request navigation"] > div, nav[aria-label*="Pull request navigation"] > ul, .tabnav-tabs, ' +
      'ul.mr-tabs, ul.nav-tabs, nav.tabs-wrapper, .merge-request-tabs, [data-testid="mr-tabs"], .gl-tabs-nav'
    );
    if (!tabList) return;

    const childPRs = (metaPR && metaPR.child_prs) ? metaPR.child_prs : [];

    const isGitLab = window.location.hostname.includes('gitlab.com');

    const subTab = document.createElement('a');
    subTab.id = 'metastackr-submodules-tab';
    subTab.href = '#metastackr';

    if (isGitLab) {
      subTab.className = 'nav-link';
      subTab.innerHTML = `⚡ MetaStackr <span class="gl-badge badge badge-pill gl-tab-counter-badge sm ml-1">${childPRs.length}</span>`;
    } else {
      subTab.className = 'tabnav-tab js-pjax-history-navigate PullRequestHeaderTabNav-module__TabNavLink__JCc1O position-relative flex-shrink-0 text-normal PullRequestHeaderNavigation-module__overrideLineHeight__TeEsl';
      subTab.innerHTML = `
        <svg data-component="Octicon" aria-hidden="true" focusable="false" class="octicon octicon-zap fg-muted mr-2 d-none d-sm-inline-block" viewBox="0 0 16 16" width="16" height="16" fill="currentColor" display="inline-block" overflow="visible" style="vertical-align: text-bottom;">
          <path d="M8.22 1.754a.75.75 0 0 0-1.44 0L4.537 7.033A.75.75 0 0 0 5.228 8.1h2.522l-1.97 6.142a.75.75 0 0 0 1.44 0l2.243-5.279A.75.75 0 0 0 8.772 7.9H6.25l1.97-6.146Z"></path>
        </svg>MetaStackr<span aria-hidden="true" data-variant="secondary" data-component="CounterLabel" class="ml-2 prc-CounterLabel-CounterLabel-X-kRU Counter">${childPRs.length}</span>
      `;
    }

    let activeTabPollInterval = null;

    function stopTabPolling() {
      if (activeTabPollInterval) {
        clearInterval(activeTabPollInterval);
        activeTabPollInterval = null;
      }
    }

    subTab.addEventListener('click', (e) => {
      e.preventDefault();
      e.stopPropagation();

      tabList.querySelectorAll('li, a').forEach(t => {
        if (t !== subTab && t !== tabContainer) {
          t.classList.remove('selected');
          t.classList.remove('active');
          t.classList.remove('PullRequestHeaderTabNav-module__selected__g5kH0');
          t.removeAttribute('aria-current');
        }
      });
      subTab.classList.add('selected');
      subTab.classList.add('active');
      subTab.setAttribute('aria-current', 'page');
      if (tabContainer !== subTab) {
        tabContainer.classList.add('active');
        tabContainer.classList.add('selected');
      }

      showSubmodulesGrid(metaPR, metaPR ? metaPR.child_prs : [], repoFullName);

      // Re-fetch latest PR status on tab click
      const prMatch = window.location.pathname.match(/^\/([^\/]+)\/([^\/]+)\/pull\/(\d+)/);
      if (prMatch) {
        const headBranchEl = document.querySelector('.head-ref, span.commit-ref, a.commit-ref, [data-hovercard-type="commit"], .branch-name, .source-branch');
        const branchName = headBranchEl ? headBranchEl.innerText.trim() : 'head';
        fetchPRStatus(repoFullName, prNumber, branchName, (latestPR) => {
          if (latestPR) {
            metaPR = latestPR;
            showSubmodulesGrid(latestPR, latestPR.child_prs, repoFullName);
          }
        });
      }

      // Auto-poll status every 3s while MetaStackr tab is active
      stopTabPolling();
      activeTabPollInterval = setInterval(() => {
        if (!document.getElementById('metastackr-submodules-tab') || (!subTab.classList.contains('selected') && !subTab.classList.contains('active'))) {
          stopTabPolling();
          return;
        }
        const prMatch = window.location.pathname.match(/^\/([^\/]+)\/([^\/]+)\/pull\/(\d+)/);
        if (prMatch) {
          const headBranchEl = document.querySelector('.head-ref, span.commit-ref, a.commit-ref, [data-hovercard-type="commit"], .branch-name, .source-branch');
          const branchName = headBranchEl ? headBranchEl.innerText.trim() : 'head';
          fetchPRStatus(repoFullName, prNumber, branchName, (latestPR) => {
            if (latestPR) {
              metaPR = latestPR;
              showSubmodulesGrid(latestPR, latestPR.child_prs, repoFullName);
            }
          });
        }
      }, 3000);
    });

    if (tabList.tagName === 'UL') {
      const li = document.createElement('li');
      li.id = 'metastackr-submodules-tab-li';
      li.className = 'nav-item metastackr-tab-li';
      li.appendChild(subTab);
      tabList.appendChild(li);
    } else {
      tabList.appendChild(subTab);
    }
  }

  function showSubmodulesGrid(metaPR, childPRs, repoFullName) {
    document.body.classList.add('metastackr-active');

    const targetRepo = (metaPR && metaPR.meta_repo_full_name) 
      ? metaPR.meta_repo_full_name 
      : (repoFullName || (window.location.pathname.match(/^\/([^\/]+\/[^\/]+)/) || [])[1]);

    let container = document.getElementById('metastackr-submodules-panel');
    const isAlreadyRendered = !!container && container.dataset.rendered === 'true';

    if (!container) {
      container = document.createElement('div');
      container.id = 'metastackr-submodules-panel';
      container.className = 'metastackr-panel';
    }

    const prHeader = document.querySelector('header[class*="PageLayout-Header"], #repository-container-header, .gh-header, .detail-page-header, .detail-page-description, .mr-title, [data-testid="mr-title"]');
    if (prHeader) prHeader.style.display = '';

    // Hide sidebar on conversation view so matrix takes 100% width
    const sidebar = document.querySelector('.discussion-sidebar, #partial-discussion-sidebar, [data-component="sidebar"], .js-issue-sidebar, [class*="PageLayout-Sidebar"], aside.right-sidebar, .issuable-sidebar, [data-testid="issuable-sidebar"], .sidebar-wrapper-inner');
    if (sidebar) sidebar.style.display = 'none';

    const tabsContainer = document.querySelector('.merge-request-tabs-container, .js-tabs-affix, .tabs-holder, .tabs-wrapper');
    if (tabsContainer && tabsContainer.parentNode) {
      if (container.parentNode !== tabsContainer.parentNode) {
        tabsContainer.parentNode.insertBefore(container, tabsContainer.nextSibling);
      }
    } else {
      const tabList = document.querySelector(
        '[class*="TabNavList"], nav[aria-label*="Pull request"] > div, nav[aria-label*="Pull request"] > ul, nav[aria-label*="Pull request navigation"] > div, nav[aria-label*="Pull request navigation"] > ul, .tabnav-tabs, ' +
        'ul.mr-tabs, ul.nav-tabs, nav.tabs-wrapper, .merge-request-tabs, [data-testid="mr-tabs"], .gl-tabs-nav'
      );

      if (tabList && tabList.parentNode) {
        if (container.parentNode !== tabList.parentNode) {
          tabList.parentNode.insertBefore(container, tabList.nextSibling);
        }
      } else {
        const contentArea = document.querySelector(
          '[class*="PageLayoutContent"], [class*="PageLayout-Content"], .diff-view, .js-diff-container, #files, #discussion_bucket, .js-pull-discussion-timeline, #commits_bucket, #checks_bucket, .Layout-main, ' +
          '.merge-request-details, .issuable-details, .tab-content'
        );
        if (contentArea && container.parentNode !== contentArea) {
          contentArea.appendChild(container);
        }
      }
    }

    let rowsHtml = '';
    (childPRs || []).forEach(child => {
      const shaStr = child.head_sha ? child.head_sha.substring(0, 7) : 'head';
      const childPRUrl = getPRUrl(child.repo_full_name, child.pr_number);
      rowsHtml += `
        <div class="metastackr-grid-row">
          <div class="col-path"><code>${child.submodule_path}</code></div>
          <div class="col-repo">${child.repo_full_name}</div>
          <div class="col-pr"><a href="${childPRUrl}">#${child.pr_number}</a></div>
          <div class="col-sha"><code>${shaStr}</code></div>
          <div class="col-status"><span class="status-badge state-${(child.status || 'open').toLowerCase()}">${child.status || 'OPEN'}</span></div>
        </div>
      `;
    });

    if (isAlreadyRendered) {
      // Surgical update of matrix grid without destroying settings panel state
      const gridContainer = container.querySelector('.metastackr-grid');
      if (gridContainer) {
        gridContainer.innerHTML = `
          <div class="metastackr-grid-header">
            <div class="col-path">Submodule Path</div>
            <div class="col-repo">Submodule Repo</div>
            <div class="col-pr">PR #</div>
            <div class="col-sha">Commit SHA</div>
            <div class="col-status">Merge Status</div>
          </div>
          ${rowsHtml || '<div class="metastackr-empty">No child submodules tracked for this branch</div>'}
        `;
      }
      const statusBadge = container.querySelector('.metastackr-title-group .status-badge');
      if (statusBadge) {
        statusBadge.className = `status-badge state-${(metaPR.status || 'open').toLowerCase()}`;
        statusBadge.innerText = metaPR.status || 'OPEN';
      }
      const lockBadge = container.querySelector('.metastackr-badge-muted');
      if (lockBadge) {
        lockBadge.innerText = `Lock Version ${metaPR.lock_version || 1}`;
      }
      container.style.display = 'block';
      return;
    }

    container.dataset.rendered = 'true';
    container.innerHTML = `
      <div class="metastackr-box">
        <div class="metastackr-box-header">
          <div class="metastackr-title-group">
            <h3 class="metastackr-box-title">
              <svg aria-hidden="true" focusable="false" class="octicon octicon-zap" viewBox="0 0 16 16" width="16" height="16" fill="currentColor">
                <path d="M8.22 1.754a.75.75 0 0 0-1.44 0L4.537 7.033A.75.75 0 0 0 5.228 8.1h2.522l-1.97 6.142a.75.75 0 0 0 1.44 0l2.243-5.279A.75.75 0 0 0 8.772 7.9H6.25l1.97-6.146Z"></path>
              </svg>
              Submodule Synchronization Matrix
            </h3>
            <span class="status-badge state-${(metaPR.status || 'open').toLowerCase()}">${metaPR.status || 'OPEN'}</span>
          </div>
          <span class="metastackr-badge-muted">Lock Version ${metaPR.lock_version || 1}</span>
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
      </div>

      <details class="metastackr-settings-details">
        <summary>
          <svg aria-hidden="true" focusable="false" class="octicon octicon-gear" viewBox="0 0 16 16" width="16" height="16" fill="currentColor">
            <path d="M8 0a8 8 0 1 1 0 16A8 8 0 0 1 8 0ZM1.5 8a6.5 6.5 0 1 0 13 0 6.5 6.5 0 0 0-13 0Z"></path>
          </svg>
          <span>Auto-Merge Policy Rules</span>
          <span class="caret"></span>
        </summary>
        <div class="metastackr-box metastackr-settings-card">
          <div style="background: transparent; border-bottom: 1px solid var(--ms-border-default); padding-bottom: 8px; margin-bottom: 12px;">
            <h3 class="metastackr-box-title">
              Policy Rules Settings
            </h3>
          </div>
          <div style="display: flex; flex-direction: column; gap: 12px; font-size: 13px;">
            <label class="metastackr-checkbox-label">
              <input type="checkbox" id="metastackr-require-approval-chk" checked>
              <span><strong>Require Root PR Approval</strong> — Root PR must have an <code>APPROVED</code> review before auto-merge</span>
            </label>
            <label class="metastackr-checkbox-label">
              <input type="checkbox" id="metastackr-auto-merge-chk" checked>
              <span><strong>Enable Auto Cascade Merge</strong> — Automatically trigger cascade merge when all policy rules pass</span>
            </label>
            <label class="metastackr-checkbox-label">
              <input type="checkbox" id="metastackr-submodule-only-chk" checked>
              <span><strong>Submodule Only Changes</strong> — Auto-merge only when changes are restricted to submodules (pause if root files are modified)</span>
            </label>
            <div class="metastackr-form-group" style="margin-top: 4px;">
              <label for="metastackr-req-checks-input"><strong>Required Status Checks:</strong> <span style="font-weight: normal; opacity: 0.8;">(comma-separated check names, e.g. <code>ci/build, lint</code>)</span></label>
              <input type="text" id="metastackr-req-checks-input" class="metastackr-form-control" value="" placeholder="e.g. ci/build, test" style="max-width: 450px;">
            </div>
            <div class="metastackr-form-group" style="margin-top: 4px;">
              <label for="metastackr-merge-method-select"><strong>Default Merge Method:</strong></label>
              <select id="metastackr-merge-method-select" class="metastackr-form-control" style="width: 160px;">
                <option value="merge" selected>Merge (commit)</option>
                <option value="squash">Squash</option>
                <option value="rebase">Rebase</option>
              </select>
            </div>
            <div style="margin-top: 8px; display: flex; align-items: center; gap: 12px;">
              <button type="button" id="metastackr-save-settings-btn" class="btn btn-sm btn-primary">Save Policy Rules</button>
              <span id="metastackr-settings-status" style="font-size: 12px; color: var(--ms-state-merged-fg, #1a7f37);"></span>
            </div>
          </div>
        </div>
      </details>
    `;

    container.style.display = 'block';

    if (targetRepo) {
      // Async load saved settings from backend
      fetch(`https://api.metastac.kr/api/v1/repos/settings?repo=${encodeURIComponent(targetRepo)}`)
        .then(res => res.ok ? res.json() : null)
        .catch(() => null)
        .then(settings => {
          if (settings) {
            const chkApproval = document.getElementById('metastackr-require-approval-chk');
            const chkAuto = document.getElementById('metastackr-auto-merge-chk');
            const chkSubOnly = document.getElementById('metastackr-submodule-only-chk');
            const inputChecks = document.getElementById('metastackr-req-checks-input');
            const selectMethod = document.getElementById('metastackr-merge-method-select');

            if (chkApproval) chkApproval.checked = settings.require_root_approval !== false;
            if (chkAuto) chkAuto.checked = settings.auto_merge_enabled !== false;
            if (chkSubOnly) chkSubOnly.checked = settings.submodule_changes_only !== false;
            if (inputChecks) inputChecks.value = (settings.required_checks || []).join(', ');
            if (selectMethod) selectMethod.value = settings.default_merge_method || 'merge';
          }
        });
    }

    const saveBtn = document.getElementById('metastackr-save-settings-btn');
    if (saveBtn) {
      saveBtn.onclick = function(e) {
        if (e) {
          e.preventDefault();
          e.stopPropagation();
        }
        const reqApproval = document.getElementById('metastackr-require-approval-chk').checked;
        const autoMerge = document.getElementById('metastackr-auto-merge-chk').checked;
        const subOnly = document.getElementById('metastackr-submodule-only-chk').checked;
        const checksRaw = document.getElementById('metastackr-req-checks-input').value;
        const mergeMethod = document.getElementById('metastackr-merge-method-select').value;
        const reqChecks = checksRaw.split(',').map(s => s.trim()).filter(Boolean);

        const statusEl = document.getElementById('metastackr-settings-status');
        if (statusEl) {
          statusEl.innerText = 'Saving...';
          statusEl.style.color = 'var(--ms-fg-muted, #57606a)';
        }

        const activeRepo = targetRepo || (window.location.pathname.match(/^\/([^\/]+\/[^\/]+)/) || [])[1];

        fetch('https://api.metastac.kr/api/v1/repos/settings', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            repo: activeRepo,
            require_root_approval: reqApproval,
            auto_merge_enabled: autoMerge,
            submodule_changes_only: subOnly,
            required_checks: reqChecks,
            default_merge_method: mergeMethod
          })
        })
        .then(res => res.json())
        .then(data => {
          if (statusEl) {
            if (data && data.success) {
              statusEl.innerText = '✅ Saved successfully!';
              statusEl.style.color = 'var(--ms-state-merged-fg, #1a7f37)';
              setTimeout(() => { statusEl.innerText = ''; }, 3000);
            } else {
              statusEl.innerText = '❌ Failed to save settings';
              statusEl.style.color = 'var(--ms-state-failed-fg, #cf222e)';
            }
          }
        })
        .catch(err => {
          if (statusEl) {
            statusEl.innerText = `❌ Error: ${err.message}`;
            statusEl.style.color = 'var(--ms-state-failed-fg, #cf222e)';
          }
        });
      };
    }
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
