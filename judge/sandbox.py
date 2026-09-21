#!/usr/bin/python3
"""Trusted isolate controller on the dedicated Lightsail host."""
import base64
import telemetry
import math
import os
from pathlib import Path
import shutil
import stat
import subprocess
import io
import zipfile

from runtimes import RUNTIMES, ROOT, ENVIRONMENT


ISOLATE = '/usr/local/bin/isolate'
META = Path('/run/judge/meta')
ARTIFACT = Path('/run/judge/main')
OUTPUT_LIMIT = 16 * 1024 * 1024


def invoke(args, timeout=10, *, box_id=0):
    with telemetry.operation('isolate_invocation_failed'):
        return subprocess.run([ISOLATE, '--cg', f'--box-id={box_id}', *args],
                          stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                          stderr=subprocess.DEVNULL, timeout=timeout, check=False,
                          env={'PATH': '/usr/bin:/bin', 'LANG': 'C', 'LC_ALL': 'C'})


def metadata(text):
    data = {}
    for line in text.splitlines():
        key, sep, value = line.partition(':')
        if not sep or key in data:
            raise ValueError('invalid isolate metadata')
        data[key] = value
    if data.get('status') == 'XX':
        raise telemetry.PlatformError('isolate_metadata_failed')
    values = {}
    for src, dst, scale in [('time', 'cpuTimeMs', 1000), ('time-wall', 'wallTimeMs', 1000),
                            ('cg-mem', 'memoryBytes', 1024)]:
        value = float(data[src])
        if not math.isfinite(value) or value < 0:
            raise ValueError('invalid measurement')
        values[dst] = math.ceil(value * scale)
    values.update(status=data.get('status', ''), oom='cg-oom-killed' in data,
                  exitCode=int(data.get('exitcode', '0')), signal=int(data.get('exitsig', '0')))
    return values


def regular_read(path, limit):
    # Never follow links created by a submission, or open FIFOs/devices.
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(fd, 'rb') as file:
        if not stat.S_ISREG(os.fstat(file.fileno()).st_mode):
            raise ValueError('not a regular file')
        return file.read(limit + 1)


def execute(request, compile_phase=False, *, artifact=None, checker_files=None):
    artifact = ARTIFACT if artifact is None else artifact
    runtime = RUNTIMES[request.get('runtime', 'cpp17-isolate')]
    if compile_phase:
        source = request.get('source')
        if not isinstance(source, str) or not 0 < len(source.encode()) <= 65536 or '\0' in source:
            raise ValueError('invalid source')
        memory, cpu, wall = 1024, 30, 40
    else:
        memory = request.get('memoryLimitMb')
        ms = request.get('timeLimitMs')
        if type(memory) is not int or not 64 <= memory <= 512 or type(ms) is not int or not 100 <= ms <= 5000 or ms % 100:
            raise ValueError('invalid limits')
        cpu, wall = ms / 1000, 3 * ms / 1000 + 1
    init = invoke(['--init'])
    if init.returncode:
        raise telemetry.PlatformError('isolate_init_failed')
    # --init prints the box root; /box maps its box/ subdirectory.
    box = Path(init.stdout.decode().strip()) / 'box'
    try:
        META.unlink(missing_ok=True)
        if compile_phase:
            (box / runtime['source']).write_text(source)
            prepare_dependencies(box, runtime)
            if runtime['source'] == 'main.go':
                for name in ('go.mod', 'go.sum'):
                    shutil.copyfile(ROOT + '/go-deps/' + name, box / name)
                (box / 'vendor').mkdir()
            command = runtime['compile']
        else:
            data = base64.b64decode(request.get('input', ''), validate=True)
            if len(data) > 16 * 1024 * 1024:
                raise ValueError('input limit')
            protocol = request.get('protocol', 'legacy')
            if checker_files is not None and protocol == 'testlib':
                checker_files = dict(checker_files, **{'submission-output': data})
                data = b''
            command = prepare_program(box, runtime, artifact, checker_files, protocol=protocol)
            (box / 'input').write_bytes(data)
        args = run_args(request.get('runtime', 'cpp17-isolate'), cpu, wall, memory,
                        compile_phase=compile_phase)
        if not compile_phase:
            args.insert(-1, '--stdin=input')
        result = invoke([*args, '--', *command], timeout=wall + 10)
        if result.returncode not in (0, 1):
            raise telemetry.PlatformError('isolate_execution_failed')
        metrics = metadata(META.read_text())
        stdout = regular_read(box / 'stdout', OUTPUT_LIMIT)
        stderr = regular_read(box / 'stderr', 65536)
        overflow = len(stdout) > OUTPUT_LIMIT or len(stderr) > 65536 or metrics['signal'] == 25
        if compile_phase:
            success = not (result.returncode or overflow or metrics['oom'] or metrics['status'])
            if success:
                binary = collect_artifact(box, runtime)
                if not binary or len(binary) > 32 * 1024 * 1024:
                    success = False
                else:
                    artifact.write_bytes(binary)
                    artifact.chmod(0o500)
            # Roslyn writes compiler diagnostics to stdout; other compilers use stderr.
            return {'compiled': success, 'compileLog': (stderr + stdout)[:65536].decode(errors='replace')}
        metrics.update(output=base64.b64encode(stdout[:OUTPUT_LIMIT]).decode(), overflow=overflow)
        if checker_files is not None:
            metrics['checkerLog'] = stderr[:65536].decode(errors='replace')
        return metrics
    finally:
        # isolate cleanup destroys the box and its cgroup, including descendants.
        try:
            if invoke(['--cleanup']).returncode:
                raise telemetry.FatalPlatformError('isolate_cleanup_failed')
        except Exception:
            raise telemetry.FatalPlatformError('isolate_cleanup_failed') from None


