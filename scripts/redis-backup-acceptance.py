#!/usr/bin/env python3
"""Disposable Docker proof for portable Redis backup and safe restore."""
import subprocess
import time

from test_client import Client


def wait(check, label, timeout=150):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        value = check()
        if value:
            return value
        time.sleep(.4)
    raise AssertionError('Timed out: ' + label)


client = Client()
client.login()
project = client.json('/api/projects', {'name': 'Redis backup ' + time.strftime('%m%d-%H%M%S')}, expected=201)


def create(name):
    service = client.json('/api/projects/' + project['id'] + '/databases', {
        'name': name, 'environment': 'production', 'template': 'redis',
    }, expected=201)
    return service


def current(service):
    state = client.json('/api/state')
    item = next(value for value in state['services'] if value['id'] == service['id'])
    deployments = [value for value in state['deployments'] if value['serviceId'] == service['id']]
    if deployments and deployments[0]['status'] == 'failed':
        raise AssertionError(deployments[0]['error'])
    active = next((value for value in deployments if value['id'] == item['activeId']), None)
    return (item, active) if active else None


def action(service, kind, backup=None, expect_failure=False):
    body = {'kind': kind}
    if backup:
        body['backupId'] = backup
    queued = client.json('/api/services/' + service['id'] + '/actions', body, expected=202)

    def finished():
        item = next(value for value in client.json('/api/state')['actions'] if value['id'] == queued['id'])
        return item if item['status'] in ('done', 'failed') else None

    result = wait(finished, kind)
    if expect_failure:
        assert result['status'] == 'failed', result
    elif result['status'] != 'done':
        raise AssertionError(result['error'])
    return result


source = create('source-cache')
source_state, source_deployment = wait(lambda: current(source), 'source Redis deployment')
source_container = 'cloudrail-app-' + source_deployment['id']
subprocess.run([
    'docker', 'exec', source_container, 'sh', '-lc',
    'REDISCLI_AUTH="$REDIS_PASSWORD" redis-cli SET portable-key restored-value',
], check=True, stdout=subprocess.DEVNULL)
action(source, 'stop')
backup = action(source, 'backup')
backups = client.json('/api/backups')
stored = next(item for item in backups if item['id'] == backup['id'])
assert stored['size'] > 0 and len(stored['checksum']) == 64

target = create('empty-target')
target_state, target_deployment = wait(lambda: current(target), 'target Redis deployment')
target_container = 'cloudrail-app-' + target_deployment['id']
action(target, 'stop')
action(target, 'restore', backup['id'])
action(target, 'start')
value = subprocess.check_output([
    'docker', 'exec', target_container, 'sh', '-lc',
    'REDISCLI_AUTH="$REDIS_PASSWORD" redis-cli GET portable-key',
]).decode().strip()
assert value == 'restored-value'
print('PASS: stopped Redis volume backup restores into a fresh compatible target', flush=True)

action(target, 'stop')
rejected = action(target, 'restore', backup['id'], expect_failure=True)
assert 'empty target Redis' in rejected['error'] and 'preserved' in rejected['error']
action(target, 'start')
value = subprocess.check_output([
    'docker', 'exec', target_container, 'sh', '-lc',
    'REDISCLI_AUTH="$REDIS_PASSWORD" redis-cli GET portable-key',
]).decode().strip()
assert value == 'restored-value'
print('PASS: non-empty Redis restore is rejected without changing existing keys', flush=True)
