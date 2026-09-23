#!/usr/bin/env python3
"""Coordinate maintenance, worker deployment, verification, and publication."""
import argparse
import base64
import hashlib
import json
import os
import re
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import urllib.request

from admission import publication
from verify import aws, collect, new_run, save, submit

ROOT = Path(__file__).resolve().parents[1]
LOG_DIRECTORY = None


def run(*args):
    result = subprocess.run(args, cwd=ROOT, capture_output=True, text=True)
    if LOG_DIRECTORY is not None:
        path = LOG_DIRECTORY / 'commands.log'
        with path.open('a') as file:
            path.chmod(0o600)
            file.write('command: ' + ' '.join(str(a) for a in args[:2]) + '\n' + result.stdout + result.stderr + '\n')
    if result.returncode:
        # Child output can contain infrastructure secrets. Keep it out of errors.
        raise RuntimeError('command failed: ' + ' '.join(str(a) for a in args[:2]) + '; see commands.log')
    return result.stdout


def update_env(region, function, changes):
    current = aws(region, 'lambda', 'get-function-configuration', '--function-name', function)
    variables = {**current['Environment']['Variables'], **changes}
    with tempfile.NamedTemporaryFile(mode='w') as file:
        json.dump(dict(FunctionName=function, RevisionId=current['RevisionId'], Environment=dict(Variables=variables)), file)
        file.flush()
        aws(region, 'lambda', 'update-function-configuration', '--cli-input-json', 'file://' + file.name)
    run('aws', 'lambda', 'wait', 'function-updated-v2', '--region', region, '--function-name', function)


def database_status(config):
    with tempfile.NamedTemporaryFile() as file:
        result = aws(config['region'], 'lambda', 'invoke', '--function-name', config['bridge'],
                     '--cli-binary-format', 'raw-in-base64-out', '--payload', '{"operation":"deployment-status"}', file.name)
        status = json.loads(Path(file.name).read_text())
    if result.get('FunctionError') or not isinstance(status, dict) or status.get('kind') != 'judge-deployment-status' or any(
            type(status.get(key)) is not int or status[key] < 0 for key in ('pending', 'undispatched')):
        raise ValueError('bridge must support deployment-status; deploy the compatible bridge package first')
    return status


def wait_empty(config, timeout=3600):
    deadline, stable = time.monotonic() + timeout, 0
    while time.monotonic() < deadline:
        status = database_status(config)
        counts = []
        for url in config['queues']:
            attributes = aws(config['region'], 'sqs', 'get-queue-attributes', '--queue-url', url,
                             '--attribute-names', 'ApproximateNumberOfMessages', 'ApproximateNumberOfMessagesNotVisible',
                             'ApproximateNumberOfMessagesDelayed')['Attributes']
            counts.extend(int(attributes[key]) for key in ('ApproximateNumberOfMessages',
                          'ApproximateNumberOfMessagesNotVisible', 'ApproximateNumberOfMessagesDelayed'))
        stable = stable + 1 if status['pending'] == status['undispatched'] == 0 and not any(counts) else 0
        if stable == 3:
            return
        time.sleep(10)
    raise TimeoutError('DB or queues did not drain; admission remains paused')


def delivery(config, enabled):
    region = config['region']
    if not enabled:
        aws(region, 'events', 'disable-rule', '--name', config['dispatch_rule'])
    aws(region, 'lambda', 'update-event-source-mapping', '--uuid', config['result_mapping'],
        '--enabled' if enabled else '--no-enabled')
    for _ in range(60):
        state = aws(region, 'lambda', 'get-event-source-mapping', '--uuid', config['result_mapping'])['State']
        if state == ('Enabled' if enabled else 'Disabled'):
            break
        time.sleep(5)
    else:
        raise TimeoutError('result mapping did not reach requested state')
    if enabled:
        aws(region, 'events', 'enable-rule', '--name', config['dispatch_rule'])


def checkpoint(directory, state, step, **values):
    state.pop('error', None)
    state.update(step=step, status='running')
    state.update(values)
    save(directory / 'state.json', state)
    print(step, flush=True)