def run_args(runtime, cpu, wall, memory, *, compile_phase=False, meta=None, interactive=False):
    # Metadata and saved artifacts remain outside the sandbox.
    environment = list(ENVIRONMENT) + RUNTIMES[runtime].get('env', [])
    if runtime == 'csharp14-isolate':
        environment.append('DOTNET_GCHeapHardLimit=' + hex((512 if compile_phase else 128 if memory == 256 else 192) * 1024 * 1024))
    if runtime == 'go127-isolate':
        environment.append('GOMEMLIMIT=' + ('160MiB' if memory == 256 else '384MiB'))
    if runtime == 'ruby-truffle40-isolate':
        environment.append('RUBYOPT=--vm.Xmx' + str(640 if compile_phase else 96 if memory == 256 else 256) + 'm')
    if runtime.startswith(('javascript-node', 'typescript-node')) or (compile_phase and runtime.startswith('typescript-bun')):
        environment.append('NODE_OPTIONS=--max-old-space-size=' + str(640 if compile_phase else 128 if memory == 256 else 320))
    if runtime.startswith(('javascript-deno', 'typescript-deno')):
        environment.append('DENO_V8_FLAGS=--max-old-space-size=' + str(640 if compile_phase else 128 if memory == 256 else 320))
    return [f'--meta={META if meta is None else meta}', f'--time={cpu}', f'--wall-time={wall}',
            f'--cg-mem={memory * 1024}', '--processes=32' if interactive else '--processes=64',
            # Pinned isolate 2.7: allow file locks only for the fixed Go build command.
            # Pure Go compilation does not execute submitted code; execution keeps all filters.
            *(['--syscalls=65531'] if compile_phase and runtime == 'go127-isolate' else []),
            '--open-files=256' if compile_phase and runtime in (
                'csharp14-isolate', 'typescript-node24-isolate', 'typescript-bun14-isolate') else '--open-files=64',
            # Leave one KiB beyond the output budget so catching SIGXFSZ/EFBIG still yields OLE.
            '--fsize=32768' if compile_phase else f'--fsize={OUTPUT_LIMIT // 1024 + 1}',
            *([] if interactive else ['--stdout=stdout']), '--stderr=stderr',
            '--env=PATH=/usr/bin:/bin', '--dir=/etc=/opt/judge/sandbox-etc',
            *(['--dir=' + ROOT] if runtime != 'cpp17-isolate' else []),
            # isolate removes symlinks from /box before running the compiler.
            *(['--dir=/box/vendor=' + ROOT + '/go-deps/vendor']
              if compile_phase and runtime == 'go127-isolate' else []),
            *(['--dir=/box/node_modules=' + ROOT + '/' + RUNTIMES[runtime]['node_modules']]
              if 'node_modules' in RUNTIMES[runtime] else []),
            *['--env=' + value for value in environment], '--run']


