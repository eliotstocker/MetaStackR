#!/usr/bin/env node
'use strict';

/**
 * MetaStackr End-to-End Demo Recorder (Terminal Asciinema & Browser Video)
 * 
 * Orchestrates the full lifecycle of a multi-repo demo with strict milestone timing:
 * 1. Bootstraps 3 real repositories (Root Meta-Repo + 2 Submodule Services: Orders & Payments)
 *    on GitHub or GitLab.
 * 2. Simultaneously starts synchronized recording:
 *    - Terminal session -> .cast (asciicast format v2)
 *    - Chrome browser with MetaStackr extension -> .mp4 / .webm video
 * 3. Act 1 (0:00 - 0:52): Terminal - Edits submodule files, commits, pushes, and creates PRs via `git-meta`.
 * 4. Act 2 (0:52 - 1:25): Browser - Inspects Root PR submodule pointer diffs, checks MetaStackr matrix,
 *    opens Sub PR #1 (Orders), merges it, and uses MetaStackr banner to navigate back.
 * 5. Act 3 (1:25 - 1:50): Coordinated CLI & Browser - Browser opens Sub PR #2 (Payments), Terminal runs
 *    `git meta status` and merges Sub PR #2 via CLI, Browser confirms merged state and navigates back.
 * 6. Act 4 (1:50 - 2:10): Browser - Root PR MetaStackr tab shows both PRs merged, Conversation tab shows
 *    auto-cascade merge completing, and Code tab displays the updated main branch with new submodule pointers.
 * 7. Act 5 (2:10 - 2:30): Terminal - Runs `git meta status`, switches to main, and executes `git meta sync`.
 * 
 * Usage:
 *   node scripts/record_meta_demo.js --provider github [options]
 *   node scripts/record_meta_demo.js --provider gitlab [options]
 */

const fs = require('fs');
const path = require('path');
const { execSync, spawn } = require('child_process');

// -------------------------------------------------------------------------
// CLI Argument Parsing & Configuration
// -------------------------------------------------------------------------

function parseArgs() {
  const args = process.argv.slice(2);
  const options = {
    provider: 'github',
    org: '',
    demoNumber: null,
    prefix: '',
    visibility: 'public',
    workspace: '',
    outputDir: path.resolve(__dirname, '..', 'demo-output'),
    server: '',
    headless: false,
    clean: true,  // Automatically delete remote and local repos after recording
    keep: false,  // Pass --keep to prevent deletion
    login: false,
    help: false
  };

  for (let i = 0; i < args.length; i++) {
    const arg = args[i];
    if (arg === '-p' || arg === '--provider') {
      options.provider = (args[++i] || '').toLowerCase();
    } else if (arg === '-o' || arg === '--org' || arg === '--group' || arg === '-g') {
      options.org = args[++i] || '';
    } else if (arg === '-n' || arg === '--demo-number' || arg === '--number') {
      options.demoNumber = parseInt(args[++i], 10);
    } else if (arg === '-s' || arg === '--server') {
      options.server = args[++i] || '';
    } else if (arg === '--prefix') {
      options.prefix = args[++i] || '';
    } else if (arg === '--visibility') {
      options.visibility = args[++i] || 'public';
    } else if (arg === '-w' || arg === '--workspace') {
      options.workspace = args[++i];
    } else if (arg === '--output-dir') {
      options.outputDir = path.resolve(args[++i]);
    } else if (arg === '--headless') {
      options.headless = true;
    } else if (arg === '--login' || arg === '--auth') {
      options.login = true;
    } else if (arg === '--clean') {
      options.clean = true;
      options.keep = false;
    } else if (arg === '--keep') {
      options.keep = true;
      options.clean = false;
    } else if (arg === '-h' || arg === '--help') {
      options.help = true;
    }
  }

  if (!options.prefix) {
    if (options.demoNumber !== null && !isNaN(options.demoNumber)) {
      options.prefix = `metastackr-demo-${options.demoNumber}`;
    } else {
      const randSuffix = Math.floor(1000 + Math.random() * 9000);
      options.prefix = `metastackr-demo-${randSuffix}`;
    }
  }

  if (!options.workspace) {
    options.workspace = path.resolve(__dirname, '..', 'scratch', options.prefix);
  }

  return options;
}

function showHelp() {
  console.log(`
⚡ MetaStackr Demo Recorder & Orchestrator
==========================================
Records a synchronized Asciinema terminal session (.cast) and Playwright browser video (.mp4)
demonstrating the complete MetaStackr multi-repo orchestration lifecycle.

Usage:
  node scripts/record_meta_demo.js [options]

Options:
  -p, --provider <github|gitlab>  VCS provider (default: github)
  -n, --demo-number <number>      Demo sequence number (e.g. 1, 2, 3) (default: timestamp-based)
  -o, --org <org/group>           VCS Organization or GitLab Group (default: authenticated user)
  --prefix <name>                 Prefix for generated repositories (default: metastackr-demo-<number>)
  --visibility <public|private>   Remote repository visibility (default: public)
  -w, --workspace <dir>           Local workspace directory for demo repos (default: scratch/<prefix>)
  --output-dir <dir>              Directory to store the final .cast and .mp4 recordings (default: demo-output)
  --keep                          Keep remote and local repos after recording (default: false, auto-deletes)
  --clean                         Explicitly delete remote and local repos after recording (default: true)
  --login                         Open browser to log into GitHub/GitLab and save session profile
  --headless                      Run browser in headless mode (default: false)
  -h, --help                      Display this help message

Examples:
  # Record GitHub Demo #1 (auto-cleans repos when finished)
  node scripts/record_meta_demo.js --provider github --demo-number 1

  # Record GitLab Demo #2 in a group and keep repositories
  node scripts/record_meta_demo.js --provider gitlab --demo-number 2 --group my-team-group --keep
`);
}

// -------------------------------------------------------------------------
// Precise Demo Clock & Milestone Coordinator
// -------------------------------------------------------------------------

class DemoClock {
  constructor() {
    this.epoch = null;
  }

  start() {
    this.epoch = Date.now();
  }

  elapsed() {
    if (!this.epoch) return 0;
    return parseFloat(((Date.now() - this.epoch) / 1000).toFixed(6));
  }

