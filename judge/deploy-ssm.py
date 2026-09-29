#!/usr/bin/env python3
"""Upload a pinned worker release and install it via SSM Run Command."""
import argparse
import hashlib
from pathlib import Path
import shlex
import subprocess
import uuid
from verify import NODE_ID, collect, new_run, submit


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--instance', required=True)
    parser.add_argument('--bucket', required=True)
    parser.add_argument('--region', default='ap-northeast-1')
    parser.add_argument('--release', type=Path, default=Path('judge/.build/worker.tar.gz'))
    parser.add_argument('--expected-sha256', help='Refuse a release changed since the rollout was started')
    parser.add_argument('--run-dir', type=Path, help='Save durable SSM command IDs for later collection')
    parser.add_argument('--no-wait', action='store_true', help='Return after submission; requires --run-dir')
    args = parser.parse_args()
    if not NODE_ID.fullmatch(args.instance):
        parser.error('expected an SSM managed-node ID')
    if args.no_wait and not args.run_dir:
        parser.error('--no-wait requires --run-dir')
    # Create the receipt before uploading or changing a host; never overwrite an earlier run.
    directory = args.run_dir or Path('judge/.build/install-' + uuid.uuid4().hex)
    receipt = new_run(directory, args.region, [args.instance], 'install')
    with args.release.open('rb') as file:
        digest = hashlib.file_digest(file, 'sha256').hexdigest()
    if args.expected_sha256 is not None and digest != args.expected_sha256:
        parser.error('release SHA-256 differs from the rollout receipt')
    uri = 's3://' + args.bucket + '/releases/' + digest + '/worker.tar.gz'
    # The key is the content hash and hosts verify it after download, so an existing object is reused.
    uploaded = subprocess.run(['aws', 's3api', 'head-object', '--bucket', args.bucket, '--key', 'releases/' + digest + '/worker.tar.gz',
                               '--region', args.region], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0
    if not uploaded:
        subprocess.run(['aws', 's3', 'cp', str(args.release), uri, '--region', args.region, '--only-show-errors'], check=True)
    url = subprocess.check_output(['aws', 's3', 'presign', uri, '--region', args.region,
        '--endpoint-url', 'https://s3.dualstack.' + args.region + '.amazonaws.com', '--expires-in', '3600'], text=True).strip()
    command = '\n'.join([
        'set -eu', 'umask 077', 'systemctl stop judge-worker.service || true',
        # Free the kept rollback tree first: download, archive and staged runtime peak at about 50 GB.
        'rm -rf /opt/judge-runtimes.previous-*',
        'install -d -m 700 /opt/judge-release',
        'curl --fail --location --proto "=https" ' + shlex.quote(url) + ' -o /opt/judge-release/worker.tar.gz',
        "printf '%s\\n' '" + digest + "  /opt/judge-release/worker.tar.gz' | sha256sum -c -",
        'tar -xzf /opt/judge-release/worker.tar.gz -C /opt/judge-release',
        # The verified release remains in S3; free its duplicate before staging the runtime tree.
        'rm /opt/judge-release/worker.tar.gz',
        'bash /opt/judge-release/install.sh',
    ])
    receipt['releaseSHA256'] = digest
    submit(directory, receipt, args.instance, command)
    print('Receipt:', directory / 'receipt.json', flush=True)
    if not args.no_wait:
        raise SystemExit(collect(directory, receipt, wait=True))
