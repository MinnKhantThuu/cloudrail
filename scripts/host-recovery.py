#!/usr/bin/env python3
"""Cold backup and empty-host recovery of one trusted Cloudrail installation."""
import argparse
import hashlib
import ipaddress
import json
import os
import pathlib
import posixpath
import re
import shutil
import signal
import subprocess
import sys
import tarfile
import time
import uuid
import maintenance as m

ROOT = m.ROOT
ROLES = ('postgres', 'registry', 'proxy', 'buildkit', 'server', 'agent')
PERSISTENT = {'database', 'registry-data', 'backups', 'server-state', 'agent-state', 'certificates'}
NAME = re.compile(r'(?:cloudrail_[a-z-]+|cloudrail-volume-[a-f0-9]+)\Z')
IMAGE = re.compile(r'[A-Za-z0-9][A-Za-z0-9._:/-]*@sha256:[a-f0-9]{64}\Z')


def docker(*args):
    return m.run(['docker', *args], capture=True)


def inspect(kind, name):
    return json.loads(docker(kind, 'inspect', name))[0]


def host():
    info = json.loads(docker('info', '--format', '{{json .}}'))
    if info['OSType'] != 'linux':
        raise RuntimeError('A Linux Docker daemon is required')
    architecture = {'x86_64': 'amd64', 'aarch64': 'arm64'}.get(info['Architecture'], info['Architecture'])
    try:
        boot_id = pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip()
    except OSError:
        boot_id = None
    return {'id': info['ID'], 'architecture': architecture, 'bootID': boot_id}


def checksum(path):
    h = hashlib.sha256()
    with path.open('rb') as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b''):
            h.update(chunk)
    return h.hexdigest()


def config_hashes():
    return {name: checksum(ROOT / name) for name in
            ('deploy/local/compose.yaml', 'deploy/local/buildkit.toml', 'deploy/vps/public.yaml')}


def volume_archive(path):
    """Reject escape paths, special files and writes through archived symlinks."""
    def clean(name):
        if name.startswith('/') or '\\' in name or '\x00' in name:
            raise RuntimeError('Unsafe archive path')
        result = posixpath.normpath(name)
        if result == '..' or result.startswith('../'):
            raise RuntimeError('Archive path escapes its volume')
        return result
    def member_path(name):
        # Normalizing a/../b before extraction can hide traversal through a
        # symlink at a. Our own tar exports never need parent path components.
        if '..' in name.split('/'):
            raise RuntimeError('Archive member contains parent traversal')
        return clean(name)
    with tarfile.open(path, 'r:') as archive:
        entries, links, total = {}, set(), 0
        for entry in archive:
            name = member_path(entry.name)
            if name == '.' and not entry.isdir():
                raise RuntimeError('Archive volume root must be a directory')
            if name in entries:
                raise RuntimeError('Duplicate volume archive path')
            if not (entry.isfile() or entry.isdir() or entry.issym() or entry.islnk()):
                raise RuntimeError('Unsupported special file in volume backup')
            if entry.issym():
                if entry.linkname.startswith('/'):
                    raise RuntimeError('Absolute volume symlink is not portable')
                clean(posixpath.join(posixpath.dirname(name), entry.linkname))
                links.add(name)
            if entry.islnk():
                member_path(entry.linkname)
            entries[name] = entry
            total += entry.size
        for name, entry in entries.items():
            parents = pathlib.PurePosixPath(name).parents
            if any(str(parent) in links for parent in parents):
                raise RuntimeError('Archive writes through a symlink')
            if entry.islnk():
                target = entries.get(clean(entry.linkname))
                if target is None or not target.isfile():
                    raise RuntimeError('Hard link must target an archived regular file')
        for name in links:
            # Resolve targets in filesystem order, including chains. Lexical
            # normalization alone misses d/up/.. when d/up points to '..'.
            resolved = []
            pending = name.split('/')
            followed = 0
            while pending:
                part, *pending = pending
                if part in ('', '.'):
                    continue
                if part == '..':
                    if not resolved:
                        raise RuntimeError('Symlink chain escapes its volume')
                    resolved.pop()
                    continue
                resolved.append(part)
                candidate = '/'.join(resolved)
                if candidate in links:
                    followed += 1
                    if followed > 40:
                        raise RuntimeError('Cyclic or excessive volume symlink chain')
                    resolved.pop()
                    pending = entries[candidate].linkname.split('/') + pending
        return total


