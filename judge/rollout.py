#!/usr/bin/env python3
"""Coordinate maintenance, worker deployment, verification, and publication."""
import argparse
import base64
import datetime
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
from verify import NODE_ID, aws, collect, new_run, save, submit

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


def update_env(region, function, changes, remove=()):
    current = aws(region, 'lambda', 'get-function-configuration', '--function-name', function)
    variables = {name: value for name, value in {**current['Environment']['Variables'], **changes}.items() if name not in remove}
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


def hold_time(seconds):
    return time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime(time.time() + seconds))


def ensure_pool(config, hold=4 * 3600):
    """Start every EC2 judge host, including stopped burst hosts, and keep it up for maintenance."""
    if not config.get('pool'):
        return
    region = config['region']
    reservations = aws(region, 'ec2', 'describe-instances', '--filters', 'Name=tag:JudgePool,Values=' + config['pool'],
                       'Name=instance-state-name,Values=pending,running,stopping,stopped')['Reservations']
    states = {i['InstanceId']: i['State']['Name'] for r in reservations for i in r['Instances']}
    # A host left out of a rollout would later be started with a stale runtime.
    if set(states) != set(config['nodes']):
        raise ValueError('EC2 pool hosts differ from deployment targets')
    nodes = sorted(states)
    # Burst hosts power themselves off when idle past this hold, even mid-rollout.
    aws(region, 'ec2', 'create-tags', '--resources', *nodes, '--tags', 'Key=JudgeMaintenanceHoldUntil,Value=' + hold_time(hold))
    stopping = [node for node in nodes if states[node] == 'stopping']
    if stopping:
        run('aws', 'ec2', 'wait', 'instance-stopped', '--region', region, '--instance-ids', *stopping)
    stopped = [node for node in nodes if states[node] in ('stopping', 'stopped')]
    if stopped:
        aws(region, 'ec2', 'start-instances', '--instance-ids', *stopped)
    run('aws', 'ec2', 'wait', 'instance-running', '--region', region, '--instance-ids', *nodes)
    for _ in range(60):
        info = aws(region, 'ssm', 'describe-instance-information', '--filters',
                   'Key=InstanceIds,Values=' + ','.join(nodes))['InstanceInformationList']
        if len(info) == len(nodes) and all(i['PingStatus'] == 'Online' for i in info):
            return
        time.sleep(10)
    raise ValueError('EC2 pool hosts did not come online in SSM')


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
    # A rolling OS update without a release keeps the installed worker archive.
    if state.get('releasePruned') or 'releaseSHA256' not in state:
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
    if not config['nodes'] or len(set(config['nodes'])) != len(config['nodes']) or len(set(config['queues'])) != 4 or any(
            not NODE_ID.fullmatch(node) for node in config['nodes']):
        raise ValueError('configure distinct hosts and request/result/dead queue URLs')
    retired = config.get('retire_nodes', [])
    if len(set(retired)) != len(retired) or set(retired) & set(config['nodes']) or any(not NODE_ID.fullmatch(node) for node in retired):
        raise ValueError('retired hosts must be distinct SSM nodes outside the deployment targets')
    if 'pool' in config and not re.fullmatch('[a-z0-9-]+', config['pool']):
        raise ValueError('configure the EC2 pool tag value')
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
    for node in config['nodes'] + config.get('retire_nodes', []):
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


def deploy_bridge(config, directory, state, maintenance):
    """Back up the API and bridge, then install the pinned bridge package; returns the bridge configuration."""
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
        checkpoint(directory, state, 'bridge-update', maintenance=maintenance)
        pinned_package = directory / 'bridge-upload.zip'
        pinned_package.write_bytes(package_bytes)
        pinned_package.chmod(0o600)
        aws(config['region'], 'lambda', 'update-function-code', '--function-name', config['bridge'],
            '--revision-id', current['RevisionId'], '--zip-file', 'fileb://' + str(pinned_package))
        run('aws', 'lambda', 'wait', 'function-updated-v2', '--region', config['region'], '--function-name', config['bridge'])
    if aws(config['region'], 'lambda', 'get-function-configuration', '--function-name', config['bridge'])['CodeSha256'] != digest:
        raise ValueError('deployed bridge package did not match')
    return current


