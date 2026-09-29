#!/usr/bin/env python3
"""Submit durable SSM verification jobs, then collect their results without an AI monitor."""
import argparse
import json
import re
import shlex
import subprocess
import tempfile
import time
import uuid
from pathlib import Path

from admission import publication

PENDING = {'Pending', 'InProgress', 'Delayed'}


class AWSOperationError(RuntimeError):
    def __init__(self, operation, stderr):
        match = re.search(r'An error occurred \(([A-Za-z0-9]+)\)', stderr)
        self.code = match[1] if match else 'Unknown'
        super().__init__('AWS operation failed: ' + operation + ' (' + self.code + ')')


def aws(region, *args):
    result = subprocess.run(['aws', *args, '--region', region, '--output', 'json'],
                            capture_output=True, text=True)
    if result.returncode:
        # Requests can include presigned URLs; never echo argv or credentials.
        raise AWSOperationError(' '.join(args[:2]), result.stderr)
    return json.loads(result.stdout or '{}')


def save(path, value):
    with tempfile.NamedTemporaryFile(mode='w', dir=path.parent, prefix='.' + path.name, delete=False) as file:
        json.dump(value, file, indent=2)
        file.write('\n')
        temporary = Path(file.name)
    try:
        temporary.replace(path)
    finally:
        temporary.unlink(missing_ok=True)


# EC2 instance IDs, plus Lightsail hybrid-activation IDs until the migration completes.
NODE_ID = re.compile(r'i-[0-9a-f]{8,17}|mi-[a-f0-9]+')


def new_run(directory, region, instances, phase):
    if not instances or len(set(instances)) != len(instances) or any(
            not NODE_ID.fullmatch(node) for node in instances):
        raise ValueError('provide distinct SSM managed-node IDs')
    account = aws(region, 'sts', 'get-caller-identity')['Account']
    directory.mkdir(parents=True, mode=0o700, exist_ok=False)
    receipt = dict(version=1, runId=uuid.uuid4().hex, region=region, account=account,
                   phase=phase, instances=instances, commands={})
    save(directory / 'receipt.json', receipt)
    return receipt


def submit(directory, receipt, node, script):
    sent = aws(receipt['region'], 'ssm', 'send-command', '--instance-ids', node,
               '--document-name', 'AWS-RunShellScript', '--parameters',
               json.dumps({'commands': [script], 'executionTimeout': ['7200']}),
               '--comment', 'Judge ' + receipt['phase'] + ' ' + receipt['runId'])
    receipt['commands'][node] = sent['Command']['CommandId']
    save(directory / 'receipt.json', receipt)
    print(node, receipt['phase'], receipt['commands'][node], flush=True)


def smoke_command(run_id):
    # The installed smoke runner remains the source of the test cases and limits.
    return '''set -eu
umask 077
exec 9>/run/judge-verification.lock
flock -n 9
if systemctl is-active --quiet judge-worker.service; then
  echo 'Drain and stop the worker before verification' >&2; exit 1
fi
root=/var/lib/judge-verification/''' + run_id + '''
mkdir -p "$root" /run/systemd/system/judge-smoke.service.d
# Snapshot cold reads can take over 30 minutes; leave room for all language tests.
printf '[Service]\\nTimeoutStartSec=90min\\n' > /run/systemd/system/judge-smoke.service.d/verification-timeout.conf
trap 'rm -f /run/systemd/system/judge-smoke.service.d/verification-timeout.conf; systemctl daemon-reload' EXIT
unset JUDGE_SMOKE_RUNTIMES
date --iso-8601=seconds > "$root/started"
/opt/judge/smoke.sh > "$root/smoke.log" 2>&1
PYTHONPATH=/opt/judge python3 - "$root" <<'PY'
import json, subprocess, sys
from pathlib import Path
from host import verify_assets
from runtimes import RUNTIMES
root = Path(sys.argv[1])
log = subprocess.check_output(['journalctl', '-u', 'judge-smoke.service', '--since',
    root.joinpath('started').read_text().strip(), '-o', 'cat', '--no-pager'], text=True)
reports = []
for line in log.splitlines():
    try: item = json.loads(line)
    except ValueError: continue
    if isinstance(item, dict) and 'runtimeDigest' in item: reports.append(item)
if not reports: raise SystemExit('smoke did not produce a report')
report = reports[-1]
if report.get('failedRuntimes') != [] or set(report['passedRuntimes']) != set(RUNTIMES):
    raise SystemExit('not all installed runtimes passed')
if report['runtimeDigest'] != verify_assets(): raise SystemExit('assets changed after smoke')
report['runId'] = root.name
root.joinpath('report.json').write_text(json.dumps(report))
print(json.dumps(report))
PY
'''


