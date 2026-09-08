#!/usr/bin/env python3
"""Disposable CI/dev workspace only: update, rollback and failure recovery rehearsal."""
import json
import os
import pathlib
import subprocess
import threading
import time
import urllib.request
import uuid
from test_client import Client, ROOT

if os.environ.get("CLOUDRAIL_DISPOSABLE_TEST") != "1":
    raise SystemExit("Set CLOUDRAIL_DISPOSABLE_TEST=1 only on a disposable test installation")

c = Client()
c.login()
state = c.json('/api/state')
service = next(s for s in state['services'] if s['activeId'] and s['desiredState'] == 'running' and s['settings']['kind'] == 'http')
active = next(d for d in state['deployments'] if d['id'] == service['activeId'])
project_ids = {p['id'] for p in state['projects']}
service_ids = {s['id'] for s in state['services']}
errors = []
stop = threading.Event()
variable = 'maintenance-check-' + uuid.uuid4().hex
c.json('/api/services/' + service['id'] + '/variables/MAINTENANCE_PROBE', {'value': variable}, method='PUT')


def route():
    request = urllib.request.Request('http://127.0.0.1:8088/', headers={'Host': service['host']})
    with urllib.request.urlopen(request, timeout=5) as response:
        assert response.status == 200


def monitor():
    while not stop.wait(.2):
        try:
            route()
        except Exception as error:
            errors.append(type(error).__name__)


def command(*args, success=True):
    result = subprocess.run(args, cwd=ROOT, text=True, capture_output=True)
    if success and result.returncode:
        raise AssertionError(result.stdout + result.stderr)
    if not success and result.returncode == 0:
        raise AssertionError('Unsafe maintenance action unexpectedly succeeded')
    return result


def sql(query):
    return command('docker', 'exec', 'cloudrail-postgres-1', 'psql', '-U', 'cloudrail', '-d', 'cloudrail', '-v', 'ON_ERROR_STOP=1', '-Atc', query).stdout.strip()


def newest():
    return max((ROOT / '.data/updates').glob('*/record.json'), key=lambda p: p.stat().st_mtime).parent


def wait_active(identifier):
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        deployment = next(d for d in c.json('/api/state')['deployments'] if d['id'] == identifier)
        if deployment['status'] == 'active':
            return
        if deployment['status'] == 'failed':
            raise AssertionError(deployment['error'])
        time.sleep(.3)
    raise AssertionError('Deployment timed out')


def verify_operation():
    state = c.json('/api/state')
    assert {p['id'] for p in state['projects']} == project_ids
    assert {s['id'] for s in state['services']} == service_ids
    deployment = c.json('/api/services/' + service['id'] + '/deployments',
                        {'image': active['image'], 'port': active['port'], 'healthPath': active['healthPath']}, expected=202)
    wait_active(deployment['id'])
    assert c.json('/api/node')['online']
    environment = json.loads(command('docker', 'inspect', 'cloudrail-app-' + deployment['id'], '--format', '{{json .Config.Env}}').stdout)
    assert 'MAINTENANCE_PROBE=' + variable in environment, 'Encrypted configuration lost across maintenance'
    route()


route()
watcher = threading.Thread(target=monitor, daemon=True)
watcher.start()
try:
    # Explicitly prevent an agent from claiming a test-owned queued deployment.
    command('bash', 'scripts/compose.sh', 'stop', 'agent')
    try:
        queued = c.json('/api/services/' + service['id'] + '/deployments',
                        {'image': active['image'], 'port': active['port'], 'healthPath': active['healthPath']}, expected=202)
        refused = command('bash', 'scripts/update.sh', success=False)
        assert 'pending job' in refused.stderr
        c.json('/api/state')
    finally:
        command('bash', 'scripts/compose.sh', 'start', 'agent')
    wait_active(queued['id'])
    print('PASS busy queue rejects update before interrupting the control API', flush=True)

    command('bash', 'scripts/update.sh')
    directory = newest()
    record = json.loads((directory / 'record.json').read_text())
    assert record['phase'] == 'updated' and record['backupComplete']
    version = (ROOT / 'VERSION').read_text().strip()
    for role in ('server', 'agent'):
        assert command('docker', 'exec', 'cloudrail-' + role + '-1', 'cat', '/usr/share/cloudrail/VERSION').stdout.strip() == version
    verify_operation()
    for role in ('server', 'agent'):
        current = command('docker', 'inspect', 'cloudrail-' + role + '-1', '--format', '{{.Image}}').stdout.strip()
        assert current != record['images'][role]['id'], 'Upgrade did not replace the old runtime image'
    command('bash', 'scripts/verify-control-restore.sh', str(directory / 'control-backup'))
    print('PASS runtime update retains owner state, serves the app and creates a restorable backup', flush=True)

    # Detect actual DDL drift even when the migration ledger was not modified.
    table = 'maintenance_guard_' + uuid.uuid4().hex[:12]
    sql('CREATE TABLE ' + table + '(id integer)')
    try:
        rejected = command('bash', 'scripts/rollback.sh', str(directory), success=False)
        assert 'schema changed' in rejected.stderr
        c.json('/api/state')
    finally:
        sql('DROP TABLE ' + table)
    command('bash', 'scripts/rollback.sh', str(directory))
    for role in ('server', 'agent'):
        current = command('docker', 'inspect', 'cloudrail-' + role + '-1', '--format', '{{.Image}}').stdout.strip()
        assert current == record['images'][role]['id']
    verify_operation()
    print('PASS incompatible rollback refused; compatible rollback restores exact prior images', flush=True)

    # The previous runtime must recover automatically if snapshot preparation fails.
    backup = ROOT / 'scripts/backup-control-plane.sh'
    original = backup.read_bytes()
    try:
        backup.write_text('#!/usr/bin/env bash\nexit 71\n')
        failed = command('bash', 'scripts/update.sh', success=False)
        assert 'Recovery record retained' in failed.stderr
        assert json.loads((newest() / 'record.json').read_text())['phase'] == 'aborted-before-start'
        c.json('/api/state')
    finally:
        backup.write_bytes(original)
    command('bash', 'scripts/update.sh')
    verify_operation()
    print('PASS failed backup restores the old API/agent, and a subsequent update succeeds', flush=True)
finally:
    stop.set()
    watcher.join(timeout=6)
assert not errors, ('Application traffic was interrupted', errors[:5])
print('PASS continuous application HTTP availability during maintenance', flush=True)