  async waitUntil(targetSeconds) {
    if (!this.epoch) return;
    const targetMs = targetSeconds * 1000;
    const currentMs = Date.now() - this.epoch;
    const diff = targetMs - currentMs;
    if (diff > 0) {
      await new Promise(resolve => setTimeout(resolve, diff));
    }
  }
}

// -------------------------------------------------------------------------
// Terminal Session Recorder (Asciicast v2 Engine)
// -------------------------------------------------------------------------

class AsciicastRecorder {
  constructor(outputFilePath, clock, width = 110, height = 32) {
    this.outputFilePath = outputFilePath;
    this.clock = clock;
    this.width = width;
    this.height = height;
    
    const header = {
      version: 2,
      width: this.width,
      height: this.height,
      timestamp: Math.floor(Date.now() / 1000),
      env: {
        SHELL: '/bin/zsh',
        TERM: 'xterm-256color'
      }
    };
    
    fs.mkdirSync(path.dirname(outputFilePath), { recursive: true });
    fs.writeFileSync(outputFilePath, JSON.stringify(header) + '\n');
  }

  getOffset() {
    return this.clock.elapsed();
  }

  recordOutput(text) {
    process.stdout.write(text);
    const eventLine = JSON.stringify([this.getOffset(), 'o', text]);
    fs.appendFileSync(this.outputFilePath, eventLine + '\n');
  }

  async sleep(ms) {
    return new Promise(resolve => setTimeout(resolve, ms));
  }

  renderPrompt(repoName = 'demo-root', branchName = 'main') {
    const prompt = `\r\n\x1b[38;5;39m➜\x1b[0m \x1b[1;32m${repoName}\x1b[0m \x1b[38;5;214mgit:(\x1b[1;31m${branchName}\x1b[38;5;214m)\x1b[0m \x1b[38;5;39m$\x1b[0m `;
    this.recordOutput(prompt);
  }

  async typeCommand(cmdText, typingDurationMs = 1200) {
    const len = Math.max(1, cmdText.length);
    const delayPerChar = Math.max(15, Math.floor(typingDurationMs / len));
    for (const char of cmdText) {
      this.recordOutput(char);
      await this.sleep(delayPerChar);
    }
    await this.sleep(200);
    this.recordOutput('\r\n');
  }

  async executeCommand(cmdStr, cwd, {
    targetStart = null,
    targetEnd = null,
    branch = 'main',
    repoName = 'demo-root'
  } = {}) {
    if (targetStart !== null) {
      await this.clock.waitUntil(targetStart);
    }

    this.renderPrompt(repoName, branch);
    await this.sleep(300);

    const typingDuration = Math.min(1800, Math.max(800, cmdStr.length * 28));
    await this.typeCommand(cmdStr, typingDuration);

    let output = '';
    try {
      const result = execSync(cmdStr, {
        cwd,
        env: {
          ...process.env,
          FORCE_COLOR: '1',
          CLICOLOR_FORCE: '1',
          PAGER: 'cat'
        },
        encoding: 'utf-8',
        stdio: ['pipe', 'pipe', 'pipe']
      });
      output = result;
    } catch (err) {
      output = (err.stdout ? err.stdout.toString() : '') + (err.stderr ? err.stderr.toString() : err.message);
    }

    if (output) {
      const formatted = output.replace(/\r?\n/g, '\r\n');
      this.recordOutput(formatted);
      if (!formatted.endsWith('\r\n')) {
        this.recordOutput('\r\n');
      }
    }

    if (targetEnd !== null) {
      await this.clock.waitUntil(targetEnd);
    } else {
      await this.sleep(1000);
    }

    return output;
  }
}

// -------------------------------------------------------------------------
// Helper: System Tool Verifications
// -------------------------------------------------------------------------

function verifyPrerequisites(provider) {
  console.log('\n🔍 Verifying system prerequisites...');
  
  let gitMetaPath = '';
  try {
    gitMetaPath = execSync('which git-meta', { encoding: 'utf-8' }).trim();
  } catch {
    const localBinary = path.resolve(__dirname, '..', 'git-meta');
    if (fs.existsSync(localBinary)) {
      gitMetaPath = localBinary;
    }
  }

  if (!gitMetaPath) {
    console.error('❌ git-meta CLI not found. Run `make build` or `make install` first.');
    process.exit(1);
  }
  console.log(`  ✅ git-meta CLI located: ${gitMetaPath}`);

  if (provider === 'github') {
    try {
      execSync('gh --version', { stdio: 'ignore' });
      console.log('  ✅ GitHub CLI (gh) available');
    } catch {
      console.warn('  ⚠️ GitHub CLI (gh) not found in PATH.');
    }
  } else if (provider === 'gitlab') {
    try {
      execSync('glab --version', { stdio: 'ignore' });
      console.log('  ✅ GitLab CLI (glab) available');
    } catch {
      console.warn('  ⚠️ GitLab CLI (glab) not found in PATH.');
    }
  }

  try {
    require('playwright');
    console.log('  ✅ Playwright installed');
  } catch {
    console.error('❌ Playwright module not found. Install via: npm install --save-dev playwright');
    process.exit(1);
  }

  const extensionPath = path.resolve(__dirname, '..', 'extensions', 'chrome');
  if (!fs.existsSync(extensionPath)) {
    console.error(`❌ Chrome extension directory not found at: ${extensionPath}`);
    process.exit(1);
  }
  console.log(`  ✅ Chrome extension directory: ${extensionPath}`);
}

// -------------------------------------------------------------------------
// Repository Bootstrapper
// -------------------------------------------------------------------------

