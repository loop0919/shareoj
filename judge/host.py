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
JOB_DEADLINE = 1800
# Scoring jobs stop starting cases here; the rest of the deadline runs the aggregator (ADR 0025).
SCORING_BUDGET = 1500
MAX_GROUPS = 100
MAX_POINTS = 1000000
MAX_CASE_SCORE = 10**18


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
        validate_judge_code(code)
    if type(job.get('easyTest', False)) is not bool or (job.get('easyTest') and (job.get('generate') or job.get('validate'))):
        raise ValueError('sample mode')
    base = job.get('generationBaseBytes', 0)
    if type(base) is not int or not 0 <= base <= TEST_SET_LIMIT:
        raise ValueError('generation budget')
    if job.get('generate') and not re.fullmatch(r'test-files/[a-f0-9]{32}/[a-f0-9-]{36}/generated/', job.get('generationPrefix', '')):
        raise ValueError('generation prefix')
    source = job.get('source')
    if not isinstance(source, str) or not 0 < len(source.encode()) <= 131072 or '\0' in source:
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
    validate_scoring(job)


def validate_judge_code(code):
    if (not isinstance(code, dict) or not isinstance(code.get('runtime'), str)
            or code['runtime'] + '-isolate' not in RUNTIMES):
        raise ValueError('judge code runtime or mode')
    source = code.get('source')
    if not isinstance(source, str) or not source.strip() or len(source.encode()) > 65536 or '\0' in source:
        raise ValueError('judge code source')


def validate_scoring(job):
    # Jobs without scoring keep ignoring per-case group keys, as before ADR 0025.
    scoring = job.get('scoring')
    if scoring is None:
        return
    if not isinstance(scoring, dict) or scoring.get('mode') not in ('groups', 'aggregate') \
            or any(job.get(mode) for mode in ('easyTest', 'generate', 'validate')):
        raise ValueError('scoring mode')
    groups = scoring.get('groups')
    if not isinstance(groups, list) or not 1 <= len(groups) <= MAX_GROUPS:
        raise ValueError('scoring groups')
    names = set()
    for group in groups:
        if (not isinstance(group, dict) or not isinstance(group.get('name'), str) or len(group['name']) > 64
                or group['name'] in names or type(group.get('points')) is not int
                or not 0 <= group['points'] <= MAX_POINTS):
            raise ValueError('scoring groups')
        names.add(group['name'])
    if not 1 <= sum(group['points'] for group in groups) <= MAX_POINTS:
        raise ValueError('scoring full score')
    for case in job['cases']:
        # Go encodes an empty membership as null.
        member = [] if case.get('groups') is None else case['groups']
        if (not isinstance(member, list) or any(not isinstance(name, str) or name not in names for name in member)
                or len(set(member)) != len(member) or type(case.get('isSample', False)) is not bool):
            raise ValueError('case groups')
    aggregator = scoring.get('aggregator')
    if (aggregator is not None) != (scoring['mode'] == 'aggregate'):
        raise ValueError('aggregator mode')
    if aggregator is not None:
        validate_judge_code(aggregator)


def case_groups(job, case):
    """0-based indexes of the case's groups, in the order of the job's groups."""
    names = [group['name'] for group in job['scoring']['groups']]
    return sorted(names.index(name) for name in case.get('groups') or [])


class ScoreError(ValueError):
    """Invalid judge-program or aggregator output; the message is the author's diagnostic."""


def legacy_score(data):
    if data is None:
        raise ScoreError('スコアファイルが通常のファイルでないか、4 KiBを超えています。')
    text = data.strip()
    if not text:
        return None
    if not re.fullmatch(rb'[0-9]{1,19}', text) or int(text) > MAX_CASE_SCORE:
        raise ScoreError('スコアファイルの内容が、0以上10の18乗以下の10進の整数ではありません。')
    return int(text)


# testlib writes this exact form with -appes; the message is XML-escaped and has no '<'.
TESTLIB_RESULT = re.compile(rb'<\?xml version="1\.0" encoding="[A-Za-z0-9_-]{1,32}"\?>'
                            rb'<result outcome = "([a-z-]{1,32})"(?: points = "([0-9.]{1,32})")?(?: pctype = "([0-9]{1,10})")?>'
                            rb'([^<]*)</result>\n?')


