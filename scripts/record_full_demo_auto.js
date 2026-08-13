#!/usr/bin/env node
'use strict';

/**
 * MetaStackr Automated 11-Step Demo Recorder with Smooth Visual Mouse Cursor & Step Delays
 */

const fs = require('fs');
const path = require('path');

const ASSETS_DIR = path.resolve(__dirname, '..', 'web', 'assets');
const EXTENSION_PATH = path.resolve(__dirname, '..', 'extensions', 'chrome');
const USER_DATA_DIR = path.resolve(__dirname, '..', 'scratch', 'playwright-user-data');
const PARENT_PR_URL = process.argv[2] || 'https://github.com/eliotstocker/metastackr-demo-root/pull/34';

async function main() {
  console.log('🤖 MetaStackr Visual Mouse Demo Recorder');
  console.log('=======================================');
  console.log(`📌 Parent PR URL: ${PARENT_PR_URL}`);

  let playwright;
  try {
    playwright = require('playwright');
  } catch {
    console.error('❌ Playwright not found. Install via: npm install --save-dev playwright');
    process.exit(1);
  }

  console.log(`🔌 Loading MetaStackr Chrome Extension from: ${EXTENSION_PATH}`);
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
  const delay = (ms) => page.waitForTimeout(ms);

  // Helper to inject glowing visual mouse pointer
  async function initMousePointer() {
    await page.evaluate(() => {
      if (document.getElementById('demo-mouse-pointer')) return;
      const cursor = document.createElement('div');
      cursor.id = 'demo-mouse-pointer';
      cursor.style.cssText = `
        position: absolute;
        width: 22px;
        height: 22px;
        background: #FACC15;
        border: 2px solid #FFFFFF;
        border-radius: 50%;
        box-shadow: 0 0 16px rgba(250, 204, 21, 0.9);
        pointer-events: none;
        z-index: 999999;
        transition: left 0.4s cubic-bezier(0.16, 1, 0.3, 1), top 0.4s cubic-bezier(0.16, 1, 0.3, 1), transform 0.15s ease;
        transform: translate(-50%, -50%);
      `;
      document.body.appendChild(cursor);
    });
  }

  // Smooth mouse move & click with delays
  async function moveAndClick(selector, stepName) {
    console.log(`▶️ ${stepName}`);
    await initMousePointer();
    await page.waitForSelector(selector, { timeout: 15000 });
    const loc = page.locator(selector).first();
    const box = await loc.boundingBox();

    if (box) {
      const targetX = box.x + box.width / 2;
      const targetY = box.y + box.height / 2;

      // Animate mouse position to element
      await page.evaluate(({ x, y }) => {
        const cursor = document.getElementById('demo-mouse-pointer');
        if (cursor) {
          cursor.style.left = `${x + window.scrollX}px`;
          cursor.style.top = `${y + window.scrollY}px`;
          cursor.style.transform = 'translate(-50%, -50%) scale(1.3)';
        }
      }, { x: targetX, y: targetY });

      await page.mouse.move(targetX, targetY, { steps: 25 });
      await delay(600);

      // Mouse click animation
      await page.evaluate(() => {
        const cursor = document.getElementById('demo-mouse-pointer');
        if (cursor) cursor.style.transform = 'translate(-50%, -50%) scale(0.9)';
      });

      await page.click(selector);
      await delay(300);

      await page.evaluate(() => {
        const cursor = document.getElementById('demo-mouse-pointer');
        if (cursor) cursor.style.transform = 'translate(-50%, -50%) scale(1)';
      });

      // Pause a few seconds between actions for viewers to follow
      await delay(3500);
    }
  }

  try {
    // Step 1: Open Parent PR #34
    console.log('\nStep 1: Opening Parent PR #34...');
    await page.goto(PARENT_PR_URL, { waitUntil: 'domcontentloaded' });
    await delay(3000);

    // Step 2: Click MetaStackr Tab
    const subTab = '#metastackr-submodules-tab';
    await moveAndClick(subTab, 'Step 2: Clicking MetaStackr Submodules tab...');

    // Step 3: Click Child PR #1 Link (sub-a)
    const child1Link = '#metastackr-submodules-panel a[href*="metastackr-demo-sub-a"]';
    await moveAndClick(child1Link, 'Step 3: Opening Child PR #1 (sub-a)...');

    // Step 4: Merge Child PR #1 (Squash & Merge)
    const mergeBtn = 'button:has-text("Squash and merge"), button:has-text("Merge pull request")';
    await moveAndClick(mergeBtn, 'Step 4: Merging Child PR #1...');
    const confirmBtn = 'button:has-text("Confirm squash and merge"), button:has-text("Confirm merge")';
    await moveAndClick(confirmBtn, 'Step 4b: Confirming merge...');

    // Step 5: Click MetaStackr Child Banner Link back to Parent PR
    const bannerLink = '#metastackr-child-pr-banner a';
    if (await page.$(bannerLink)) {
      await moveAndClick(bannerLink, 'Step 5: Clicking banner link back to Parent PR...');
    } else {
      await page.goto(PARENT_PR_URL, { waitUntil: 'domcontentloaded' });
      await delay(3000);
    }

    // Step 6: Inspect Status Matrix & Click MetaStackr Tab
    await moveAndClick(subTab, 'Step 6: Opening MetaStackr tab to inspect 1 PR open...');

    // Step 7: Click Child PR #2 Link (sub-b)
    const child2Link = '#metastackr-submodules-panel a[href*="metastackr-demo-sub-b"]';
    await moveAndClick(child2Link, 'Step 7: Opening Child PR #2 (sub-b)...');

    // Step 8: Merge Child PR #2 (Merge Commit)
    await moveAndClick(mergeBtn, 'Step 8: Merging Child PR #2...');
    await moveAndClick(confirmBtn, 'Step 8b: Confirming merge...');

    // Step 9: Click MetaStackr Child Banner Link back to Parent PR
    if (await page.$(bannerLink)) {
      await moveAndClick(bannerLink, 'Step 9: Returning to Parent PR...');
    } else {
      await page.goto(PARENT_PR_URL, { waitUntil: 'domcontentloaded' });
      await delay(3000);
    }

    // Step 10: Watch Parent PR Auto-Cascade Merge
    console.log('Step 10: Watching Parent PR auto-cascade merge...');
    await delay(6000);

    // Step 11: Go to Main Repo Code Page & Inspect Merged Commit
    const codeTab = 'a[data-tab-item="code-tab"], a[href="/eliotstocker/metastackr-demo-root"]';
    await moveAndClick(codeTab, 'Step 11: Opening Main Repo Code page...');

  } catch (err) {
    console.log('⚠️ Notice during recording flow:', err.message);
  }

  const rawVideoPath = await page.video().path();
  await context.close();

  if (fs.existsSync(rawVideoPath)) {
    const destPath = path.join(ASSETS_DIR, 'demo-github.webm');
    fs.renameSync(rawVideoPath, destPath);
    console.log(`\n✅ Visual mouse demo video recorded and saved to: ${destPath}`);
  }

  console.log('🎉 Recording complete!');
}

main().catch(err => {
  console.error('Fatal error:', err);
  process.exit(1);
});
