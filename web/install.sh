#!/usr/bin/env bash
set -e

# MetaStackr (git-meta) One-Line Installer
# Usage: curl -fsSL https://metastac.kr/install.sh | bash

REPO="eliotstocker/MetaStackR"
BINARY_NAME="git-meta"

# Colors for terminal output
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

echo -e "${BLUE}${BOLD}🚀 MetaStackr (git-meta) Installer${NC}\n"

# 1. Detect OS
OS_NAME="$(uname -s)"
case "${OS_NAME}" in
    Linux*)     OS="linux";;
    Darwin*)    OS="darwin";;
    *)          echo -e "${RED}Error: Unsupported operating system: ${OS_NAME}${NC}"; exit 1;;
esac

# 2. Detect Architecture
ARCH_RAW="$(uname -m)"
case "${ARCH_RAW}" in
    x86_64|amd64)   ARCH="amd64";;
    arm64|aarch64)  ARCH="arm64";;
    *)              echo -e "${RED}Error: Unsupported architecture: ${ARCH_RAW}${NC}"; exit 1;;
esac

echo -e "  • Detected System: ${BOLD}${OS}/${ARCH}${NC}"

# 3. Determine Installation Directory
if [ -w "/usr/local/bin" ]; then
    INSTALL_DIR="/usr/local/bin"
    SUDO=""
elif command -v sudo >/dev/null 2>&1; then
    INSTALL_DIR="/usr/local/bin"
    SUDO="sudo"
else
    INSTALL_DIR="${HOME}/.local/bin"
    SUDO=""
    mkdir -p "${INSTALL_DIR}"
fi

# 4. Fetch Latest Tag / Release URL
if [ -n "${VERSION}" ]; then
    TARGET_TAG="${VERSION}"
else
    TARGET_TAG=$(curl -sSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' 2>/dev/null || echo "")
    if [ -z "${TARGET_TAG}" ]; then
        TARGET_TAG="latest"
    fi
fi

echo -e "  • Target Version:  ${BOLD}${TARGET_TAG}${NC}"
echo -e "  • Target Location: ${BOLD}${INSTALL_DIR}/${BINARY_NAME}${NC}\n"

TMP_DIR="$(mktemp -d)"
cleanup() { rm -rf "${TMP_DIR}"; }
trap cleanup EXIT

TARGET_FILE="${TMP_DIR}/${BINARY_NAME}"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${TARGET_TAG}/${BINARY_NAME}-${OS}-${ARCH}"
FALLBACK_URL="https://github.com/${REPO}/releases/download/${TARGET_TAG}/${BINARY_NAME}"

echo -e "📥 Downloading ${BINARY_NAME}..."

HTTP_CODE=$(curl -sSL -w "%{http_code}" -o "${TARGET_FILE}" "${DOWNLOAD_URL}" 2>/dev/null || echo "000")

if [ "${HTTP_CODE}" != "200" ]; then
    # Try fallback generic binary name
    HTTP_CODE=$(curl -sSL -w "%{http_code}" -o "${TARGET_FILE}" "${FALLBACK_URL}" 2>/dev/null || echo "000")
fi

if [ "${HTTP_CODE}" != "200" ]; then
    # Fallback to Go build if installed
    if command -v go >/dev/null 2>&1; then
        echo -e "${BLUE}Release binary download unavailable (${HTTP_CODE}). Installing via Go toolchain...${NC}"
        if [ -d "./cmd/git-meta" ] && [ -f "./go.mod" ]; then
            go build -o "${TARGET_FILE}" ./cmd/git-meta
        else
            go build -o "${TARGET_FILE}" "github.com/${REPO}/cmd/git-meta@latest" || go install "github.com/${REPO}/cmd/git-meta@latest"
        fi
    else
        echo -e "${RED}Error: Failed to download ${BINARY_NAME} (HTTP ${HTTP_CODE}) and Go is not installed.${NC}"
        echo -e "Please download binaries manually from: https://github.com/${REPO}/releases"
        exit 1
    fi
fi

chmod +x "${TARGET_FILE}" 2>/dev/null || true
${SUDO} cp "${TARGET_FILE}" "${INSTALL_DIR}/${BINARY_NAME}"
${SUDO} chmod +x "${INSTALL_DIR}/${BINARY_NAME}"

# 5. Verify Installation
if command -v "${BINARY_NAME}" >/dev/null 2>&1; then
    echo -e "\n${GREEN}${BOLD}✨ MetaStackr CLI (${BINARY_NAME}) successfully installed!${NC}\n"
    echo -e "Try running:"
    echo -e "  ${BOLD}git meta status${NC}   # Inspect local submodule drift and PR status"
    echo -e "  ${BOLD}git meta init${NC}     # Onboard current workspace with MetaStackr"
    echo -e "  ${BOLD}git meta help${NC}     # View full command reference\n"
else
    echo -e "\n${GREEN}Installed ${BINARY_NAME} to ${INSTALL_DIR}/${BINARY_NAME}${NC}"
    echo -e "${RED}Note: ${INSTALL_DIR} is not in your \$PATH.${NC}"
    echo -e "Add it to your PATH by adding this line to your shell profile (~/.zshrc or ~/.bashrc):"
    echo -e "  ${BOLD}export PATH=\"${INSTALL_DIR}:\$PATH\"${NC}\n"
fi