def testlib_score(data, exit_code, verdict):
    """Return (verdict, score or None, message) for a finished testlib program in aggregate mode."""
    if data is None:
        raise ScoreError('結果ファイルが通常のファイルでないか、256 KiBを超えています。')
    outcome = points = pctype = None
    message = b''
    if data:
        match = TESTLIB_RESULT.fullmatch(data)
        if match is None:
            raise ScoreError('結果ファイルがtestlibの形式ではありません。')
        outcome, points, pctype, message = match.groups()
    for entity, char in ((b'&lt;', b'<'), (b'&gt;', b'>'), (b'&quot;', b'"'), (b'&amp;', b'&')):
        message = message.replace(entity, char)
    message = message.decode(errors='replace')
    # _pc(n) exits with n, so only the result file tells it apart from _ok or _wa.
    if outcome == b'partially-correct' or pctype is not None:
        raise ScoreError('testlibの_pcは使えません。quitpに整数の点を渡してください。')
    if exit_code == 7:
        # quitp(37) is written as "37.0"; any other fraction is a non-integer score.
        match = re.fullmatch(rb'([0-9]{1,7})(?:\.0+)?', points or b'') if outcome == b'points' else None
        if match is None or int(match.group(1)) > 1000000:
            raise ScoreError('quitpの点が、0以上1,000,000以下の整数ではありません。')
        return 'AC', int(match.group(1)), message
    if outcome == b'points':
        raise ScoreError('結果ファイルの点と終了コードが一致しません。')
    return verdict, None, message


def aggregator_input(job, cases):
    groups = job['scoring']['groups']
    lines = [f"{len(cases)} {len(groups)}", *(str(group['points']) for group in groups)]
    for case, item in zip(job['cases'], cases):
        member = case_groups(job, case)
        score = item.get('score')
        lines.append(' '.join([item['verdict'], '-' if score is None else str(score),
                               '1' if case.get('isSample') else '0', str(len(member)), *map(str, member)]))
    return ''.join(line + '\n' for line in lines)


def aggregator_scores(output, groups):
    """Parse one decimal per group and truncate it to hundredths."""
    if len(output) > 65536:
        raise ScoreError('集計コードの出力が64 KiBを超えています。')
    tokens = re.findall(rb'[^ \t\n\r\v\f]+', output)
    if len(tokens) != len(groups):
        raise ScoreError(f'集計コードの出力は{len(groups)}個の数でなければなりませんが、{len(tokens)}個でした。')
    scores = []
    for index, (token, group) in enumerate(zip(tokens, groups)):
        match = re.fullmatch(rb'([0-9]+)(?:\.([0-9]+))?', token)
        if match is None:
            raise ScoreError(f'グループ{index + 1}の得点「{token[:32].decode(errors="replace")}」が10進の小数ではありません。')
        whole, fraction = match.group(1).lstrip(b'0') or b'0', match.group(2) or b''
        # Compare the exact value, so 10.001 is out of range for a 10-point group.
        if len(whole) > 7 or int(whole) > group['points'] or (int(whole) == group['points'] and fraction.strip(b'0')):
            raise ScoreError(f'グループ{index + 1}の得点が、0以上{group["points"]}以下ではありません。')
        # Truncate rather than round, so a score below full never becomes full.
        scores.append(int(whole) * 100 + int((fraction + b'00')[:2]))
    return scores


def scoring_result(job, scores):
    groups = job['scoring']['groups']
    return dict(score=sum(scores), maxScore=100 * sum(group['points'] for group in groups),
                groups=[dict(score=score, maxScore=100 * group['points']) for score, group in zip(scores, groups)])


def group_scores(job, cases):
    """A group scores its points only if it has cases and all of them are AC."""
    members = [[] for _ in job['scoring']['groups']]
    for index, case in enumerate(job['cases']):
        for group in case_groups(job, case):
            members[group].append(index)
    return [100 * group['points'] if member and all(cases[i]['verdict'] == 'AC' for i in member) else 0
            for group, member in zip(job['scoring']['groups'], members)]


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


def case_score(protocol, verdict, data, exit_code):
    """Score one case in aggregate mode. Only AC and WA judgements are read: a judge
    program stopped after a submission failure may leave a partial score (ADR 0025)."""
    if verdict not in ('AC', 'WA'):
        return verdict, None, ''
    if protocol == 'testlib':
        return testlib_score(data, exit_code, verdict)
    return verdict, legacy_score(data), ''