def prune_worker_releases(config, state):
    """Keep only the worker archive version from the published rollout."""
    if state.get('status') != 'passed' or state.get('step') != 'complete':
        raise ValueError('release cleanup requires a completed rollout')
    region, bucket = config['region'], config['bucket']
    if aws(region, 'sts', 'get-caller-identity')['Account'] != config['account']:
        raise ValueError('wrong AWS account')
    digest = state['runtimeDigest']
    api = aws(region, 'lambda', 'get-function-configuration', '--function-name', config['api'])
    bridge = aws(region, 'lambda', 'get-function-configuration', '--function-name', config['bridge'])
    if api['Environment']['Variables'].get('JUDGE_CPP_IMAGE') != digest or \
            bridge['Environment']['Variables'].get('JUDGE_RUNTIME_DIGEST') != digest:
        raise ValueError('published runtime digest changed; release cleanup skipped')
    key = 'releases/' + state['releaseSHA256'] + '/worker.tar.gz'
    head = aws(region, 's3api', 'head-object', '--bucket', bucket, '--key', key)
    versions = aws(region, 's3api', 'list-object-versions', '--bucket', bucket, '--prefix', 'releases/').get('Versions', [])
    workers = [v for v in versions if re.fullmatch(r'releases/[0-9a-f]{64}/worker\.tar\.gz', v['Key'])]
    active = [v for v in workers if v['Key'] == key and v['IsLatest'] and v['VersionId'] == head['VersionId']]
    if len(active) != 1 or any(v['Key'] != key and v['LastModified'] >= active[0]['LastModified'] for v in workers):
        raise ValueError('worker release changed or a newer upload exists; release cleanup skipped')
    stale = [v for v in workers if v is not active[0]]
    if any(not v.get('VersionId') or v['VersionId'] == 'null' for v in stale):
        raise ValueError('unversioned worker release found; release cleanup skipped')
    for offset in range(0, len(stale), 1000):
        objects = [dict(Key=v['Key'], VersionId=v['VersionId']) for v in stale[offset:offset + 1000]]
        result = aws(region, 's3api', 'delete-objects', '--bucket', bucket, '--delete', json.dumps(dict(Objects=objects)))
        if result.get('Errors'):
            raise RuntimeError('S3 rejected one or more worker release deletions')
    return len(stale), sum(v['Size'] for v in stale)


def cleanup_after_success(config, directory, state, strict=False):
    if state.get('releasePruned'):
        return
    try:
        count, size = prune_worker_releases(config, state)
    except Exception as error:
        state['releasePruneError'] = str(error)
        save(directory / 'state.json', state)
        if strict:
            raise
        print('release cleanup pending: ' + str(error), file=sys.stderr, flush=True)
    else:
        state['releasePruned'] = True
        state.pop('releasePruneError', None)
        save(directory / 'state.json', state)
        print(f'release cleanup: {count} old versions, {size} bytes', flush=True)


