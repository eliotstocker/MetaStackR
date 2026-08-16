'use strict'

const fs = require('fs')
const path = require('path')
const { marked } = require('marked')

const SITE_DIR = __dirname
const CONTENT_DIR = path.join(SITE_DIR, 'content')
const TEMPLATE_PATH = path.join(SITE_DIR, 'template.html')
const OUTPUT_PATH = path.join(SITE_DIR, 'index.html')
const CONFIG_DETAILS_PATH = path.join(SITE_DIR, 'config-details.html')

const readMd = (name) => fs.readFileSync(path.join(CONTENT_DIR, name), 'utf8')

// features.md is several "### Title\n\nBody" blocks separated by a lone
// "---" line — each becomes its own card in the grid.
function renderFeatures(md) {
  return md
    .trim()
    .split(/\n---\n/)
    .map((block) => `    <div class="feature">\n${marked.parse(block.trim())}    </div>`)
    .join('\n')
}

function renderExtensions(md) {
  const parts = md.trim().split(/\n---\n/)
  const headerMd = parts[0]
  const cards = parts.slice(1).map((block) => `    <div class="feature extension-card">\n${marked.parse(block.trim())}    </div>`).join('\n')
  return `${marked.parse(headerMd.trim())}\n  <div class="features extension-grid">\n${cards}\n  </div>`
}

function renderFlow(md) {
  const parts = md.trim().split(/\n---\n/)
  const headerMd = parts[0]
  const steps = parts.slice(1).map((block, idx) => `    <div class="flow-step">\n      <div class="step-num">0${idx + 1}</div>\n      <div class="step-content">\n${marked.parse(block.trim())}      </div>\n    </div>`).join('\n')
  return `${marked.parse(headerMd.trim())}\n  <div class="flow-grid">\n${steps}\n  </div>`
}

function renderCli(md) {
  const content = md.replace(/^---[\s\S]*?---\s*/, '')
  return marked.parse(content.trim())
}

const template = fs.readFileSync(TEMPLATE_PATH, 'utf8')

const output = template
  .replace('{{HERO}}', marked.parse(readMd('hero.md')))
  .replace('{{DEMO_CAPTION}}', marked.parse(readMd('demo-caption.md')))
  .replace('{{FLOW}}', renderFlow(readMd('flow.md')))
  .replace('{{COMPARISON}}', marked.parse(readMd('comparison.md')))
  .replace('{{FEATURES}}', renderFeatures(readMd('features.md')))
  .replace('{{EXTENSIONS}}', renderExtensions(readMd('extensions.md')))
  .replace('{{AGENTS}}', marked.parse(readMd('agents.md')))
  .replace('{{CLI}}', renderCli(readMd('cli.md')))

fs.writeFileSync(OUTPUT_PATH, output)

const configDetailsOutput = template
  .replace(/<section id="cli" class="cli">[\s\S]*?<\/section>/, `<section id="config" class="config">\n${marked.parse(readMd('config-details.md'))}    </section>`)
  .replace(/\s*<section class="hero">[\s\S]*?<\/section>/, '')
  .replace(/\s*<section class="demo">[\s\S]*?<\/section>/, '')
  .replace(/\s*<section id="flow" class="flow-section">[\s\S]*?<\/section>/, '')
  .replace(/\s*<section id="comparison" class="comparison-section">[\s\S]*?<\/section>/, '')
  .replace(/\s*<section id="features" class="features">[\s\S]*?<\/section>/, '')
  .replace(/\s*<section id="extensions" class="extensions">[\s\S]*?<\/section>/, '')
  .replace(/\s*<section id="agents" class="agents">[\s\S]*?<\/section>/, '')
  .replace(/<script src="https:\/\/cdn\.jsdelivr\.net\/npm\/asciinema-player[\s\S]*?<\/script>\s*<script>[\s\S]*?<\/script>/, '')

fs.writeFileSync(CONFIG_DETAILS_PATH, configDetailsOutput)

const INSTALL_SCRIPT_SRC = path.join(SITE_DIR, '..', 'install.sh')
const INSTALL_SCRIPT_DEST = path.join(SITE_DIR, 'install.sh')
if (fs.existsSync(INSTALL_SCRIPT_SRC)) {
  fs.copyFileSync(INSTALL_SCRIPT_SRC, INSTALL_SCRIPT_DEST)
  console.log(`[site] copied ${INSTALL_SCRIPT_DEST}`)
}

console.log(`[site] built ${OUTPUT_PATH}`)
console.log(`[site] built ${CONFIG_DETAILS_PATH}`)
