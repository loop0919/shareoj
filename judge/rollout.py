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
import shlex
import signal
import subprocess
import sys
import tempfile
import time
import urllib.request

from admission import publication
from verify import NODE_ID, aws, collect, new_run, save, submit

ROOT = Path(__file__).resolve().parents[1]
LOG_DIRECTORY = None


def run(*args, timeout=None):
    result = subprocess.run(args, cwd=ROOT, capture_output=True, text=True, timeout=timeout)
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


def requeue(config, digest):
    """Return dispatched, unfinished items of the bridge's previous digest to dispatch; returns how many."""
    with tempfile.NamedTemporaryFile() as file:
        result = aws(config['region'], 'lambda', 'invoke', '--function-name', config['bridge'], '--cli-binary-format', 'raw-in-base64-out',
                     '--payload', json.dumps(dict(operation='requeue-digest', digest=digest)), file.name)
        response = json.loads(Path(file.name).read_text())
    # The bridge accepts only its previous digest and changes nothing when it refuses.
    if result.get('FunctionError') or not isinstance(response, dict) or response.get('kind') != 'judge-requeue' or \
            type(response.get('requeued')) is not int or response['requeued'] < 0:
        raise RuntimeError('bridge did not requeue the previous digest')
    return response['requeued']


def all_queues(config):
    """Every request, result and dead letter queue, including the second request lane when configured."""
    return config['queues'] + ([config['request_queue_b']] if config.get('request_queue_b') else [])


def lanes(config):
    """Resolve the request lanes from the queue the live bridge sends to (the single source of truth)."""
    bridge = aws(config['region'], 'lambda', 'get-function-configuration', '--function-name', config['bridge'])
    url = bridge['Environment']['Variables']['JUDGE_REQUEST_QUEUE_URL']
    first, second = config['queues'][0], config.get('request_queue_b')
    if url == first:
        return dict(active='a', active_url=first, standby_url=second)
    if second and url == second:
        return dict(active='b', active_url=second, standby_url=first)
    raise ValueError('bridge request queue is not a configured lane')


def live_lane(config):
    """The request queue the live bridge sends to, or None for a configuration with one request queue."""
    return lanes(config)['active_url'] if config.get('request_queue_b') else None


def queue_counts(config, url):
    """Visible, in-flight and delayed messages of one queue, summed."""
    attributes = aws(config['region'], 'sqs', 'get-queue-attributes', '--queue-url', url,
                     '--attribute-names', 'ApproximateNumberOfMessages', 'ApproximateNumberOfMessagesNotVisible',
                     'ApproximateNumberOfMessagesDelayed')['Attributes']
    return sum(int(attributes[key]) for key in ('ApproximateNumberOfMessages',
               'ApproximateNumberOfMessagesNotVisible', 'ApproximateNumberOfMessagesDelayed'))


def sync_lane(config, lane):
    """Point Terraform's request_lane at the lane in use, keeping every other rollout-owned value."""
    path = ROOT / 'infra/judge/zz-rollout.auto.tfvars.json'
    values = json.loads(path.read_text() or '{}') if path.exists() else {}
    save(path, {**values, 'request_lane': lane})


def wait_empty(config, timeout=3600):
    deadline, stable = time.monotonic() + timeout, 0
    while time.monotonic() < deadline:
        status = database_status(config)
        counts = [queue_counts(config, url) for url in all_queues(config)]
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
    published = config.get('publish_runtimes', config['runtimes'])
    if not isinstance(published, list) or any(not isinstance(name, str) or not re.fullmatch('[a-z][a-z0-9-]*', name) for name in published) or \
            len(set(published)) != len(published) or not set(config['runtimes']) <= set(published):
        raise ValueError('rolling publication may only add distinct runtime IDs')
    if aws(region, 'sts', 'get-caller-identity')['Account'] != config['account']:
        raise ValueError('wrong AWS account')
    if not config['nodes'] or len(set(config['nodes'])) != len(config['nodes']) or len(set(config['queues'])) != 4 or any(
            not NODE_ID.fullmatch(node) for node in config['nodes']):
        raise ValueError('configure distinct hosts and request/result/dead queue URLs')
    second = config.get('request_queue_b')
    if second is not None and (not isinstance(second, str) or second in config['queues']):
        raise ValueError('request_queue_b must be a fifth queue distinct from the four queues')
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
    lane = lanes(config)
    if bridge['Environment']['Variables']['JUDGE_JOB_BUCKET'] != config['bucket']:
        raise ValueError('request queue or release bucket differs from bridge configuration')
    queues = [aws(region, 'sqs', 'get-queue-attributes', '--queue-url', url,
                  '--attribute-names', 'QueueArn', 'RedrivePolicy')['Attributes'] for url in config['queues']]
    if mapping['EventSourceArn'] != queues[1]['QueueArn'] or any(
            json.loads(queues[i]['RedrivePolicy'])['deadLetterTargetArn'] != queues[i + 2]['QueueArn'] for i in (0, 1)):
        raise ValueError('result queue or dead letter queue does not match')
    if second is not None:
        standby = aws(region, 'sqs', 'get-queue-attributes', '--queue-url', second,
                      '--attribute-names', 'RedrivePolicy')['Attributes']
        if json.loads(standby['RedrivePolicy'])['deadLetterTargetArn'] != queues[2]['QueueArn']:
            raise ValueError('request queue b does not redrive to the request dead letter queue')
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
    if second is not None:
        # Terraform's precondition compares request_lane with the live bridge; a stale file would block a resumed run.
        sync_lane(config, lane['active'])
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


def healthy_workers(config, directory, digest, queue=None):
    """Confirm every host runs `digest` and is ready; with `queue`, also that each worker reads that request queue."""
    health_dir = directory / ('health-' + str(time.time_ns()))
    receipt = new_run(health_dir, config['region'], config['nodes'], 'install')
    command = '''set -eu
systemctl is-active --quiet judge-worker
PYTHONPATH=/opt/judge python3 - ''' + digest + ('' if queue is None else ' ' + shlex.quote(queue)) + ''' <<'PY'
import json, subprocess, sys
from pathlib import Path
from host import verify_assets
digest = sys.argv[1]
if verify_assets() != digest: raise SystemExit('installed digest differs')
pid = subprocess.check_output(['systemctl', 'show', 'judge-worker', '-p', 'MainPID', '--value'], text=True).strip()
env = Path('/proc/' + pid + '/environ').read_bytes().split(b'\\0')
if ('JUDGE_RUNTIME_DIGEST=' + digest).encode() not in env: raise SystemExit('running process digest differs')
# A worker on the lane the bridge does not send to is running but takes no submission.
if sys.argv[2:] and ('JUDGE_REQUEST_QUEUE_URL=' + sys.argv[2]).encode() not in env: raise SystemExit('worker reads another request queue')
invocation = subprocess.check_output(['systemctl', 'show', 'judge-worker', '-p', 'InvocationID', '--value'], text=True).strip()
if not invocation: raise SystemExit('worker invocation missing')
log = subprocess.check_output(['journalctl', '_SYSTEMD_INVOCATION_ID=' + invocation, '-o', 'cat', '--no-pager'], text=True)
events = []
for line in log.splitlines():
    try: events.append(json.loads(line))
    except ValueError: pass
if not any(e.get('event') == 'worker_started' for e in events): raise SystemExit('worker is not ready')
# Judges keep running during a rolling update, so a problem author's failing checker is no worker fault.
if any(e.get('event') == 'failure' and e.get('category') != 'judge_code' for e in events): raise SystemExit('worker reported failures')
subprocess.run(['systemctl', 'is-active', '--quiet', 'judge-worker'], check=True)
print('worker ready')
PY
'''
    for node in config['nodes']:
        submit(health_dir, receipt, node, command)
    if collect(health_dir, receipt, wait=True) != 0:
        raise RuntimeError('workers are not ready')