def preflight(config):
    region = config['region']
    if not isinstance(config['runtimes'], list) or not config['runtimes'] or \
            any(not isinstance(name, str) or not re.fullmatch('[a-z][a-z0-9-]*', name) for name in config['runtimes']) or \
            len(set(config['runtimes'])) != len(config['runtimes']) or config['smoke_runtime'] not in config['runtimes']:
        raise ValueError('configure distinct approved runtimes, including the Python smoke runtime')
    if not config['api_url'].startswith('https://'):
        raise ValueError('configure an HTTPS API URL')
    if aws(region, 'sts', 'get-caller-identity')['Account'] != config['account']:
        raise ValueError('wrong AWS account')
    if len(config['nodes']) != 2 or len(set(config['nodes'])) != 2 or len(set(config['queues'])) != 4 or any(
            not re.fullmatch('mi-[a-f0-9]+', node) for node in config['nodes']):
        raise ValueError('configure two distinct hosts and request/result/dead queue URLs')
    bridge = aws(region, 'lambda', 'get-function-configuration', '--function-name', config['bridge'])
    mapping = aws(region, 'lambda', 'get-event-source-mapping', '--uuid', config['result_mapping'])
    targets = aws(region, 'events', 'list-targets-by-rule', '--rule', config['dispatch_rule'])['Targets']
    if mapping['FunctionArn'] != bridge['FunctionArn'] or not any(t['Arn'] == bridge['FunctionArn'] for t in targets):
        raise ValueError('dispatch or result mapping belongs to a different bridge')
    if bridge['Environment']['Variables']['JUDGE_REQUEST_QUEUE_URL'] != config['queues'][0] or \
            bridge['Environment']['Variables']['JUDGE_JOB_BUCKET'] != config['bucket']:
        raise ValueError('request queue or release bucket differs from bridge configuration')
    queues = [aws(region, 'sqs', 'get-queue-attributes', '--queue-url', url,
                  '--attribute-names', 'QueueArn', 'RedrivePolicy')['Attributes'] for url in config['queues']]
    if mapping['EventSourceArn'] != queues[1]['QueueArn'] or any(
            json.loads(queues[i]['RedrivePolicy'])['deadLetterTargetArn'] != queues[i + 2]['QueueArn'] for i in (0, 1)):
        raise ValueError('result queue or dead letter queue does not match')
    if not config['worker_alarms']:
        raise ValueError('configure the worker alarms')
    alarms = aws(region, 'cloudwatch', 'describe-alarms', '--alarm-names', *config['worker_alarms'])['MetricAlarms']
    if {a['AlarmName'] for a in alarms} != set(config['worker_alarms']):
        raise ValueError('a worker alarm is missing')
    for node in config['nodes']:
        info = aws(region, 'ssm', 'describe-instance-information', '--filters', 'Key=InstanceIds,Values=' + node)['InstanceInformationList']
        if len(info) != 1 or info[0]['PingStatus'] != 'Online':
            raise ValueError('SSM node is not online: ' + node)
    run('gh', 'variable', 'list', '--repo', config['repository'], '--env', config['github_environment'], '--json', 'name,value')
    for root in ('infra/api', 'infra/judge'):
        if not (ROOT / root / '.terraform').exists():
            raise ValueError('initialize Terraform before maintenance: ' + root)
        run('terraform', '-chdir=' + root, 'plan', '-input=false', '-refresh-only')


def backup_lambda(config, directory, function):
    path = directory / (function + '.json')
    if path.exists():
        return
    data = aws(config['region'], 'lambda', 'get-function', '--function-name', function)
    with urllib.request.urlopen(data['Code']['Location'], timeout=60) as response:
        code = response.read()
    archive = directory / (function + '.zip')
    archive.write_bytes(code)
    archive.chmod(0o600)
    save(path, data['Configuration'])


def prepare(config, directory, state):
    preflight(config)
    if state.get('prepared'):
        state.update(prepared=False, stopDirectory=str(directory / ('stop-' + str(time.time_ns()))))
        save(directory / 'state.json', state)
    for function in (config['api'], config['bridge']):
        backup_lambda(config, directory, function)
    package = Path(config['bridge_package'])
    package_bytes = package.read_bytes()
    package_hash = hashlib.sha256(package_bytes).hexdigest()
    if state.get('bridge_packageSHA256', package_hash) != package_hash:
        raise ValueError('bridge package changed after rollout preflight')
    digest = base64.b64encode(bytes.fromhex(package_hash)).decode()
    current = aws(config['region'], 'lambda', 'get-function-configuration', '--function-name', config['bridge'])
    if current['CodeSha256'] != digest:
        checkpoint(directory, state, 'bridge-update', maintenance=True)
        pinned_package = directory / 'bridge-upload.zip'
        pinned_package.write_bytes(package_bytes)
        pinned_package.chmod(0o600)
        aws(config['region'], 'lambda', 'update-function-code', '--function-name', config['bridge'],
            '--revision-id', current['RevisionId'], '--zip-file', 'fileb://' + str(pinned_package))
        run('aws', 'lambda', 'wait', 'function-updated-v2', '--region', config['region'], '--function-name', config['bridge'])
    if aws(config['region'], 'lambda', 'get-function-configuration', '--function-name', config['bridge'])['CodeSha256'] != digest:
        raise ValueError('deployed bridge package did not match')
    database_status(config)  # Older bridges returning null are never mistaken for an empty DB.
    checkpoint(directory, state, 'pause', maintenance=True)
    update_env(config['region'], config['api'], dict(JUDGE_ENABLED_RUNTIMES='none', JUDGE_CPP_IMAGE=''))
    state['admissionPaused'] = True
    api = aws(config['region'], 'lambda', 'get-function-configuration', '--function-name', config['api'])
    time.sleep(api['Timeout'] + 5)  # Let requests running with the previous admission configuration finish.
    checkpoint(directory, state, 'drain')
    wait_empty(config)
    checkpoint(directory, state, 'disable-delivery')
    delivery(config, False)
    time.sleep(current['Timeout'] + 5)  # Settle in-flight scheduled dispatch invocations.
    wait_empty(config)
    alarms = aws(config['region'], 'cloudwatch', 'describe-alarms', '--alarm-names', *config['worker_alarms'])['MetricAlarms']
    if 'alarm_actions' not in state:
        state['alarm_actions'] = {a['AlarmName']: a['ActionsEnabled'] for a in alarms}
        save(directory / 'state.json', state)
    aws(config['region'], 'cloudwatch', 'disable-alarm-actions', '--alarm-names', *config['worker_alarms'])
    checkpoint(directory, state, 'stop-workers')
    stop_dir = Path(state.get('stopDirectory', str(directory / 'stop')))
    if not stop_dir.exists():
        receipt = new_run(stop_dir, config['region'], config['nodes'], 'install')
        for node in config['nodes']:
            submit(stop_dir, receipt, node, '''set -eu
umask 077
systemctl disable --now judge-worker.service
root=/var/lib/judge-backups/''' + receipt['runId'] + '''
mkdir -p "$root"
tar -czf "$root/control.tar.gz" /opt/judge /etc/systemd/system/judge-worker.service /usr/local/etc/isolate /usr/local/bin/isolate
test -s "$root/control.tar.gz"
''')
    else:
        receipt = json.loads((stop_dir / 'receipt.json').read_text())
    if collect(stop_dir, receipt, wait=True) != 0:
        raise RuntimeError('worker stop/backup is incomplete')
    checkpoint(directory, state, 'prepared', prepared=True, status='prepared')