def stop_command(run_id):
    return '''set -eu
umask 077
systemctl disable --now judge-worker.service
root=/var/lib/judge-backups/''' + run_id + '''
mkdir -p "$root"
tar -czf "$root/control.tar.gz" /opt/judge /etc/systemd/system/judge-worker.service /usr/local/etc/isolate /usr/local/bin/isolate
test -s "$root/control.tar.gz"
'''


def prepare(config, directory, state):
    preflight(config)
    if state.get('prepared'):
        state.update(prepared=False, stopDirectory=str(directory / ('stop-' + str(time.time_ns()))))
        save(directory / 'state.json', state)
    current = deploy_bridge(config, directory, state, maintenance=True)
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
        # Retired hosts keep the old runtime; they must never receive a new-digest request.
        targets = config['nodes'] + config.get('retire_nodes', [])
        receipt = new_run(stop_dir, config['region'], targets, 'install')
        for node in targets:
            submit(stop_dir, receipt, node, stop_command(receipt['runId']))
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


def retired_workers_stopped(config, directory):
    if not config.get('retire_nodes'):
        return
    check_dir = directory / ('retired-' + str(time.time_ns()))
    receipt = new_run(check_dir, config['region'], config['retire_nodes'], 'install')
    for node in config['retire_nodes']:
        submit(check_dir, receipt, node, '! systemctl is-active --quiet judge-worker && ! systemctl is-enabled --quiet judge-worker')
    if collect(check_dir, receipt, wait=True) != 0:
        raise RuntimeError('a retired worker is still enabled')


def sync_settings(config, directory, digest, runtimes, previous=None):
    for name, value in {'JUDGE_RUNTIME_DIGEST': digest, 'JUDGE_ENABLED_RUNTIMES': json.dumps(runtimes)}.items():
        run('gh', 'variable', 'set', name, '--repo', config['repository'], '--env', config['github_environment'], '--body', value)
    saved = json.loads(run('gh', 'variable', 'list', '--repo', config['repository'], '--env', config['github_environment'], '--json', 'name,value'))
    values = {v['name']: v['value'] for v in saved}
    if values.get('JUDGE_RUNTIME_DIGEST') != digest or json.loads(values.get('JUDGE_ENABLED_RUNTIMES', 'null')) != runtimes:
        raise ValueError('GitHub deployment variables did not match')
    # Terraform owns the fleet shape; keep values such as the host count that rollout does not manage.
    for root, values in [('infra/api', dict(judge_runtime_digest=digest, judge_enabled_runtimes=runtimes)),
                         ('infra/judge', dict(runtime_digest=digest, enabled=True, enabled_runtimes=runtimes,
                                              **({} if previous is None else dict(previous_runtime_digest=previous))))]:
        path = ROOT / root / 'zz-rollout.auto.tfvars.json'
        backup = directory / (root.replace('/', '-') + '-variables.json')
        if not backup.exists():
            save(backup, dict(existed=path.exists(), content=path.read_text() if path.exists() else ''))
        previous = json.loads(backup.read_text())
        save(path, {**json.loads(previous['content'] or '{}'), **values})


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
    retired_workers_stopped(config, directory)
    if config.get('pool'):
        # The bridge starts a stopped host only when this matches its dispatch digest.
        aws(config['region'], 'ec2', 'create-tags', '--resources', *config['nodes'], '--tags', 'Key=JudgeInstalledDigest,Value=' + digest)
    checkpoint(directory, state, 'sync-bridge', runtimeDigest=digest)
    update_env(config['region'], config['bridge'], dict(JUDGE_RUNTIME_DIGEST=digest, JUDGE_ENABLED_RUNTIMES=published))
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
        *[option for node in config['nodes'] for option in ('--instance', node)])
    if json.loads(api_report.read_text())['status'] != 'passed':
        raise ValueError('API smoke did not pass')
    checkpoint(directory, state, 'refresh-state')
    for root in ('infra/api', 'infra/judge'):
        plan = directory / (root.replace('/', '-') + '.tfplan')
        run('terraform', '-chdir=' + root, 'plan', '-input=false', '-refresh-only', '-out=' + str(plan))
        run('terraform', '-chdir=' + root, 'apply', '-input=false', str(plan))
    released = True
    if config.get('pool'):
        # Let idle burst hosts stop soon; otherwise the 4-hour maintenance hold keeps them running.
        try:
            aws(config['region'], 'ec2', 'create-tags', '--resources', *config['nodes'], '--tags', 'Key=JudgeMaintenanceHoldUntil,Value=' + hold_time(900))
        except Exception:
            released = False
    checkpoint(directory, state, 'complete', maintenance=False, admissionPaused=False, status='passed', apiReport=str(api_report),
               maintenanceHoldReleased=released)


