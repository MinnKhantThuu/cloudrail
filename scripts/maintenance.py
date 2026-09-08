#!/usr/bin/env python3
"""Guarded single-node runtime update and schema-compatible image rollback."""
import argparse
import contextlib
import datetime
import fcntl
import hashlib
import json
import os
import pathlib
import re
import signal
import subprocess
import sys
import uuid

ROOT = pathlib.Path(__file__).resolve().parents[1]
ROLES = ('server', 'agent')


def run(args, capture=False):
    result = subprocess.run(args, cwd=ROOT, check=True, text=True,
                            stdout=subprocess.PIPE if capture else None)
    return result.stdout.strip() if capture else ''


def compose(*args):
    return run(['bash', 'scripts/compose.sh', *args])


def sql(query):
    return run(['docker', 'exec', 'cloudrail-postgres-1', 'psql', '-U', 'cloudrail',
                '-d', 'cloudrail', '-v', 'ON_ERROR_STOP=1', '-Atc', query], capture=True)


def idle():
    pending = int(sql("""SELECT
      (SELECT count(*) FROM deployments WHERE status NOT IN ('active','failed','superseded')) +
      (SELECT count(*) FROM service_actions WHERE status NOT IN ('done','failed')) +
      (SELECT count(*) FROM source_builds WHERE status IN ('queued','building')
        OR (status='succeeded' AND deployment_id=''))"""))
    if pending:
        raise RuntimeError(f'{pending} pending job(s); finish or cancel them before maintenance')


def image(ref):
    return run(['docker', 'image', 'inspect', ref, '--format', '{{.Id}}'], capture=True)


def schema():
    ledger = sql("SELECT COALESCE(json_agg(t ORDER BY name),'[]'::json) FROM (SELECT name,checksum FROM schema_migrations) t")
    dump = run(['docker', 'exec', 'cloudrail-postgres-1', 'pg_dump', '-U', 'cloudrail',
                '-d', 'cloudrail', '--schema-only', '--schema=public', '--no-owner', '--no-privileges'], capture=True)
    # PostgreSQL emits randomized psql restriction tokens; they are not schema.
    stable = '\n'.join(line for line in dump.splitlines()
                       if not line.startswith(('\\restrict ', '\\unrestrict ', '--')))
    return {'migrations': json.loads(ledger), 'ddlSHA256': hashlib.sha256(stable.encode()).hexdigest()}


def identity():
    return {'dockerID': run(['docker', 'info', '--format', '{{.ID}}'], capture=True),
            'environmentSHA256': hashlib.sha256((ROOT / 'deploy/local/.env').read_bytes()).hexdigest()}


def save(path, record):
    temporary = path.with_suffix('.tmp')
    temporary.write_text(json.dumps(record, indent=2) + '\n')
    temporary.chmod(0o600)
    temporary.replace(path)


def validate_rollback(record, current_identity, current_schema):
    if record.get('format') != 1 or not record.get('backupComplete'):
        raise RuntimeError('No completed maintenance backup in this record')
    if record['identity'] != current_identity:
        raise RuntimeError('Docker host or installation environment changed; image-only rollback refused')
    if record['schema'] != current_schema:
        raise RuntimeError('Database schema changed; image-only rollback refused. Use the recovery runbook')
    for role in ROLES:
        item = record['images'][role]
        if not re.fullmatch(r'sha256:[a-f0-9]{64}', item['id']):
            raise RuntimeError('Invalid recorded image identity')
        if not re.fullmatch(r'cloudrail-' + role + r':previous-[a-zA-Z0-9-]+', item['ref']):
            raise RuntimeError('Invalid recorded recovery tag')


def overlay(directory, record):
    # JSON is valid Compose YAML. Never execute a caller-supplied YAML file.
    target = directory / 'previous-images.yaml'
    target.write_text(json.dumps({'services': {role: {'image': record['images'][role]['ref']} for role in ROLES}}, indent=2))
    return str(target)


def start_previous(directory, record):
    for role in ROLES:
        if image(record['images'][role]['ref']) != record['images'][role]['id']:
            raise RuntimeError('Recorded recovery image changed or is missing')
    compose('-f', overlay(directory, record), 'up', '-d', '--no-build', '--no-deps', '--wait', '--wait-timeout', '120', *ROLES)


