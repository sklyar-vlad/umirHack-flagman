#!/usr/bin/env bash
set -euo pipefail
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root/${FRONTEND_DIR:-frontend}"
for task in lint typecheck test build; do
  node -e 'const p=require("./package.json");process.exit(p.scripts?.[process.argv[1]] ? 0 : 1)' "$task"
  CI=1 npm run "$task"
done
