---
tag: // Quick Start
title: Get Started in Seconds
---

## Quick Start Installation Guide

Follow these steps to compile and install `git meta` and launch the `metastackrd` daemon:

```bash
# 1. Clone repository & build
git clone https://github.com/org/MetaStackr.git
cd MetaStackr
make build

# 2. Install CLI executable into system PATH
make install

# 3. Start backend daemon
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/metastackr?sslmode=disable"
./metastackrd
```