def helper(image, volume, reading=True):
    args = ['docker', 'run', '--rm', '-i', '--log-driver', 'none', '--network', 'none', '--read-only',
            '--security-opt', 'no-new-privileges', '--cap-drop', 'ALL', '--cap-add', 'DAC_OVERRIDE']
    if not reading:
        args += ['--cap-add', 'CHOWN', '--cap-add', 'FOWNER']
    args += ['--mount', 'type=volume,src=' + volume + ',dst=/volume' + (',readonly' if reading else ''),
             '--entrypoint', 'tar', image, '-C', '/volume', '-cpf' if reading else '-xpf', '-']
    if reading:
        args.append('.')
    return args


def check_healthy(names, timeout=180):
    end = time.monotonic() + timeout
    while time.monotonic() < end:
        states = [inspect('container', name)['State'] for name in names]
        if all(s['Running'] and s.get('Health', {}).get('Status', 'healthy') == 'healthy' for s in states):
            return
        time.sleep(1)
    raise RuntimeError('Restored runtime did not become healthy; inspect its logs before resuming work')


def workspace_inventory():
    query = """SELECT COALESCE(json_agg(t),'[]'::json) FROM (
      SELECT s.id,s.active_id,s.desired_state,s.settings,d.image
      FROM services s LEFT JOIN deployments d ON d.id=s.active_id) t"""
    return json.loads(m.sql(query))


def backup(directory):
    directory = pathlib.Path(directory).resolve()
    if directory.exists():
        raise RuntimeError('Backup destination already exists')
    m.idle()
    current_host = host()
    core = {role: inspect('container', 'cloudrail-' + role + '-1') for role in ROLES}
    for role, item in core.items():
        if item['Config']['Labels'].get('com.docker.compose.project') != 'cloudrail':
            raise RuntimeError('A platform container is owned by another installation')
        if not item['State']['Running']:
            raise RuntimeError('Start the complete platform before cold backup')
    version = (ROOT / 'VERSION').read_text().strip()
    if docker('exec', 'cloudrail-server-1', 'cat', '/usr/share/cloudrail/VERSION') != version:
        raise RuntimeError('Use the source version matching the installed runtime')
    directory.mkdir(parents=True, mode=0o700)
    record = {'format': 1, 'complete': False, 'id': uuid.uuid4().hex,
              'host': current_host, 'version': version, 'config': config_hashes(),
              'public': (ROOT / '.data/public-installation').exists(), 'images': {}, 'volumes': [], 'files': {}}
    path = directory / 'manifest.json'
    m.save(path, record)
    running = []
    frozen = False
    try:
        frozen = True
        m.compose('stop', 'server', 'agent')
        m.idle()
        services = workspace_inventory()
        record['applicationImages'] = sorted({s['image'] for s in services if s['image']})
        known_services = {s['id'] for s in services}
        names = docker('ps', '-aq', '--filter', 'label=cloudrail.managed=true').splitlines()
        applications = [inspect('container', name) for name in names]
        for item in applications:
            if item['Config']['Labels'].get('cloudrail.service') not in known_services:
                raise RuntimeError('Unknown managed container; inspect ownership before backup')
        volume_names = {s['settings']['volumeName'] for s in services if s['settings'].get('volumeName')}
        available = set(docker('volume', 'ls', '-q').splitlines())
        if any(s['active_id'] and s['settings'].get('volumeName') and s['settings']['volumeName'] not in available for s in services):
            raise RuntimeError('An active application volume is missing')
        volume_names &= available  # Configured but never deployed services have no volume yet.
        volume_names |= {'cloudrail_' + name for name in PERSISTENT} & available
        for item in core.values():
            for mount in item['Mounts']:
                if mount['Type'] == 'volume' and mount['Name'].removeprefix('cloudrail_') in PERSISTENT:
                    volume_names.add(mount['Name'])
        if record['public'] and 'cloudrail_certificates' not in volume_names:
            raise RuntimeError('Public installation is missing certificate storage')
        owned_ids = {item['Id'] for item in core.values()} | {item['Id'] for item in applications}
        for name in sorted(volume_names):
            if not NAME.fullmatch(name):
                raise RuntimeError('Unexpected persistent volume name')
            item = inspect('volume', name)
            labels = item.get('Labels') or {}
            if name.startswith('cloudrail-volume-'):
                if labels.get('cloudrail.service') != name.removeprefix('cloudrail-volume-'):
                    raise RuntimeError('Application volume ownership does not match')
            elif labels.get('com.docker.compose.project') != 'cloudrail' or labels.get('com.docker.compose.volume') != name.removeprefix('cloudrail_'):
                raise RuntimeError('Platform volume ownership does not match')
            if item['Driver'] != 'local' or item.get('Options'):
                raise RuntimeError('External volume drivers require their own snapshot procedure')
            users = docker('ps', '-q', '--no-trunc', '--filter', 'volume=' + name).splitlines()
            if any(identifier not in owned_ids for identifier in users):
                raise RuntimeError('An unrelated running container shares a persistent volume')
            record['volumes'].append({'name': name, 'labels': item.get('Labels') or {}})
        running = [item['Name'].lstrip('/') for item in applications if item['State']['Running']]
        if running:
            docker('stop', '--time', '30', *running)
        m.compose('stop', 'proxy', 'registry', 'buildkit', 'postgres')
        for role, item in core.items():
            ref = 'cloudrail-host-' + role + ':' + record['id']
            docker('tag', item['Image'], ref)
            record['images'][role] = {'id': item['Image'], 'ref': ref}
        shutil.copyfile(ROOT / 'deploy/local/.env', directory / 'installation.env')
        for item in record['volumes']:
            target = directory / (item['name'] + '.tar')
            with target.open('wb') as output:
                subprocess.run(helper(core['agent']['Image'], item['name']), check=True, stdout=output)
            volume_archive(target)
        docker('image', 'save', '-o', str(directory / 'platform-images.tar'),
               *(item['ref'] for item in record['images'].values()))
        for item in directory.iterdir():
            if item.name != 'manifest.json':
                item.chmod(0o600)
                record['files'][item.name] = {'sha256': checksum(item), 'bytes': item.stat().st_size}
        record['complete'] = True
        m.save(path, record)
    finally:
        if frozen:
            docker('start', *(core[role]['Id'] for role in ('postgres', 'registry', 'proxy', 'buildkit')))
            if running:
                docker('start', *running)
            docker('start', core['server']['Id'], core['agent']['Id'])
            check_healthy(['cloudrail-' + role + '-1' for role in ROLES])
    print('PASS cold host backup; original platform resumed. Export this private directory off-server:', directory)


