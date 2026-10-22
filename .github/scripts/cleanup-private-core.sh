#!/usr/bin/env bash
set -euo pipefail

# Remove only the two files we created, never recurse into a runner directory.
if [[ -n "${NEKOBOX_CORE_SSH_DIR:-}" ]]; then
  rm -f -- "$NEKOBOX_CORE_SSH_DIR/deploy_key" "$NEKOBOX_CORE_SSH_DIR/known_hosts"
  rmdir -- "$NEKOBOX_CORE_SSH_DIR"
fi