def start_command(run_id, digest):
    return '''set -eu
umask 077
exec 9>/run/judge-verification.lock
flock -n 9
PYTHONPATH=/opt/judge python3 - ''' + shlex.quote(run_id) + ' ' + shlex.quote(digest) + ''' <<'PY'
import json, subprocess, sys, time
from pathlib import Path
from host import verify_assets
run_id, digest = sys.argv[1:]
root = Path('/var/lib/judge-verification') / run_id
report = json.loads(root.joinpath('report.json').read_text())
if report['runId'] != run_id or report['runtimeDigest'] != digest or report['failedRuntimes']:
    raise SystemExit('smoke report mismatch')
if verify_assets() != digest: raise SystemExit('assets changed after smoke')
env = Path('/opt/judge/worker.env')
lines = env.read_text().splitlines()
if sum(line.startswith('JUDGE_RUNTIME_DIGEST=') for line in lines) != 1:
    raise SystemExit('expected exactly one worker digest setting')
active = subprocess.run(['systemctl', 'is-active', '--quiet', 'judge-worker']).returncode == 0
if active: raise SystemExit('worker is already active; inspect the previous start command')
env.write_text('\\n'.join('JUDGE_RUNTIME_DIGEST=' + digest if line.startswith('JUDGE_RUNTIME_DIGEST=')
                        else line for line in lines) + '\\n')
env.chmod(0o600)
subprocess.run(['systemctl', 'enable', '--now', 'judge-worker'], check=True, stdout=subprocess.DEVNULL)
# Match this systemd invocation, not a worker_started event from an earlier boot.
invocation = subprocess.check_output(['systemctl', 'show', 'judge-worker', '-p', 'InvocationID', '--value'], text=True).strip()
if not invocation: raise SystemExit('worker invocation is missing')
for _ in range(360):
    current = subprocess.check_output(['systemctl', 'show', 'judge-worker', '-p', 'InvocationID', '--value'], text=True).strip()
    if current != invocation: raise SystemExit('worker restarted during readiness check')
    log = subprocess.check_output(['journalctl', '_SYSTEMD_INVOCATION_ID=' + invocation, '-o', 'cat', '--no-pager'], text=True)
    events = []
    for line in log.splitlines():
        try: events.append(json.loads(line))
        except ValueError: pass
    if any(e.get('event') == 'failure' for e in events): raise SystemExit('worker reported a failure')
    if any(e.get('event') == 'worker_started' for e in events):
        subprocess.run(['systemctl', 'is-active', '--quiet', 'judge-worker'], check=True)
        report['ready'] = True
        print(json.dumps(report))
        break
    time.sleep(10)
else: raise SystemExit('worker readiness timed out')
PY
'''


def merge_reports(receipt, reports):
    if set(reports) != set(receipt['instances']):
        raise ValueError('a host report is missing')
    first = next(iter(reports.values()))
    passed = first.get('passedRuntimes')
    if not isinstance(passed, list) or not passed or any(
            not isinstance(name, str) or not name.endswith('-isolate') for name in passed):
        raise ValueError('invalid runtime list')
    requested = ','.join(name.removesuffix('-isolate') for name in passed)
    for report in reports.values():
        publication(report, requested)
        if report.get('runId') != receipt['runId'] or report.get('failedRuntimes') != [] or \
                report['runtimeDigest'] != first['runtimeDigest'] or set(report['passedRuntimes']) != set(passed):
            raise ValueError('host reports disagree or belong to another run')
        if receipt['phase'] == 'start' and report.get('ready') is not True:
            raise ValueError('worker is not ready')
    return dict(runtimeDigest=first['runtimeDigest'], passedRuntimes=passed, failedRuntimes=[],
                runId=receipt['runId'], hosts=reports, ready=receipt['phase'] == 'start')


