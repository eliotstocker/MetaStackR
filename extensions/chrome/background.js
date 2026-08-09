chrome.runtime.onMessage.addListener((request, sender, sendResponse) => {
  if (request.action === 'fetchPRStatus') {
    const { repo, prNumber, branch } = request;
    const tryFetch = (serverURL, fallbackURL) => {
      const prParam = prNumber ? `&pr=${prNumber}` : '';
      const url = `${serverURL}/api/v1/prs/status?repo=${encodeURIComponent(repo)}${prParam}&branch=${encodeURIComponent(branch)}`;
      fetch(url)
        .then(res => {
          if (!res.ok) throw new Error('Unreachable server');
          return res.json();
        })
        .then(data => {
          if (data && data.meta_pr) {
            sendResponse({ success: true, metaPR: data.meta_pr, serverURL });
          } else if (fallbackURL) {
            tryFetch(fallbackURL, null);
          } else {
            sendResponse({ success: false, metaPR: null });
          }
        })
        .catch(err => {
          if (fallbackURL) {
            tryFetch(fallbackURL, null);
          } else {
            sendResponse({ success: false, error: err.message });
          }
        });
    };

    tryFetch('https://api.metastac.kr', 'http://localhost:8080');
    return true; // async sendResponse
  }
});