async function bootstrapRepositories(options) {
  console.log(`\n🏗️ Bootstrapping demo repositories for [${options.provider.toUpperCase()}]...`);
  const { provider, org, prefix, visibility, workspace } = options;

  const rootRepoName = `${prefix}-root`;
  const sub1RepoName = `${prefix}-orders-service`;
  const sub2RepoName = `${prefix}-payments-service`;

  let owner = org;
  if (!owner) {
    try {
      if (provider === 'github') {
        owner = execSync('gh api user --jq .login', { encoding: 'utf-8' }).trim();
      } else if (provider === 'gitlab') {
        const userJson = JSON.parse(execSync('glab api user', { encoding: 'utf-8' }));
        owner = userJson.username || userJson.nickname || 'eliotstocker';
      }
    } catch {
      owner = process.env.USER || 'demo-user';
    }
  }

  console.log(`  👤 Owner / Namespace: ${owner}`);
  console.log(`  📁 Workspace Directory: ${workspace}`);

  fs.rmSync(workspace, { recursive: true, force: true });
  fs.mkdirSync(workspace, { recursive: true });

  const rootLocalDir = path.join(workspace, rootRepoName);
  const sub1LocalDir = path.join(workspace, sub1RepoName);
  const sub2LocalDir = path.join(workspace, sub2RepoName);

  const deleteRemoteRepo = (repoName) => {
    const fullRepo = owner ? `${owner}/${repoName}` : repoName;
    try {
      if (provider === 'github') {
        execSync(`gh repo delete ${fullRepo} --yes`, { stdio: 'ignore' });
      } else if (provider === 'gitlab') {
        execSync(`glab repo delete ${fullRepo} --yes`, { stdio: 'ignore' });
      }
    } catch {}
  };

  deleteRemoteRepo(sub1RepoName);
  deleteRemoteRepo(sub2RepoName);
  deleteRemoteRepo(rootRepoName);

  const createRemoteRepo = (repoName) => {
    const fullRepo = owner ? `${owner}/${repoName}` : repoName;
    console.log(`  📦 Creating remote repo: ${fullRepo}...`);
    try {
      if (provider === 'github') {
        execSync(`gh repo create ${fullRepo} --${visibility} --confirm`, { stdio: 'ignore' });
      } else if (provider === 'gitlab') {
        const groupFlag = org ? `--group ${org}` : '';
        const visFlag = visibility === 'private' ? '--private' : '--public';
        execSync(`glab repo create ${repoName} ${visFlag} ${groupFlag} --skipGitInit`, { stdio: 'ignore' });
      }
    } catch {
      console.log(`    ℹ️ Remote repo ${fullRepo} already exists or was initialized.`);
    }
  };

  createRemoteRepo(sub1RepoName);
  createRemoteRepo(sub2RepoName);
  createRemoteRepo(rootRepoName);

  const getRemoteURL = (repoName) => {
    if (provider === 'github') {
      return `https://github.com/${owner}/${repoName}.git`;
    }
    return `git@gitlab.com:${owner}/${repoName}.git`;
  };

  const sub1RemoteURL = getRemoteURL(sub1RepoName);
  const sub2RemoteURL = getRemoteURL(sub2RepoName);
  const rootRemoteURL = getRemoteURL(rootRepoName);

  const initGitRepo = (dir, remoteURL, commitMsg) => {
    fs.rmSync(path.join(dir, '.git'), { recursive: true, force: true });
    execSync('git init -b main', { cwd: dir, stdio: 'ignore' });
    try {
      execSync('git remote remove origin', { cwd: dir, stdio: 'ignore' });
    } catch {}
    try {
      execSync(`git remote add origin ${remoteURL}`, { cwd: dir, stdio: 'ignore' });
    } catch {
      execSync(`git remote set-url origin ${remoteURL}`, { cwd: dir, stdio: 'ignore' });
    }
    execSync(`git add . && git commit -m "${commitMsg}"`, { cwd: dir, stdio: 'ignore' });
  };

  const pushWithRetry = (dir, name, maxRetries = 6) => {
    for (let i = 0; i < maxRetries; i++) {
      try {
        execSync('git push -u origin main --force', { cwd: dir, stdio: 'pipe' });
        return;
      } catch (err) {
        if (i === maxRetries - 1) {
          console.warn(`    ⚠️ Failed pushing ${name}: ${err.message}`);
          throw err;
        }
        execSync('sleep 2');
      }
    }
  };

  const addSubmoduleWithRetry = (dir, remoteURL, subPath, maxRetries = 6) => {
    for (let i = 0; i < maxRetries; i++) {
      try {
        execSync(`git submodule add --force -b main ${remoteURL} ${subPath}`, { cwd: dir, stdio: 'pipe' });
        return;
      } catch (err) {
        if (i === maxRetries - 1) {
          throw err;
        }
        execSync('sleep 2');
      }
    }
  };

  // 1. Submodule 1 (Orders Service - Go)
  console.log(`\n  ⚙️ Initializing Submodule 1 (Orders Service)...`);
  fs.mkdirSync(path.join(sub1LocalDir, 'handlers'), { recursive: true });
  fs.writeFileSync(path.join(sub1LocalDir, 'go.mod'), 'module metastackr/orders\n\ngo 1.22\n');
  fs.writeFileSync(path.join(sub1LocalDir, 'main.go'), `package main

import (
\t"fmt"
\t"net/http"
)

func main() {
\tfmt.Println("⚡ Orders Service running on :8081")
\thttp.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {
\t\tw.Write([]byte("{\\"status\\":\\"ok\\",\\"orders\\":[]}"))
\t})
\t_ = http.ListenAndServe(":8081", nil)
}
`);
  fs.writeFileSync(path.join(sub1LocalDir, 'handlers', 'order.go'), `package handlers

type OrderItem struct {
\tID       string  \`json:"id"\`
\tQuantity int     \`json:"qty"\`
\tPrice    float64 \`json:"price"\`
}

func CalculateSubtotal(items []OrderItem) float64 {
\tvar total float64
\tfor _, it := range items {
\t\ttotal += it.Price * float64(it.Quantity)
\t}
\treturn total
}
`);
  fs.writeFileSync(path.join(sub1LocalDir, 'README.md'), '# Orders Microservice\n\nHandles cart and checkout order processing.');
  
  initGitRepo(sub1LocalDir, sub1RemoteURL, 'feat(orders): initial orders microservice setup');
  pushWithRetry(sub1LocalDir, sub1RepoName);

  // 2. Submodule 2 (Payments Service - Node.js)
  console.log(`  ⚙️ Initializing Submodule 2 (Payments Service)...`);
  fs.mkdirSync(path.join(sub2LocalDir, 'routes'), { recursive: true });
  fs.writeFileSync(path.join(sub2LocalDir, 'package.json'), JSON.stringify({
    name: "payments-service",
    version: "1.0.0",
    description: "Payment gateway integration service",
    main: "index.js"
  }, null, 2));
  fs.writeFileSync(path.join(sub2LocalDir, 'index.js'), `const express = require('express');
const app = express();
app.use(express.json());

app.get('/health', (req, res) => res.json({ status: 'payments-healthy' }));
app.listen(8082, () => console.log('💳 Payments Service listening on :8082'));
`);
  fs.writeFileSync(path.join(sub2LocalDir, 'routes', 'payment.js'), `module.exports = {
  processPayment: async (amount, currency = 'USD') => {
    console.log(\`Processing \${amount} \${currency}\`);
    return { success: true, txnId: 'txn_' + Date.now() };
  }
};
`);
  fs.writeFileSync(path.join(sub2LocalDir, 'README.md'), '# Payments Microservice\n\nIntegrates third-party payment gateways.');

  initGitRepo(sub2LocalDir, sub2RemoteURL, 'feat(payments): initial payment gateway setup');
  pushWithRetry(sub2LocalDir, sub2RepoName);

  // 3. Parent Meta-Repository
  console.log(`  ⚙️ Initializing Parent Meta-Repo with Submodules...`);
  fs.mkdirSync(rootLocalDir, { recursive: true });
  fs.writeFileSync(path.join(rootLocalDir, 'README.md'), `# MetaStore Demo Architecture\n\nA unified meta-repository managing multi-service deployments with MetaStackr.`);
  fs.writeFileSync(path.join(rootLocalDir, 'docker-compose.yml'), `version: '3.8'
services:
  orders:
    build: ./services/orders
    ports: ["8081:8081"]
  payments:
    build: ./services/payments
    ports: ["8082:8082"]
`);

  initGitRepo(rootLocalDir, rootRemoteURL, 'chore(meta): initialize meta-repository baseline');

  fs.mkdirSync(path.join(rootLocalDir, 'services'), { recursive: true });
  console.log(`    🔗 Adding submodule services/orders...`);
  addSubmoduleWithRetry(rootLocalDir, sub1RemoteURL, 'services/orders');

  console.log(`    🔗 Adding submodule services/payments...`);
  addSubmoduleWithRetry(rootLocalDir, sub2RemoteURL, 'services/payments');

  execSync('git add . && git commit -m "chore(meta): link orders and payments submodules"', { cwd: rootLocalDir, stdio: 'ignore' });
  pushWithRetry(rootLocalDir, rootRepoName);

  // 4. Onboard repository to MetaStackr
  console.log(`    ⚡ Onboarding repository to MetaStackr via git meta init...`);
  try {
    const serverArg = options.server ? `--server ${options.server}` : '';
    execSync(`git meta init ${serverArg} --skip-webhooks`, { cwd: rootLocalDir, stdio: 'inherit' });
  } catch (err) {
    console.warn(`    ⚠️ Notice during git meta init: ${err.message}`);
  }

  console.log('✅ Demo repositories bootstrapped & onboarded successfully!\n');

  return {
    owner,
    rootRepoName,
    sub1RepoName,
    sub2RepoName,
    rootLocalDir,
    sub1LocalDir,
    sub2LocalDir,
    rootRemoteURL,
    sub1RemoteURL,
    sub2RemoteURL
  };
}