def judge(job, runtime, load_file=None, progress=None, save_output=None):
    validate_job(job, runtime)
    started = time.monotonic()
    deadline = started + JOB_DEADLINE
    generated_bytes = job.get('generationBaseBytes', 0)
    result = dict(verdict='AC', passed=0, total=len(job['cases']), cases=[])
    tle_count = 0
    checker = job.get('checker')
    judge_code = job.get('interactor') or checker
    checker_artifact = sandbox.ARTIFACT.with_name('checker')
    scoring = job.get('scoring')
    # Only aggregate mode reads score and result files; existing judge programs may write anything there.
    aggregate = scoring is not None and scoring['mode'] == 'aggregate'
    aggregator_artifact = sandbox.ARTIFACT.with_name('aggregator')
    group_tles = [0] * len(scoring['groups']) if scoring is not None else None

    def diagnostic(message, keep=False):
        if not keep:
            result['checkerLog'] = (result.get('checkerLog', '') + message).encode()[:16384].decode(errors='ignore')
            return
        # The reason for a JE must survive the cap, even after long per-case logs.
        data = message.encode()[:4096]
        result['checkerLog'] = (result.get('checkerLog', '').encode()[:16384 - len(data)].decode(errors='ignore')
                                + data.decode(errors='ignore'))

    def checker_error(reason):
        telemetry.note_failure('judge_code', reason)
        return dict(verdict='JE', passed=0, total=len(job['cases']), checkerLog=result.get('checkerLog', ''))

    try:
        if progress:
            progress('PREPARING', 0, result['total'])
        compiled = sandbox.execute(dict(source=job['source'], runtime=job['runtime']), True)
        if not compiled['compiled']:
            failed = dict(verdict='CE', passed=0, total=len(job['cases']), compileLog=compiled['compileLog'])
            if scoring is not None:
                failed['scoring'] = scoring_result(job, [0] * len(scoring['groups']))
            return failed
        if judge_code is not None:
            compiled = sandbox.execute(dict(source=judge_code['source'], runtime=judge_code['runtime'] + '-isolate'),
                                       True, artifact=checker_artifact)
            if not compiled['compiled']:
                diagnostic('検証コードのコンパイル失敗\n' + compiled.get('compileLog', ''))
                return checker_error(('interactor' if job.get('interactor') else 'checker') + '_compile_failed')
        if aggregate:
            # Compile before any case runs, so a broken aggregator never spends the case budget.
            aggregator = scoring['aggregator']
            compiled = sandbox.execute(dict(source=aggregator['source'], runtime=aggregator['runtime'] + '-isolate'),
                                       True, artifact=aggregator_artifact)
            if not compiled['compiled']:
                diagnostic('集計コードのコンパイル失敗\n' + compiled.get('compileLog', ''))
                return checker_error('aggregator_compile_failed')
        if progress:
            progress('JUDGING', 0, result['total'])
        for index, case in enumerate(job['cases']):
            if scoring is not None:
                # Group-wise cutoff and the soft budget replace the global rules (ADR 0025).
                member = case_groups(job, case)
                if time.monotonic() - started >= SCORING_BUDGET or (member and all(group_tles[g] >= 2 for g in member)):
                    result['cases'].append(dict(name=case.get('name') or f'ケース{index + 1}', verdict='SKIPPED'))
                    if progress:
                        progress('JUDGING', index + 1, result['total'], result['verdict'] if result['verdict'] != 'AC' else None)
                    continue
            elif time.monotonic() >= deadline:
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
                item = interactive.execute(job, case, checker_artifact, case_diagnostic, score_file=aggregate)
                if job.get('easyTest'):
                    item['checkerLog'] = sample_preview(bytes(case_log), log_limit)
                item['name'] = case.get('name') or f'ケース{index + 1}'
                if item['verdict'] == 'JE':
                    return checker_error('interactor_execution_failed')
                if aggregate:
                    data, exit_code = item.pop('scoreFile'), item.pop('judgeExitCode')
                    try:
                        verdict, score, message = case_score(job['interactor'].get('protocol', 'legacy'), item['verdict'], data, exit_code)
                    except ScoreError as error:
                        diagnostic(f"{error}\n", keep=True)
                        return checker_error('judge_score_invalid')
                    item['verdict'] = verdict
                    if score is not None:
                        item['score'] = score
                        diagnostic(f"点: {score}\n")
                    if message:
                        diagnostic(f"結果ファイル: {message}\n")
            else:
                reply = sandbox.execute(dict(runtime=job['runtime'], input=base64.b64encode(case['input'].encode()).decode(),
                                             timeLimitMs=job['timeLimitMs'], memoryLimitMb=job['memoryLimitMb']))
                reply['index'] = index
                item = case_result(reply, index, case, job.get('generate', False), job.get('validate', False) or checker is not None)
                if checker is not None and item['verdict'] == 'AC':
                    protocol = checker.get('protocol', 'legacy')
                    checked = sandbox.execute(dict(runtime=checker['runtime'] + '-isolate', input=reply['output'], protocol=protocol,
                                                   timeLimitMs=5000, memoryLimitMb=512), artifact=checker_artifact,
                                              checker_files={'test-input': case['input'].encode(),
                                                             'expected-output': case['output'].encode(),
                                                             'submission-source': job['source'].encode()}, score_file=aggregate)
                    checked['index'] = index
                    verdict = sandbox.judge_verdict(checked, protocol, points=aggregate)
                    score, message = None, ''
                    if aggregate:
                        try:
                            verdict, score, message = case_score(protocol, verdict, checked['scoreFile'], checked['exitCode'])
                        except ScoreError as error:
                            diagnostic(f"ケース{index + 1}: JE\n" + checked.get('checkerLog', '') + '\n')
                            diagnostic(f"{error}\n", keep=True)
                            return checker_error('judge_score_invalid')
                    diagnostic(f"ケース{index + 1}: {verdict}" + ('' if score is None else f" (点: {score})") + '\n'
                               + checked.get('checkerLog', '') + '\n' + (f"結果ファイル: {message}\n" if message else ''))
                    if protocol == 'testlib' and checked['exitCode'] == 7 and not aggregate:
                        diagnostic(interactive.points_note(job))
                    if verdict == 'JE':
                        return checker_error('checker_execution_failed')
                    item['verdict'] = verdict
                    if score is not None:
                        item['score'] = score
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
                if scoring is not None:
                    # A case in several groups counts toward each of them.
                    for group in case_groups(job, case):
                        group_tles[group] += 1
            if scoring is None and tle_count >= 2 and not any(job.get(mode) for mode in ('easyTest', 'validate', 'generate')):
                result['cases'].extend(dict(name=remaining.get('name') or f'ケース{i + 1}', verdict='SKIPPED')
                                       for i, remaining in enumerate(job['cases'][index + 1:], index + 1))
                break
        if aggregate:
            # Run once with the checker limits; its stderr is as private as the checker log.
            reply = sandbox.execute(dict(runtime=scoring['aggregator']['runtime'] + '-isolate',
                                         input=base64.b64encode(aggregator_input(job, result['cases']).encode()).decode(),
                                         timeLimitMs=5000, memoryLimitMb=512), artifact=aggregator_artifact, stderr_log=True)
            stderr = reply.get('checkerLog', '')
            failure = ('OLE' if reply['overflow'] else 'MLE' if reply['oom'] else 'TLE' if reply['status'] == 'TO'
                       else 'RE' if reply['status'] or reply['exitCode'] or reply['signal'] else None)
            if failure:
                diagnostic(f"集計コードの実行失敗: {failure}\n" + stderr + '\n', keep=True)
                return checker_error('aggregator_execution_failed')
            try:
                scores = aggregator_scores(base64.b64decode(reply['output'], validate=True), scoring['groups'])
            except ScoreError as error:
                diagnostic(f"{error}\n" + stderr + '\n', keep=True)
                return checker_error('aggregator_output_invalid')
            if stderr:
                diagnostic('集計コードの標準エラー:\n' + stderr + '\n')
        elif scoring is not None:
            scores = group_scores(job, result['cases'])
        if scoring is not None:
            result['scoring'] = scoring_result(job, scores)
    finally:
        sandbox.ARTIFACT.unlink(missing_ok=True)
        checker_artifact.unlink(missing_ok=True)
        aggregator_artifact.unlink(missing_ok=True)
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