# A rolling update takes about two hours: one smoke on the first burst, the switch, and one smoke on the rest.
ROLLING_HOLD = 6 * 3600
SNAPSHOT_ID = re.compile(r'[0-9]{8}T[0-9]{6}Z')
PACKAGE_LIST = "dpkg-query -W -f='${Package}=${Version}\\n'"


def os_update_command(run_id, snapshot, expected_packages=None):
    """Upgrade from a pinned archive snapshot, reboot through SSM (exit 194), then confirm the new kernel."""
    if snapshot is not None and not SNAPSHOT_ID.fullmatch(snapshot):
        raise ValueError('invalid APT snapshot ID')
    if expected_packages is not None and not re.fullmatch('[a-f0-9]{64}', expected_packages):
        raise ValueError('invalid package list digest')
    # A retry without a snapshot must not inherit the pin from an earlier attempt.
    pin = '  rm -f /etc/apt/apt.conf.d/99judge-snapshot\n'
    if snapshot is not None:
        pin = '''  # install.sh also runs apt; it stays on this snapshot until the rollout removes the pin.
  printf 'APT::Snapshot "%s";\\n' ''' + snapshot + ''' > /etc/apt/apt.conf.d/99judge-snapshot
'''
    compare = ''
    if expected_packages is not None:
        compare = '''  if [ "$packages" != ''' + expected_packages + ''' ]; then
    echo "packages differ from the first updated host; inspect $state/packages.txt" >&2; exit 1
  fi
'''
    return '''set -eu
umask 022
state=/var/lib/judge-os-update/''' + run_id + '''
install -d -m 700 /var/lib/judge-os-update "$state"
if [ ! -e "$state/boot-id" ]; then
  if systemctl is-active --quiet judge-worker.service || systemctl is-enabled --quiet judge-worker.service; then
    echo 'Stop and disable the worker before the OS update' >&2; exit 1
  fi
''' + pin + '''  export DEBIAN_FRONTEND=noninteractive NEEDRESTART_SUSPEND=1
  # SSM keeps only the first 24,000 characters of output; the last line must report the package list.
  # Phased updates depend on the machine ID; every host must select the same versions.
  if ! { apt-get update && apt-get -y -o APT::Get::Never-Include-Phased-Updates=true \\
      -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold full-upgrade; } > "$state/apt.log" 2>&1; then
    tail -n 40 "$state/apt.log" >&2; exit 1
  fi
  LC_ALL=C ''' + PACKAGE_LIST + ''' > "$state/packages.txt"
  packages=$(sha256sum < "$state/packages.txt" | cut -d ' ' -f1)
''' + compare + '''  cat /proc/sys/kernel/random/boot_id > "$state/boot-id"
  # SSM Agent reboots and runs this script again; the command finishes after the second run.
  exit 194
fi
if [ "$(cat /proc/sys/kernel/random/boot_id)" = "$(cat "$state/boot-id")" ]; then
  echo 'the host did not reboot' >&2; exit 1
fi
kernel=$(basename "$(readlink -f /boot/vmlinuz)")
test "$(uname -r)" = "${kernel#vmlinuz-}"
LC_ALL=C ''' + PACKAGE_LIST + ''' | cmp -s - "$state/packages.txt"
echo "packages=$(sha256sum < "$state/packages.txt" | cut -d ' ' -f1) kernel=$(uname -r)"
'''


