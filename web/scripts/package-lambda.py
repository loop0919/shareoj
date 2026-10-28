"""Package Nitro's Lambda output, including public assets at their expected paths."""
from pathlib import Path
from zipfile import ZipFile, ZipInfo, ZIP_DEFLATED

import bun_release

root = Path(__file__).resolve().parents[1]
output = root / '.output-lambda'
if not (output / 'server/index.mjs').is_file():
    raise SystemExit('Run bun run build:lambda first')
(root / '.build').mkdir(exist_ok=True)
bun_release.check_pins()
# The function runs on arm64; bootstrap starts this Bun for the provided.al2023 runtime.
bun = bun_release.binary('bun-linux-aarch64.zip', root / '.build')
archive = root / '.build/web.zip'
with ZipFile(archive, 'w', ZIP_DEFLATED) as bundle:
    def add_directory(directory, ancestors=frozenset()):
        resolved = directory.resolve()
        if not resolved.is_relative_to(output.resolve()) or resolved in ancestors:
            raise RuntimeError(f'Unsafe or circular bundle link: {directory}')
        for path in sorted(directory.iterdir()):
            if path.is_dir():
                add_directory(path, ancestors | {resolved})
            elif path.is_file():
                bundle.write(path, path.relative_to(output))
    # Nitro uses directory symlinks for dependencies; dereference them in the zip.
    add_directory(output)
    # The Lambda handler wraps Nitro's to expose API Gateway's source IP. The Node.js
    # runtime calls it directly; on provided.al2023, runtime.mjs calls it.
    bundle.write(root / 'lambda/index.mjs', 'lambda/index.mjs')
    bundle.write(root / 'lambda/runtime.mjs', 'lambda/runtime.mjs')
    for name, data in [('bootstrap', (root / 'lambda/bootstrap').read_bytes()), ('bin/bun', bun)]:
        info = ZipInfo(name)
        info.create_system = 3  # Unix, so that Lambda reads the mode below.
        info.external_attr = 0o100755 << 16
        bundle.writestr(info, data, compress_type=ZIP_DEFLATED)
    unpacked = sum(entry.file_size for entry in bundle.infolist())
# Lambda refuses a package over 250 MB unzipped.
if unpacked >= 250 * 1024 * 1024:
    raise SystemExit(f'{archive.name} unpacks to {unpacked:,} bytes, over Lambda\'s limit')
print(f'Packaged {archive.name}: {archive.stat().st_size:,} bytes, {unpacked:,} unpacked')
