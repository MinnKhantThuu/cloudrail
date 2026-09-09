#!/usr/bin/env python3
"""Disposable Docker proof for MongoDB persistence, binding and safe recovery."""
import json
import subprocess
import time

from test_client import Client


def wait(check, label, timeout=240):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        value = check()
        if value:
            return value
        time.sleep(.5)
    raise AssertionError('Timed out: ' + label)


client = Client()
client.login()
catalog = client.json('/api/templates')
template = next(item for item in catalog if item['key'] == 'mongo')
assert template == {
    'key': 'mongo', 'version': '8.0.29', 'name': 'MongoDB',
    'description': 'Document database with persistent storage',
    'port': 27017, 'mountPath': '/data/db', 'memoryMB': 512,
}
assert 'password' not in json.dumps(catalog).lower()
project = client.json('/api/projects', {'name': 'MongoDB proof ' + time.strftime('%m%d-%H%M%S')}, expected=201)


def create(name):
    service = client.json('/api/projects/' + project['id'] + '/databases', {
        'name': name, 'environment': 'production', 'template': 'mongo',
    }, expected=201)
    assert service['template'] == 'mongo' and service['templateVersion'] == '8.0.29'
    assert service['settings']['kind'] == 'mongo' and service['settings']['mountPath'] == '/data/db'
    assert 'mongodb://root:' not in json.dumps(service)
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


def mongo_eval(container, statement):
    return subprocess.check_output([
        'docker', 'exec', container, 'sh', '-lc',
        'mongosh --quiet --host 127.0.0.1 --port 27017 '
        '-u "$MONGO_INITDB_ROOT_USERNAME" -p "$MONGO_INITDB_ROOT_PASSWORD" '
        '--authenticationDatabase admin "$MONGO_INITDB_DATABASE" --eval ' + json.dumps(statement),
    ]).decode().strip()


source = create('catalog-source')
source_state, source_deployment = wait(lambda: current(source), 'source MongoDB deployment')
source_container = 'cloudrail-app-' + source_deployment['id']
details = json.loads(subprocess.check_output(['docker', 'inspect', source_container]))[0]
assert details['State']['Health']['Status'] == 'healthy'
assert details['HostConfig']['PortBindings'] in (None, {})
assert details['HostConfig']['RestartPolicy']['Name'] == 'unless-stopped'
mount = next(item for item in details['Mounts'] if item['Destination'] == '/data/db')
assert mount['Name'] == source['settings']['volumeName']
names = client.json('/api/services/' + source['id'] + '/variables')['names']
assert names == ['MONGO_INITDB_DATABASE', 'MONGO_INITDB_ROOT_PASSWORD', 'MONGO_INITDB_ROOT_USERNAME', 'MONGO_URL']

app = client.json('/api/projects/' + project['id'] + '/services', {'name': 'api'}, expected=201)
client.json('/api/services/' + source['id'] + '/bindings', {'targetServiceId': app['id'], 'variableName': 'MONGO_URL'})
assert 'MONGO_URL' in client.json('/api/services/' + app['id'] + '/variables')['names']
canvas = client.json('/api/projects/' + project['id'] + '/environments/production/canvas')
resource = next(item for item in canvas['resources'] if item['id'] == source['id'])
assert resource['template'] == 'mongo' and resource['privateAddress'] == 'db-' + source['id'] + ':27017'
assert any(link['kind'] == 'variable-reference' and link['label'] == 'MONGO_URL → MONGO_URL' for link in canvas['links'])

mongo_eval(source_container, 'db.proof.insertOne({_id: 1, value: "persistent-document"})')
action(source, 'restart')
assert mongo_eval(source_container, 'print(db.proof.findOne({_id: 1}).value)') == 'persistent-document'
print('PASS: MongoDB is private, bound on the canvas and retains a document after restart', flush=True)

backup = action(source, 'backup')
stored = next(item for item in client.json('/api/backups') if item['id'] == backup['id'])
assert stored['size'] > 0 and len(stored['checksum']) == 64
target = create('catalog-restore')
target_state, target_deployment = wait(lambda: current(target), 'target MongoDB deployment')
target_container = 'cloudrail-app-' + target_deployment['id']
action(target, 'restore', backup['id'])
assert mongo_eval(target_container, 'print(db.proof.findOne({_id: 1}).value)') == 'persistent-document'
print('PASS: a logical MongoDB backup restores into a fresh compatible target', flush=True)

rejected = action(target, 'restore', backup['id'], expect_failure=True)
assert 'empty target MongoDB' in rejected['error'] and 'preserved' in rejected['error']
assert mongo_eval(target_container, 'print(db.proof.findOne({_id: 1}).value)') == 'persistent-document'
print('PASS: non-empty MongoDB restore is rejected without changing existing documents', flush=True)
