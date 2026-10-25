#!/usr/bin/env python3
"""Pause or publish runtime admission after a target-host smoke report.

The smoke report is the final JSON line from smoke.py, captured from the target
host. Keep Terraform/GitHub deployment variables in sync with published values.
"""
import argparse
import json
from pathlib import Path
import re
import subprocess


def aws(*args):
    return json.loads(subprocess.check_output(['aws', *args, '--output', 'json']) or '{}')


def publication(report, runtimes):
    digest = report['runtimeDigest']
    requested = runtimes.split(',')
    passed = report['passedRuntimes']
    if not isinstance(passed, list) or not re.fullmatch('sha256:[a-f0-9]{64}', digest) or any(
            not name or name + '-isolate' not in passed or
            name + '-isolate' in report.get('failedRuntimes', []) for name in requested):
        raise ValueError('requested runtimes have not passed smoke for this digest')
    if 'hosts' in report:
        hosts = report['hosts']
        if report.get('ready') is not True or not isinstance(hosts, dict) or not hosts or \
                not isinstance(report.get('runId'), str) or not re.fullmatch('[a-f0-9]{32}', report['runId']):
            raise ValueError('all verified workers must be ready before publication')
        for host in hosts.values():
            if not isinstance(host, dict) or 'hosts' in host or host.get('ready') is not True or host.get('runId') != report.get('runId') or \
                    host.get('runtimeDigest') != digest or host.get('failedRuntimes') != []:
                raise ValueError('worker readiness or digest mismatch')
            publication(host, runtimes)
    return digest, ','.join(requested)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--function', required=True)
    parser.add_argument('--pause', action='store_true')
    parser.add_argument('--smoke-report', type=Path)
    parser.add_argument('--runtimes', default='cpp23-gcc')
    args = parser.parse_args()
    digest, published = '', 'none'
    if not args.pause:
        if not args.smoke_report:
            parser.error('--smoke-report is required to publish')
        report = json.loads(args.smoke_report.read_text())
        try:
            digest, published = publication(report, args.runtimes)
        except (KeyError, TypeError, ValueError):
            parser.error('requested runtimes have not passed smoke for this digest')
    current = aws('lambda', 'get-function-configuration', '--function-name', args.function)
    variables = current['Environment']['Variables']
    variables.update(JUDGE_CPP_IMAGE=digest, JUDGE_RUNTIME='cpp17-isolate', JUDGE_ENABLED_RUNTIMES=published)
    # Do not print the environment or include it in a command line/traceback.
    request = json.dumps({'FunctionName': args.function, 'RevisionId': current['RevisionId'], 'Environment': {'Variables': variables}})
    import tempfile
    with tempfile.NamedTemporaryFile(mode='w', prefix='judge-admission-', suffix='.json') as file:
        file.write(request)
        file.flush()
        result = subprocess.run(['aws', 'lambda', 'update-function-configuration', '--cli-input-json', 'file://' + file.name], stdout=subprocess.DEVNULL)
        if result.returncode:
            raise SystemExit('admission update failed')
    subprocess.run(['aws', 'lambda', 'wait', 'function-updated-v2', '--function-name', args.function], check=True)
    print('Admission:', published, 'runtime digest:', digest or '(disabled)')
