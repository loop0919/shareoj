#!/usr/bin/python3
"""Single-slot isolate runner. All paths and executables are operator-owned."""
import base64
import contextlib
import hashlib
import json
import math
import os
from pathlib import Path
import re
import subprocess
import time
import uuid

import sandbox
import telemetry
import interactive
from runtimes import RUNTIMES, ROOT

ASSETS = Path('/opt/judge/assets')
TEST_FILE_LIMIT = 16 * 1024 * 1024
TEST_SET_LIMIT = 512 * 1024 * 1024


def pointer(body):
    if len(body) > 4096:
        raise ValueError('pointer too large')
    item = json.loads(body)
    for name in ('submissionId', 'attemptId'):
        if str(uuid.UUID(item[name])) != item[name]:
            raise ValueError('invalid identity')
    expected = f"jobs/{item['submissionId']}/{item['attemptId']}.json"
    if item['key'] != expected or not re.fullmatch('[a-f0-9]{64}', item['sha256']):
        raise ValueError('invalid object pointer')
    if not isinstance(item['versionId'], str) or not 0 < len(item['versionId']) <= 1024:
        raise ValueError('invalid version')
    return item


def validate_job(job, runtime):
    if job.get('runtimeDigest') != runtime or job.get('runtime') not in RUNTIMES:
        raise telemetry.PlatformError('runtime_mismatch')
    if type(job.get('generate', False)) is not bool or type(job.get('validate', False)) is not bool or (job.get('generate') and job.get('validate')):
        raise ValueError('generation mode')
    if job.get('checker') is not None and job.get('interactor') is not None:
        raise ValueError('conflicting judge modes')
    for field in ('checker', 'interactor'):
        code = job.get(field)
        if code is None:
            continue
        if (not isinstance(code, dict) or not isinstance(code.get('runtime'), str)
                or code['runtime'] + '-isolate' not in RUNTIMES
                or job.get('generate') or job.get('validate')):
            raise ValueError('judge code runtime or mode')
        source = code.get('source')
        protocol = code.get('protocol', 'legacy')
        if protocol not in ('legacy', 'testlib') or (protocol == 'testlib' and code['runtime'] not in ('cpp23-gcc', 'cpp23-clang')):
            raise ValueError('judge code protocol')
        if not isinstance(source, str) or not source.strip() or len(source.encode()) > 65536 or '\0' in source:
            raise ValueError('judge code source')
    if type(job.get('easyTest', False)) is not bool or (job.get('easyTest') and (job.get('generate') or job.get('validate'))):
        raise ValueError('sample mode')
    base = job.get('generationBaseBytes', 0)
    if type(base) is not int or not 0 <= base <= TEST_SET_LIMIT:
        raise ValueError('generation budget')
    if job.get('generate') and not re.fullmatch(r'test-files/[a-f0-9]{32}/[a-f0-9-]{36}/generated/', job.get('generationPrefix', '')):
        raise ValueError('generation prefix')
    source = job.get('source')
    if not isinstance(source, str) or not 0 < len(source.encode()) <= 65536 or '\0' in source:
        raise ValueError('source')
    if type(job.get('memoryLimitMb')) is not int or not 64 <= job['memoryLimitMb'] <= 512:
        raise ValueError('memory limit')
    ms = job.get('timeLimitMs')
    if type(ms) is not int or not 100 <= ms <= 5000 or ms % 100:
        raise ValueError('time limit')
    cases = job.get('cases')
    if not isinstance(cases, list) or not 1 <= len(cases) <= 100:
        raise ValueError('cases')
    size = 0
    for case in cases:
        if not isinstance(case, dict) or not isinstance(case.get('name', ''), str) or len(case.get('name', '')) > 64:
            raise ValueError('case')
        for key in ('input', 'output'):
            value, file = case.get(key), case.get(key + 'File')
            if file is None:
                if not isinstance(value, str) or len(value.encode()) > 65536 or '\0' in value:
                    raise ValueError('test data')
                size += len(value.encode())
                continue
            if value != '' or not isinstance(file, dict) or set(file) != {'id', 'size', 'sha256', 'key', 'versionId'}:
                raise ValueError('test file')
            if str(uuid.UUID(file['id'])) != file['id'] or type(file['size']) is not int or not 0 < file['size'] <= TEST_FILE_LIMIT:
                raise ValueError('test file')
            if not re.fullmatch('[a-f0-9]{64}', file['sha256']) or not isinstance(file['versionId'], str) or not 0 < len(file['versionId']) <= 1024:
                raise ValueError('test file')
            if not isinstance(file['key'], str) or not re.fullmatch(r'test-files/[a-f0-9]{32}/[a-f0-9-]{36}/(?:generated/)?[a-f0-9-]{36}', file['key']):
                raise ValueError('test file')
            size += file['size']
    if size > TEST_SET_LIMIT:
        raise ValueError('test set limit')


