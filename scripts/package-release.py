#!/usr/bin/env python3
"""Build deterministic source and optional prebuilt-runtime archives. Never publishes."""
import argparse
import gzip
import hashlib
import io
import json
import pathlib
import re
import tarfile

ROOT = pathlib.Path(__file__).resolve().parents[1]
p = argparse.ArgumentParser(description=__doc__)
p.add_argument('--output', default=str(ROOT / '.data/releases'))
p.add_argument('--runtime', help='Directory containing linux-amd64/ and linux-arm64/ runtime files')
a = p.parse_args()
version = (ROOT / 'VERSION').read_text().strip()
if not re.fullmatch(r'\d+\.\d+\.\d+(?:-[a-z0-9.]+)?', version):
    p.error('Invalid VERSION')
out = pathlib.Path(a.output)
out.mkdir(parents=True, exist_ok=True)
excluded = {'node_modules', '.data', '.tools', '.cache', '.git', '__pycache__', 'dist', 'test-results', 'playwright-report'}
roots = ['.github', 'apps', 'cmd', 'deploy', 'docs', 'internal', 'migrations', 'scripts']
files = [ROOT / n for n in ['.dockerignore', '.gitignore', 'AGENTS.md', 'README.md', 'LICENSE', 'VERSION', 'SECURITY.md', 'CONTRIBUTING.md', 'THIRD_PARTY_NOTICES.md', 'go.mod', 'go.sum']]
for name in roots:
    for f in sorted((ROOT / name).rglob('*')):
        rel = f.relative_to(ROOT)
        if any(part in excluded for part in rel.parts) or f.name.startswith('.env') or f.name == '.DS_Store' or f.suffix in {'.pem', '.key', '.pyc', '.tsbuildinfo'}:
            continue
        if f.is_symlink():
            raise SystemExit(f'Refusing release symlink: {rel}')
        if f.is_file():
            files.append(f)

def archive(name, entries):
    destination = out / name
    with destination.open('wb') as raw, gzip.GzipFile(fileobj=raw, mode='wb', filename='', mtime=0) as gz, tarfile.open(fileobj=gz, mode='w', format=tarfile.PAX_FORMAT) as tar:
        for path, data, executable in sorted(entries):
            info = tarfile.TarInfo(path)
            info.size = len(data)
            info.mode = 0o755 if executable else 0o644
            info.mtime = info.uid = info.gid = 0
            tar.addfile(info, io.BytesIO(data))
    return destination

prefix = 'cloudrail-' + version
manifest = {str(f.relative_to(ROOT)): hashlib.sha256(f.read_bytes()).hexdigest() for f in sorted(files)}
entries = [(prefix + '/' + str(f.relative_to(ROOT)), f.read_bytes(), f.suffix == '.sh') for f in files]
entries.append((prefix + '/SOURCE-MANIFEST.json', (json.dumps(manifest, indent=2, sort_keys=True) + '\n').encode(), False))
artifacts = [archive(prefix + '-source.tar.gz', entries)]
if a.runtime:
    for platform in ('linux-amd64', 'linux-arm64'):
        stage = pathlib.Path(a.runtime) / platform
        for required in ('server', 'agent', 'web/index.html'):
            if not (stage / required).is_file():
                raise SystemExit(f'Missing runtime artifact: {stage / required}')
        runtime = [(prefix + '/' + str(f.relative_to(stage)), f.read_bytes(), f.name in {'server', 'agent'}) for f in stage.rglob('*') if f.is_file()]
        for name in ('LICENSE', 'VERSION', 'THIRD_PARTY_NOTICES.md'):
            runtime.append((prefix + '/' + name, (ROOT / name).read_bytes(), False))
        artifacts.append(archive(prefix + '-' + platform + '.tar.gz', runtime))
(out / 'SHA256SUMS').write_text(''.join(hashlib.sha256(f.read_bytes()).hexdigest() + '  ' + f.name + '\n' for f in artifacts))
print('Created local release artifacts:', ', '.join(f.name for f in artifacts))
print('Checksums:', out / 'SHA256SUMS')