def seal_command(expected_digest=None):
    """Drop the snapshot pin and print the fingerprinted digest, failing if it differs from the switched one."""
    if expected_digest is not None and not re.fullmatch('sha256:[a-f0-9]{64}', expected_digest):
        raise ValueError('invalid runtime digest')
    return '''set -eu
rm -f /etc/apt/apt.conf.d/99judge-snapshot
PYTHONPATH=/opt/judge python3 - ''' + (expected_digest or '') + ''' <<'PY'
import sys
from host import verify_assets
digest = verify_assets()
if sys.argv[1:] and digest != sys.argv[1]:
    raise SystemExit('runtime digest ' + digest + ' differs from ' + sys.argv[1])
print(digest)
PY
'''


def command_output(receipt, node):
    return aws(receipt['region'], 'ssm', 'get-command-invocation', '--command-id', receipt['commands'][node],
               '--instance-id', node)['StandardOutputContent']


def ssm_step(directory, name, config, nodes, script):
    """Run one script on each node once; a resumed rollout only collects the saved command IDs."""
    path = directory / name
    if path.exists():
        receipt = json.loads((path / 'receipt.json').read_text())
    else:
        receipt = new_run(path, config['region'], nodes, 'install')
        for node in nodes:
            submit(path, receipt, node, script(receipt['runId']) if callable(script) else script)
    if collect(path, receipt, wait=True) != 0:
        raise RuntimeError(name + ' did not finish; inspect ' + str(path / 'receipt.json'))
    return receipt


def pool_bursts(config):
    """Return the burst hosts by name; every rolling target must carry a primary or burst role."""
    reservations = aws(config['region'], 'ec2', 'describe-instances', '--instance-ids', *config['nodes'])['Reservations']
    hosts = []
    for instance in (i for r in reservations for i in r['Instances']):
        tags = {t['Key']: t['Value'] for t in instance.get('Tags', [])}
        hosts.append((tags.get('JudgeRole'), tags.get('Name', ''), instance['InstanceId']))
    if sorted(h[2] for h in hosts) != sorted(config['nodes']) or any(h[0] not in ('primary', 'burst') for h in hosts):
        raise ValueError('every rolling target needs a primary or burst JudgeRole tag')
    bursts = [node for _, node in sorted((name, node) for role, name, node in hosts if role == 'burst')]
    if not bursts:
        raise ValueError('a rolling update prepares the new environment on a burst host')
    return bursts


def contest_guard(config):
    """Refuse to change the environment within or shortly before a contest window (ADR 0014)."""
    status = database_status(config)
    if 'nextContestWindowAt' not in status or 'dispatchPaused' not in status:
        raise ValueError('bridge must support the rolling update; deploy the compatible bridge package first')
    window = status['nextContestWindowAt']
    hours = config.get('rolling_guard_hours', 4)
    if window is not None and datetime.datetime.fromisoformat(window).timestamp() < time.time() + hours * 3600:
        raise ValueError('a contest window starts at ' + window + '; run the rolling update outside contests')
    return status