def number(value, maximum):
    if type(value) not in (float, int) or not math.isfinite(value) or not 0 <= value <= maximum:
        raise ValueError('invalid measurement')
    return value


def case_result(reply, index, case, generate=False, validate=False):
    if reply.get('index') != index or reply.get('status') not in ('', 'RE', 'SG', 'TO'):
        raise ValueError('invalid case response')
    for key in ('oom', 'overflow'):
        if type(reply.get(key)) is not bool:
            raise ValueError('invalid flag')
    for key in ('exitCode', 'signal'):
        if type(reply.get(key)) is not int or not 0 <= reply[key] <= 255:
            raise ValueError('invalid exit status')
    cpu = number(reply.get('cpuTimeMs'), 120000)
    wall = number(reply.get('wallTimeMs'), 120000)
    memory = number(reply.get('memoryBytes'), 4 * 1024**3)
    output = base64.b64decode(reply['output'], validate=True)
    if len(output) > TEST_FILE_LIMIT:
        raise ValueError('output limit')
    if reply['overflow']:
        verdict = 'OLE'
    elif reply['oom']:
        verdict = 'MLE'
    elif reply['status'] == 'TO':
        verdict = 'TLE'
    elif reply['status'] or reply['exitCode'] or reply['signal']:
        verdict = 'RE'
    elif not generate and not validate and re.findall(rb'[^ \t\n\r\v\f]+', output) != re.findall(rb'[^ \t\n\r\v\f]+', case['output'].encode()):
        verdict = 'WA'
    else:
        verdict = 'AC'
    item = dict(name=case.get('name') or f'ケース{index + 1}', verdict=verdict,
                cpuTimeMs=cpu, wallTimeMs=wall, memoryBytes=memory)
    if generate and verdict == 'AC':
        try:
            text = output.decode('utf-8')
            if '\0' in text:
                raise ValueError('NUL output')
            item['output'] = text
        except (UnicodeDecodeError, ValueError):
            item['verdict'] = 'RE'
    return item


def sample_preview(data, limit):
    text = data.decode('utf-8', errors='replace').replace('\0', '�').encode('utf-8')
    return dict(text=text[:limit].decode('utf-8', errors='ignore'), truncated=len(text) > limit)


def prepare_cgroup():
    # The systemd unit delegates this subtree and caps its aggregate memory/pids.
    relative = next(line[3:] for line in Path('/proc/self/cgroup').read_text().splitlines() if line.startswith('0::'))
    root = Path('/sys/fs/cgroup') / relative.lstrip('/')
    if root.name == 'controller':
        root = root.parent
    leaf = root / 'controller'
    leaf.mkdir(exist_ok=True)
    (leaf / 'cgroup.procs').write_text(str(os.getpid()))
    (root / 'cgroup.subtree_control').write_text('+cpu +memory +pids')
    # Kill the whole service only if its aggregate limit is exceeded.
    (root / 'memory.oom.group').write_text('1')
    Path('/run/judge/cgroup').write_text(str(root))
    for child in root.iterdir():
        if child.is_dir() and child.name != 'controller':
            (child / 'cgroup.kill').write_text('1')
            child.rmdir()
    return root