def healthy_workers(config, directory, digest):
    health_dir = directory / ('health-' + str(time.time_ns()))
    receipt = new_run(health_dir, config['region'], config['nodes'], 'install')
    command = '''set -eu
systemctl is-active --quiet judge-worker
PYTHONPATH=/opt/judge python3 - ''' + digest + ''' <<'PY'
import json, subprocess, sys
from pathlib import Path
from host import verify_assets
digest = sys.argv[1]
if verify_assets() != digest: raise SystemExit('installed digest differs')
pid = subprocess.check_output(['systemctl', 'show', 'judge-worker', '-p', 'MainPID', '--value'], text=True).strip()
env = Path('/proc/' + pid + '/environ').read_bytes().split(b'\\0')
if ('JUDGE_RUNTIME_DIGEST=' + digest).encode() not in env: raise SystemExit('running process digest differs')
invocation = subprocess.check_output(['systemctl', 'show', 'judge-worker', '-p', 'InvocationID', '--value'], text=True).strip()
if not invocation: raise SystemExit('worker invocation missing')
log = subprocess.check_output(['journalctl', '_SYSTEMD_INVOCATION_ID=' + invocation, '-o', 'cat', '--no-pager'], text=True)
events = []
for line in log.splitlines():
    try: events.append(json.loads(line))
    except ValueError: pass
if not any(e.get('event') == 'worker_started' for e in events): raise SystemExit('worker is not ready')
if any(e.get('event') == 'failure' for e in events): raise SystemExit('worker reported failures')
subprocess.run(['systemctl', 'is-active', '--quiet', 'judge-worker'], check=True)
print('worker ready')
PY
'''
    for node in config['nodes']:
        submit(health_dir, receipt, node, command)
    if collect(health_dir, receipt, wait=True) != 0:
        raise RuntimeError('workers are not ready')