# Outlast one 20-second long poll, so a failing receive (such as a missing permission) is in the log.
STANDBY_WAIT = 35
# A failing worker logs a receive failure and retries about every 5 seconds, so one failing now has a failure
# within this window, which is longer than the wait. Failures from before a fix stay in the same invocation's
# journal and must not block a resumed run. The host's own clock bounds the window, so controller drift cannot hide one.
STANDBY_WINDOW = 60


def lane_worker_ready(config, directory, nodes, url, digest=None, label='lane'):
    """Confirm each node's running worker reads `url` (and runs `digest`, when given) and is not failing to receive.

    Every switch and every return of capacity relies on this: a worker that is active but reads the other lane, or
    runs another digest, judges nothing the bridge sends. Raises RuntimeError naming the receipt to inspect.
    """
    if digest is not None and not re.fullmatch('sha256:[a-f0-9]{64}', digest):
        raise ValueError('invalid runtime digest')
    check_dir = directory / (label + '-' + str(time.time_ns()))
    receipt = new_run(check_dir, config['region'], nodes, 'install')
    command = '''set -eu
systemctl is-active --quiet judge-worker
python3 - ''' + shlex.quote(url) + ' ' + str(STANDBY_WINDOW) + ('' if digest is None else ' ' + digest) + ''' <<'PY'
import json, subprocess, sys, time
from datetime import datetime
from pathlib import Path
url, since = sys.argv[1], time.time() - int(sys.argv[2])
def recent(event):
    # Telemetry stamps every record with an aware UTC ISO time; a record without a readable one counts as recent.
    try: return datetime.fromisoformat(event['timestamp']).timestamp() >= since
    except (KeyError, TypeError, ValueError): return True
pid = subprocess.check_output(['systemctl', 'show', 'judge-worker', '-p', 'MainPID', '--value'], text=True).strip()
env = Path('/proc/' + pid + '/environ').read_bytes().split(b'\\0')
# Old and new workers must never read the same queue.
if ('JUDGE_REQUEST_QUEUE_URL=' + url).encode() not in env: raise SystemExit('worker reads another request queue')
if sys.argv[3:] and ('JUDGE_RUNTIME_DIGEST=' + sys.argv[3]).encode() not in env: raise SystemExit('worker runs another runtime digest')
invocation = subprocess.check_output(['systemctl', 'show', 'judge-worker', '-p', 'InvocationID', '--value'], text=True).strip()
if not invocation: raise SystemExit('worker invocation missing')
log = subprocess.check_output(['journalctl', '_SYSTEMD_INVOCATION_ID=' + invocation, '-o', 'cat', '--no-pager'], text=True)
events = []
for line in log.splitlines():
    try: events.append(json.loads(line))
    except ValueError: pass
if not any(e.get('event') == 'worker_started' for e in events): raise SystemExit('worker is not ready')
# A receive failure is logged and retried, so the worker stays active while it cannot take any job.
if any(e.get('reason') == 'queue_receive_failed' and recent(e) for e in events):
    raise SystemExit('worker cannot receive from its lane (failed within ' + sys.argv[2] + ' seconds)')
subprocess.run(['systemctl', 'is-active', '--quiet', 'judge-worker'], check=True)
print('lane worker ready')
PY
'''
    for node in nodes:
        submit(check_dir, receipt, node, command)
    try:
        finished = collect(check_dir, receipt, wait=True) == 0
    except RuntimeError:
        finished = False
    if not finished:
        raise RuntimeError('a worker is not ready on ' + url + '; inspect ' + str(check_dir / 'receipt.json'))


def standby_ready(config, directory, node, url):
    """Confirm the standby worker still runs on the standby lane and is not failing to receive from it."""
    try:
        lane_worker_ready(config, directory, [node], url, label='standby')
    except RuntimeError as error:
        raise RuntimeError('the standby worker is not ready; the bridge was not switched (' + str(error) + ')') from None


def retired_workers_stopped(config, directory):
    if not config.get('retire_nodes'):
        return
    check_dir = directory / ('retired-' + str(time.time_ns()))
    receipt = new_run(check_dir, config['region'], config['retire_nodes'], 'install')
    for node in config['retire_nodes']:
        submit(check_dir, receipt, node, '! systemctl is-active --quiet judge-worker && ! systemctl is-enabled --quiet judge-worker')
    if collect(check_dir, receipt, wait=True) != 0:
        raise RuntimeError('a retired worker is still enabled')


def sync_settings(config, directory, digest, runtimes, previous=None, lane=None):
    for name, value in {'JUDGE_RUNTIME_DIGEST': digest, 'JUDGE_ENABLED_RUNTIMES': json.dumps(runtimes)}.items():
        run('gh', 'variable', 'set', name, '--repo', config['repository'], '--env', config['github_environment'], '--body', value)
    saved = json.loads(run('gh', 'variable', 'list', '--repo', config['repository'], '--env', config['github_environment'], '--json', 'name,value'))
    values = {v['name']: v['value'] for v in saved}
    if values.get('JUDGE_RUNTIME_DIGEST') != digest or json.loads(values.get('JUDGE_ENABLED_RUNTIMES', 'null')) != runtimes:
        raise ValueError('GitHub deployment variables did not match')
    # Terraform owns the fleet shape; keep values such as the host count that rollout does not manage.
    for root, values in [('infra/api', dict(judge_runtime_digest=digest, judge_enabled_runtimes=runtimes)),
                         ('infra/judge', dict(runtime_digest=digest, enabled=True, enabled_runtimes=runtimes,
                                              **({} if previous is None else dict(previous_runtime_digest=previous)),
                                              **({} if lane is None else dict(request_lane=lane))))]:
        path = ROOT / root / 'zz-rollout.auto.tfvars.json'
        backup = directory / (root.replace('/', '-') + '-variables.json')
        if not backup.exists():
            save(backup, dict(existed=path.exists(), content=path.read_text() if path.exists() else ''))
        # Merge over the current file, not the backup: sync_lane may have changed request_lane since the backup.
        save(path, {**(json.loads(path.read_text() or '{}') if path.exists() else {}), **values})


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
    healthy_workers(config, directory, digest, live_lane(config))
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
    api_report = api_smoke(config, directory, config['nodes'])
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
PACKAGE_LIST = "dpkg-query -W -f='${Package}=${Version}\\n'"