def wait_requests_idle(config, timeout=2700):
    """Wait until every dispatched submission has finished and the request queue is empty."""
    deadline, stable = time.monotonic() + timeout, 0
    while time.monotonic() < deadline:
        status = database_status(config)
        attributes = aws(config['region'], 'sqs', 'get-queue-attributes', '--queue-url', config['queues'][0],
                         '--attribute-names', 'ApproximateNumberOfMessages', 'ApproximateNumberOfMessagesNotVisible',
                         'ApproximateNumberOfMessagesDelayed')['Attributes']
        busy = status['pending'] != status['undispatched'] or any(int(value) for value in attributes.values())
        stable = 0 if busy else stable + 1
        if stable == 3:
            return
        time.sleep(10)
    raise TimeoutError('dispatched submissions did not finish; dispatch remains paused')


def set_dispatch(config, directory, state, paused):
    if paused:
        update_env(config['region'], config['bridge'], dict(JUDGE_DISPATCH_PAUSED='true'))
    else:
        update_env(config['region'], config['bridge'], {}, remove=('JUDGE_DISPATCH_PAUSED',))
    state['dispatchPaused'] = paused
    save(directory / 'state.json', state)
    if database_status(config)['dispatchPaused'] is not paused:
        raise ValueError('bridge dispatch pause did not take effect')


def tag_digest(config, nodes, digest):
    # The bridge starts a stopped host only when this matches its dispatch digest.
    aws(config['region'], 'ec2', 'create-tags', '--resources', *nodes, '--tags', 'Key=JudgeInstalledDigest,Value=' + digest)


def prepare_hosts(config, directory, state, nodes, label, expected_digest=None):
    """Update the OS on hosts with stopped workers, install or fingerprint, then smoke-test them."""
    first = expected_digest is None
    receipt = ssm_step(directory, 'os-' + label, config, nodes,
                       lambda run_id: os_update_command(run_id, state['aptSnapshot'], None if first else state['packages']))
    if first and 'packages' not in state:
        lines = command_output(receipt, nodes[0]).strip().splitlines()
        match = re.fullmatch(r'packages=([a-f0-9]{64}) kernel=(\S+)', lines[-1] if lines else '')
        if not match:
            raise ValueError('OS update did not report the package list')
        checkpoint(directory, state, 'install-' + label, packages=match[1], kernel=match[2])
    if config.get('release'):
        for node in nodes:
            install = directory / ('install-' + node)
            if not install.exists():
                run(sys.executable, 'judge/deploy-ssm.py', '--instance', node, '--bucket', config['bucket'],
                    '--region', config['region'], '--release', config['release'], '--expected-sha256', state['releaseSHA256'],
                    '--run-dir', str(install), '--no-wait')
        for node in nodes:
            run(sys.executable, 'judge/verify.py', 'collect', '--run-dir', str(directory / ('install-' + node)), '--wait')
    else:
        ssm_step(directory, 'fingerprint-' + label, config, nodes, 'set -eu\n/usr/bin/python3 /opt/judge/fingerprint.py\n')
    ssm_step(directory, 'seal-' + label, config, nodes, seal_command(expected_digest))
    verification = directory / ('verify-' + label)
    if not verification.exists():
        run(sys.executable, 'judge/verify.py', 'submit', '--run-dir', str(verification), '--region', config['region'],
            *[option for node in nodes for option in ('--instance', node)])
    run(sys.executable, 'judge/verify.py', 'collect', '--run-dir', str(verification), '--wait')
    report = json.loads((verification / 'report.json').read_text())
    publication(report['hosts'][nodes[0]], ','.join(config['runtimes']))
    if not first and report['runtimeDigest'] != expected_digest:
        raise ValueError('updated hosts differ from the switched runtime digest')
    return verification


def start_workers(verification):
    receipt = json.loads((verification / 'receipt.json').read_text())
    if receipt['phase'] == 'smoke':
        run(sys.executable, 'judge/verify.py', 'start', '--run-dir', str(verification))
    run(sys.executable, 'judge/verify.py', 'collect', '--run-dir', str(verification), '--wait')


