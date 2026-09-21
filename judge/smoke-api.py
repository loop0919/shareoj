#!/usr/bin/env python3
"""Verify production judge results with temporary private problems and suppressed-email users."""
import argparse
from datetime import datetime
import json
from pathlib import Path
import secrets
import tempfile
import time
import urllib.request
import urllib.error
import uuid

from verify import AWSOperationError, aws, save


def request(base, method, path, data=None, token=''):
    req = urllib.request.Request(base + path, method=method,
        data=None if data is None else json.dumps(data).encode(),
        headers={'Content-Type': 'application/json', **({'Authorization': 'Bearer ' + token} if token else {})})
    with urllib.request.urlopen(req, timeout=30) as response:
        raw = response.read()
        return json.loads(raw) if raw else None


def cognito(region, operation, payload):
    # Passwords must not appear in argv, logs, or subprocess exception messages.
    with tempfile.NamedTemporaryFile(mode='w', suffix='.json') as file:
        json.dump(payload, file)
        file.flush()
        try:
            return aws(region, 'cognito-idp', operation, '--cli-input-json', 'file://' + file.name)
        except AWSOperationError as error:
            if operation == 'admin-delete-user' and error.code == 'UserNotFoundException':
                return {}
            raise


def login(args, pool, user):
    password = secrets.token_urlsafe(32) + 'aA1!'
    cognito(args.region, 'admin-set-user-password', dict(UserPoolId=pool,
        Username=user['username'], Password=password, Permanent=True))
    return request(args.api_url, 'POST', '/auth/login',
                   {'username': user['username'], 'password': password})['access_token']


def cleanup(args, pool, users, path):
    remaining = []
    for user in users:
        try:
            if user.get('problem'):
                token = login(args, pool, user)
                problem_path = '/my/problems/' + user['problem']
                try:
                    problem = request(args.api_url, 'GET', problem_path, token=token)
                    request(args.api_url, 'DELETE', problem_path + '?version=' + str(problem['version']), token=token)
                except urllib.error.HTTPError as error:
                    if error.code != 404:
                        raise
                user['problem'] = None
                save(path, dict(pool=pool, api=args.api_url, users=users))
            cognito(args.region, 'admin-delete-user', dict(UserPoolId=pool, Username=user['username']))
        except Exception:
            remaining.append(user)
    save(path, dict(pool=pool, api=args.api_url, users=remaining))
    if remaining:
        raise RuntimeError('cleanup incomplete; rerun with --cleanup and the same --report')


def assert_result(result, expected):
    if result['verdict'] != 'TLE' or result['total'] != 4 or result['passed'] != expected.count('AC') or \
            [case['verdict'] for case in result['cases']] != expected:
        raise ValueError('unexpected judge result: ' + json.dumps(result))
    for case in result['cases']:
        if case['verdict'] == 'SKIPPED' and any(case.get(key) is not None for key in
                                              ('cpuTimeMs', 'wallTimeMs', 'memoryBytes')):
            raise ValueError('an unexecuted case has resource measurements')