def os_update_command(run_id, expected_packages=None):
    """Upgrade all packages, reboot through SSM (exit 194), then confirm the new kernel.

    snapshot.ubuntu.com has no IPv6 address, so the IPv6-only hosts cannot pin an archive snapshot.
    Hosts updated after the first must end with the same package list, checked before they reboot.
    """
    if expected_packages is not None and not re.fullmatch('[a-f0-9]{64}', expected_packages):
        raise ValueError('invalid package list digest')
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
  # The first attempt on 2026-10-10 left a pin to the unreachable snapshot server.
  rm -f /etc/apt/apt.conf.d/99judge-snapshot
  export DEBIAN_FRONTEND=noninteractive NEEDRESTART_SUSPEND=1
  # SSM keeps only the first 24,000 characters of output; the last line must report the package list.
  # Without --error-on=any, apt-get update only warns on failed fetches and the upgrade sees stale indexes.
  # Phased updates depend on the machine ID; every host must select the same versions.
  if ! { apt-get update --error-on=any && apt-get -y -o APT::Get::Never-Include-Phased-Updates=true \\
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
    """Print the fingerprinted digest, failing if it differs from the switched one."""
    if expected_digest is not None and not re.fullmatch('sha256:[a-f0-9]{64}', expected_digest):
        raise ValueError('invalid runtime digest')
    return '''set -eu
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


def contest_guard(config, window=True):
    """Refuse to change the environment within or shortly before a contest window (ADR 0014).

    With window=False only the bridge's fields are checked: a run that may already have switched must be able to
    finish near a contest, because stopping there leaves every burst held and the old workers on the old lane.
    """
    status = database_status(config)
    if 'nextContestWindowAt' not in status or 'dispatchPaused' not in status or \
            type(status.get('requestQueueUrl')) is not str or type(status.get('previousInFlight')) is not int:
        raise ValueError('bridge must support the rolling update; deploy the compatible bridge package first')
    starts = status['nextContestWindowAt']
    hours = config.get('rolling_guard_hours', 4)
    if window and starts is not None and datetime.datetime.fromisoformat(starts).timestamp() < time.time() + hours * 3600:
        raise ValueError('a contest window starts at ' + starts + '; run the rolling update outside contests')
    return status


def bridge_switched(config, state):
    """Whether the live bridge already sends the new digest to the new lane; False while it is unchanged."""
    status = database_status(config)
    live = status.get('requestQueueUrl'), status.get('runtimeDigest'), status.get('previousRuntimeDigest')
    if live == (state['activeQueue'], state['runtimeDigest'], state['previousDigest']):
        return True
    if live[:2] == (state['previousQueue'], state['previousDigest']):
        return False
    raise ValueError('bridge is on neither the previous nor the new lane; inspect deployment-status before resuming')


def drain_previous(config, directory, state, url, timeout=2700):
    """Wait until the previous lane is empty and no previous-digest work is left, re-sending stranded work once."""
    state.setdefault('requeued', 0)
    deadline, stable, empty, requeued = time.monotonic() + timeout, 0, 0, False
    while time.monotonic() < deadline:
        count = queue_counts(config, url)
        in_flight = database_status(config)['previousInFlight']
        stable = stable + 1 if count == 0 and in_flight == 0 else 0
        if stable == 3:
            return
        empty = empty + 1 if count == 0 else 0
        # With the lane empty, previous-digest work left means its message went to the DLQ or an old worker
        # died mid-job; no old worker will finish it, so the new environment must. The DLQ message stays for the operator.
        if empty >= 3 and in_flight > 0 and not requeued:
            state['requeued'] += requeue(config, state['previousDigest'])
            save(directory / 'state.json', state)
            requeued = True
        time.sleep(10)
    raise TimeoutError('the previous lane did not drain; old workers keep running')


def set_dispatch(config, directory, state, paused):
    # Kept for manual operator use; rolling never pauses dispatch.
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
                       lambda run_id: os_update_command(run_id, None if first else state['packages']))
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


def start_workers(verification, request_queue=None):
    receipt = json.loads((verification / 'receipt.json').read_text())
    if receipt['phase'] == 'smoke':
        run(sys.executable, 'judge/verify.py', 'start', '--run-dir', str(verification),
            *(() if request_queue is None else ('--request-queue', request_queue)))
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


# The switch latency watcher (judge/smoke-api.py --watch): the baseline it must collect before the switch, how long it
# keeps submitting after the API switch, and how long it may take to finish and clean up once stopped.
WATCH_BASELINE_SAMPLES = 6
WATCH_BASELINE_TIMEOUT = 180
WATCH_TAIL = 60
WATCH_STOP_WAIT = 180
WATCH_ABORT_WAIT = 120
WATCH_CLEANUP_TIMEOUT = 120
WATCH_SCRIPT = 'judge/smoke-api.py'
WATCH_FAILED = 'switch latency check failed; see switch-watch.json'


def watch_failed_message(state):
    """The failure with its cause: latency, a watcher that did not start or died, or test users left behind."""
    detail = {key: state[key] for key in ('switchWatch', 'switchWatchLeftovers') if key in state}
    return WATCH_FAILED + ' (' + json.dumps(detail, sort_keys=True) + ')'


def watch_paths(directory):
    return directory / 'switch-watch.json', directory / 'switch-watch.stop', directory / 'switch-at'


def read_watch_report(path):
    """The watcher's report only if it saw the stop file and finished normally; a terminated, capped or crashed
    watcher (or one still running) leaves a report that says nothing about the whole switch."""
    try:
        report = json.loads(path.read_text())
    except (OSError, ValueError):
        return None
    if report.get('status') not in ('passed', 'failed') or report.get('terminated') or report.get('capped') \
            or report.get('stopped') is not True:
        return None
    return report


def watch_command(config, directory):
    """The watcher's argv. The cadence is left to the watcher's own default, which the user-count assertion in
    smoke-api.py is written against, so rollout cannot drift from it."""
    report, stop, switch_at = watch_paths(directory)
    return [sys.executable, WATCH_SCRIPT, '--watch', '--function', config['api'], '--api-url', config['api_url'],
            '--region', config['region'], '--runtime', config['smoke_runtime'],
            '--stop-file', str(stop), '--switch-at-file', str(switch_at), '--report', str(report)]


def watcher_alive(pid, report):
    """Whether `pid` is still this run's watcher (the pid may have been reused by something else): its argv holds the
    script (as start_watch passes it, however the path is spelled), --watch and this run's report path."""
    try:
        command = Path('/proc/%d/cmdline' % pid).read_bytes().split(b'\0')
    except OSError:
        return False
    return any(Path(part.decode(errors='replace')).name == Path(WATCH_SCRIPT).name for part in command) \
        and b'--watch' in command and str(report).encode() in command


def leftover_users(report):
    try:
        return json.loads(report.with_suffix('.cleanup.json').read_text())['users']
    except (OSError, ValueError, KeyError):
        return []


def clean_leftovers(config, directory):
    """Remove test users and problems a watcher left behind; True once none remain."""
    report = watch_paths(directory)[0]
    if not leftover_users(report):
        return True
    try:
        # A timeout keeps an API or Cognito outage from holding the switch; what remains is recorded as leftovers.
        run(sys.executable, WATCH_SCRIPT, '--cleanup', '--function', config['api'], '--api-url', config['api_url'],
            '--region', config['region'], '--report', str(report), timeout=WATCH_CLEANUP_TIMEOUT)
    except Exception as error:
        print('watch cleanup incomplete:', error, flush=True)
    return not leftover_users(report)


def signal_watcher(pid, report, number):
    try:
        if watcher_alive(pid, report):
            os.kill(pid, number)
    except (ProcessLookupError, PermissionError):
        pass


def collect_stale_watch(config, directory, state):
    """End a watcher left by an earlier process (SIGTERM, so it cleans up), then return its report if it had finished."""
    report_path = watch_paths(directory)[0]
    old = state.pop('switchWatcher', None)
    if old:
        signal_watcher(old['pid'], report_path, signal.SIGTERM)
        for _ in range(90):
            if not watcher_alive(old['pid'], report_path):
                break
            time.sleep(1)
        else:
            signal_watcher(old['pid'], report_path, signal.SIGKILL)
    clean_leftovers(config, directory)
    return read_watch_report(report_path)


def start_watch(config, directory, state):
    """Start the latency watcher as a child process. It stops itself after its hard cap even if nobody stops it."""
    collect_stale_watch(config, directory, state)
    report, stop, switch_at = watch_paths(directory)
    # Never forget test users: a new watcher starts only when the earlier one left none.
    if leftover_users(report):
        raise RuntimeError('an earlier watch left test users; run smoke-api.py --cleanup with ' + str(report))
    for path in (report, stop, switch_at, report.with_suffix('.cleanup.json'), report.with_suffix('.progress.json')):
        path.unlink(missing_ok=True)
    with (directory / 'switch-watch.log').open('ab') as log:
        # Its own session: Ctrl-C or a closed terminal reaches the rollout only, which then stops the watcher itself.
        process = subprocess.Popen(watch_command(config, directory), cwd=ROOT, stdout=log, stderr=subprocess.STDOUT,
                                   start_new_session=True)
    state['switchWatcher'] = dict(pid=process.pid, stopFile=str(stop), report=str(report))
    save(directory / 'state.json', state)
    return process


def wait_for_baseline(process, directory):
    """Wait until the watcher has seen enough submissions start (or exited, or the timeout); the switch goes on either way."""
    progress = watch_paths(directory)[0].with_suffix('.progress.json')
    deadline = time.monotonic() + WATCH_BASELINE_TIMEOUT
    while process.poll() is None and time.monotonic() < deadline:
        try:
            if json.loads(progress.read_text())['started'] >= WATCH_BASELINE_SAMPLES:
                return
        except (OSError, ValueError, KeyError):
            pass
        time.sleep(2)


def stop_watch(process, directory):
    """Ask the watcher to finish, wait for its report and cleanup, and return the report (None if it never finished)."""
    report, stop, _ = watch_paths(directory)
    stop.touch()
    try:
        process.wait(WATCH_STOP_WAIT)
    except subprocess.TimeoutExpired:
        process.terminate()
        try:
            process.wait(60)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()
    return read_watch_report(report)


def abort_watch(config, directory, state, process):
    """End a watcher because the switch failed or was interrupted. SIGTERM makes it clean up; this never raises."""
    if process is None:
        return
    try:
        if process.poll() is None:
            process.terminate()
            try:
                process.wait(WATCH_ABORT_WAIT)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
        state.pop('switchWatcher', None)
        if not clean_leftovers(config, directory):
            state['switchWatchLeftovers'] = True
            save(directory / 'state.json', state)
    except BaseException as error:
        print('could not stop the switch watch:', error, flush=True)


def settle_watch(config, directory, state, process):
    """Record how the watch ended. It never changes what the switch does, and never raises into it; a bad watch only
    fails the run at the end."""
    if 'switchWatch' in state:
        # Already decided (for example not-started because test users were left): retry removing them, and name them.
        try:
            if not clean_leftovers(config, directory):
                state['switchWatchLeftovers'] = True
                state['switchWatchFailed'] = True
                save(directory / 'state.json', state)
        except Exception as error:
            print('watch cleanup failed:', error, flush=True)
        return
    try:
        if process is not None:
            time.sleep(WATCH_TAIL)
            report, outcome = stop_watch(process, directory), 'died'
        else:
            # The run was resumed after the switch: a watcher from the dead run is ended, and only a finished report counts.
            report, outcome = collect_stale_watch(config, directory, state), 'interrupted'
    except Exception as error:
        print('switch watch could not be settled:', error, flush=True)
        report, outcome = None, 'died' if process is not None else 'interrupted'
    state.pop('switchWatcher', None)
    if report is None:
        state['switchWatch'] = outcome
        failed = outcome == 'died'
    else:
        state['switchWatch'] = dict(report=str(watch_paths(directory)[0]), status=report['status'], maxWaitMs=report.get('maxWaitMs'))
        failed = report['status'] != 'passed'
    # Test users left behind are removed now; if some remain, say so rather than blaming latency.
    try:
        if not clean_leftovers(config, directory):
            state['switchWatchLeftovers'] = True
            failed = True
    except Exception as error:
        print('watch cleanup failed:', error, flush=True)
    if failed:
        state['switchWatchFailed'] = True
    else:
        state.pop('switchWatchFailed', None)
    try:
        save(directory / 'state.json', state)
    except Exception as error:
        print('could not save the watch result:', error, flush=True)


class NothingToSwitch(Exception):
    """The prepared runtime digest equals the published one, so the bridge has nothing to switch to."""


UNCHANGED = ('nothing to roll out: the prepared runtime digest equals the published one; the burst workers, '
             'the alarm actions and the maintenance hold were put back (status: unchanged)')
# Undoes hold-bursts on a host that was only stopped: its worker.env still names the lane and digest it judged with.
ENABLE_WORKER = 'set -eu\nsystemctl enable --now judge-worker.service\n'


def restore_alarms(config, state):
    """Put the worker alarm actions back as they were before the run disabled them."""
    for name, enabled in state['alarm_actions'].items():
        aws(config['region'], 'cloudwatch', 'enable-alarm-actions' if enabled else 'disable-alarm-actions', '--alarm-names', name)


def release_hold(config):
    """Shorten the maintenance hold to 15 minutes; False if the tag call failed. A burst idle-stops only past it."""
    try:
        aws(config['region'], 'ec2', 'create-tags', '--resources', *config['nodes'], '--tags',
            'Key=JudgeMaintenanceHoldUntil,Value=' + hold_time(900))
    except Exception:
        return False
    return True


def finish_unchanged(config, directory, state, bursts, verification):
    """Nothing to switch: put the pool back as the run found it, then end the run as unchanged.

    Left as they were, held bursts never power off (idle stop runs in the worker) and give no contest capacity, and
    the alarm actions stay off. An equal digest means the first burst's kernel, packages and runtime files match the
    published environment, so it may judge again. It starts like any prepared host, on the lane in use, which also
    corrects a worker.env that a revert left on the other lane. The other bursts were only stopped.
    """
    first, digest, lane = state['firstHost'], state['runtimeDigest'], state['previousQueue']
    if not state.get('nothingToSwitch'):
        # From here a resumed run only finishes this restore; it never holds the bursts or disables the alarms again.
        checkpoint(directory, state, 'restore-unchanged', nothingToSwitch=True)
    start_workers(verification, request_queue=lane)
    tag_digest(config, [first], digest)
    others = [node for node in bursts if node != first]
    if others:
        ssm_step(directory, 'release-bursts', config, others, ENABLE_WORKER)
    restore_alarms(config, state)
    lane_worker_ready(config, directory, bursts, lane, digest, label='bursts')
    checkpoint(directory, state, 'unchanged', status='unchanged', maintenanceHoldReleased=release_hold(config))
    print(UNCHANGED, flush=True)


def switch(config, directory, state, first, rest, verification):
    """Switch the lane; if anything stops it before the watch is settled, the watcher is stopped too."""
    handle = [None]
    try:
        switch_steps(config, directory, state, first, rest, verification, handle)
    except BaseException:
        abort_watch(config, directory, state, handle[0])
        raise


def switch_steps(config, directory, state, first, rest, verification, handle):
    """Start the prepared burst on the standby lane, move the bridge to it, then drain and stop the previous lane.

    Dispatch and admission stay open; old and new workers never read the same queue. Each step runs once:
    a resumed run continues after the last recorded one and never switches the bridge twice.
    """
    region, digest, previous = config['region'], state['runtimeDigest'], state['previousDigest']
    if not state.get('digestSwitched'):
        checkpoint(directory, state, 'standby-first')
        # A run stopped right after the bridge update finds the bridge already switched, and the new lane busy.
        if not bridge_switched(config, state):
            # With an unchanged digest the bridge would get previous == current: previousInFlight would count the
            # new lane's work too, and the drain's requeue would restart submissions the new worker is judging.
            if digest == previous:
                raise NothingToSwitch()
            # Work left in the standby lane would reach the new environment before the switch is decided.
            if queue_counts(config, state['activeQueue']) != 0:
                raise ValueError('the standby lane is not empty')
            start_workers(verification, request_queue=state['activeQueue'])
            time.sleep(STANDBY_WAIT)
            standby_ready(config, directory, first, state['activeQueue'])
            # The watch only measures; a watcher that cannot start fails the watch, never the switch.
            state.pop('switchWatch', None)
            state.pop('switchWatchFailed', None)
            try:
                handle[0] = start_watch(config, directory, state)
            except Exception as error:
                state.update(switchWatch='not-started', switchWatchFailed=True)
                print('switch watch did not start:', error, flush=True)
            else:
                # Baseline submissions from the old environment, before anything changes.
                wait_for_baseline(handle[0], directory)
            # Everything above leaves users on the old environment; the bridge update below is the first visible change.
            checkpoint(directory, state, 'switch-lane')
            contest_guard(config)
            # Saved before the update is sent. A run that stops after the update has switched without recording it,
            # and only a run with this mark makes rolling-revert ask the bridge whether that happened.
            state['laneSwitchAttempted'] = True
            # The watcher compares submissions made before and after this wall-clock moment.
            state['switchAt'] = time.time()
            save(watch_paths(directory)[2], state['switchAt'])
            save(directory / 'state.json', state)
            # One update moves the queue, the digest and the previous digest together. The bridge then never sends
            # new-digest work to the old lane, and rebinds undispatched old-digest submissions to the new digest.
            update_env(region, config['bridge'], dict(JUDGE_REQUEST_QUEUE_URL=state['activeQueue'], JUDGE_RUNTIME_DIGEST=digest,
                                                      JUDGE_PREVIOUS_RUNTIME_DIGEST=previous))
            if not bridge_switched(config, state):
                raise ValueError('bridge did not switch to the new lane')
        state['digestSwitched'] = True
        save(directory / 'state.json', state)
    if not state.get('apiSwitched'):
        checkpoint(directory, state, 'switch-api')
        bridge = aws(region, 'lambda', 'get-function-configuration', '--function-name', config['bridge'])
        # A bridge invocation still running with the old configuration would mark a new-digest submission as JE.
        # Meanwhile the API creates old-digest submissions, which the switched bridge rebinds, so nobody waits.
        time.sleep(bridge['Timeout'] + 5)
        update_env(region, config['api'], dict(JUDGE_CPP_IMAGE=digest))
        tag_digest(config, [first], digest)
        state['apiSwitched'] = True
        save(directory / 'state.json', state)
    settle_watch(config, directory, state, handle[0])
    handle[0] = None
    if 'firstApiReport' not in state:
        checkpoint(directory, state, 'sync-settings')
        sync_settings(config, directory, digest, config['runtimes'], previous=previous, lane=state['activeLane'])
        checkpoint(directory, state, 'api-smoke-first')
        state['firstApiReport'] = str(api_smoke(config, directory, [first]))
    if not state.get('oldWorkersStopped'):
        checkpoint(directory, state, 'drain-previous')
        # Old workers stop only once nothing is left for them; users are judged on the new lane meanwhile.
        # The stop exists only after a passed drain, so a resumed stop does not wait for another one.
        if not (directory / 'switch-stop').exists():
            drain_previous(config, directory, state, state['previousQueue'])
        checkpoint(directory, state, 'stop-old-workers')
        ssm_step(directory, 'switch-stop', config, rest, stop_command)
        state['oldWorkersStopped'] = True
        save(directory / 'state.json', state)


def publish_rolling_runtimes(config, directory, state):
    """Add languages only after both rolling batches have passed smoke and started."""
    requested = config.get('publish_runtimes', config['runtimes'])
    if requested == config['runtimes']:
        return config
    hosts = set()
    for label in ('first', 'rest'):
        report = json.loads((directory / ('verify-' + label) / 'report.json').read_text())
        digest, published = publication(report, ','.join(requested))
        if digest != state['runtimeDigest'] or hosts & set(report['hosts']):
            raise ValueError('rolling publication reports disagree')
        hosts.update(report['hosts'])
    if hosts != set(config['nodes']):
        raise ValueError('rolling publication is missing verified hosts')
    checkpoint(directory, state, 'publish-runtimes')
    # The bridge must understand the added languages before the API accepts them.
    update_env(config['region'], config['bridge'], dict(JUDGE_ENABLED_RUNTIMES=published))
    update_env(config['region'], config['api'], dict(JUDGE_ENABLED_RUNTIMES=published))
    sync_settings(config, directory, digest, requested, previous=state['previousDigest'])
    return dict(config, runtimes=requested)


def refuse_reverted(state):
    """Refuse a run that was reverted, or is being reverted: it must never drain or stop the workers users are back on."""
    if state.get('status') == 'reverted':
        raise ValueError('this run was reverted; start a new run')
    if state.get('revertStarted'):
        raise ValueError('this run is being reverted; finish it with rolling-revert')


def check_rolling_config(config):
    """Refuse a configuration rolling cannot use. main checks it before starting or holding any host."""
    if not config.get('pool') or config.get('retire_nodes'):
        raise ValueError('a rolling update needs an EC2 pool and no retired hosts')
    # The new environment waits on the second lane; with one queue old and new workers would share it.
    if not config.get('request_queue_b'):
        raise ValueError('a rolling update needs request_queue_b')


def rolling(config, directory, state):
    """Roll an OS update through the pool while admission and dispatch stay open (ADR 0014, two request lanes)."""
    refuse_reverted(state)
    if state.get('step') == 'switch-watch-failed':
        raise ValueError(watch_failed_message(state))
    if state.get('status') == 'unchanged':
        print(UNCHANGED, flush=True)
        return
    region = config['region']
    check_rolling_config(config)
    preflight(config)
    bursts = pool_bursts(config)
    first = state.setdefault('firstHost', bursts[0])
    rest = [node for node in config['nodes'] if node != first]
    if state.get('nothingToSwitch'):
        finish_unchanged(config, directory, state, bursts, directory / 'verify-first')
        return
    if not state.get('switched'):
        deploy_bridge(config, directory, state, maintenance=False)
        # Once the bridge update may have been sent, only the bridge's fields are checked here. A run that switched
        # must finish near a contest too; switch_steps checks the window again wherever the bridge has not switched.
        contest_guard(config, window=not (state.get('laneSwitchAttempted') or state.get('digestSwitched')))
        if 'previousDigest' not in state:
            bridge = aws(region, 'lambda', 'get-function-configuration', '--function-name', config['bridge'])['Environment']['Variables']
            api = aws(region, 'lambda', 'get-function-configuration', '--function-name', config['api'])['Environment']['Variables']
            if bridge['JUDGE_RUNTIME_DIGEST'] != api.get('JUDGE_CPP_IMAGE') or \
                    sorted(api.get('JUDGE_ENABLED_RUNTIMES', '').split(',')) != sorted(config['runtimes']):
                raise ValueError('API and bridge must publish the same digest and the approved runtimes before a rolling update')
            checkpoint(directory, state, 'hold-bursts', previousDigest=bridge['JUDGE_RUNTIME_DIGEST'])
        if 'activeQueue' not in state:
            # Decided once from the live bridge: after the switch it points at the new lane, so a resumed run
            # uses these. "previous" is the lane in use before the switch, "active" the standby lane in use after it.
            lane = lanes(config)
            # Refuse before preparing anything; the standby check repeats right before the new worker starts.
            if queue_counts(config, lane['standby_url']) != 0:
                raise ValueError('the standby lane is not empty')
            # The primaries judge alone while the bursts are held, so each must judge on the live lane with the
            # published digest. One that does not is usually an earlier run stopped between its switch and its
            # old-worker stop: holding the bursts would stop that run's only new worker, and this run's new worker
            # would share the old primary's lane. Refused here, before the alarms or any burst change.
            primaries = [node for node in config['nodes'] if node not in bursts]
            if primaries:
                try:
                    lane_worker_ready(config, directory, primaries, lane['active_url'], state['previousDigest'], label='primaries')
                except RuntimeError as error:
                    raise ValueError('an earlier rolling has not finished, or a primary is not judging: every primary must '
                                     'judge on the live lane with the published digest; resume or rolling-revert an '
                                     'unfinished run (' + str(error) + ')') from None
            state.update(previousLane=lane['active'], previousQueue=lane['active_url'],
                         activeLane='b' if lane['active'] == 'a' else 'a', activeQueue=lane['standby_url'])
            save(directory / 'state.json', state)
        if 'alarm_actions' not in state:
            alarms = aws(region, 'cloudwatch', 'describe-alarms', '--alarm-names', *config['worker_alarms'])['MetricAlarms']
            state['alarm_actions'] = {a['AlarmName']: a['ActionsEnabled'] for a in alarms}
            save(directory / 'state.json', state)
        aws(region, 'cloudwatch', 'disable-alarm-actions', '--alarm-names', *config['worker_alarms'])
        # Burst workers start at boot with the old digest; keep every burst out of the queue from here on.
        ssm_step(directory, 'hold-bursts', config, bursts, stop_command)
        checkpoint(directory, state, 'prepare-first')
        verification = prepare_hosts(config, directory, state, [first], 'first')
        if not state.get('digestSwitched'):
            # Once switched, the saved digest is the one the bridge publishes.
            state['runtimeDigest'] = json.loads((verification / 'report.json').read_text())['runtimeDigest']
        try:
            switch(config, directory, state, first, rest, verification)
        except NothingToSwitch:
            finish_unchanged(config, directory, state, bursts, verification)
            return
        checkpoint(directory, state, 'prepare-rest', switched=True)
    digest = state['runtimeDigest']
    aws(region, 'ec2', 'create-tags', '--resources', *config['nodes'], '--tags', 'Key=JudgeMaintenanceHoldUntil,Value=' + hold_time(ROLLING_HOLD))
    verification = prepare_hosts(config, directory, state, rest, 'rest', expected_digest=digest)
    checkpoint(directory, state, 'start-rest')
    start_workers(verification, request_queue=state['activeQueue'])
    tag_digest(config, rest, digest)
    checkpoint(directory, state, 'api-smoke')
    # Every worker must read the lane in use: one left on the other lane is running but judges nothing.
    healthy_workers(config, directory, digest, state['activeQueue'])
    config = publish_rolling_runtimes(config, directory, state)
    report = api_smoke(config, directory, config['nodes'])
    restore_alarms(config, state)
    checkpoint(directory, state, 'refresh-state')
    for root in ('infra/api', 'infra/judge'):
        plan = directory / (root.replace('/', '-') + '.tfplan')
        run('terraform', '-chdir=' + root, 'plan', '-input=false', '-refresh-only', '-out=' + str(plan))
        run('terraform', '-chdir=' + root, 'apply', '-input=false', str(plan))
    released = release_hold(config)
    if state.get('switchWatchFailed'):
        # The rollout itself is done, but it must not be reported as passed.
        checkpoint(directory, state, 'switch-watch-failed', apiReport=str(report), maintenanceHoldReleased=released)
        raise ValueError(watch_failed_message(state))
    checkpoint(directory, state, 'complete', status='passed', apiReport=str(report), maintenanceHoldReleased=released)


REVERT_WINDOW = 'revert is only possible between the lane switch and the old worker stop'
# SQS accepts one purge of a queue every 60 seconds, and a purge deletes the queue's messages within 60 seconds.
PURGE_INTERVAL = 65


def bridge_reverted(config, state):
    """Whether the live bridge sends the previous digest to the previous lane again; False while it is still switched."""
    status = database_status(config)
    live = status.get('requestQueueUrl'), status.get('runtimeDigest'), status.get('previousRuntimeDigest')
    if live == (state['previousQueue'], state['previousDigest'], state['runtimeDigest']):
        return True
    if live == (state['activeQueue'], state['runtimeDigest'], state['previousDigest']):
        return False
    raise ValueError('bridge is on neither the new lane nor the reverted one; inspect deployment-status before resuming')


def send_purge(config, directory, state, url):
    """Purge a lane that no worker reads. A resumed revert waits out the SQS purge interval before purging again."""
    if state.get('purgeSent'):
        # The previous run may have purged moments ago. Its purge finishes within the interval and SQS refuses
        # another one inside it, so wait, then purge again only if messages are left.
        time.sleep(PURGE_INTERVAL)
    if not state.get('purgeSent') or queue_counts(config, url) != 0:
        # Saved before the call, so a run that stops right after it knows a purge may be in progress.
        state['purgeSent'] = True
        save(directory / 'state.json', state)
        aws(config['region'], 'sqs', 'purge-queue', '--queue-url', url)


def wait_purged(config, directory, state, url, timeout=180):
    """Wait until a purged lane reads empty. After a timeout the resumed revert sends the purge again."""
    deadline = time.monotonic() + timeout
    while queue_counts(config, url) != 0:
        if time.monotonic() >= deadline:
            state.pop('newLanePurgeSent', None)
            save(directory / 'state.json', state)
            raise RuntimeError('the new lane did not empty after the purge; resume rolling-revert to purge it again')
        time.sleep(10)


def end_stale_watch(config, directory, state):
    """End a watcher that a killed rolling left, and retry removing its test users. This never raises: the watch
    only measures, and the revert must not wait on it."""
    try:
        collect_stale_watch(config, directory, state)
        if leftover_users(watch_paths(directory)[0]):
            state['switchWatchLeftovers'] = True
        else:
            state.pop('switchWatchLeftovers', None)
        save(directory / 'state.json', state)
    except Exception as error:
        print('switch watch cleanup failed:', error, flush=True)


def revert(config, directory, state):
    """Move a switched rolling update back to the previous lane and digest while the old workers still run there.

    The operator decides to revert; nothing calls this automatically. Each step is recorded, so the same command
    resumes an interrupted revert without updating the bridge, purging the new lane or requeueing again.
    It ends as reverted only once the other bursts judge again and the old environment passes the API smoke.
    """
    if state.get('status') == 'reverted':
        print('already reverted', flush=True)
        return
    # Users go back to the old workers, so those must still run. switch-stop exists from the moment they are told
    # to stop, before oldWorkersStopped is saved. These checks read only local files, so a refusal touches nothing.
    if state.get('oldWorkersStopped') or (directory / 'switch-stop').exists():
        raise ValueError(REVERT_WINDOW)
    if not state.get('revertStarted'):
        if not state.get('digestSwitched'):
            # A run can stop between the bridge update and saving digestSwitched. Only a run that went as far as
            # sending the update asks the bridge, the way a resumed rolling update does; any other run is before the switch.
            if not state.get('laneSwitchAttempted') or not bridge_switched(config, state):
                raise ValueError(REVERT_WINDOW)
    region, digest, previous, first = config['region'], state['runtimeDigest'], state['previousDigest'], state['firstHost']
    bursts = pool_bursts(config)
    primaries = [node for node in config['nodes'] if node not in bursts]
    if not state.get('revertStarted'):
        # Every user goes back to the old lane, and only the primaries still read it: hold-bursts disabled the other
        # bursts. A primary whose worker died (a runtime_mismatch exit is never restarted) would leave nobody
        # judging once the new worker stops. Checked before anything changes, so a refusal leaves users where they are.
        for node in primaries:
            try:
                lane_worker_ready(config, directory, [node], state['previousQueue'], previous, label='revert-primary')
            except RuntimeError:
                continue
            break
        else:
            raise ValueError('no primary worker judges on the previous lane ' + state['previousQueue'] + ' with the previous '
                             'digest; the bridge was not reverted. Restart an old worker, then run rolling-revert again')
    if not state.get('bridgeReverted'):
        # Saved before the bridge changes. From here rolling refuses the run, so nothing drains or stops the old
        # workers, and a resumed revert skips the window check, which a reverted bridge would no longer pass.
        checkpoint(directory, state, 'revert-bridge', revertStarted=True, digestSwitched=True)
        # Bridge first, in one update. From then on new submissions go to the old workers, and new-digest submissions
        # not yet dispatched are rebound to the old digest, because the new digest becomes the bridge's previous one.
        if not bridge_reverted(config, state):
            update_env(region, config['bridge'], dict(JUDGE_REQUEST_QUEUE_URL=state['previousQueue'], JUDGE_RUNTIME_DIGEST=previous,
                                                      JUDGE_PREVIOUS_RUNTIME_DIGEST=digest))
            if not bridge_reverted(config, state):
                raise ValueError('bridge did not return to the previous lane')
        state['bridgeReverted'] = True
        save(directory / 'state.json', state)
    if not state.get('apiReverted'):
        checkpoint(directory, state, 'revert-api')
        bridge = aws(region, 'lambda', 'get-function-configuration', '--function-name', config['bridge'])
        # A bridge invocation still running with the pre-revert configuration treats an old-digest submission as the
        # previous environment's: it rebinds it to the new digest and sends it to the new lane, or marks it JE for a
        # runtime it does not publish. So the API creates old-digest submissions only once every such invocation is over.
        # Meanwhile the reverted bridge rebinds the API's new-digest submissions to the old digest, so nobody waits.
        # After the wait nothing sends to the new lane any more, which the purge below relies on.
        time.sleep(bridge['Timeout'] + 5)
        update_env(region, config['api'], dict(JUDGE_CPP_IMAGE=previous))
        state['apiReverted'] = True
        save(directory / 'state.json', state)
    # A rolling killed mid-switch leaves its watcher submitting and maybe test users behind; nobody else retries them,
    # because the run closes as reverted. Done after users are back on the old environment, so it never delays that.
    end_stale_watch(config, directory, state)
    if not state.get('newWorkerStopped'):
        checkpoint(directory, state, 'revert-stop')
        # Stop the new worker before the purge and the requeue. A worker still running could take a job from the
        # new lane, or finish one whose attempt the requeue is about to replace.
        ssm_step(directory, 'revert-stop', config, [first], stop_command)
        # The host now holds the new environment. Without the tag the bridge never starts it as an old-environment
        # host: it starts a stopped host only when the tag matches its dispatch digest. A missing tag is no error.
        aws(region, 'ec2', 'delete-tags', '--resources', first, '--tags', 'Key=JudgeInstalledDigest')
        state['newWorkerStopped'] = True
        save(directory / 'state.json', state)
    if not state.get('newLanePurgeSent'):
        checkpoint(directory, state, 'revert-purge')
        # Purge before the requeue. The requeue re-sends these submissions with new attempts through the old lane,
        # so their messages on the new lane are stale. Left there, they would block the next rolling update,
        # which starts its standby worker on this lane only when it is empty.
        send_purge(config, directory, state, state['activeQueue'])
        state['newLanePurgeSent'] = True
        save(directory / 'state.json', state)
    if 'revertRequeued' not in state:
        checkpoint(directory, state, 'revert-requeue')
        # Submissions the new environment did not finish go back to the old one. The bridge accepts only its previous
        # digest, which the bridge update made the new one, and rebinds them to the old digest when it sends them.
        # Submissions the new environment finished keep their results. A run that stops before saving the count
        # requeues again on resume. That is safe: once the first call has run, nothing is dispatched with the new
        # digest any more, because the reverted bridge rebinds before it dispatches.
        # It does not wait for the purged lane to read empty: the requeued work goes to the old lane, and these users
        # wait until it runs (past 6 hours the bridge would mark them JE).
        count = requeue(config, digest)
        state.update(requeued=state.get('requeued', 0) + count, revertRequeued=count)
        save(directory / 'state.json', state)
    if not state.get('newLanePurged'):
        checkpoint(directory, state, 'revert-purge-wait')
        wait_purged(config, directory, state, state['activeQueue'])
        state['newLanePurged'] = True
        save(directory / 'state.json', state)
    if not state.get('revertBurstsReady'):
        checkpoint(directory, state, 'revert-bursts')
        # hold-bursts stopped every burst. The others still hold the old environment and their worker.env still names
        # the old lane, so enabling them is enough; checked, because a burst that judges nothing gives no contest capacity.
        others = [node for node in bursts if node != first]
        if others:
            ssm_step(directory, 'revert-bursts', config, others, ENABLE_WORKER)
            lane_worker_ready(config, directory, others, state['previousQueue'], previous, label='revert-bursts-check')
        state['revertBurstsReady'] = True
        save(directory / 'state.json', state)
    if not state.get('firstHostStopped'):
        checkpoint(directory, state, 'revert-first-host')
        # The first host holds the broken environment with its worker disabled. It would never idle-stop (idle stop runs
        # in the worker), and with the alarm actions back its worker alarm would fire. Stopped, it stays down: without
        # its digest tag the bridge never starts it. The next rolling or run starts and prepares it again.
        aws(region, 'ec2', 'stop-instances', '--instance-ids', first)
        state['firstHostStopped'] = True
        save(directory / 'state.json', state)
    if 'maintenanceHoldReleased' not in state:
        # The other bursts judge again, so they may power off once idle, as after a finished rolling.
        state['maintenanceHoldReleased'] = release_hold(config)
        save(directory / 'state.json', state)
    checkpoint(directory, state, 'revert-settings')
    # Terraform's previous digest is the new one, as on the live bridge, so an apply keeps the bridge as it is.
    sync_settings(config, directory, previous, config['runtimes'], previous=digest, lane=state['previousLane'])
    restore_alarms(config, state)
    if 'revertApiReport' not in state:
        checkpoint(directory, state, 'revert-smoke')
        # `reverted` means the old environment judges through the public API, not only that the settings went back.
        state['revertApiReport'] = str(api_smoke(config, directory, primaries[:1]))
        save(directory / 'state.json', state)
    checkpoint(directory, state, 'reverted', status='reverted')


def main():
    global LOG_DIRECTORY
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['prepare', 'finish', 'run', 'prune', 'rolling', 'rolling-revert'])
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
        if args.action == 'rolling-revert':
            # An emergency action uses no artifact and needs only the first host, so neither an artifact check, a pool
            # start nor preflight runs; a stale local file or an unrelated offline host must not keep users on a broken environment.
            try:
                revert(config, directory, state)
            except BaseException as error:
                # A refusal changes nothing; a started revert records where it stopped.
                if state.get('revertStarted'):
                    state.update(status='failed', error=str(error))
                    save(state_path, state)
                raise
            return
        if state.get('status') == 'unchanged':
            # Finished with nothing to switch and the pool put back. Repeating the command repeats the result without
            # touching AWS; no other action may take the run up, because nothing in it is left to finish.
            if args.action != 'rolling':
                raise ValueError(UNCHANGED + '; start a new run')
            print(UNCHANGED, flush=True)
            return
        # Before anything is saved, so a reverted run keeps its status and stays closed.
        refuse_reverted(state)
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
            if args.action == 'rolling' and state.get('step') == 'switch-watch-failed':
                # Everything else finished and the hold was released; do not touch the pool, only repeat the verdict.
                raise ValueError(watch_failed_message(state))
            if args.action == 'rolling':
                # A configuration rolling refuses must not start the stopped bursts with a 6-hour hold first.
                check_rolling_config(config)
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
                checkpoint(directory, state, 'start')
                # run never switches lanes, but it points every host at the lane in use: a worker.env can name the
                # other lane (rolling-revert leaves the prepared burst so), and a worker there takes no submission.
                start_workers(verification, request_queue=live_lane(config))
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