def validate(directory):
    record = json.loads((directory / 'manifest.json').read_text())
    if record.get('format') != 1 or record.get('complete') is not True:
        raise RuntimeError('Backup is incomplete or unsupported')
    if not re.fullmatch(r'[a-f0-9]{32}', record['id']):
        raise RuntimeError('Invalid backup identity')
    required = {'installation.env', 'platform-images.tar'}
    volumes = set()
    for item in record['volumes']:
        name = item['name']
        if not NAME.fullmatch(name) or name in volumes:
            raise RuntimeError('Invalid or duplicate volume')
        volumes.add(name)
        required.add(name + '.tar')
    if not {'cloudrail_database', 'cloudrail_server-state', 'cloudrail_agent-state', 'cloudrail_registry-data', 'cloudrail_backups'} <= volumes:
        raise RuntimeError('Missing platform persistent volumes')
    if record['public'] and 'cloudrail_certificates' not in volumes:
        raise RuntimeError('Missing public certificate storage')
    if set(record['files']) != required or set(record['images']) != set(ROLES):
        raise RuntimeError('Incomplete backup inventory')
    for name, expected in record['files'].items():
        path = directory / name
        if path.is_symlink() or not path.is_file() or path.stat().st_size != expected['bytes'] or checksum(path) != expected['sha256']:
            raise RuntimeError('Backup checksum or size mismatch: ' + name)
    for role, image in record['images'].items():
        if image['ref'] != 'cloudrail-host-' + role + ':' + record['id'] or not re.fullmatch(r'sha256:[a-f0-9]{64}', image['id']):
            raise RuntimeError('Invalid platform image identity')
    for ref in record['applicationImages']:
        if not IMAGE.fullmatch(ref):
            raise RuntimeError('Invalid application image digest')
    record['requiredBytes'] = sum(volume_archive(directory / (name + '.tar')) for name in volumes)
    return record


