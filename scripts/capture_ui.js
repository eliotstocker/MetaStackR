#!/usr/bin/env node
'use strict';

/**
 * MetaStackr UI Capture Script
 * Uses Playwright to automatically launch Chromium, navigate to GitHub PRs & GitLab MRs,
 * wait for the MetaStackr status matrix to render, and record crisp MP4/WebM video clips.
 *
 * Usage:
 *   node scripts/capture_ui.js [github-url] [gitlab-url]
 */

const fs = require('fs');
const path = require('path');

const ASSETS_DIR = path.join(__dirname, '..', 'web', 'assets');
const GH_URL = process.argv[2] || 'https://github.com/eliotstocker/metastackr-demo-root/pull/31';
const GL_URL = process.argv[3] || 'https://gitlab.com/eliotstocker/metastackr-demo-root/-/merge_requests/1';

async function main() {
  console.log('🎬 MetaStackr UI Video Capture');
  console.log('===============================');

  let playwright;
  try {
    playwright = require('playwright');
  } catch {
    console.log('ℹ️ Playwright module not found.');
    console.log('');
    console.log('To automate screen recording programmatically, install Playwright via:');
    console.log('  npm install --save-dev playwright');
    console.log('  npx playwright install chromium');
    console.log('');
    console.log('Alternatively, press Cmd+Shift+5 on macOS to record your browser window and save clips to:');
    console.log(`  - ${path.join(ASSETS_DIR, 'demo-github.mp4')}`);
    console.log(`  - ${path.join(ASSETS_DIR, 'demo-gitlab.mp4')}`);
    return;
  }

  console.log('🚀 Launching Chromium with video recording enabled...');
  const browser = await playwright.chromium.launch({ headless: false });
  
  // Capture GitHub PR
  console.log(`\n1️⃣ Capturing GitHub PR matrix: ${GH_URL}...`);
  const ghContext = await browser.newContext({
    viewport: { width: 1280, height: 720 },
    recordVideo: { dir: ASSETS_DIR, size: { width: 1280, height: 720 } }
  });
  const ghPage = await ghContext.newPage();
  await ghPage.goto(GH_URL, { waitUntil: 'networkidle' });
  await ghPage.waitForTimeout(5000);
  const ghVideoPath = await ghPage.video().path();
  await ghContext.close();

  if (fs.existsSync(ghVideoPath)) {
    const destGH = path.join(ASSETS_DIR, 'demo-github.webm');
    fs.renameSync(ghVideoPath, destGH);
    console.log(`  ✅ GitHub clip saved to ${destGH}`);
  }

  // Capture GitLab MR
  console.log(`\n2️⃣ Capturing GitLab MR overlay: ${GL_URL}...`);
  const glContext = await browser.newContext({
    viewport: { width: 1280, height: 720 },
    recordVideo: { dir: ASSETS_DIR, size: { width: 1280, height: 720 } }
  });
  const glPage = await glContext.newPage();
  await glPage.goto(GL_URL, { waitUntil: 'networkidle' });
  await glPage.waitForTimeout(5000);
  const glVideoPath = await glPage.video().path();
  await glContext.close();

  if (fs.existsSync(glVideoPath)) {
    const destGL = path.join(ASSETS_DIR, 'demo-gitlab.webm');
    fs.renameSync(glVideoPath, destGL);
    console.log(`  ✅ GitLab clip saved to ${destGL}`);
  }

  await browser.close();
  console.log('\n🎉 Video recording capture completed!');
}

main().catch(err => {
  console.error('Error during capture:', err);
  process.exit(1);
});
