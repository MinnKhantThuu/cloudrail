#!/usr/bin/env python3
"""Verify packaged source contents, repeatability and absence of local credentials."""
import hashlib
import json
import pathlib
import subprocess
import sys
import tarfile
import tempfile

root = pathlib.Path(__file__).resolve().parents[1]
version = (root / 'VERSION').read_text().strip()
prefix = 'cloudrail-' + version + '/'
secrets = []
env = root / 'deploy/local/.env'
if env.exists():
    secrets += [line.split('=', 1)[1].encode() for line in env.read_text().splitlines() if '=' in line and len(line.split('=', 1)[1]) >= 24]
owner = root / '.data/test-owner.json'
if owner.exists():
    secrets.append(json.loads(owner.read_text())['password'].encode())
with tempfile.TemporaryDirectory(prefix='cloudrail-package-check-') as temporary:
    outputs = [pathlib.Path(temporary) / n for n in ('first', 'second')]
    for out in outputs:
        subprocess.run([sys.executable, str(root / 'scripts/package-release.py'), '--output', str(out)], check=True, stdout=subprocess.DEVNULL)
    name = 'cloudrail-' + version + '-source.tar.gz'
    assert (outputs[0] / name).read_bytes() == (outputs[1] / name).read_bytes(), 'Source archive is not repeatable'
    with tarfile.open(outputs[0] / name) as tar:
        members = tar.getmembers()
        names = [m.name for m in members]
        assert len(names) == len(set(names)), 'Duplicate archive members'
        manifest = json.load(tar.extractfile(prefix + 'SOURCE-MANIFEST.json'))
        for m in members:
            assert m.isfile() and m.name.startswith(prefix) and '..' not in pathlib.PurePosixPath(m.name).parts
            assert not any(p in {'.data', '.env', 'node_modules', '.tools', '.cache', '.git'} for p in pathlib.PurePosixPath(m.name).parts)
            content = tar.extractfile(m).read()
            assert not any(value and value in content for value in secrets), 'Local credential found in archive'
            relative = m.name[len(prefix):]
            if relative != 'SOURCE-MANIFEST.json':
                assert manifest[relative] == hashlib.sha256(content).hexdigest()
        assert len(members) == len(manifest) + 1
print('PASS deterministic source archive, manifest checksums, safe paths and no local installation/test credentials')