def restore(directory, fenced, public_ip):
    if not fenced:
        raise RuntimeError('Fence the old host first, then pass --source-fenced; never run both agents')
    directory = pathlib.Path(directory).resolve()
    record = validate(directory)
    target = host()
    # VM templates can clone Docker's persistent engine ID. A kernel boot ID
    # disambiguates the running source; empty storage is still required below.
    if target['id'] == record['host']['id'] and target.get('bootID') and target['bootID'] == record['host'].get('bootID'):
        raise RuntimeError('Restore requires a different Docker host/kernel from the running source')
    if target['architecture'] != record['host']['architecture']:
        raise RuntimeError('Physical database recovery requires the same CPU architecture')
    if record['version'] != (ROOT / 'VERSION').read_text().strip() or record['config'] != config_hashes():
        raise RuntimeError('Use the matching source release and deployment configuration')
    if (ROOT / 'deploy/local/.env').exists() or docker('ps', '-aq') or docker('volume', 'ls', '-q'):
        raise RuntimeError('Restore requires an empty Docker host and no installed environment')
    if record['public'] and (not public_ip or ipaddress.ip_address(public_ip).version != 4 or not ipaddress.ip_address(public_ip).is_global):
        raise RuntimeError('Public recovery needs --public-ip for the new host; prepare DNS and ports first')
    checkpoint = ROOT / '.data/host-restore.json'
    m.save(checkpoint, {'phase': 'restoring', 'backup': str(directory), 'host': target})
    docker('image', 'load', '-i', str(directory / 'platform-images.tar'))
    for item in record['images'].values():
        if m.image(item['ref']) != item['id']:
            raise RuntimeError('Loaded platform image identity does not match the backup')
    image = record['images']['agent']['ref']
    free = int(docker('run', '--rm', '--network', 'none', '--read-only', '--entrypoint', 'df', image, '-Pk', '/').splitlines()[-1].split()[3]) * 1024
    if free < record['requiredBytes'] + (2 << 30):
        raise RuntimeError('Not enough target Docker disk for restored volumes plus 2 GiB reserve')
    for item in record['volumes']:
        args = ['volume', 'create', '--driver', 'local']
        for key, value in sorted(item['labels'].items()):
            args += ['--label', key + '=' + value]
        docker(*args, item['name'])
        with (directory / (item['name'] + '.tar')).open('rb') as data:
            subprocess.run(helper(image, item['name'], reading=False), stdin=data, check=True, stdout=subprocess.DEVNULL)
    env = (directory / 'installation.env').read_text()
    if record['public']:
        env = '\n'.join(line for line in env.splitlines() if not line.startswith('PUBLIC_IP=')) + '\nPUBLIC_IP=' + public_ip + '\n'
        (ROOT / '.data/public-installation').touch(mode=0o600)
    (ROOT / 'deploy/local/.env').write_text(env)
    (ROOT / 'deploy/local/.env').chmod(0o600)
    overlay = ROOT / '.data/host-images.yaml'
    overlay.write_text(json.dumps({'services': {role: {'image': item['ref']} for role, item in record['images'].items()}}))
    configuration = json.loads(m.run(['bash', 'scripts/compose.sh', 'config', '--format', 'json'], capture=True))
    for role in ROLES:
        target_ref = configuration['services'][role].get('image', 'cloudrail-' + role)
        docker('tag', record['images'][role]['ref'], target_ref)
    roles = [role for role in ROLES if role != 'agent']
    m.compose('up', '-d', '--no-build', '--pull', 'never', '--wait', '--wait-timeout', '180', *roles)
    # Retained private images resolve through the restored loopback registry. Public
    # images still require their upstream registry (and operator login if private).
    for ref in record['applicationImages']:
        docker('pull', ref)
    m.compose('up', '-d', '--no-build', '--no-deps', '--pull', 'never', '--wait', '--wait-timeout', '180', 'agent')
    m.save(checkpoint, {'phase': 'restored', 'backup': str(directory), 'host': target})
    print('PASS platform and persistent data restored; verify owner login, application routes/data and a new deployment before DNS cutover')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest='command', required=True)
    sub.add_parser('backup').add_argument('directory')
    r = sub.add_parser('restore'); r.add_argument('directory'); r.add_argument('--source-fenced', action='store_true'); r.add_argument('--public-ip')
    sub.add_parser('verify').add_argument('directory')
    args = parser.parse_args()
    os.umask(0o077)
    with m.maintenance_lock():
        if args.command == 'backup': backup(args.directory)
        elif args.command == 'verify': validate(pathlib.Path(args.directory).resolve()); print('PASS backup inventory, checksums and volume archive boundaries')
        else: restore(args.directory, args.source_fenced, args.public_ip)


if __name__ == '__main__':
    def interrupted(signum, frame):
        raise InterruptedError('Host backup/recovery interrupted')
    signal.signal(signal.SIGTERM, interrupted)
    try:
        main()
    except (RuntimeError, OSError, ValueError, KeyError, subprocess.CalledProcessError, tarfile.TarError) as error:
        print('Host recovery stopped:', error, file=sys.stderr)
        sys.exit(1)
