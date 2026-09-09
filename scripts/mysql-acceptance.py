#!/usr/bin/env python3
"""Disposable Docker proof for MySQL persistence, binding and safe recovery."""
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
template = next(item for item in catalog if item['key'] == 'mysql')
assert template == {
    'key': 'mysql', 'version': '8.4.7', 'name': 'MySQL',
    'description': 'Relational database with persistent InnoDB storage',
    'port': 3306, 'mountPath': '/var/lib/mysql', 'memoryMB': 512,
}
assert 'password' not in json.dumps(catalog).lower()
project = client.json('/api/projects', {'name': 'MySQL proof ' + time.strftime('%m%d-%H%M%S')}, expected=201)


def create(name):
    service = client.json('/api/projects/' + project['id'] + '/databases', {
        'name': name, 'environment': 'production', 'template': 'mysql',
    }, expected=201)
    assert service['template'] == 'mysql' and service['templateVersion'] == '8.4.7'
    assert service['settings']['kind'] == 'mysql' and service['settings']['mountPath'] == '/var/lib/mysql'
    assert 'mysql://app:' not in json.dumps(service)
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


def sql(container, statement):
    return subprocess.check_output([
        'docker', 'exec', container, 'sh', '-lc',
        'MYSQL_PWD="$MYSQL_PASSWORD" mysql -Nse ' + json.dumps(statement) + ' -u"$MYSQL_USER" "$MYSQL_DATABASE"',
    ]).decode().strip()


source = create('orders-source')
source_state, source_deployment = wait(lambda: current(source), 'source MySQL deployment')
source_container = 'cloudrail-app-' + source_deployment['id']
details = json.loads(subprocess.check_output(['docker', 'inspect', source_container]))[0]
assert details['State']['Health']['Status'] == 'healthy'
assert details['HostConfig']['PortBindings'] in (None, {})
assert details['HostConfig']['RestartPolicy']['Name'] == 'unless-stopped'
mount = next(item for item in details['Mounts'] if item['Destination'] == '/var/lib/mysql')
assert mount['Name'] == source['settings']['volumeName']
names = client.json('/api/services/' + source['id'] + '/variables')['names']
assert names == ['MYSQL_DATABASE', 'MYSQL_PASSWORD', 'MYSQL_ROOT_PASSWORD', 'MYSQL_URL', 'MYSQL_USER']

app = client.json('/api/projects/' + project['id'] + '/services', {'name': 'api'}, expected=201)
client.json('/api/services/' + source['id'] + '/bindings', {'targetServiceId': app['id'], 'variableName': 'MYSQL_URL'})
assert 'MYSQL_URL' in client.json('/api/services/' + app['id'] + '/variables')['names']
canvas = client.json('/api/projects/' + project['id'] + '/environments/production/canvas')
resource = next(item for item in canvas['resources'] if item['id'] == source['id'])
assert resource['template'] == 'mysql' and resource['privateAddress'] == 'db-' + source['id'] + ':3306'
assert any(link['kind'] == 'variable-reference' and link['label'] == 'MYSQL_URL → MYSQL_URL' for link in canvas['links'])

sql(source_container, 'CREATE TABLE proof (id INT PRIMARY KEY, value VARCHAR(80)); INSERT INTO proof VALUES (1, "persistent-order");')
action(source, 'restart')
assert sql(source_container, 'SELECT value FROM proof WHERE id=1') == 'persistent-order'
print('PASS: MySQL is private, bound on the canvas and retains InnoDB data after restart', flush=True)

backup = action(source, 'backup')
stored = next(item for item in client.json('/api/backups') if item['id'] == backup['id'])
assert stored['size'] > 0 and len(stored['checksum']) == 64
target = create('orders-restore')
target_state, target_deployment = wait(lambda: current(target), 'target MySQL deployment')
target_container = 'cloudrail-app-' + target_deployment['id']
action(target, 'restore', backup['id'])
assert sql(target_container, 'SELECT value FROM proof WHERE id=1') == 'persistent-order'
print('PASS: a logical MySQL backup restores into a fresh compatible target', flush=True)

rejected = action(target, 'restore', backup['id'], expect_failure=True)
assert 'empty target MySQL' in rejected['error'] and 'preserved' in rejected['error']
assert sql(target_container, 'SELECT value FROM proof WHERE id=1') == 'persistent-order'
print('PASS: non-empty MySQL restore is rejected without changing existing rows', flush=True)
