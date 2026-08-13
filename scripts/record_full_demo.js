#!/usr/bin/env node
'use strict';

/**
 * MetaStackr Full End-to-End Demo Recorder
 * Launches Chromium with MetaStackr Chrome extension and records a 3-minute video
 * allowing you to perform (or automate) the full 11-step PR cascade merge lifecycle:
 *
 * 1. Go to MetaStackr tab on parent PR
 * 2. Enter first subrepo PR
 * 3. Squash and merge it
 * 4. Click banner link back to parent PR (see matrix updated)
 * 5. Look at checks matrix (1 PR open)
 * 6. Navigate back to MetaStackr tab
 * 7. Go to second subrepo PR
 * 8. Merge it with a merge commit
 * 9. Click banner link back to parent PR
 * 10. Watch parent PR auto-merge
 * 11. Navigate to main repo code page & inspect merged commit pointer
 */

const fs = require('fs');
const path = require('path');

const ASSETS_DIR = path.resolve(__dirname, '..', 'web', 'assets');
const EXTENSION_PATH = path.resolve(__dirname, '..', 'extensions', 'chrome');
const USER_DATA_DIR = path.resolve(__dirname, '..', 'scratch', 'playwright-user-data');
const PARENT_PR_URL = process.argv[2] || 'https://github.com/eliotstocker/metastackr-demo-root/pull/31';
const DURATION_SEC = parseInt(process.argv[3] || '120', 10);

async function main() {
  console.log('🎬 MetaStackr Full Cascade Merge Demo Recorder');
  console.log('==============================================');
  console.log(`📌 Parent PR URL: ${PARENT_PR_URL}`);
  console.log(`⏱️ Recording Duration: ${DURATION_SEC} seconds`);

  let playwright;
  try {
    playwright = require('playwright');
  } catch {
    console.error('❌ Playwright not found. Install via: npm install --save-dev playwright');
    process.exit(1);
  }

  console.log(`\n🔌 Loading MetaStackr Chrome Extension from: ${EXTENSION_PATH}`);
  const context = await playwright.chromium.launchPersistentContext(USER_DATA_DIR, {
    headless: false,
    viewport: { width: 1280, height: 720 },
    recordVideo: { dir: ASSETS_DIR, size: { width: 1280, height: 720 } },
    args: [
      `--disable-extensions-except=${EXTENSION_PATH}`,
      `--load-extension=${EXTENSION_PATH}`
    ]
  });

  const page = await context.newPage();
  console.log(`\n🌐 Navigating to Parent PR: ${PARENT_PR_URL}...`);
  await page.goto(PARENT_PR_URL, { waitUntil: 'domcontentloaded' });

  console.log('\n🔴 RECORDING STARTED!');
  console.log('--------------------------------------------------');
  console.log('Perform steps 1 to 11 in the opened browser window:');
  console.log('  1. Click MetaStackr tab');
  console.log('  2. Open child PR #1 (sub-a)');
  console.log('  3. Squash & merge child PR #1');
  console.log('  4. Click MetaStackr banner link back to parent PR');
  console.log('  5. Inspect updated checks matrix (1 PR open)');
  console.log('  6. Go back to MetaStackr tab');
  console.log('  7. Open child PR #2 (sub-b)');
  console.log('  8. Merge child PR #2 with merge commit');
  console.log('  9. Click MetaStackr banner link back to parent PR');
  console.log(' 10. Watch parent PR auto-cascade merge');
  console.log(' 11. View main repo code page & inspect merged commit');
  console.log('--------------------------------------------------');

  for (let i = DURATION_SEC; i > 0; i -= 10) {
    console.log(`⏳ ${i} seconds remaining...`);
    await page.waitForTimeout(10000);
  }

  const rawVideoPath = await page.video().path();
  await context.close();

  if (fs.existsSync(rawVideoPath)) {
    const destPath = path.join(ASSETS_DIR, 'demo-github.webm');
    fs.renameSync(rawVideoPath, destPath);
    console.log(`\n✅ Full demo video saved to: ${destPath}`);
  }

  console.log('🎉 Recording complete!');
}

main().catch(err => {
  console.error('Fatal error:', err);
  process.exit(1);
});
