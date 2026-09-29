#!/usr/bin/python3
"""Enroll the installed runtime while dispatch and the worker are stopped."""
import hashlib
import json
from pathlib import Path
from host import ASSETS, platform_fingerprint, runtime_inventory

files = [str(Path('/opt/judge') / name) for name in
         ('host.py', 'sandbox.py', 'interactive.py', 'worker.py', 'telemetry.py', 'pool.py', 'runtimes.py', 'smoke.py', 'interactive_smoke.py', 'testlib_smoke.py', 'language-smoke.json', 'sandbox-etc/passwd', 'sandbox-etc/group')]
files += ['/usr/local/bin/isolate', '/usr/local/etc/isolate', '/etc/systemd/system/judge-worker.service',
          '/opt/judge/assets/isolate-commit', '/opt/judge/assets/runtime-archive.sha256']
(ASSETS / 'runtime-tree.json').write_text(json.dumps(runtime_inventory(), sort_keys=True, separators=(',', ':')))
files.append(str(ASSETS / 'runtime-tree.json'))
manifest = dict(platform=platform_fingerprint(), files={
    name: hashlib.sha256(Path(name).read_bytes()).hexdigest() for name in files
})
raw = json.dumps(manifest, sort_keys=True, separators=(',', ':')).encode()
(ASSETS / 'manifest.json').write_bytes(raw)
print('sha256:' + hashlib.sha256(raw).hexdigest())
