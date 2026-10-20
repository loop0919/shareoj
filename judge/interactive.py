"""Two isolated programs connected by bounded, byte-preserving pipes."""
import os
import telemetry
from pathlib import Path
import select
import signal
import subprocess
import time

import sandbox
from runtimes import RUNTIMES

BUFFER_LIMIT = 65536


def relay(processes, observe, wall, diagnostic, protocol='legacy', points=False):
    """observe returns completed metrics or an early output-limit observation."""
    buffers = [bytearray(), bytearray()]
    totals = [0, 0]
    outputs = [p.stdout for p in processes]
    inputs = [p.stdin for p in processes]
    for stream in outputs + inputs:
        os.set_blocking(stream.fileno(), False)
    deadline = time.monotonic() + wall
    while True:
        metrics = [observe(i) for i in range(2)]
        errors = ['OLE' if totals[i] > sandbox.OUTPUT_LIMIT else sandbox.failure_verdict(m) for i, m in enumerate(metrics)]
        # In aggregate mode quitp ends like _ok: the submission must still exit normally (ADR 0025).
        if points and errors[1] == 'RE' and sandbox.judge_verdict(metrics[1], protocol, points=True) == 'AC':
            errors[1] = None
        # An isolate wall timeout belongs to the conversation, including input waits.
        wall_expired = any(m and m.get('wallTimeout') for m in metrics)
        if errors[1] in ('MLE', 'OLE') or (errors[1] == 'TLE' and not metrics[1].get('wallTimeout')):
            return 'JE'
        if errors[0]:
            return errors[0]
        if errors[1] == 'RE':
            return sandbox.judge_verdict(metrics[1], protocol)
        if wall_expired or time.monotonic() >= deadline:
            diagnostic('対話全体の経過時間超過（応答待ちを含む）\n')
            return 'TLE'
        if all(p.poll() is not None for p in processes) and all(s.closed for s in outputs):
            return 'AC'
        reads = [outputs[i] for i in range(2) if not outputs[i].closed and
                 (inputs[1-i].closed or len(buffers[i]) < BUFFER_LIMIT)]
        writes = [inputs[1-i] for i in range(2) if buffers[i] and not inputs[1-i].closed]
        ready_read, ready_write, _ = select.select(reads, writes, [], .02)
        for i in range(2):
            source, target = outputs[i], inputs[1-i]
            if source in ready_read:
                try:
                    data = os.read(source.fileno(), BUFFER_LIMIT - len(buffers[i]) if not target.closed else BUFFER_LIMIT)
                except BlockingIOError:
                    data = None
                if data == b'':
                    source.close()
                elif data:
                    totals[i] += len(data)
                    diagnostic(('提出 → ジャッジ: ' if i == 0 else 'ジャッジ → 提出: ') + data.decode(errors='replace') + '\n')
                    if not target.closed:
                        buffers[i].extend(data)
            if target in ready_write:
                try:
                    written = os.write(target.fileno(), buffers[i])
                    del buffers[i][:written]
                except BlockingIOError:
                    pass
                except BrokenPipeError:
                    target.close()
                    buffers[i].clear()
            if source.closed and not buffers[i] and not target.closed:
                target.close()


def points_note(job):
    # Jobs without scoring keep the diagnostic they had before ADR 0025.
    if job.get('scoring') is None:
        return 'testlibの部分点（_points）は未対応です。\n'
    return 'testlibの部分点（_points）は、「集計コードで採点する」をオンにした問題でだけ使えます。\n'