@contextlib.contextmanager
def maintenance_lock():
    (ROOT / '.data').mkdir(exist_ok=True)
    with (ROOT / '.data/maintenance.lock').open('a') as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise RuntimeError('Another update or rollback is already running') from None
        try:
            yield
        finally:
            fcntl.flock(lock, fcntl.LOCK_UN)


def update():
    idle()
    stamp = datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ') + '-' + uuid.uuid4().hex[:8]
    directory = ROOT / '.data/updates' / stamp
    directory.mkdir(parents=True, mode=0o700)
    record = {'format': 1, 'phase': 'prepared', 'backupComplete': False,
              'identity': identity(), 'images': {}, 'targetVersion': (ROOT / 'VERSION').read_text().strip()}
    path = directory / 'record.json'
    for role in ROLES:
        current = run(['docker', 'inspect', 'cloudrail-' + role + '-1', '--format', '{{.Image}}'], capture=True)
        ref = 'cloudrail-' + role + ':previous-' + stamp
        run(['docker', 'tag', current, ref])
        record['images'][role] = {'id': current, 'ref': ref}
    save(path, record)
    overlay(directory, record)
    frozen = migration_possible = False
    try:
        compose('build', *ROLES)
        idle()
        # Freeze API and agent before the authoritative check/snapshot: requests
        # arriving during the build may have enqueued work since the first check.
        frozen = True
        compose('stop', *ROLES)
        idle()
        if identity() != record['identity']:
            raise RuntimeError('Installation identity changed during preparation')
        record['schema'] = schema()
        run(['bash', 'scripts/backup-control-plane.sh', str(directory / 'control-backup')])
        record.update(backupComplete=True, phase='backed-up')
        save(path, record)
        migration_possible = True
        compose('up', '-d', '--no-build', '--no-deps', '--wait', '--wait-timeout', '180', *ROLES)
        record.update(phase='updated', afterSchema=schema())
        save(path, record)
        print('PASS runtime update. Verify node heartbeat and application routes.')
        print('Maintenance record:', directory)
    except BaseException:
        record['phase'] = 'failed-after-start' if migration_possible else 'aborted-before-start'
        save(path, record)
        if frozen and not migration_possible:
            start_previous(directory, record)
        print('Recovery record retained:', directory, file=sys.stderr)
        if migration_possible:
            print('New runtime may have applied migrations; no automatic image downgrade was attempted.', file=sys.stderr)
        raise


def rollback(directory):
    directory = pathlib.Path(directory).resolve()
    path = directory / 'record.json'
    record = json.loads(path.read_text())
    validate_rollback(record, identity(), schema())
    idle()
    for role in ROLES:
        if image(record['images'][role]['ref']) != record['images'][role]['id']:
            raise RuntimeError('Recorded recovery image changed or is missing')
    # Preserve the current runtime in case a final race check rejects rollback.
    current = {role: run(['docker', 'inspect', 'cloudrail-' + role + '-1', '--format', '{{.Image}}'], capture=True) for role in ROLES}
    try:
        compose('stop', *ROLES)
        idle()
        validate_rollback(record, identity(), schema())
    except BaseException:
        recovery = directory / 'interrupted-rollback.json'
        recovery.write_text(json.dumps({'services': {role: {'image': current[role]} for role in ROLES}}))
        compose('-f', str(recovery), 'up', '-d', '--no-build', '--no-deps', '--wait', '--wait-timeout', '120', *ROLES)
        raise
    start_previous(directory, record)
    record['phase'] = 'rolled-back'
    save(path, record)
    print('PASS schema-compatible runtime rollback; database and application data preserved')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest='command', required=True)
    commands.add_parser('update')
    commands.add_parser('rollback').add_argument('directory')
    args = parser.parse_args()
    os.umask(0o077)
    if not (ROOT / 'deploy/local/.env').is_file():
        parser.error('No installed environment; run the installer first')
    with maintenance_lock():
        if args.command == 'update':
            update()
        else:
            rollback(args.directory)


if __name__ == '__main__':
    def interrupted(signum, frame):
        raise InterruptedError('Maintenance interrupted')
    signal.signal(signal.SIGTERM, interrupted)
    try:
        main()
    except (RuntimeError, OSError, subprocess.CalledProcessError, ValueError, KeyError) as error:
        print('Maintenance stopped:', str(error), file=sys.stderr)
        sys.exit(1)
