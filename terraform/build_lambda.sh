#!/bin/bash
set -e

# Navigate to script directory
cd "$(dirname "$0")"

echo "🚀 Compiling metastackrd for AWS Lambda (Linux/ARM64 Graviton)..."
GOOS=linux GOARCH=arm64 go build -o bootstrap ../cmd/metastackrd

echo "📦 Packaging bootstrap.zip..."
zip -q bootstrap.zip bootstrap

# Clean up local bootstrap binary
rm bootstrap

echo "✅ bootstrap.zip created successfully in the terraform directory."