def sync_settings(config, directory, digest, runtimes):
    for name, value in {'JUDGE_RUNTIME_DIGEST': digest, 'JUDGE_ENABLED_RUNTIMES': json.dumps(runtimes)}.items():
        run('gh', 'variable', 'set', name, '--repo', config['repository'], '--env', config['github_environment'], '--body', value)
    saved = json.loads(run('gh', 'variable', 'list', '--repo', config['repository'], '--env', config['github_environment'], '--json', 'name,value'))
    values = {v['name']: v['value'] for v in saved}
    if values.get('JUDGE_RUNTIME_DIGEST') != digest or json.loads(values.get('JUDGE_ENABLED_RUNTIMES', 'null')) != runtimes:
        raise ValueError('GitHub deployment variables did not match')
    for root, values in [('infra/api', dict(judge_runtime_digest=digest, judge_enabled_runtimes=runtimes)),
                         ('infra/judge', dict(runtime_digest=digest, enabled=True, worker_count=len(config['nodes'])))]:
        path = ROOT / root / 'zz-rollout.auto.tfvars.json'
        backup = directory / (root.replace('/', '-') + '-variables.json')
        if not backup.exists():
            save(backup, dict(existed=path.exists(), content=path.read_text() if path.exists() else ''))
        save(path, values)


def finish(config, directory, state, report_path):
    if not state.get('prepared'):
        raise ValueError('prepare must finish first')
    report = json.loads(report_path.read_text())
    if set(report.get('hosts', {})) != set(config['nodes']):
        raise ValueError('verified hosts differ from deployment targets')
    digest, published = publication(report, ','.join(config['runtimes']))
    checkpoint(directory, state, 'publication-preflight', maintenance=True)
    update_env(config['region'], config['api'], dict(JUDGE_ENABLED_RUNTIMES='none', JUDGE_CPP_IMAGE=''))
    api = aws(config['region'], 'lambda', 'get-function-configuration', '--function-name', config['api'])
    time.sleep(api['Timeout'] + 5)
    wait_empty(config)
    healthy_workers(config, directory, digest)
    checkpoint(directory, state, 'sync-bridge', runtimeDigest=digest)
    update_env(config['region'], config['bridge'], dict(JUDGE_RUNTIME_DIGEST=digest))
    if database_status(config)['runtimeDigest'] != digest:
        raise ValueError('bridge digest did not match')
    checkpoint(directory, state, 'sync-settings')
    sync_settings(config, directory, digest, config['runtimes'])
    checkpoint(directory, state, 'enable-delivery')
    delivery(config, True)
    for name, enabled in state['alarm_actions'].items():
        aws(config['region'], 'cloudwatch', 'enable-alarm-actions' if enabled else 'disable-alarm-actions', '--alarm-names', name)
    checkpoint(directory, state, 'publish')
    update_env(config['region'], config['api'], dict(JUDGE_CPP_IMAGE=digest, JUDGE_RUNTIME='cpp17-isolate', JUDGE_ENABLED_RUNTIMES=published))
    checkpoint(directory, state, 'api-smoke')
    with urllib.request.urlopen(config['api_url'].rstrip('/') + '/runtimes', timeout=30) as response:
        catalog = json.load(response)
    if catalog['maintenance'] or sorted(item['id'] for item in catalog['items']) != sorted(config['runtimes']):
        raise ValueError('public runtime catalog differs from approved runtimes')
    api_report = directory / ('api-' + str(time.time_ns()) + '.json')
    run(sys.executable, 'judge/smoke-api.py', '--function', config['api'], '--api-url', config['api_url'],
        '--region', config['region'], '--runtime', config['smoke_runtime'], '--report', str(api_report),
        '--instance', config['nodes'][0], '--instance', config['nodes'][1])
    if json.loads(api_report.read_text())['status'] != 'passed':
        raise ValueError('API smoke did not pass')
    checkpoint(directory, state, 'refresh-state')
    for root in ('infra/api', 'infra/judge'):
        plan = directory / (root.replace('/', '-') + '.tfplan')
        run('terraform', '-chdir=' + root, 'plan', '-input=false', '-refresh-only', '-out=' + str(plan))
        run('terraform', '-chdir=' + root, 'apply', '-input=false', str(plan))
    checkpoint(directory, state, 'complete', maintenance=False, admissionPaused=False, status='passed', apiReport=str(api_report))