@contextlib.contextmanager
def slot():
    # One slot across worker and manual smoke tests, including separate services.
    import fcntl
    with open('/run/judge-slot.lock', 'w') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        yield


def judge(job, runtime, load_file=None, progress=None, save_output=None):
    validate_job(job, runtime)
    deadline = time.monotonic() + 1800
    generated_bytes = job.get('generationBaseBytes', 0)
    result = dict(verdict='AC', passed=0, total=len(job['cases']), cases=[])
    tle_count = 0
    checker = job.get('checker')
    judge_code = job.get('interactor') or checker
    checker_artifact = sandbox.ARTIFACT.with_name('checker')

    def diagnostic(message):
        result['checkerLog'] = (result.get('checkerLog', '') + message).encode()[:16384].decode(errors='ignore')

    def checker_error(reason):
        telemetry.note_failure('judge_code', reason)
        return dict(verdict='JE', passed=0, total=len(job['cases']), checkerLog=result.get('checkerLog', ''))

    try:
        if progress:
            progress('PREPARING', 0, result['total'])
        compiled = sandbox.execute(dict(source=job['source'], runtime=job['runtime']), True)
        if not compiled['compiled']:
            return dict(verdict='CE', passed=0, total=len(job['cases']), compileLog=compiled['compileLog'])
        if judge_code is not None:
            compiled = sandbox.execute(dict(source=judge_code['source'], runtime=judge_code['runtime'] + '-isolate'),
                                       True, artifact=checker_artifact)
            if not compiled['compiled']:
                diagnostic('検証コードのコンパイル失敗\n' + compiled.get('compileLog', ''))
                return checker_error(('interactor' if job.get('interactor') else 'checker') + '_compile_failed')
        if progress:
            progress('JUDGING', 0, result['total'])
        for index, case in enumerate(job['cases']):
            if time.monotonic() >= deadline:
                raise telemetry.PlatformError('job_deadline_exceeded')
            case = dict(case)
            for key in ('input', 'output'):
                if case.get(key + 'File') is not None:
                    if load_file is None:
                        raise ValueError('test file loader unavailable')
                    case[key] = load_file(case[key + 'File'])
            if job.get('interactor') is not None:
                diagnostic(f"ケース{index + 1}:\n")
                case_log = bytearray()
                log_limit = min(4096, (24 * 1024) // len(job['cases']))
                def case_diagnostic(message):
                    diagnostic(message)
                    data = message.encode()
                    case_log.extend(data[:max(0, log_limit + 1 - len(case_log))])
                item = interactive.execute(job, case, checker_artifact, case_diagnostic)
                if job.get('easyTest'):
                    item['checkerLog'] = sample_preview(bytes(case_log), log_limit)
                item['name'] = case.get('name') or f'ケース{index + 1}'
                if item['verdict'] == 'JE':
                    return checker_error('interactor_execution_failed')
            else:
                reply = sandbox.execute(dict(runtime=job['runtime'], input=base64.b64encode(case['input'].encode()).decode(),
                                             timeLimitMs=job['timeLimitMs'], memoryLimitMb=job['memoryLimitMb']))
                reply['index'] = index
                item = case_result(reply, index, case, job.get('generate', False), job.get('validate', False) or checker is not None)
                if checker is not None and item['verdict'] == 'AC':
                    checked = sandbox.execute(dict(runtime=checker['runtime'] + '-isolate', input=reply['output'], protocol=checker.get('protocol', 'legacy'),
                                                   timeLimitMs=5000, memoryLimitMb=512), artifact=checker_artifact,
                                              checker_files={'test-input': case['input'].encode(),
                                                             'expected-output': case['output'].encode(),
                                                             'submission-source': job['source'].encode()})
                    checked['index'] = index
                    verdict = sandbox.judge_verdict(checked, checker.get('protocol', 'legacy'))
                    diagnostic(f"ケース{index + 1}: {verdict}\n" + checked.get('checkerLog', '') + '\n')
                    if checker.get('protocol') == 'testlib' and checked['exitCode'] == 7:
                        diagnostic('testlibの部分点（_points）は未対応です。\n')
                    if verdict == 'JE':
                        return checker_error('checker_execution_failed')
                    item['verdict'] = verdict
            if job.get('easyTest') and not job.get('interactor'):
                limit = min(4096, (24 * 1024) // len(job['cases']) // 3)
                actual = base64.b64decode(reply['output'], validate=True)
                item['sampleDetails'] = dict(input=sample_preview(case['input'].encode(), limit),
                                             expectedOutput=sample_preview(case['output'].encode(), limit),
                                             actualOutput=sample_preview(actual, limit))
            if 'output' in item:
                generated_bytes += len(item['output'].encode())
                if generated_bytes > TEST_SET_LIMIT:
                    del item['output']
                    item['verdict'] = 'OLE'
                elif item['output']:
                    if save_output is None:
                        raise ValueError('output storage unavailable')
                    item['outputFile'] = save_output(item.pop('output').encode())
            result['cases'].append(item)
            if item['verdict'] == 'AC':
                result['passed'] += 1
            elif result['verdict'] == 'AC':
                result['verdict'] = item['verdict']
            if progress:
                progress('JUDGING', index + 1, result['total'], result['verdict'] if result['verdict'] != 'AC' else None)
            if time.monotonic() >= deadline:
                raise telemetry.PlatformError('job_deadline_exceeded')
            if item['verdict'] == 'TLE':
                tle_count += 1
            if tle_count >= 2 and not any(job.get(mode) for mode in ('easyTest', 'validate', 'generate')):
                result['cases'].extend(dict(name=remaining.get('name') or f'ケース{i + 1}', verdict='SKIPPED')
                                       for i, remaining in enumerate(job['cases'][index + 1:], index + 1))
                break
    finally:
        sandbox.ARTIFACT.unlink(missing_ok=True)
        checker_artifact.unlink(missing_ok=True)
        sandbox.META.unlink(missing_ok=True)
    return result


def platform_fingerprint():
    # Detect compiler/library or kernel updates before consuming another job.
    packages = subprocess.check_output(['dpkg-query', '-W', '-f=${Package}=${Version}\n'], env={'PATH': '/usr/bin:/bin', 'LC_ALL': 'C'})
    return dict(kernel=os.uname().release, packages=hashlib.sha256(packages).hexdigest())


def runtime_inventory():
    tree = {}
    root = Path(ROOT)
    if not root.is_dir():
        raise ValueError('runtime bundle missing')
    for path in sorted(root.rglob('*')):
        if path.is_symlink():
            tree[str(path.relative_to(root))] = {'link': os.readlink(path)}
        elif path.is_file():
            with path.open('rb') as file:
                tree[str(path.relative_to(root))] = {'sha256': hashlib.file_digest(file, 'sha256').hexdigest(), 'mode': path.stat().st_mode & 0o777}
    return tree


def verify_assets(full=False):
    manifest_data = (ASSETS / 'manifest.json').read_bytes()
    manifest = json.loads(manifest_data)
    if manifest['platform'] != platform_fingerprint():
        raise ValueError('system changed; fingerprint and smoke-test before enabling worker')
    for name, digest in manifest['files'].items():
        with open(name, 'rb') as file:
            if hashlib.file_digest(file, 'sha256').hexdigest() != digest:
                raise ValueError('runtime checksum mismatch')
    if full and json.loads((ASSETS / 'runtime-tree.json').read_text()) != runtime_inventory():
        raise ValueError('runtime bundle changed')
    return 'sha256:' + hashlib.sha256(manifest_data).hexdigest()
