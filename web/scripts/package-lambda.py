"""Package Nitro's Lambda output, including public assets at their expected paths."""
from pathlib import Path
from zipfile import ZipFile, ZIP_DEFLATED

root = Path(__file__).resolve().parents[1]
output = root / '.output-lambda'
if not (output / 'server/index.mjs').is_file():
    raise SystemExit('Run npm run build:lambda first')
(root / '.build').mkdir(exist_ok=True)
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
    # The Lambda handler wraps Nitro's to expose API Gateway's source IP.
    bundle.write(root / 'lambda/index.mjs', 'lambda/index.mjs')
print(f'Packaged {archive.name}: {archive.stat().st_size:,} bytes')