def api_smoke(config, directory, nodes):
    """Judge through the public API; with two or more hosts, also require concurrent judging."""
    with urllib.request.urlopen(config['api_url'].rstrip('/') + '/runtimes', timeout=30) as response:
        catalog = json.load(response)
    if catalog['maintenance'] or sorted(item['id'] for item in catalog['items']) != sorted(config['runtimes']):
        raise ValueError('public runtime catalog differs from approved runtimes')
    report = directory / ('api-' + str(time.time_ns()) + '.json')
    run(sys.executable, 'judge/smoke-api.py', '--function', config['api'], '--api-url', config['api_url'],
        '--region', config['region'], '--runtime', config['smoke_runtime'], '--report', str(report),
        *[option for node in nodes if len(nodes) > 1 for option in ('--instance', node)])
    if json.loads(report.read_text())['status'] != 'passed':
        raise ValueError('API smoke did not pass')
    return report


def switch(config, directory, state, first, rest, verification):
    """Hold dispatch, retire the old digest, and start the prepared burst; admission stays open."""
    region, digest = config['region'], state['runtimeDigest']
    if not state.get('digestSwitched'):
        contest_guard(config)
        if not state.get('dispatchPaused'):
            set_dispatch(config, directory, state, True)
            bridge = aws(region, 'lambda', 'get-function-configuration', '--function-name', config['bridge'])
            time.sleep(bridge['Timeout'] + 5)  # Let dispatches started before the pause finish sending.
        checkpoint(directory, state, 'drain-requests')
        if not (directory / 'switch-stop').exists():
            try:
                wait_requests_idle(config)
            except TimeoutError:
                # The old workers are still judging, so resuming dispatch restores normal service.
                set_dispatch(config, directory, state, False)
                raise
        checkpoint(directory, state, 'stop-old-workers')
        ssm_step(directory, 'switch-stop', config, rest, stop_command)
        wait_requests_idle(config)
        checkpoint(directory, state, 'switch-digest')
        update_env(region, config['bridge'], dict(JUDGE_RUNTIME_DIGEST=digest, JUDGE_PREVIOUS_RUNTIME_DIGEST=state['previousDigest']))
        status = database_status(config)
        if status['runtimeDigest'] != digest or status['previousRuntimeDigest'] != state['previousDigest'] or not status['dispatchPaused']:
            raise ValueError('bridge digest did not switch')
        # Requests the API creates with the old digest from here on are rebound by the bridge.
        update_env(region, config['api'], dict(JUDGE_CPP_IMAGE=digest))
        tag_digest(config, [first], digest)
        checkpoint(directory, state, 'start-first', digestSwitched=True)
    start_workers(verification)
    checkpoint(directory, state, 'resume-dispatch')
    set_dispatch(config, directory, state, False)
    aws(region, 'lambda', 'invoke', '--function-name', config['bridge'], '--invocation-type', 'Event',
        '--cli-binary-format', 'raw-in-base64-out', '--payload', '{}', os.devnull)
    checkpoint(directory, state, 'sync-settings')
    sync_settings(config, directory, digest, config['runtimes'], previous=state['previousDigest'])
    checkpoint(directory, state, 'api-smoke-first')
    api_smoke(config, directory, [first])