def execute(job, case, artifact, diagnostic, score_file=False):
    wall = 3 * (job['timeLimitMs'] / 1000 + 5) + 1
    boxes, metas, processes = [], [], []
    replies = [None, None]
    stderr = [b'', b'']
    try:
        # Prepare both boxes before starting either clock.
        commands = []
        for i, (name, binary) in enumerate([(job['runtime'], sandbox.ARTIFACT),
                                           (job['interactor']['runtime'] + '-isolate', artifact)]):
            init = sandbox.invoke(['--init'], box_id=i)
            if init.returncode:
                raise telemetry.PlatformError('isolate_init_failed')
            box = Path(init.stdout.decode().strip()) / 'box'
            boxes.append(box)
            meta = sandbox.META.with_name(f'interactive-{i}.meta')
            meta.unlink(missing_ok=True)
            metas.append(meta)
            files = None if i == 0 else {'test-input': case['input'].encode(),
                                        'expected-output': case['output'].encode(),
                                        'submission-source': job['source'].encode()}
            command = sandbox.prepare_program(box, RUNTIMES[name], binary, files,
                                              protocol=job['interactor'].get('protocol', 'legacy') if i else 'legacy', interactive=True,
                                              result_file=score_file and i == 1)
            memory = job['memoryLimitMb'] if i == 0 else 256
            if RUNTIMES[name]['artifact'] == 'java':
                command = sandbox.java_heap(command, memory)
            args = sandbox.run_args(name, job['timeLimitMs'] / 1000 if i == 0 else 5,
                                    wall, memory, meta=meta, interactive=True, interactor=i == 1)
            commands.append([sandbox.ISOLATE, '--cg', f'--box-id={i}', *args, '--', *command])
        for command in commands:
            processes.append(subprocess.Popen(command, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                              stderr=subprocess.DEVNULL, start_new_session=True,
                                              env={'PATH': '/usr/bin:/bin', 'LANG': 'C', 'LC_ALL': 'C'}, bufsize=0))

        def observe(i):
            if replies[i] is not None:
                return replies[i]
            try:
                stderr[i] = sandbox.regular_read(boxes[i] / 'stderr', 65536)
            except FileNotFoundError:
                pass  # isolate has not opened the child's stderr yet.
            if processes[i].poll() is not None:
                if processes[i].returncode not in (0, 1):
                    raise telemetry.PlatformError('isolate_execution_failed')
                raw = metas[i].read_text()
                replies[i] = sandbox.metadata(raw)
                replies[i].update(overflow=len(stderr[i]) > 65536 or replies[i]['signal'] == 25,
                                  wallTimeout='wall clock' in raw, output='')
                return replies[i]
            if len(stderr[i]) > 65536:
                return dict(status='', exitCode=0, signal=0, oom=False, overflow=True)
            return None

        protocol = job['interactor'].get('protocol', 'legacy')
        verdict = relay(processes, observe, wall, diagnostic, protocol, points=score_file)
        # Capture natural exits before signalling the still-running peer.
        observed = [observe(i) for i in range(2)]
        for i, process in enumerate(processes):
            if process.poll() is None:
                process.send_signal(signal.SIGTERM)
        for process in processes:
            process.wait(timeout=5)
        if observed[0] is None or 'cpuTimeMs' not in observed[0]:
            observed[0] = sandbox.metadata(metas[0].read_text())
        diagnostic('対話判定: ' + verdict + '\nジャッジ標準エラー:\n' + stderr[1][:65536].decode(errors='replace') + '\n')
        if job['interactor'].get('protocol') == 'testlib' and observed[1] and observed[1]['exitCode'] == 7 and not score_file:
            diagnostic(points_note(job))
        item = dict(verdict=verdict, **{key: observed[0][key] for key in ('cpuTimeMs', 'wallTimeMs', 'memoryBytes')})
        if score_file:
            # Both programs have stopped, so the file can no longer change.
            item.update(scoreFile=sandbox.read_judge_file(boxes[1], protocol),
                        judgeExitCode=observed[1]['exitCode'] if observed[1] and 'exitCode' in observed[1] else None)
        return item
    finally:
        for process in processes:
            if process.poll() is None:
                process.kill()
                process.wait(timeout=5)
            for stream in (process.stdin, process.stdout):
                stream.close()
        failed_cleanup = False
        for i in reversed(range(len(boxes))):
            try:
                failed_cleanup |= sandbox.invoke(['--cleanup'], box_id=i).returncode != 0
            except Exception:
                failed_cleanup = True
        for meta in metas:
            meta.unlink(missing_ok=True)
        if failed_cleanup:
            raise telemetry.FatalPlatformError('isolate_cleanup_failed')