def collect(directory, receipt, wait=False, timeout=7200):
    deadline = time.monotonic() + timeout
    if set(receipt['commands']) != set(receipt['instances']):
        raise ValueError('submission was interrupted; recover missing command IDs before collecting')
    while True:
        states, reports = {}, {}
        for node, command in receipt['commands'].items():
            result = aws(receipt['region'], 'ssm', 'list-command-invocations',
                         '--command-id', command, '--instance-id', node, '--details')
            invocations = result['CommandInvocations']
            status = invocations[0]['Status'] if invocations else 'Pending'
            states[node] = status
            if status == 'Success' and receipt['phase'] != 'install':
                output = aws(receipt['region'], 'ssm', 'get-command-invocation',
                             '--command-id', command, '--instance-id', node)
                try:
                    reports[node] = json.loads(output['StandardOutputContent'].strip())
                except (ValueError, KeyError):
                    save(directory / 'status.json', dict(phase=receipt['phase'], states=states,
                                                         status='failed', reason='missing or malformed host report'))
                    raise ValueError('missing or malformed report from ' + node) from None
        failed = any(status not in PENDING | {'Success'} for status in states.values())
        complete = all(status == 'Success' for status in states.values())
        save(directory / 'status.json', dict(phase=receipt['phase'], states=states,
                                             status='failed' if failed else 'pending'))
        if failed:
            raise RuntimeError('SSM verification failed; inspect command IDs in receipt.json and remote smoke.log')
        if complete:
            if receipt['phase'] != 'install':
                try:
                    report = merge_reports(receipt, reports)
                except (ValueError, KeyError, TypeError):
                    save(directory / 'status.json', dict(phase=receipt['phase'], states=states,
                                                         status='failed', reason='invalid host reports'))
                    raise
                save(directory / 'report.json', report)
            save(directory / 'status.json', dict(phase=receipt['phase'], states=states, status='passed'))
            print(receipt['phase'] + ': all hosts passed', flush=True)
            return 0
        if not wait or time.monotonic() >= deadline:
            print('pending; collect again using the same run directory', flush=True)
            return 3
        time.sleep(30)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['submit', 'collect', 'start'])
    parser.add_argument('--run-dir', type=Path, required=True)
    parser.add_argument('--instance', action='append', default=[])
    parser.add_argument('--region', default='ap-northeast-1')
    parser.add_argument('--wait', action='store_true')
    args = parser.parse_args()
    if args.action == 'submit':
        receipt = new_run(args.run_dir, args.region, args.instance, 'smoke')
        for node in receipt['instances']:
            submit(args.run_dir, receipt, node, smoke_command(receipt['runId']))
        return collect(args.run_dir, receipt, wait=True) if args.wait else 0
    receipt = json.loads((args.run_dir / 'receipt.json').read_text())
    if aws(receipt['region'], 'sts', 'get-caller-identity')['Account'] != receipt['account']:
        raise ValueError('AWS account differs from receipt')
    if args.action == 'start':
        if receipt['phase'] != 'smoke':
            raise ValueError('start has already been submitted; use collect')
        if collect(args.run_dir, receipt) != 0:
            raise ValueError('smoke has not finished')
        report = json.loads((args.run_dir / 'report.json').read_text())
        save(args.run_dir / 'smoke-receipt.json', receipt)
        receipt.update(phase='start', commands={})
        save(args.run_dir / 'receipt.json', receipt)
        for node in receipt['instances']:
            submit(args.run_dir, receipt, node, start_command(receipt['runId'], report['runtimeDigest']))
        return collect(args.run_dir, receipt, wait=True) if args.wait else 0
    return collect(args.run_dir, receipt, args.wait)


if __name__ == '__main__':
    try:
        raise SystemExit(main())
    except (ValueError, KeyError, OSError, RuntimeError) as error:
        raise SystemExit(str(error)) from None
