const http = require('http');
const fs = require('fs');
const path = require('path');
const playwright = require('playwright');

// Simple HTTP server with range request support for video seeking
function createVideoServer(rootDir, port = 8999) {
  const server = http.createServer((req, res) => {
    const filePath = path.join(rootDir, req.url.split('?')[0]);
    if (!fs.existsSync(filePath) || fs.statSync(filePath).isDirectory()) {
      res.writeHead(404);
      return res.end('Not Found');
    }

    const stat = fs.statSync(filePath);
    const fileSize = stat.size;
    const range = req.headers.range;

    if (range) {
      const parts = range.replace(/bytes=/, '').split('-');
      const start = parseInt(parts[0], 10);
      const end = parts[1] ? parseInt(parts[1], 10) : fileSize - 1;
      const chunksize = end - start + 1;
      const file = fs.createReadStream(filePath, { start, end });
      const head = {
        'Content-Range': `bytes ${start}-${end}/${fileSize}`,
        'Accept-Ranges': 'bytes',
        'Content-Length': chunksize,
        'Content-Type': 'video/webm'
      };
      res.writeHead(206, head);
      file.pipe(res);
    } else {
      const head = {
        'Content-Length': fileSize,
        'Content-Type': 'video/webm'
      };
      res.writeHead(200, head);
      fs.createReadStream(filePath).pipe(res);
    }
  });

  return new Promise((resolve) => {
    server.listen(port, () => resolve(server));
  });
}

async function verifyVideo(serverPort, videoRelPath, label) {
  console.log(`\n🔍 Verifying ${label} [${videoRelPath}]...`);
  const browser = await playwright.chromium.launch({
    headless: true,
    args: ['--no-sandbox', '--disable-web-security']
  });
  const page = await browser.newPage({ viewport: { width: 1280, height: 720 } });

  const videoUrl = `http://localhost:${serverPort}/${videoRelPath}`;
  const html = `
    <!DOCTYPE html>
    <html>
      <body style="margin:0; background:#000;">
        <video id="v" src="${videoUrl}" preload="auto" style="width:1280px; height:720px; object-fit:contain;"></video>
      </body>
    </html>
  `;

  await page.setContent(html);

  const duration = await page.evaluate(async () => {
    const v = document.getElementById('v');
    return new Promise((resolve) => {
      if (v.duration && !isNaN(v.duration) && v.duration > 0) return resolve(v.duration);
      v.addEventListener('loadedmetadata', () => resolve(v.duration));
      setTimeout(() => resolve(v.duration || 0), 4000);
    });
  });

  console.log(`  ⏱️ Duration: ${duration.toFixed(2)}s`);

  const milestones = [
    { time: 10, name: 'act1_terminal_edits' },
    { time: 60, name: 'act2_root_diffs' },
    { time: 76, name: 'act2_child_merge' },
    { time: 92, name: 'act3_sub2_cli_merge' },
    { time: 115, name: 'act4_matrix_all_merged' },
    { time: 140, name: 'act5_terminal_sync' }
  ];

  const outDir = path.resolve(__dirname, '..', 'scratch', 'video-frames', label);
  fs.mkdirSync(outDir, { recursive: true });

  for (const m of milestones) {
    if (m.time <= duration) {
      await page.evaluate(async (t) => {
        const v = document.getElementById('v');
        v.currentTime = t;
        await new Promise(r => {
          v.onseeked = r;
          setTimeout(r, 400);
        });
      }, m.time);
      await page.waitForTimeout(300);

      const framePath = path.join(outDir, `${m.name}_${m.time}s.png`);
      await page.screenshot({ path: framePath });
      console.log(`  📸 Frame saved at t=${m.time}s -> ${framePath}`);
    }
  }

  await browser.close();
  return duration;
}

async function main() {
  const rootDir = path.resolve(__dirname, '..', 'demo-output');
  const server = await createVideoServer(rootDir, 9876);
  console.log('⚡ Local Video Server running on :9876');

  try {
    const ghDur = await verifyVideo(9876, 'demo-github.webm', 'github');
    const glDur = await verifyVideo(9876, 'demo-gitlab.webm', 'gitlab');

    console.log('\n=============================================');
    console.log(`✅ GitHub Video Duration: ${ghDur.toFixed(2)}s`);
    console.log(`✅ GitLab Video Duration: ${glDur.toFixed(2)}s`);
    console.log(`⏱️ Duration Difference:   ${Math.abs(ghDur - glDur).toFixed(2)}s`);
    console.log('=============================================\n');
  } finally {
    server.close();
  }
}

main().catch(console.error);