// -------------------------------------------------------------------------
// Browser Automation & Video Recording (Playwright)
// -------------------------------------------------------------------------

class BrowserRecorder {
  constructor(outputDir, extensionPath, clock, headless = false) {
    this.outputDir = outputDir;
    this.extensionPath = extensionPath;
    this.clock = clock;
    this.headless = headless;
    this.userDataDir = path.resolve(__dirname, '..', 'scratch', 'playwright-demo-profile');
    this.context = null;
    this.page = null;
  }

  async start() {
    const playwright = require('playwright');
    fs.mkdirSync(this.outputDir, { recursive: true });

    console.log(`\n🌐 Launching Chromium browser with MetaStackr extension...`);
    this.context = await playwright.chromium.launchPersistentContext(this.userDataDir, {
      headless: this.headless,
      viewport: { width: 1280, height: 720 },
      recordVideo: { dir: this.outputDir, size: { width: 1280, height: 720 } },
      args: [
        `--disable-extensions-except=${this.extensionPath}`,
        `--load-extension=${this.extensionPath}`
      ]
    });

    const pages = this.context.pages();
    this.page = pages.length > 0 ? pages[0] : await this.context.newPage();
  }

  async initMousePointer() {
    await this.page.evaluate(() => {
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

  async moveAndClick(selector, stepDescription, { targetStart = null, targetEnd = null, timeout = 4000 } = {}) {
    if (targetStart !== null) {
      await this.clock.waitUntil(targetStart);
    }

    console.log(`  🖱️ [Browser t=${this.clock.elapsed()}s] ${stepDescription}`);
    try {
      await this.initMousePointer();
      await this.page.waitForSelector(selector, { timeout });
      const loc = this.page.locator(selector).first();
      const box = await loc.boundingBox();

      if (box) {
        const targetX = box.x + box.width / 2;
        const targetY = box.y + box.height / 2;

        await this.page.evaluate(({ x, y }) => {
          const cursor = document.getElementById('demo-mouse-pointer');
          if (cursor) {
            cursor.style.left = `${x + window.scrollX}px`;
            cursor.style.top = `${y + window.scrollY}px`;
            cursor.style.transform = 'translate(-50%, -50%) scale(1.3)';
          }
        }, { x: targetX, y: targetY });

        await this.page.mouse.move(targetX, targetY, { steps: 20 });
        await this.page.waitForTimeout(300);

        await this.page.evaluate(() => {
          const cursor = document.getElementById('demo-mouse-pointer');
          if (cursor) cursor.style.transform = 'translate(-50%, -50%) scale(0.9)';
        });

        await this.page.click(selector, { timeout: 3000, force: true });
        await this.page.waitForTimeout(200);

        await this.page.evaluate(() => {
          const cursor = document.getElementById('demo-mouse-pointer');
          if (cursor) cursor.style.transform = 'translate(-50%, -50%) scale(1)';
        });
      }
    } catch (err) {
      console.warn(`    ⚠️ [Browser] Element not clicked (${selector}): ${err.message}`);
    }

    if (targetEnd !== null) {
      await this.clock.waitUntil(targetEnd);
    }
  }

  async navigateTo(url, { targetStart = null, targetEnd = null, timeout = 30000 } = {}) {
    if (targetStart !== null) {
      await this.clock.waitUntil(targetStart);
    }

    const cleanUrl = (url || '')
      .replace(/\x1B\[[0-?]*[ -/]*[@-~]/g, '')
      .replace(/[\u001b\u009b][[()#;?]*(?:[0-9]{1,4}(?:;[0-9]{0,4})*)?[0-9A-ORZcf-nqry=><]/g, '')
      .trim()
      .replace(/[\)\],;'"\.]+$/, '');

    console.log(`  🌐 [Browser t=${this.clock.elapsed()}s] Navigating to: ${cleanUrl}`);
    try {
      await this.page.goto(cleanUrl, { waitUntil: 'domcontentloaded', timeout });
      await this.page.waitForTimeout(800);
      await this.initMousePointer();
    } catch (err) {
      console.warn(`    ⚠️ [Browser] Navigation notice: ${err.message}`);
    }

    if (targetEnd !== null) {
      await this.clock.waitUntil(targetEnd);
    }
  }

  async stop(finalVideoPath) {
    const rawVideoPath = await this.page.video().path();
    await this.context.close();

    if (fs.existsSync(rawVideoPath)) {
      fs.mkdirSync(path.dirname(finalVideoPath), { recursive: true });
      fs.renameSync(rawVideoPath, finalVideoPath);
      console.log(`\n✅ Browser recording saved to: ${finalVideoPath}`);
      return finalVideoPath;
    }
    return null;
  }
}

// -------------------------------------------------------------------------
// Main Synchronized Orchestrator
// -------------------------------------------------------------------------

async function main() {
  const options = parseArgs();

  if (options.help) {
    showHelp();
    process.exit(0);
  }

  console.log('🎬 MetaStackr Synchronized Demo Recorder');
  console.log('========================================');
  console.log(`• Provider:       ${options.provider}`);
  console.log(`• Prefix:         ${options.prefix}`);
  console.log(`• Visibility:     ${options.visibility}`);
  console.log(`• Output Dir:     ${options.outputDir}`);
  console.log(`• Local Workdir:  ${options.workspace}`);

  // Handle interactive one-time login
  if (options.login) {
    console.log(`\n🔐 Opening browser for ${options.provider} login...`);
    console.log('Please log into your account in the browser window.');
    console.log('Your session will be permanently preserved in scratch/playwright-demo-profile for all automated recordings.');
    const clock = new DemoClock();
    const browser = new BrowserRecorder(
      options.outputDir,
      path.resolve(__dirname, '..', 'extensions', 'chrome'),
      clock,
      false
    );
    await browser.start();
    const loginUrl = options.provider === 'gitlab' ? 'https://gitlab.com/users/sign_in' : 'https://github.com/login';
    await browser.navigateTo(loginUrl);
    console.log('\n👉 Press [Enter] in this terminal once you have finished signing in...');
    await new Promise(resolve => process.stdin.once('data', resolve));
    await browser.context.close();
    console.log('\n✅ Login session successfully saved! You can now run the demo recorder.');
    process.exit(0);
  }

  // 1. Verify Prerequisites
  verifyPrerequisites(options.provider);

  // 2. Bootstrap Repositories
  const bootstrap = await bootstrapRepositories(options);

  // Setup Output Paths
  const demoNumSuffix = options.demoNumber !== null && !isNaN(options.demoNumber) ? `-${options.demoNumber}` : '';
  const baseName = `demo-${options.provider}${demoNumSuffix}`;
  const defaultBaseName = `demo-${options.provider}`;

  const castFilePath = path.join(options.outputDir, `${baseName}.cast`);
  const webmFilePath = path.join(options.outputDir, `${baseName}.webm`);
  const mp4FilePath = path.join(options.outputDir, `${baseName}.mp4`);
  const defaultCastFilePath = path.join(options.outputDir, `${defaultBaseName}.cast`);
  const defaultMp4FilePath = path.join(options.outputDir, `${defaultBaseName}.mp4`);

  const clock = new DemoClock();
  const term = new AsciicastRecorder(castFilePath, clock);
  const browser = new BrowserRecorder(
    options.outputDir,
    path.resolve(__dirname, '..', 'extensions', 'chrome'),
    clock,
    options.headless
  );

  const isGitLab = options.provider === 'gitlab';
  const repoName = bootstrap.rootRepoName;
  const featureBranch = 'feat/instant-discounts';

  await browser.start();

  // Navigate immediately to Root Repo so browser is ready and warm
  const repoWebURL = bootstrap.rootRemoteURL
    .replace(/\.git$/, '')
    .replace(/^git@github\.com:/, 'https://github.com/')
    .replace(/^git@gitlab\.com:/, 'https://gitlab.com/');
  console.log(`\n🌐 [Browser] Warming up root repository view: ${repoWebURL}`);
  await browser.page.goto(repoWebURL, { waitUntil: 'domcontentloaded' });
  await browser.page.waitForTimeout(1000);

  // -------------------------------------------------------------------------
  // SYNCHRONIZED CLOCK START (t = 0.00s)
  // -------------------------------------------------------------------------
  clock.start();

  console.log('\n🔴 SYNCHRONIZED RECORDING STARTED at t = 0.00s!');
  console.log('--------------------------------------------------');

  // =========================================================================
  // ACT 1: Terminal - Submodule Changes & PR Creation (0.0s -> 52.0s)
  // =========================================================================
  console.log('\n=== ACT 1: Terminal (Submodule Changes & git-meta PR Creation) [0s -> 52s] ===');

  // Step 1.1: Switch to feature branch across all repos (2.0s -> 8.0s)
  await term.executeCommand(`git meta checkout -b ${featureBranch}`, bootstrap.rootLocalDir, {
    targetStart: 2.0,
    targetEnd: 8.0,
    branch: 'main',
    repoName
  });

  // Step 1.2: Make modifications to submodules (8.0s -> 16.0s)
  const sub1FilePath = path.join(bootstrap.rootLocalDir, 'services', 'orders', 'handlers', 'order.go');
  const sub2FilePath = path.join(bootstrap.rootLocalDir, 'services', 'payments', 'routes', 'payment.js');

  fs.appendFileSync(sub1FilePath, '\n// feat(orders): add 15% VIP checkout promotion rule\n');
  fs.appendFileSync(sub2FilePath, '\n// feat(payments): enable Apple Pay instant tokenization\n');

  await term.executeCommand('echo "// feat: 15% VIP promo" >> services/orders/handlers/order.go', bootstrap.rootLocalDir, {
    targetStart: 8.0,
    targetEnd: 12.0,
    branch: featureBranch,
    repoName
  });
  await term.executeCommand('echo "// feat: Apple Pay tokenization" >> services/payments/routes/payment.js', bootstrap.rootLocalDir, {
    targetStart: 12.0,
    targetEnd: 16.0,
    branch: featureBranch,
    repoName
  });

  // Step 1.3: Inspect local submodule drift (16.0s -> 23.0s)
  await term.executeCommand('git meta status', bootstrap.rootLocalDir, {
    targetStart: 16.0,
    targetEnd: 23.0,
    branch: featureBranch,
    repoName
  });

  // Step 1.4: Coordinated atomic commit across dirty submodules and root pointer (23.0s -> 31.0s)
  await term.executeCommand('git meta commit -m "feat(checkout): add promotional discounts and apple pay"', bootstrap.rootLocalDir, {
    targetStart: 23.0,
    targetEnd: 31.0,
    branch: featureBranch,
    repoName
  });

  // Step 1.5: Bottom-up push enforcement (31.0s -> 39.0s)
  await term.executeCommand('git meta push', bootstrap.rootLocalDir, {
    targetStart: 31.0,
    targetEnd: 39.0,
    branch: featureBranch,
    repoName
  });

  // Step 1.6: Create PRs across all modified submodules and root meta-repo (39.0s -> 47.0s)
  const prOutput = await term.executeCommand('git meta create-pr --title "feat(checkout): add promotional discounts and apple pay"', bootstrap.rootLocalDir, {
    targetStart: 39.0,
    targetEnd: 47.0,
    branch: featureBranch,
    repoName
  });

  // Step 1.7: Run git meta status showing CLEAN and open PRs (47.0s -> 52.0s)
  await term.executeCommand('git meta status', bootstrap.rootLocalDir, {
    targetStart: 47.0,
    targetEnd: 52.0,
    branch: featureBranch,
    repoName
  });

  // Determine PR URLs
  let rootPRURL = isGitLab
    ? `https://gitlab.com/${bootstrap.owner}/${bootstrap.rootRepoName}/-/merge_requests/1`
    : `https://github.com/${bootstrap.owner}/${bootstrap.rootRepoName}/pull/1`;

  let sub1PRURL = isGitLab
    ? `https://gitlab.com/${bootstrap.owner}/${bootstrap.sub1RepoName}/-/merge_requests/1`
    : `https://github.com/${bootstrap.owner}/${bootstrap.sub1RepoName}/pull/1`;

  let sub2PRURL = isGitLab
    ? `https://gitlab.com/${bootstrap.owner}/${bootstrap.sub2RepoName}/-/merge_requests/1`
    : `https://github.com/${bootstrap.owner}/${bootstrap.sub2RepoName}/pull/1`;

  const cleanPrOutput = (prOutput || '')
    .replace(/\x1B\[[0-?]*[ -/]*[@-~]/g, '')
    .replace(/[\u001b\u009b][[()#;?]*(?:[0-9]{1,4}(?:;[0-9]{0,4})*)?[0-9A-ORZcf-nqry=><]/g, '');

  const urlMatches = cleanPrOutput.match(/https:\/\/(?:github\.com|gitlab\.com)\/[a-zA-Z0-9_\-\.\/]+/g);
  if (urlMatches && urlMatches.length >= 1) {
    for (const rawU of urlMatches) {
      const cleanU = rawU.replace(/[\)\],;'"\.]+$/, '').trim();
      if (cleanU.includes(bootstrap.rootRepoName) && (cleanU.includes('/pull/') || cleanU.includes('/merge_requests/'))) {
        rootPRURL = cleanU;
      }
      if (cleanU.includes(bootstrap.sub1RepoName) && (cleanU.includes('/pull/') || cleanU.includes('/merge_requests/'))) {
        sub1PRURL = cleanU;
      }
      if (cleanU.includes(bootstrap.sub2RepoName) && (cleanU.includes('/pull/') || cleanU.includes('/merge_requests/'))) {
        sub2PRURL = cleanU;
      }
    }
  }

  // =========================================================================
  // ACT 2: Browser - Root PR Diff, Matrix & Sub PR #1 Merge (52.0s -> 85.0s)
  // =========================================================================
  console.log('\n=== ACT 2: Browser (Root PR Diff, Matrix & Merge Sub PR #1) [52s -> 85s] ===');

  // Step 2.1: Open Root PR (52.0s -> 58.0s)
  await browser.navigateTo(rootPRURL, {
    targetStart: 52.0,
    targetEnd: 58.0
  });

  // Step 2.2: Click "Files changed" / "Changes" tab to show submodule pointer diffs (58.0s -> 64.0s)
  const diffTabSelector = isGitLab
    ? '#tab-diffs, a[data-action="diffs"], a:has-text("Changes")'
    : '#files-tab, a[href$="/files"], a:has-text("Files changed")';
  await browser.moveAndClick(diffTabSelector, 'Opening Files Changed tab to inspect Submodule Pointer Diffs...', {
    targetStart: 58.0,
    targetEnd: 64.0,
    timeout: 3000
  });

  // Step 2.3: Navigate to MetaStackr Matrix tab (64.0s -> 70.0s)
  const metaTabSelector = '#metastackr-submodules-tab, a[href="#metastackr"]';
  await browser.moveAndClick(metaTabSelector, 'Opening MetaStackr Submodules Matrix...', {
    targetStart: 64.0,
    targetEnd: 70.0,
    timeout: 3000
  });

  // Step 2.4: Navigate to Sub PR #1 (Orders Service) (70.0s -> 75.0s)
  const sub1LinkSelector = `#metastackr-submodules-panel a[href*="${bootstrap.sub1RepoName}"], a[href*="${bootstrap.sub1RepoName}"]`;
  await browser.moveAndClick(sub1LinkSelector, 'Navigating to Child PR #1 (Orders Service)...', {
    targetStart: 70.0,
    targetEnd: 72.0,
    timeout: 3000
  });
  await browser.navigateTo(sub1PRURL, {
    targetStart: 72.0,
    targetEnd: 75.0
  });

  // Step 2.5: Merge Sub PR #1 via UI (75.0s -> 80.0s)
  const mergeBtnSelector = isGitLab
    ? '[data-testid="ready-to-merge-btn"], [data-testid="merge-immediately-button"], button.qa-merge-button, button:has-text("Merge")'
    : 'button:has-text("Squash and merge"), button:has-text("Merge pull request"), .js-merge-box button';
  await browser.moveAndClick(mergeBtnSelector, 'Clicking Merge on Child PR #1...', {
    targetStart: 75.0,
    targetEnd: 78.0,
    timeout: 3000
  });

  const confirmMergeBtnSelector = isGitLab
    ? '[data-testid="merge-immediately-button"], button:has-text("Merge")'
    : 'button:has-text("Confirm squash and merge"), button:has-text("Confirm merge")';
  await browser.moveAndClick(confirmMergeBtnSelector, 'Confirming Merge on Child PR #1...', {
    targetStart: 78.0,
    targetEnd: 80.0,
    timeout: 3000
  });

  // Background fallback to guarantee merged status on remote
  try {
    if (isGitLab) {
      execSync(`glab mr merge 1 --repo ${bootstrap.owner}/${bootstrap.sub1RepoName} --squash --yes`, { cwd: bootstrap.rootLocalDir, stdio: 'ignore' });
    } else {
      execSync(`gh pr merge 1 --repo ${bootstrap.owner}/${bootstrap.sub1RepoName} --squash --auto || gh pr merge 1 --repo ${bootstrap.owner}/${bootstrap.sub1RepoName} --merge`, { cwd: bootstrap.rootLocalDir, stdio: 'ignore' });
    }
  } catch {}

  // Step 2.6: Click top banner link "← Return to Parent PR #1" (80.0s -> 85.0s)
  const bannerLinkSelector = '#metastackr-child-pr-banner a, .metastackr-banner-container a, a[href*="/pull/1"], a[href*="/merge_requests/1"]';
  await browser.moveAndClick(bannerLinkSelector, 'Navigating back to Parent PR via MetaStackr bar...', {
    targetStart: 80.0,
    targetEnd: 82.0,
    timeout: 3000
  });
  await browser.navigateTo(rootPRURL, {
    targetStart: 82.0,
    targetEnd: 85.0
  });

  // =========================================================================
  // ACT 3: Terminal & Browser Coordinated - Sub PR #2 Merge via CLI (85.0s -> 110.0s)
  // =========================================================================
  console.log('\n=== ACT 3: Terminal & Browser Coordinated (Merge Sub PR #2 via CLI) [85s -> 110s] ===');

  // Step 3.1: Browser navigates to Sub PR #2 (Payments Service PR) (85.0s -> 90.0s)
  await browser.navigateTo(sub2PRURL, {
    targetStart: 85.0,
    targetEnd: 90.0
  });

  // Step 3.2: Terminal runs git meta status showing Orders merged, Payments open (87.0s -> 94.0s)
  await term.executeCommand('git meta status', bootstrap.rootLocalDir, {
    targetStart: 87.0,
    targetEnd: 94.0,
    branch: featureBranch,
    repoName
  });

  // Step 3.3: Terminal executes merge on Sub PR #2 via CLI (94.0s -> 102.0s)
  if (isGitLab) {
    await term.executeCommand(`glab mr merge 1 --repo ${bootstrap.owner}/${bootstrap.sub2RepoName} --squash --yes`, bootstrap.rootLocalDir, {
      targetStart: 94.0,
      targetEnd: 102.0,
      branch: featureBranch,
      repoName
    });
  } else {
    await term.executeCommand(`gh pr merge 1 --repo ${bootstrap.owner}/${bootstrap.sub2RepoName} --squash --auto`, bootstrap.rootLocalDir, {
      targetStart: 94.0,
      targetEnd: 102.0,
      branch: featureBranch,
      repoName
    });
  }

  // Step 3.4: Browser reloads or verifies Sub PR #2 is Merged (102.0s -> 106.0s)
  await browser.navigateTo(sub2PRURL, {
    targetStart: 102.0,
    targetEnd: 106.0
  });

  // Step 3.5: Browser clicks banner back to Root PR (106.0s -> 110.0s)
  await browser.moveAndClick(bannerLinkSelector, 'Returning to Parent PR via MetaStackr bar...', {
    targetStart: 106.0,
    targetEnd: 108.0,
    timeout: 3000
  });
  await browser.navigateTo(rootPRURL, {
    targetStart: 108.0,
    targetEnd: 110.0
  });

  // =========================================================================
  // ACT 4: Browser - Root PR All Merged & Cascade Auto-Merge (110.0s -> 130.0s)
  // =========================================================================
  console.log('\n=== ACT 4: Browser (Root PR All Merged & Cascade Auto-Merge) [110s -> 130s] ===');

  // Step 4.1: Open MetaStackr tab showing BOTH PRs MERGED (110.0s -> 118.0s)
  await browser.moveAndClick(metaTabSelector, 'Opening MetaStackr tab to show BOTH child PRs MERGED ✅...', {
    targetStart: 110.0,
    targetEnd: 118.0,
    timeout: 3000
  });

  // Step 4.2: Switch to Conversation / Overview tab and watch Root PR cascade auto-merge (118.0s -> 124.0s)
  const convTabSelector = isGitLab
    ? '#tab-overview, a[data-action="show"], a:has-text("Overview")'
    : '#discussion-tab, a[href$="/pull/1"], a:has-text("Conversation")';
  await browser.moveAndClick(convTabSelector, 'Watching Root PR auto-cascade merge complete...', {
    targetStart: 118.0,
    targetEnd: 124.0,
    timeout: 3000
  });

  // Step 4.3: Navigate to Code tab on main branch to inspect updated submodule pointers (124.0s -> 130.0s)
  const codeTabSelector = isGitLab
    ? `a:has-text("Repository"), a[href*="/-/tree/main"]`
    : `a[data-tab-item="code-tab"], a[href="/${bootstrap.owner}/${bootstrap.rootRepoName}"]`;
  await browser.moveAndClick(codeTabSelector, 'Navigating to Code tab on main branch...', {
    targetStart: 124.0,
    targetEnd: 126.0,
    timeout: 3000
  });
  await browser.navigateTo(repoWebURL, {
    targetStart: 126.0,
    targetEnd: 130.0
  });

  // =========================================================================
  // ACT 5: Terminal - Final Status & Upstream Sync (130.0s -> 150.0s)
  // =========================================================================
  console.log('\n=== ACT 5: Terminal (Sync Main Branch) [130s -> 150s] ===');

  // Step 5.1: Run git meta status (130.0s -> 135.0s)
  await term.executeCommand('git meta status', bootstrap.rootLocalDir, {
    targetStart: 130.0,
    targetEnd: 135.0,
    branch: featureBranch,
    repoName
  });

  // Step 5.2: Switch back to main branch (135.0s -> 139.0s)
  await term.executeCommand('git meta checkout main', bootstrap.rootLocalDir, {
    targetStart: 135.0,
    targetEnd: 139.0,
    branch: featureBranch,
    repoName
  });

  // Step 5.3: Sync all submodules and root pointer with upstream (139.0s -> 145.0s)
  await term.executeCommand('git meta sync', bootstrap.rootLocalDir, {
    targetStart: 139.0,
    targetEnd: 145.0,
    branch: 'main',
    repoName
  });

  // Step 5.4: Final status check (145.0s -> 150.0s)
  await term.executeCommand('git meta status', bootstrap.rootLocalDir, {
    targetStart: 145.0,
    targetEnd: 150.0,
    branch: 'main',
    repoName
  });

  term.renderPrompt(repoName, 'main');
  await clock.waitUntil(150.0);

  // =========================================================================
  // Finalizing & Video Conversion
  // =========================================================================
  console.log('\n==================================================');
  console.log('💾 Finalizing recordings...');

  const savedWebm = await browser.stop(webmFilePath);

  // Convert WebM to MP4 if ffmpeg is available
  let hasFFmpeg = false;
  try {
    execSync('ffmpeg -version', { stdio: 'ignore' });
    hasFFmpeg = true;
  } catch {
    hasFFmpeg = false;
  }

  if (hasFFmpeg && savedWebm && fs.existsSync(savedWebm)) {
    console.log('🎞️ Converting recorded WebM to MP4 via ffmpeg...');
    try {
      execSync(`ffmpeg -y -i "${savedWebm}" -c:v libx264 -pix_fmt yuv420p "${mp4FilePath}"`, { stdio: 'ignore' });
      console.log(`  ✅ MP4 video saved to: ${mp4FilePath}`);
    } catch (err) {
      console.warn(`  ⚠️ FFmpeg conversion notice: ${err.message}`);
    }
  } else {
    if (savedWebm && fs.existsSync(savedWebm)) {
      try {
        fs.copyFileSync(savedWebm, mp4FilePath);
        console.log(`  ℹ️ Saved video to: ${mp4FilePath} (and ${webmFilePath})`);
      } catch {}
    }
  }

  // Also copy to standard default demo name
  if (mp4FilePath !== defaultMp4FilePath && fs.existsSync(mp4FilePath)) {
    try {
      fs.copyFileSync(mp4FilePath, defaultMp4FilePath);
    } catch {}
  }
  if (castFilePath !== defaultCastFilePath && fs.existsSync(castFilePath)) {
    try {
      fs.copyFileSync(castFilePath, defaultCastFilePath);
    } catch {}
  }

  // Automatically sync to web/assets/
  const webAssetsDir = path.resolve(__dirname, '..', 'web', 'assets');
  if (fs.existsSync(webAssetsDir)) {
    console.log('\n📦 Syncing recording assets to web/assets...');
    try {
      if (fs.existsSync(castFilePath)) {
        fs.copyFileSync(castFilePath, path.join(webAssetsDir, `${defaultBaseName}.cast`));
        if (options.provider === 'github') {
          fs.copyFileSync(castFilePath, path.join(webAssetsDir, 'demo.cast'));
        }
      }
      if (fs.existsSync(webmFilePath)) {
        fs.copyFileSync(webmFilePath, path.join(webAssetsDir, `${defaultBaseName}.webm`));
      }
      if (fs.existsSync(mp4FilePath)) {
        fs.copyFileSync(mp4FilePath, path.join(webAssetsDir, `${defaultBaseName}.mp4`));
      }
      console.log('  ✅ web/assets updated successfully.');
    } catch (err) {
      console.warn(`  ⚠️ web/assets sync notice: ${err.message}`);
    }
  }

  // Automatic Cleanup of remote and local repos (unless --keep was specified)
  if (options.clean && !options.keep) {
    console.log('\n🧹 Cleaning up remote and local demo repositories...');
    try {
      if (options.provider === 'github') {
        execSync(`gh repo delete ${bootstrap.owner}/${bootstrap.rootRepoName} --yes`, { stdio: 'ignore' });
        execSync(`gh repo delete ${bootstrap.owner}/${bootstrap.sub1RepoName} --yes`, { stdio: 'ignore' });
        execSync(`gh repo delete ${bootstrap.owner}/${bootstrap.sub2RepoName} --yes`, { stdio: 'ignore' });
      } else if (options.provider === 'gitlab') {
        execSync(`glab repo delete ${bootstrap.owner}/${bootstrap.rootRepoName} --yes`, { stdio: 'ignore' });
        execSync(`glab repo delete ${bootstrap.owner}/${bootstrap.sub1RepoName} --yes`, { stdio: 'ignore' });
        execSync(`glab repo delete ${bootstrap.owner}/${bootstrap.sub2RepoName} --yes`, { stdio: 'ignore' });
      }
      fs.rmSync(options.workspace, { recursive: true, force: true });
      console.log('  ✅ Deleted remote repositories and local workspace.');
    } catch (err) {
      console.warn(`  ⚠️ Notice during cleanup: ${err.message}`);
    }
  } else {
    console.log('\n📌 Repositories retained (--keep active):');
    console.log(`  • Root:     ${bootstrap.rootRemoteURL}`);
    console.log(`  • Orders:   ${bootstrap.sub1RemoteURL}`);
    console.log(`  • Payments: ${bootstrap.sub2RemoteURL}`);
  }

  console.log('\n🎉 DEMO RECORDING COMPLETE!');
  console.log('==================================================');
  console.log(`📹 Browser Video:     ${fs.existsSync(mp4FilePath) ? mp4FilePath : webmFilePath}`);
  console.log(`📺 Terminal Asciicast: ${castFilePath}`);
  console.log('');
  console.log('To view the recordings:');
  console.log(`  • Terminal: asciinema play "${castFilePath}"`);
  console.log(`  • Browser:  open "${fs.existsSync(mp4FilePath) ? mp4FilePath : webmFilePath}"`);
  console.log('==================================================\n');
}

main().catch(err => {
  console.error('\n❌ Fatal error during demo execution:', err);
  process.exit(1);
});
