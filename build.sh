#!/usr/bin/env bash

set -euo pipefail

# Resolve repo root (directory containing this script)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "[build] Building Claude implementation (TypeScript → dist)"
# Install deps only if node_modules missing to avoid unnecessary network usage
if [ ! -d "claude/node_modules" ]; then
  (cd claude && npm install)
fi
(cd claude && npm run build)

# Create a lightweight launcher at repo root: ./ai-agent
LAUNCHER="$SCRIPT_DIR/ai-agent"
cat > "$LAUNCHER" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

# Resolve this script to its realpath (follow symlinks) to locate repo root
SOURCE="$0"
while [ -L "$SOURCE" ]; do
  DIR="$([[ "$SOURCE" = /* ]] && dirname "$SOURCE" || cd -P "$(dirname "$SOURCE")" && pwd)"
  SOURCE="$(readlink "$SOURCE")"
  [[ "$SOURCE" != /* ]] && SOURCE="$DIR/$SOURCE"
done
DIR="$(cd -P "$(dirname "$SOURCE")" && pwd)"

exec node "$DIR/claude/dist/cli.js" "$@"
EOF
chmod +x "$LAUNCHER"

# Ensure global symlink in /usr/local/bin using sudo (always)
TARGET_LINK="/usr/local/bin/ai-agent"
echo "[build] Installing symlink with sudo: $TARGET_LINK -> $LAUNCHER"
sudo ln -sf "$LAUNCHER" "$TARGET_LINK"

echo "[build] OK -> ai-agent installed at $TARGET_LINK"
