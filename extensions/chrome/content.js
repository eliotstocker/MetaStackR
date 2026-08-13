(function () {
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
    const glMatch = window.location.pathname.match(/^\/(.+?)\/(?:-\/)?merge_requests\/(\d+)/);
    if (glMatch) {
      const fullPath = glMatch[1];
      const prNumber = parseInt(glMatch[2], 10);
      const parts = fullPath.split('/');
      const repo = parts[parts.length - 1];
      const owner = parts.slice(0, parts.length - 1).join('/');
      return { owner, repo, prNumber, repoFullName: fullPath };
    }
    return null;
  }

  function getBranchName() {
    if (window.location.hostname.includes('gitlab.com')) {
      const glSourceEl = document.querySelector(
        '[data-testid="mr-widget-source-branch"], .js-source-branch, .mr-source-branch, ' +
        'a[data-testid="source-branch-link"], .source-branch, .mr-widget-section .source-branch'
      );
      if (glSourceEl) {
        let text = glSourceEl.innerText.trim();
        text = text.split(/\s+/)[0];
        if (text && !/^[0-9a-f]{7,40}$/i.test(text)) {
          return text;
        }
      }
    }

    const headBranchEl = document.querySelector(
      '.head-ref, span.commit-ref, a.commit-ref, [data-hovercard-type="commit"], ' +
      '.branch-name, .ref-name, .source-branch, [data-testid="ref-name"]'
    );
    if (headBranchEl) {
      let text = headBranchEl.innerText.trim();
      text = text.split(/\s+/)[0];
      if (text && !/^[0-9a-f]{7,40}$/i.test(text)) {
        return text;
      }
    }

    return 'head';
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
    const branchName = getBranchName();

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
    console.log('[MetaStackr] Fetching status for repo:', repo, 'pr:', prNumber, 'branch:', branch);
    if (typeof chrome !== 'undefined' && chrome.runtime && chrome.runtime.sendMessage) {
      chrome.runtime.sendMessage({ action: 'fetchPRStatus', repo, prNumber, branch }, (response) => {
        if (!chrome.runtime.lastError && response && response.metaPR) {
          if (response.serverURL) activeServerURL = response.serverURL;
          console.log('[MetaStackr] Status response received via background script:', response.metaPR);
          callback(response.metaPR);
          return;
        }
        console.log('[MetaStackr] Background script fallback -> running directFetchPRStatus');
        directFetchPRStatus(repo, prNumber, branch, callback);
      });
    } else {
      directFetchPRStatus(repo, prNumber, branch, callback);
    }
  }

  function directFetchPRStatus(repo, prNumber, branch, callback) {
    const tryFetch = (serverURL, branchParam, nextFallback) => {
      const url = `${serverURL}/api/v1/prs/status?repo=${encodeURIComponent(repo)}&pr=${prNumber}&branch=${encodeURIComponent(branchParam)}`;
      console.log('[MetaStackr Direct Fetch] Requesting:', url);
      fetch(url)
        .then(res => res.ok ? res.json() : null)
        .then(data => {
          if (data && data.meta_pr) {
            activeServerURL = serverURL;
            console.log('[MetaStackr Direct Fetch] Received MetaPR:', data.meta_pr);
            callback(data.meta_pr);
          } else if (nextFallback) {
            nextFallback();
          } else {
            callback(null);
          }
        })
        .catch(() => {
          if (nextFallback) {
            nextFallback();
          } else {
            console.log('[MetaStackr] Standalone backend not active or repo untracked.');
            callback(null);
          }
        });
    };

    // Try live API with branch -> localhost with branch -> live API fallback (head) -> localhost fallback (head)
    tryFetch('https://api.metastac.kr', branch, () => {
      tryFetch('http://localhost:8080', branch, () => {
        if (branch !== 'head' && branch !== '') {
          tryFetch('https://api.metastac.kr', 'head', () => {
            tryFetch('http://localhost:8080', 'head', () => callback(null));
          });
        } else {
          callback(null);
        }
      });
    });
  }

  let activeTabPollInterval = null;

  function stopTabPolling() {
    if (typeof activeTabPollInterval !== 'undefined' && activeTabPollInterval) {
      clearInterval(activeTabPollInterval);
      activeTabPollInterval = null;
    }
  }

  function cleanupMetaStackrPanel() {
    if (typeof stopTabPolling === 'function') {
      stopTabPolling();
    }
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
      if (typeof stopTabPolling === 'function') {
        stopTabPolling();
      }
      cleanupMetaStackrPanel();
    }
  }, true);

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

  function injectSubmodulesTab(metaPR, repoFullName, prNumber, retryCount = 0) {
    if (document.getElementById('metastackr-submodules-tab')) {
      const existingCounter = document.querySelector(
        '#metastackr-submodules-tab .gl-badge-content, ' +
        '#metastackr-submodules-tab [data-component="CounterLabel"], ' +
        '#metastackr-submodules-tab .Counter, ' +
        '#metastackr-submodules-tab .badge, ' +
        '#metastackr-submodules-tab .gl-badge, ' +
        '#metastackr-submodules-tab .gl-tab-counter-badge'
      );
      if (existingCounter && metaPR && metaPR.child_prs) {
        existingCounter.innerText = metaPR.child_prs.length;
      }
      return;
    }

    let tabList = document.querySelector(
      '[class*="TabNavList"], nav[aria-label*="Pull request"] > div, nav[aria-label*="Pull request"] > ul, nav[aria-label*="Pull request navigation"] > div, nav[aria-label*="Pull request navigation"] > ul, .tabnav-tabs, ' +
      'ul.mr-tabs, ul.nav-tabs, nav.tabs-wrapper, .merge-request-tabs, [data-testid="mr-tabs"], .gl-tabs-nav, ul.gl-tabs-nav, ' +
      '[data-testid="merge-request-tabs"], .issuable-tabs, .tabs-holder ul, .merge-request-tabs-container ul, .js-tabs-affix ul, [role="tablist"]'
    );

    if (!tabList) {
      const anyTabLink = document.querySelector('.notes-tab, .commits-tab, .pipelines-tab, .diffs-tab, .gl-tab-nav-item, a[href*="#notes"], a[href*="#diffs"], a[href*="merge_requests"]');
      if (anyTabLink) {
        tabList = anyTabLink.closest('ul, nav, [role="tablist"]') || anyTabLink.parentElement;
      }
    }

    if (!tabList) {
      if (retryCount < 10) {
        setTimeout(() => injectSubmodulesTab(metaPR, repoFullName, prNumber, retryCount + 1), 300);
      }
      return;
    }

    const childPRs = (metaPR && metaPR.child_prs) ? metaPR.child_prs : [];

    const isGitLab = window.location.hostname.includes('gitlab.com');

    const subTab = document.createElement('a');
    subTab.id = 'metastackr-submodules-tab';
    subTab.href = '#metastackr';

    if (isGitLab) {
      subTab.className = 'nav-link';
      const sampleTab = tabList ? tabList.querySelector('a.nav-link, button.gl-tab-nav-item, li.nav-item a') : null;
      if (sampleTab && sampleTab.className) {
        subTab.className = sampleTab.className.replace(/\b(active|selected)\b/g, '').trim();
      }

      const badgeHtml = `<span class="gl-tab-counter-badge gl-badge badge badge-pill sm"><span class="gl-badge-content">${childPRs.length}</span></span>`;
      subTab.innerHTML = `<span>MetaStackr</span> ${badgeHtml}`;
    } else {
      subTab.className = 'tabnav-tab js-pjax-history-navigate PullRequestHeaderTabNav-module__TabNavLink__JCc1O position-relative flex-shrink-0 text-normal PullRequestHeaderNavigation-module__overrideLineHeight__TeEsl';
      subTab.innerHTML = `
        <svg data-component="Octicon" aria-hidden="true" focusable="false" class="octicon octicon-zap fg-muted mr-2 d-none d-sm-inline-block" viewBox="0 0 16 16" width="16" height="16" fill="currentColor" display="inline-block" overflow="visible" style="vertical-align: text-bottom;">
          <path d="M8.22 1.754a.75.75 0 0 0-1.44 0L4.537 7.033A.75.75 0 0 0 5.228 8.1h2.522l-1.97 6.142a.75.75 0 0 0 1.44 0l2.243-5.279A.75.75 0 0 0 8.772 7.9H6.25l1.97-6.146Z"></path>
        </svg>MetaStackr<span aria-hidden="true" data-variant="secondary" data-component="CounterLabel" class="ml-2 prc-CounterLabel-CounterLabel-X-kRU Counter">${childPRs.length}</span>
      `;
    }

    let tabContainer = subTab;
    if (tabList.tagName === 'UL') {
      const li = document.createElement('li');
      li.id = 'metastackr-submodules-tab-li';
      li.className = 'nav-item metastackr-tab-li';
      li.appendChild(subTab);
      tabContainer = li;
    }

    subTab.addEventListener('click', (e) => {
      e.preventDefault();
      e.stopPropagation();

      window.location.hash = '#metastackr';

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
      const prInfo = getPRInfoFromURL();
      if (prInfo) {
        const branchName = getBranchName();
        fetchPRStatus(prInfo.repoFullName, prInfo.prNumber, branchName, (latestPR) => {
          if (latestPR) {
            metaPR = latestPR;
            showSubmodulesGrid(latestPR, latestPR.child_prs, prInfo.repoFullName);
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
        const prInfoPoll = getPRInfoFromURL();
        if (prInfoPoll) {
          const branchName = getBranchName();
          fetchPRStatus(prInfoPoll.repoFullName, prInfoPoll.prNumber, branchName, (latestPR) => {
            if (latestPR) {
              metaPR = latestPR;
              showSubmodulesGrid(latestPR, latestPR.child_prs, prInfoPoll.repoFullName);
            }
          });
        }
      }, 3000);
    });

    tabList.appendChild(tabContainer);
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

    const tabsContainer = document.querySelector('.merge-request-tabs-container, .js-tabs-affix, .tabs-holder, .tabs-wrapper, .gl-tabs');
    if (tabsContainer && tabsContainer.parentNode) {
      if (container.parentNode !== tabsContainer.parentNode) {
        tabsContainer.parentNode.insertBefore(container, tabsContainer.nextSibling);
      }
    } else {
      let tabList = document.querySelector(
        '[class*="TabNavList"], nav[aria-label*="Pull request"] > div, nav[aria-label*="Pull request"] > ul, nav[aria-label*="Pull request navigation"] > div, nav[aria-label*="Pull request navigation"] > ul, .tabnav-tabs, ' +
        'ul.mr-tabs, ul.nav-tabs, nav.tabs-wrapper, .merge-request-tabs, [data-testid="mr-tabs"], .gl-tabs-nav, ul.gl-tabs-nav, [role="tablist"]'
      );
      if (!tabList) {
        const anyTabLink = document.querySelector('.notes-tab, .commits-tab, .pipelines-tab, .diffs-tab, .gl-tab-nav-item, a[href*="#notes"], a[href*="#diffs"]');
        if (anyTabLink) {
          tabList = anyTabLink.closest('ul, nav, [role="tablist"]') || anyTabLink.parentElement;
        }
      }

      if (tabList && tabList.parentNode) {
        const insertTarget = tabList.closest('.merge-request-tabs, .tabs-holder, .tabs-wrapper, .gl-tabs-nav, .nav-tabs, [role="tablist"]') || tabList;
        if (insertTarget.parentNode && container.parentNode !== insertTarget.parentNode) {
          insertTarget.parentNode.insertBefore(container, insertTarget.nextSibling);
        }
      } else {
        const contentArea = document.querySelector(
          '[class*="PageLayoutContent"], [class*="PageLayout-Content"], .diff-view, .js-diff-container, #files, #discussion_bucket, .js-pull-discussion-timeline, #commits_bucket, #checks_bucket, .Layout-main, ' +
          '.merge-request-details, .issuable-details, .tab-content, #content-body, .content-wrapper'
        );
        if (contentArea && container.parentNode !== contentArea) {
          contentArea.appendChild(container);
        }
      }
    }

    const isGitLab = window.location.hostname.includes('gitlab.com') || !!document.querySelector('.mr-title, .detail-page-header');
    if (isGitLab) {
      container.classList.add('metastackr-gitlab');
    }

    const itemTerm = isGitLab ? 'MR' : 'PR';
    const itemTermLong = isGitLab ? 'Merge Request' : 'Pull Request';
    const numPrefix = isGitLab ? '!' : '#';
    const matrixTitleText = isGitLab ? 'Submodule Merge Request Matrix' : 'Submodule Synchronization Matrix';
    const colPrText = isGitLab ? 'MR !' : 'PR #';

    const list = (childPRs && childPRs.length > 0)
      ? childPRs
      : (metaPR ? (metaPR.child_prs || metaPR.childPRs || []) : []);

    let rowsHtml = '';
    (list || []).forEach(child => {
      const path = child.submodule_path || child.submodulePath || child.path || child.SubmodulePath || '';
      const repo = child.repo_full_name || child.repoFullName || child.repo || child.RepoFullName || '';
      const prNum = child.pr_number || child.prNumber || child.mr_number || child.pr || child.PRNumber || 0;
      const headSha = child.head_sha || child.headSha || child.sha || child.HeadSHA || '';
      const shaStr = headSha ? headSha.substring(0, 7) : 'head';
      const status = child.status || child.Status || 'OPEN';
      const childPRUrl = getPRUrl(repo, prNum);

      rowsHtml += `
        <div class="metastackr-grid-row">
          <div class="col-path"><code>${path}</code></div>
          <div class="col-repo">${repo}</div>
          <div class="col-pr"><a href="${childPRUrl}">${numPrefix}${prNum}</a></div>
          <div class="col-sha"><code>${shaStr}</code></div>
          <div class="col-status"><span class="status-badge state-${status.toLowerCase()}">${status}</span></div>
        </div>
      `;
    });

    const emptyMessage = `<div class="metastackr-empty">No child submodules tracked for this branch.<br><span style="font-size: 11px; opacity: 0.8; margin-top: 4px; display: inline-block;">Run <code>git meta push</code> and <code>git meta create-pr</code> to create &amp; sync submodule ${itemTermLong}s.</span></div>`;

    if (isAlreadyRendered) {
      // Surgical update of matrix grid without destroying settings panel state
      const gridContainer = container.querySelector('.metastackr-grid');
      if (gridContainer) {
        gridContainer.innerHTML = `
          <div class="metastackr-grid-header">
            <div class="col-path">Submodule Path</div>
            <div class="col-repo">Submodule Repo</div>
            <div class="col-pr">${colPrText}</div>
            <div class="col-sha">Commit SHA</div>
            <div class="col-status">Merge Status</div>
          </div>
          ${rowsHtml || emptyMessage}
        `;
      }
      const statusBadge = container.querySelector('.metastackr-title-group .status-badge');
      if (statusBadge) {
        statusBadge.className = `status-badge state-${(metaPR ? (metaPR.status || 'open') : 'open').toLowerCase()}`;
        statusBadge.innerText = metaPR ? (metaPR.status || 'Open') : 'Open';
      }
      const lockBadge = container.querySelector('.metastackr-badge-muted');
      if (lockBadge) {
        lockBadge.innerText = `Lock Version ${metaPR ? (metaPR.lock_version || 1) : 1}`;
      }
      container.style.display = 'block';
      return;
    }

    container.dataset.rendered = 'true';

    container.dataset.rendered = 'true';

    container.innerHTML = `
      <div class="metastackr-box">
        <div class="metastackr-box-header">
          <div class="metastackr-title-group">
            <h3 class="metastackr-box-title">
              <svg aria-hidden="true" focusable="false" class="octicon octicon-zap" viewBox="0 0 16 16" width="16" height="16" fill="currentColor">
                <path d="M8.22 1.754a.75.75 0 0 0-1.44 0L4.537 7.033A.75.75 0 0 0 5.228 8.1h2.522l-1.97 6.142a.75.75 0 0 0 1.44 0l2.243-5.279A.75.75 0 0 0 8.772 7.9H6.25l1.97-6.146Z"></path>
              </svg>
              ${matrixTitleText}
            </h3>
            <span class="status-badge state-${(metaPR ? (metaPR.status || 'open') : 'open').toLowerCase()}">${metaPR ? (metaPR.status || 'Open') : 'Open'}</span>
          </div>
          <span class="metastackr-badge-muted">Lock Version ${metaPR ? (metaPR.lock_version || 1) : 1}</span>
        </div>
        <div class="metastackr-grid">
          <div class="metastackr-grid-header">
            <div class="col-path">Submodule Path</div>
            <div class="col-repo">Submodule Repo</div>
            <div class="col-pr">${colPrText}</div>
            <div class="col-sha">Commit SHA</div>
            <div class="col-status">Merge Status</div>
          </div>
          ${rowsHtml || emptyMessage}
        </div>
      </div>

      ${isGitLab ? `
      <details class="metastackr-settings-details">
        <summary>
          <svg aria-hidden="true" focusable="false" class="octicon octicon-gear" viewBox="0 0 16 16" width="16" height="16" fill="currentColor">
            <path d="M8 0a8 8 0 1 1 0 16A8 8 0 0 1 8 0ZM1.5 8a6.5 6.5 0 1 0 13 0 6.5 6.5 0 0 0-13 0Z"></path>
          </svg>
          <span>Auto-Merge Policy Rules</span>
          <span class="caret"></span>
        </summary>

        <div class="metastackr-box metastackr-settings-card metastackr-gitlab-popout" style="margin-top: 10px; border: 1px solid var(--ms-border-default); border-radius: 8px; overflow: hidden; background: var(--ms-bg-default);">
          <!-- Popped-out Policy Header -->
          <div class="metastackr-box-header" style="padding: 12px 16px; background: var(--ms-bg-subtle); border-bottom: 1px solid var(--ms-border-default); display: flex; align-items: center; justify-content: space-between;">
            <div style="display: flex; align-items: center; gap: 8px;">
              <span style="width: 20px; height: 20px; border-radius: 50%; background: #108548; color: #ffffff; display: flex; align-items: center; justify-content: center; flex-shrink: 0;">
                <svg viewBox="0 0 16 16" width="12" height="12" fill="currentColor">
                  <path d="M13.78 4.22a.75.75 0 0 1 0 1.06l-7.25 7.25a.75.75 0 0 1-1.06 0L2.22 9.28a.751.751 0 0 1 .018-1.042.751.751 0 0 1 1.042-.018L6 10.94l6.72-6.72a.75.75 0 0 1 1.06 0Z"></path>
                </svg>
              </span>
              <h3 class="metastackr-box-title" style="font-size: 14px; font-weight: 600; margin: 0; color: var(--ms-fg-default);">
                ${(metaPR && metaPR.status === 'MERGED') ? 'Ready to merge!' : 'Auto-Merge Policy Rules'}
              </h3>
            </div>
            <span style="font-size: 12px; color: var(--ms-fg-muted);">Active Policy Rules</span>
          </div>

          <!-- Popped-out Policy Body -->
          <div style="padding: 16px; background: var(--ms-bg-default); display: flex; flex-direction: column; gap: 14px;">
            <!-- Policy Rules Checkboxes Row -->
            <div style="display: flex; flex-wrap: wrap; gap: 18px; align-items: center;">
              <label class="metastackr-checkbox-label" style="display: inline-flex; align-items: center; gap: 6px; font-size: 13px; font-weight: 500; cursor: pointer;">
                <input type="checkbox" id="metastackr-auto-merge-chk" checked style="accent-color: #1f75cb; width: 16px; height: 16px;">
                <span><strong>Enable Auto Cascade Merge</strong></span>
              </label>
              <label class="metastackr-checkbox-label" style="display: inline-flex; align-items: center; gap: 6px; font-size: 13px; font-weight: 500; cursor: pointer;">
                <input type="checkbox" id="metastackr-require-approval-chk" checked style="accent-color: #1f75cb; width: 16px; height: 16px;">
                <span><strong>Require Root MR Approval</strong></span>
              </label>
              <label class="metastackr-checkbox-label" style="display: inline-flex; align-items: center; gap: 6px; font-size: 13px; font-weight: 500; cursor: pointer;">
                <input type="checkbox" id="metastackr-submodule-only-chk" checked style="accent-color: #1f75cb; width: 16px; height: 16px;">
                <span><strong>Submodule Only Changes</strong></span>
              </label>
            </div>

            <!-- Bullet Summary Line matching screenshot style -->
            <div style="font-size: 13px; color: var(--ms-fg-muted); padding-left: 2px;">
              • <strong>${(list || []).length} submodule MRs</strong> and <strong>1 root MR</strong> will be merged to <code>${metaPR ? (metaPR.base_branch || 'main') : 'main'}</code> when checks pass.
            </div>

            <!-- Action & Status Control Bar matching screenshot split-button -->
            <div style="display: flex; align-items: center; gap: 12px; margin-top: 2px;">
              <button type="button" id="metastackr-save-settings-btn" class="gl-btn-dark-split" style="background: #292929; color: #ffffff; border: none; padding: 7px 14px; border-radius: 4px; font-weight: 600; font-size: 13px; cursor: pointer; display: inline-flex; align-items: center; gap: 6px;">
                <span>Set to auto-merge</span>
                <svg viewBox="0 0 16 16" width="12" height="12" fill="currentColor"><path d="m3.5 6 4.5 4.5L12.5 6"></path></svg>
              </button>
              <span style="font-size: 13px; color: var(--ms-fg-muted); display: inline-flex; align-items: center; gap: 5px;">
                Merge when all merge checks pass
                <svg viewBox="0 0 16 16" width="14" height="14" fill="currentColor" style="opacity: 0.7;"><path d="M8 1.5a6.5 6.5 0 1 0 0 13 6.5 6.5 0 0 0 0-13M0 8a8 8 0 1 1 16 0A8 8 0 0 1 0 8m6.5-2a1.5 1.5 0 1 1 3 0v.5a.5.5 0 0 1-1 0V6a.5.5 0 1 0-1 0v1.5a.5.5 0 0 1-1 0zm1.5 5.5a.75.75 0 1 0 0-1.5.75.75 0 0 0 0 1.5"></path></svg>
              </span>
              <span id="metastackr-settings-status" style="font-size: 12px; color: var(--ms-state-merged-fg, #108548); margin-left: 6px;"></span>
            </div>

            <!-- Form Controls for Checks & Merge Method -->
            <div style="display: flex; flex-direction: column; gap: 10px; margin-top: 4px; padding-top: 12px; border-top: 1px solid var(--ms-border-default);">
              <div class="metastackr-form-group">
                <label for="metastackr-req-checks-input" style="font-size: 12px;"><strong>Required Status Checks:</strong> <span style="font-weight: normal; opacity: 0.8;">(comma-separated check names, e.g. <code>ci/build, lint</code>)</span></label>
                <input type="text" id="metastackr-req-checks-input" class="metastackr-form-control" value="" placeholder="e.g. ci/build, test" style="max-width: 450px;">
              </div>
              <div class="metastackr-form-group">
                <label for="metastackr-merge-method-select" style="font-size: 12px;"><strong>Default Merge Method:</strong></label>
                <select id="metastackr-merge-method-select" class="metastackr-form-control" style="width: 160px;">
                  <option value="merge" selected>Merge (commit)</option>
                  <option value="squash">Squash</option>
                  <option value="rebase">Rebase</option>
                </select>
              </div>
            </div>
          </div>
        </div>
      </details>
      ` : `
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
      `}
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
      saveBtn.onclick = function (e) {
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

    const status = (metaPR.status || 'Open').toUpperCase();
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
