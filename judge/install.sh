#!/usr/bin/env bash
# Run as root from a verified release directory, with dispatch disabled.
set -euo pipefail
cd "$(dirname "$0")"
test "$(id -u)" = 0
test "$(uname -m)" = x86_64
test -f /sys/fs/cgroup/cgroup.controllers
. /etc/os-release
test "$ID" = ubuntu
test "$VERSION_ID" = 24.04
systemctl stop judge-worker.service 2>/dev/null || true
# Apply security updates during planned maintenance, then fingerprint and smoke.
# Unattended upgrades change the platform digest and stop the verified worker.
printf 'APT::Periodic::Unattended-Upgrade "0";\n' > /etc/apt/apt.conf.d/99judge-maintenance
systemctl disable --now apt-daily-upgrade.timer
swapoff -a
apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
  build-essential pkg-config libcap-dev libseccomp-dev libsystemd-dev python3 python3-boto3 \
  libssl3t64 libffi8 libbz2-1.0 liblzma5 libsqlite3-0 libreadline8t64 libzstd1 libncursesw6 libxml2 libedit2 zlib1g-dev libicu74
# UID/GID belongs exclusively to the sandbox, never to an operator/service.
for sandbox_id in 60000 60001; do
if getent passwd "$sandbox_id" >/dev/null || getent group "$sandbox_id" >/dev/null; then
  echo 'Sandbox UID/GID must remain unassigned' >&2
  exit 1
fi
done
build_dir=$(mktemp -d)
trap 'rm -rf "$build_dir"' EXIT
tar -xzf isolate.tar.gz -C "$build_dir" --strip-components=1
make -C "$build_dir" -j1 isolate
install -m 0755 "$build_dir/isolate" /usr/local/bin/isolate
install -d -m 700 /opt/judge /opt/judge/assets /root/.aws
install -d -m 755 /opt/judge/sandbox-etc /var/local/lib/isolate
# Only a minimal synthetic /etc is visible to submissions.
printf 'root:x:0:0:root:/:/usr/sbin/nologin\nisolate:x:60000:60000::/box:/usr/sbin/nologin\nisolate1:x:60001:60001::/box:/usr/sbin/nologin\n' > /opt/judge/sandbox-etc/passwd
printf 'root:x:0:\nisolate:x:60000:\nisolate1:x:60001:\n' > /opt/judge/sandbox-etc/group
chmod 644 /opt/judge/sandbox-etc/*
install -m 0644 host.py sandbox.py interactive.py interactive_smoke.py testlib_smoke.py worker.py telemetry.py pool.py smoke.py fingerprint.py runtimes.py language-smoke.json /opt/judge/
test -f runtime.tar.gz
runtime_archive_sha=$(sha256sum runtime.tar.gz | cut -d ' ' -f1)
if [ -e /opt/judge-runtimes ]; then
  old_runtime_sha=$(cat /opt/judge/assets/runtime-archive.sha256)
  [[ "$old_runtime_sha" =~ ^[a-f0-9]{64}$ ]]
else
  old_runtime_sha=''
fi
if [ "$old_runtime_sha" != "$runtime_archive_sha" ]; then
  # Keep one previous tree: the disk holds the current tree, archive and staged tree during the swap.
  rm -rf /opt/judge-runtimes.previous-*
  runtime_stage=$(mktemp -d /opt/judge-runtime.XXXXXX)
  tar -xzf runtime.tar.gz -C "$runtime_stage"
  test -d "$runtime_stage/judge-runtimes"
  if [ -n "$old_runtime_sha" ]; then
    runtime_backup="/opt/judge-runtimes.previous-$old_runtime_sha"
    test ! -e "$runtime_backup"
    mv /opt/judge-runtimes "$runtime_backup"
  fi
  mv "$runtime_stage/judge-runtimes" /opt/judge-runtimes
  rmdir "$runtime_stage"
  printf '%s\n' "$runtime_archive_sha" > /opt/judge/assets/runtime-archive.sha256
fi
install -m 0755 smoke.sh /opt/judge/
install -m 0644 isolate-commit /opt/judge/assets/
install -d /usr/local/etc
cat > /usr/local/etc/isolate <<'CONFIG'
box_root = /var/local/lib/isolate
lock_root = /run/isolate/locks
cg_root = auto:/run/judge/cgroup
first_uid = 60000
first_gid = 60000
num_boxes = 2
restricted_init = 1
CONFIG
/usr/local/bin/isolate --check-config
# Global flock prevents smoke and queue services from using the same box ID.
printf 'f /run/judge-slot.lock 0600 root root -\n' > /etc/tmpfiles.d/judge.conf
systemd-tmpfiles --create /etc/tmpfiles.d/judge.conf
install -m 0644 judge-worker.service /etc/systemd/system/judge-worker.service
systemctl daemon-reload
/usr/bin/python3 /opt/judge/fingerprint.py
# The release stays in S3; drop the local archive copy only after a complete installation.
rm -f runtime.tar.gz
printf 'Installed. Configure credentials/environment, run smoke, then enable the worker.\n'
