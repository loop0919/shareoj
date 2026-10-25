#!/usr/bin/env python3
"""Use the production judging rules with the local worker's Docker sandbox."""
import base64
import json
from pathlib import Path
import sys
import tempfile

sys.path.insert(0, str(Path(__file__).resolve().parent / 'runner'))
import host
import sandbox


def rpc(action, **fields):
    print(json.dumps(dict(action=action, **fields)), flush=True)
    reply = json.loads(sys.stdin.readline())
    if reply.get('error'):
        raise RuntimeError(reply['error'])
    return reply['value']


def execute(request, compile_phase=False, *, artifact=None, checker_files=None, score_file=False, stderr_log=False):
    reply = rpc('execute', request=request, compile=compile_phase,
                artifact=(artifact or sandbox.ARTIFACT).name,
                checkerFiles={key: base64.b64encode(value).decode() for key, value in (checker_files or {}).items()},
                scoreFile=score_file)
    if 'scoreFile' in reply and reply['scoreFile'] is not None:
        reply['scoreFile'] = base64.b64decode(reply['scoreFile'], validate=True)
    return reply


def main():
    job = json.loads(sys.stdin.readline())
    if job.get('interactor'):
        raise ValueError('対話形式はisolateワーカーで実行してください。')
    if job['runtime'] not in ('cpp17-local', 'cpp23-gcc-local', 'python314-local'):
        raise ValueError('unsupported local runtime')
    job['runtime'] = job['runtime'].removesuffix('-local') + '-isolate'
    # Retain the old command for queued C++17 jobs without publishing it in production.
    host.RUNTIMES['cpp17-isolate'] = {}
    sandbox.execute = execute
    with tempfile.TemporaryDirectory(prefix='shareoj-local-') as directory:
        sandbox.ARTIFACT = Path(directory) / 'main'
        sandbox.META = Path(directory) / 'meta'
        if job.get('codeRun') is True:
            result = host.run_code(job, job['runtimeDigest'])
            # The local sandbox reports 0 for these, which would read as a real measurement.
            result.pop('cpuTimeMs', None)
            result.pop('memoryBytes', None)
            # Docker timing is a float; the result schema holds whole milliseconds.
            if 'wallTimeMs' in result:
                result['wallTimeMs'] = round(result['wallTimeMs'])
            print(json.dumps(dict(action='result', value=result)), flush=True)
            return
        result = host.judge(job, job['runtimeDigest'],
                            lambda file: rpc('load', file=file),
                            save_output=lambda data: rpc('save', data=base64.b64encode(data).decode()))
    for case in result.get('cases', []):
        for key in ('cpuTimeMs', 'wallTimeMs', 'memoryBytes'):
            case.pop(key, None)
    print(json.dumps(dict(action='result', value=result)), flush=True)


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        print(json.dumps(dict(action='result', value=dict(verdict='JE', passed=0, total=0,
                                                         checkerLog=str(error)[:4096]))), flush=True)