def parallel_check(args, ids):
    events_by_node = {}
    script = """python3 - <<'PY'
import json
ids = IDS
for line in open('/var/log/judge/worker.jsonl'):
    event = json.loads(line)
    if event.get('submissionId') in ids and event.get('event') in ('judge_started', 'judge_finished'):
        print(json.dumps({key: event[key] for key in ('timestamp', 'event', 'submissionId')}))
PY
""".replace('IDS', repr(ids))
    for node in args.instance:
        command = aws(args.region, 'ssm', 'send-command', '--instance-ids', node,
                      '--document-name', 'AWS-RunShellScript', '--parameters',
                      json.dumps({'commands': [script]}))['Command']['CommandId']
        for _ in range(30):
            time.sleep(2)
            entries = aws(args.region, 'ssm', 'list-command-invocations', '--command-id', command)['CommandInvocations']
            if entries and entries[0]['Status'] not in ('Pending', 'InProgress', 'Delayed'):
                break
        if not entries or entries[0]['Status'] != 'Success':
            raise RuntimeError('could not retrieve worker evidence: ' + command)
        output = aws(args.region, 'ssm', 'get-command-invocation', '--command-id', command, '--instance-id', node)
        events_by_node[node] = [json.loads(line) for line in output['StandardOutputContent'].splitlines()]
    intervals = []
    for node, events in events_by_node.items():
        for submission in ids:
            pair = {e['event']: datetime.fromisoformat(e['timestamp']) for e in events if e['submissionId'] == submission}
            if 'judge_started' in pair and 'judge_finished' in pair:
                intervals.append((node, pair['judge_started'], pair['judge_finished']))
    if not all(events_by_node.values()) or not any(
            a[0] != b[0] and max(a[1], b[1]) < min(a[2], b[2]) for a in intervals for b in intervals):
        raise ValueError('concurrent judging was not observed on both workers')
    return events_by_node


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--function', required=True, help='API Lambda used to resolve the Cognito pool')
    parser.add_argument('--api-url', required=True)
    parser.add_argument('--region', default='ap-northeast-1')
    parser.add_argument('--runtime', default='python314', help='Published CPython/PyPy runtime used for the API fixture')
    parser.add_argument('--report', type=Path, required=True)
    parser.add_argument('--instance', action='append', default=[], help='Specify both hosts to require concurrent judging')
    parser.add_argument('--cleanup', action='store_true')
    args = parser.parse_args()
    args.api_url = args.api_url.rstrip('/')
    if not args.api_url.startswith('https://'):
        parser.error('use an HTTPS API URL')
    if args.instance and (len(set(args.instance)) != 2 or len(args.instance) != 2):
        parser.error('parallel verification requires exactly two distinct hosts')
    pool = aws(args.region, 'lambda', 'get-function-configuration', '--function-name', args.function)['Environment']['Variables']['COGNITO_USER_POOL_ID']
    cleanup_path = args.report.with_suffix('.cleanup.json')
    if args.cleanup:
        saved = json.loads(cleanup_path.read_text())
        if saved['pool'] != pool or saved['api'] != args.api_url:
            raise ValueError('cleanup target does not match saved run')
        cleanup(args, pool, saved['users'], cleanup_path)
        return
    if args.report.exists() or cleanup_path.exists():
        parser.error('choose a new report path; use --cleanup for an interrupted run')
    catalog = request(args.api_url, 'GET', '/runtimes')
    if catalog['maintenance'] or args.runtime not in {r['id'] for r in catalog['items']}:
        raise ValueError('the selected Python runtime must be published before the API smoke test')
    args.report.parent.mkdir(parents=True, exist_ok=True)
    users, results, tokens = [], [], []
    save(args.report, dict(status='running', results=[]))
    try:
        for _ in range(2):
            username = 'judge-smoke-' + uuid.uuid4().hex + '@example.invalid'
            user = dict(username=username, problem=None)
            users.append(user)
            save(cleanup_path, dict(pool=pool, api=args.api_url, users=users))
            cognito(args.region, 'admin-create-user', dict(UserPoolId=pool, Username=username,
                MessageAction='SUPPRESS', UserAttributes=[dict(Name='email', Value=username), dict(Name='email_verified', Value='true')]))
            token = login(args, pool, user)
            tokens.append(token)
            request(args.api_url, 'PUT', '/my/profile', dict(handle='smoke_' + uuid.uuid4().hex[:10], avatar='', version=0), token)
            user['problem'] = str(uuid.uuid4())
            save(cleanup_path, dict(pool=pool, api=args.api_url, users=users))
            draft = dict(title='ジャッジ配布検証', markdown='非公開の一時テスト', timeLimitMs='1000', memoryLimitMb='315',
                         testCases=[dict(name='case' + str(n), input=str(n), output='0', isSample=True) for n in range(1, 5)])
            request(args.api_url, 'PUT', '/my/problems/' + user['problem'], dict(version=0, draft=draft), token)
        for batch in range(2):
            pending = []
            for i, (user, token) in enumerate(zip(users, tokens)):
                sample = batch == 1 and i == 1
                condition = 'n in (1, 3)' if batch == 0 or sample else 'n == 1'
                source = 'n=int(input())\nif ' + condition + ':\n while True: pass\nprint(0)\n'
                item = request(args.api_url, 'POST', '/my/submissions',
                               dict(problemId=user['problem'], runtime=args.runtime, source=source, easyTest=sample), token)
                expected = ['TLE', 'AC', 'TLE', 'SKIPPED'] if batch == 0 else ['TLE', 'AC', 'TLE', 'AC'] if sample else ['TLE', 'AC', 'AC', 'AC']
                pending.append((item['id'], token, expected))
            deadline = time.monotonic() + 480
            while pending:
                for submission, token, expected in pending[:]:
                    item = request(args.api_url, 'GET', '/my/submissions/' + submission, token=token)
                    if item['status'] == 'DONE':
                        assert_result(item['result'], expected)
                        results.append(dict(id=submission, cases=expected))
                        pending.remove((submission, token, expected))
                if pending and time.monotonic() >= deadline:
                    raise TimeoutError('API smoke submissions did not finish')
                if pending:
                    time.sleep(3)
        evidence = parallel_check(args, [r['id'] for r in results]) if args.instance else {}
    except Exception:
        save(args.report, dict(status='failed', results=results))
        raise
    finally:
        try:
            cleanup(args, pool, users, cleanup_path)
        except Exception:
            save(args.report, dict(status='failed', results=results, cleanup='incomplete'))
            raise
    save(args.report, dict(status='passed', results=results, workers=evidence, cleanup='complete'))
    print('API smoke: 4 passed; temporary problems and users removed; report:', args.report)


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        raise SystemExit(str(error)) from None