def prepare_dependencies(box, runtime):
    if 'node_modules' in runtime:
        (box / 'node_modules').mkdir(exist_ok=True)
    if runtime.get('deno_cache'):
        # This source is operator-owned. Never share a writable cache across submissions.
        shutil.copytree(ROOT + '/deno-deps/cache', box / 'deno-cache')
        for path in [box / 'deno-cache', *(box / 'deno-cache').rglob('*')]:
            path.chmod(0o777 if path.is_dir() else 0o666)


def prepare_program(box, runtime, artifact, files=None, *, protocol='legacy', interactive=False):
    prepare_dependencies(box, runtime)
    program = runtime.get('program', 'main.dll' if runtime['artifact'] == 'dotnet' else 'main')
    shutil.copyfile(artifact, box / program)
    (box / program).chmod(0o555)
    for name in runtime.get('files', []):
        shutil.copyfile(ROOT + '/dotnet-libs/' + name, box / name)
        (box / name).chmod(0o444)
    command = list(runtime['run'])
    if files is not None:
        names = ('test-input', 'expected-output') if protocol == 'testlib' else ('test-input', 'expected-output', 'submission-source')
        for name in names:
            (box / name).write_bytes(files[name])
            (box / name).chmod(0o444)
        if protocol == 'testlib':
            output = 'test-output' if interactive else 'submission-output'
            (box / output).write_bytes(b'' if interactive else files['submission-output'])
            (box / output).chmod(0o666 if interactive else 0o444)
            command += ['/box/test-input', '/box/' + output, '/box/expected-output']
        else:
            (box / 'score').write_bytes(b'')
            (box / 'score').chmod(0o666)
            command += ['/box/test-input', '/box/expected-output', '/box/submission-source', '/box/score']
        if runtime['artifact'] == 'java':
            command.insert(1, '-ea')
    return command


def judge_verdict(metrics, protocol='legacy'):
    if metrics['overflow'] or metrics['oom'] or metrics['status'] == 'TO':
        return 'JE'
    if protocol == 'testlib':
        if metrics['signal'] or metrics['status'] not in ('', 'RE'):
            return 'JE'
        return {0: 'AC', 1: 'WA', 2: 'WA', 4: 'WA', 8: 'WA'}.get(metrics['exitCode'], 'JE')
    return 'WA' if metrics['status'] or metrics['exitCode'] or metrics['signal'] else 'AC'


def collect_artifact(box, runtime):
    if runtime['artifact'] == 'source':
        return regular_read(box / runtime['source'], 65536)
    if runtime['artifact'] == 'generated-source':
        if not stat.S_ISDIR((box / runtime['compile_output']).parent.lstat().st_mode):
            raise ValueError('invalid compiler output directory')
        return regular_read(box / runtime['compile_output'], 32 * 1024 * 1024)
    if runtime['artifact'] != 'java':
        return regular_read(box / ('main.dll' if runtime['artifact'] == 'dotnet' else 'main'), 32 * 1024 * 1024)
    # Build a jar ourselves; never execute a submission-supplied packager or
    # traverse symlinked directories while reading javac output as root.
    output = io.BytesIO()
    count, size = 0, 0
    root = box / 'classes'
    if not stat.S_ISDIR(root.lstat().st_mode):
        raise ValueError('invalid class directory')
    with zipfile.ZipFile(output, 'w') as jar:
        for directory, dirs, files in os.walk(root, followlinks=False):
            for name in dirs:
                if not stat.S_ISDIR((Path(directory) / name).lstat().st_mode):
                    raise ValueError('invalid class directory')
            for name in files:
                if not name.endswith('.class'):
                    raise ValueError('unexpected compiler output')
                path = Path(directory) / name
                data = regular_read(path, 32 * 1024 * 1024)
                count += 1
                size += len(data)
                if count > 4096 or size > 32 * 1024 * 1024:
                    raise ValueError('artifact limit')
                jar.writestr(str(path.relative_to(root)), data)
    return output.getvalue()