def main():
    global LOG_DIRECTORY
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['prepare', 'finish', 'run', 'prune'])
    parser.add_argument('--config', type=Path, required=True)
    parser.add_argument('--run-dir', type=Path, required=True)
    parser.add_argument('--report', type=Path)
    args = parser.parse_args()
    os.chdir(ROOT)
    config = json.loads(args.config.read_text())
    directory = args.run_dir.resolve()
    os.environ['AWS_DEFAULT_REGION'] = config['region']
    if aws(config['region'], 'sts', 'get-caller-identity')['Account'] != config['account']:
        raise ValueError('wrong AWS account')
    directory.mkdir(parents=True, exist_ok=True, mode=0o700)
    directory.chmod(0o700)
    LOG_DIRECTORY = directory
    # ponytail: local operator lock; use CI concurrency before allowing multiple control machines.
    import fcntl
    locks = ROOT / 'judge/.build/rollout-locks'
    locks.mkdir(parents=True, exist_ok=True, mode=0o700)
    target = hashlib.sha256((config['account'] + config['region'] + config['api']).encode()).hexdigest()
    with (locks / target).open('w') as target_lock, (directory / 'lock').open('w') as lock:
        fcntl.flock(target_lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        config_path = directory / 'config.json'
        if config_path.exists() and json.loads(config_path.read_text()) != config:
            raise ValueError('configuration changed; use the saved configuration for recovery')
        save(config_path, config)
        state_path = directory / 'state.json'
        state = json.loads(state_path.read_text()) if state_path.exists() else {}
        if args.action == 'prune':
            cleanup_after_success(config, directory, state, strict=True)
            return
        try:
            for key in ('release', 'bridge_package'):
                with Path(config[key]).open('rb') as file:
                    digest = hashlib.file_digest(file, 'sha256').hexdigest()
                if key + 'SHA256' in state and state[key + 'SHA256'] != digest:
                    raise ValueError('release files changed; do not resume with different artifacts')
                state[key + 'SHA256'] = digest
            if state.get('status') == 'passed':
                cleanup_after_success(config, directory, state)
                print('already complete for these artifact hashes')
                return
            save(state_path, state)
            if args.action == 'prepare' or (args.action == 'run' and not state.get('prepared')):
                prepare(config, directory, state)
            if args.action == 'run':
                # Even a resumed controller must verify maintenance before touching workers.
                api = aws(config['region'], 'lambda', 'get-function-configuration', '--function-name', config['api'])
                if api['Environment']['Variables'].get('JUDGE_ENABLED_RUNTIMES') != 'none':
                    raise ValueError('admission is no longer paused; prepare and drain before resuming')
                wait_empty(config)
                checkpoint(directory, state, 'install')
                for i, node in enumerate(config['nodes']):
                    install = directory / ('install-' + str(i + 1))
                    if not install.exists():
                        run(sys.executable, 'judge/deploy-ssm.py', '--instance', node, '--bucket', config['bucket'],
                            '--region', config['region'], '--release', config['release'], '--expected-sha256', state['releaseSHA256'],
                            '--run-dir', str(install), '--no-wait')
                for i in range(len(config['nodes'])):
                    run(sys.executable, 'judge/verify.py', 'collect', '--run-dir', str(directory / ('install-' + str(i + 1))), '--wait')
                checkpoint(directory, state, 'verify')
                verification = directory / 'verify'
                if not verification.exists():
                    run(sys.executable, 'judge/verify.py', 'submit', '--run-dir', str(verification), '--region', config['region'],
                        '--instance', config['nodes'][0], '--instance', config['nodes'][1])
                run(sys.executable, 'judge/verify.py', 'collect', '--run-dir', str(verification), '--wait')
                receipt = json.loads((verification / 'receipt.json').read_text())
                checkpoint(directory, state, 'start')
                if receipt['phase'] == 'smoke':
                    run(sys.executable, 'judge/verify.py', 'start', '--run-dir', str(verification))
                run(sys.executable, 'judge/verify.py', 'collect', '--run-dir', str(verification), '--wait')
                finish(config, directory, state, verification / 'report.json')
            elif args.action == 'finish':
                if not args.report:
                    raise ValueError('--report is required for finish')
                finish(config, directory, state, args.report)
            if state.get('status') == 'passed':
                cleanup_after_success(config, directory, state)
        except BaseException as error:
            state.update(status='failed', error=str(error))
            if state.get('maintenance'):
                try:
                    update_env(config['region'], config['api'], dict(JUDGE_ENABLED_RUNTIMES='none', JUDGE_CPP_IMAGE=''))
                    state['admissionPaused'] = True
                except Exception:
                    state['admissionPaused'] = False
            save(state_path, state)
            raise


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        raise SystemExit(str(error)) from None