def rolling(config, directory, state):
    """Roll an OS update through the pool while admission stays open (ADR 0014)."""
    region = config['region']
    if not config.get('pool') or config.get('retire_nodes'):
        raise ValueError('a rolling update needs an EC2 pool and no retired hosts')
    preflight(config)
    bursts = pool_bursts(config)
    first = state.setdefault('firstHost', bursts[0])
    rest = [node for node in config['nodes'] if node != first]
    if not state.get('switched'):
        deploy_bridge(config, directory, state, maintenance=False)
        contest_guard(config)
        if 'previousDigest' not in state:
            bridge = aws(region, 'lambda', 'get-function-configuration', '--function-name', config['bridge'])['Environment']['Variables']
            api = aws(region, 'lambda', 'get-function-configuration', '--function-name', config['api'])['Environment']['Variables']
            if bridge['JUDGE_RUNTIME_DIGEST'] != api.get('JUDGE_CPP_IMAGE') or \
                    sorted(api.get('JUDGE_ENABLED_RUNTIMES', '').split(',')) != sorted(config['runtimes']):
                raise ValueError('API and bridge must publish the same digest and the approved runtimes before a rolling update')
            snapshot = None if config.get('apt_snapshot') is False else time.strftime('%Y%m%dT%H%M%SZ', time.gmtime())
            checkpoint(directory, state, 'hold-bursts', previousDigest=bridge['JUDGE_RUNTIME_DIGEST'], aptSnapshot=snapshot)
        if 'alarm_actions' not in state:
            alarms = aws(region, 'cloudwatch', 'describe-alarms', '--alarm-names', *config['worker_alarms'])['MetricAlarms']
            state['alarm_actions'] = {a['AlarmName']: a['ActionsEnabled'] for a in alarms}
            save(directory / 'state.json', state)
        aws(region, 'cloudwatch', 'disable-alarm-actions', '--alarm-names', *config['worker_alarms'])
        # Burst workers start at boot with the old digest; keep every burst out of the queue from here on.
        ssm_step(directory, 'hold-bursts', config, bursts, stop_command)
        checkpoint(directory, state, 'prepare-first')
        verification = prepare_hosts(config, directory, state, [first], 'first')
        digest = json.loads((verification / 'report.json').read_text())['runtimeDigest']
        checkpoint(directory, state, 'pause-dispatch', runtimeDigest=digest)
        switch(config, directory, state, first, rest, verification)
        checkpoint(directory, state, 'prepare-rest', switched=True)
    digest = state['runtimeDigest']
    aws(region, 'ec2', 'create-tags', '--resources', *config['nodes'], '--tags', 'Key=JudgeMaintenanceHoldUntil,Value=' + hold_time(ROLLING_HOLD))
    verification = prepare_hosts(config, directory, state, rest, 'rest', expected_digest=digest)
    checkpoint(directory, state, 'start-rest')
    start_workers(verification)
    tag_digest(config, rest, digest)
    checkpoint(directory, state, 'api-smoke')
    healthy_workers(config, directory, digest)
    report = api_smoke(config, directory, config['nodes'])
    for name, enabled in state['alarm_actions'].items():
        aws(region, 'cloudwatch', 'enable-alarm-actions' if enabled else 'disable-alarm-actions', '--alarm-names', name)
    checkpoint(directory, state, 'refresh-state')
    for root in ('infra/api', 'infra/judge'):
        plan = directory / (root.replace('/', '-') + '.tfplan')
        run('terraform', '-chdir=' + root, 'plan', '-input=false', '-refresh-only', '-out=' + str(plan))
        run('terraform', '-chdir=' + root, 'apply', '-input=false', str(plan))
    released = True
    try:
        aws(region, 'ec2', 'create-tags', '--resources', *config['nodes'], '--tags', 'Key=JudgeMaintenanceHoldUntil,Value=' + hold_time(900))
    except Exception:
        released = False
    checkpoint(directory, state, 'complete', status='passed', apiReport=str(report), maintenanceHoldReleased=released)


def main():
    global LOG_DIRECTORY
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['prepare', 'finish', 'run', 'prune', 'rolling'])
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
                if key == 'release' and args.action == 'rolling' and not config.get('release'):
                    continue  # An OS-only rolling update fingerprints the installed worker.
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
            ensure_pool(config, ROLLING_HOLD if args.action == 'rolling' else 4 * 3600)
            if args.action == 'rolling':
                rolling(config, directory, state)
            elif args.action == 'prepare' or (args.action == 'run' and not state.get('prepared')):
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
                        *[option for node in config['nodes'] for option in ('--instance', node)])
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
