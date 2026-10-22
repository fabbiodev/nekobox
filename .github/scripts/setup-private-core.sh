#!/usr/bin/env bash
set -euo pipefail

test -n "${CORE_REPO_DEPLOY_KEY:-}"
test -n "${RUNNER_TEMP:-}"
test -n "${GITHUB_ENV:-}"

# Do not chmod the runner's pre-existing ~/.ssh. Windows images can protect
# that directory with ACLs that MSYS install/chmod cannot change.
temp_root="$RUNNER_TEMP"
ssh_bin="$(command -v ssh)"
keyscan_bin="$(command -v ssh-keyscan)"
if [[ "${RUNNER_OS:-}" == Windows ]]; then
  temp_root="$(cygpath -u "$temp_root")"
  # Use Git's MSYS OpenSSH with MSYS paths/permissions, not Windows OpenSSH.
  ssh_bin=/usr/bin/ssh.exe
  keyscan_bin=/usr/bin/ssh-keyscan.exe
fi
test -x "$ssh_bin"
test -x "$keyscan_bin"

umask 077
key_dir="$(mktemp -d "$temp_root/nekobox-core-ssh.XXXXXX")"
printf 'NEKOBOX_CORE_SSH_DIR=%s\n' "$key_dir" >> "$GITHUB_ENV"
printf '%s\n' "${CORE_REPO_DEPLOY_KEY//$'\r'/}" > "$key_dir/deploy_key"

scanned=false
for attempt in 1 2 3; do
  if "$keyscan_bin" -T 10 -t ed25519 github.com > "$key_dir/known_hosts" && [[ -s "$key_dir/known_hosts" ]]; then
    scanned=true
    break
  fi
  sleep "$attempt"
done
[[ "$scanned" == true ]]

printf -v ssh_command '%q -F /dev/null -i %q -o IdentitiesOnly=yes -o UserKnownHostsFile=%q -o StrictHostKeyChecking=yes -o BatchMode=yes -o ConnectTimeout=20' \
  "$ssh_bin" "$key_dir/deploy_key" "$key_dir/known_hosts"
printf 'GIT_SSH_COMMAND=%s\n' "$ssh_command" >> "$GITHUB_ENV"
git config --global url."git@github.com:1maxwarner/nekobox-sing-box-core".insteadOf \
  "https://github.com/1maxwarner/nekobox-sing-box-core"

# Check the same Git/SSH path Go uses, before starting the expensive tests/build.
GIT_SSH_COMMAND="$ssh_command" git ls-remote \
  https://github.com/1maxwarner/nekobox-sing-box-core HEAD > /dev/null
