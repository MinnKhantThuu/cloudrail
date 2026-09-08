#!/usr/bin/env python3
"""Disposable Docker proof for standalone volume create, attach and detach."""
import json
import subprocess
import time

from test_client import Client


def wait(check, label, timeout=120):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        value = check()
        if value:
            return value
        time.sleep(.4)
    raise AssertionError('Timed out: ' + label)


client = Client()
client.login()
subprocess.run(['docker', 'pull', 'alpine:3.23'], check=True, stdout=subprocess.DEVNULL)
image = json.loads(subprocess.check_output(['docker', 'image', 'inspect', 'alpine:3.23', '--format', '{{json .RepoDigests}}']))[0]
project = client.json('/api/projects', {'name': 'Volume resource ' + time.strftime('%m%d-%H%M%S')}, expected=201)


def worker(name):
    service = client.json('/api/projects/' + project['id'] + '/resources', {
        'name': name, 'environment': 'production', 'sourceType': 'image', 'workloadMode': 'worker',
    }, expected=201)
    client.json('/api/services/' + service['id'] + '/runtime', {
        'startCommand': 'while true; do sleep 30; done', 'preDeployCommand': '', 'preDeployTimeoutSeconds': 0,
        'restartPolicy': 'always', 'restartMaxRetries': 0,
    }, method='PUT')
    return service


def active(service):
    state = client.json('/api/state')
    current = next(item for item in state['services'] if item['id'] == service['id'])
    deployments = [item for item in state['deployments'] if item['serviceId'] == service['id']]
    if deployments and deployments[0]['status'] == 'failed':
        raise AssertionError(deployments[0]['error'])
    return next((item for item in deployments if item['id'] == current['activeId']), None)


def deploy(service):
    client.json('/api/services/' + service['id'] + '/deployments', {'image': image, 'port': 80, 'healthPath': '/'}, expected=202)
    return wait(lambda: active(service), service['name'] + ' deployment')


def action(service, kind):
    queued = client.json('/api/services/' + service['id'] + '/actions', {'kind': kind}, expected=202)
    def finished():
        item = next(value for value in client.json('/api/state')['actions'] if value['id'] == queued['id'])
        if item['status'] == 'failed':
            raise AssertionError(item['error'])
        return item if item['status'] == 'done' else None
    return wait(finished, kind)


first = worker('writer-one')
volume = client.json('/api/projects/' + project['id'] + '/volumes', {'name': 'shared uploads', 'environment': 'production'}, expected=201)
client.json('/api/volumes/' + volume['id'] + '/attachment', {'serviceId': first['id'], 'mountPath': '/uploads'}, expected=200, method='PUT')
first_deployment = deploy(first)
first_container = 'cloudrail-app-' + first_deployment['id']
subprocess.run(['docker', 'exec', first_container, 'sh', '-c', 'echo retained-by-volume > /uploads/proof.txt'], check=True)
rejected = client.json('/api/volumes/' + volume['id'] + '/attachment', method='DELETE', expected=400)
assert 'stop the application' in rejected['error']
first_mount = next(item for item in json.loads(subprocess.check_output(['docker', 'inspect', first_container]))[0]['Mounts'] if item['Destination'] == '/uploads')
action(first, 'stop')
client.json('/api/volumes/' + volume['id'] + '/attachment', method='DELETE')

second = worker('writer-two')
client.json('/api/volumes/' + volume['id'] + '/attachment', {'serviceId': second['id'], 'mountPath': '/data'}, expected=200, method='PUT')
second_deployment = deploy(second)
second_container = 'cloudrail-app-' + second_deployment['id']
second_mount = next(item for item in json.loads(subprocess.check_output(['docker', 'inspect', second_container]))[0]['Mounts'] if item['Destination'] == '/data')
assert first_mount['Name'] == second_mount['Name']
value = subprocess.check_output(['docker', 'exec', second_container, 'cat', '/data/proof.txt']).decode().strip()
assert value == 'retained-by-volume'
canvas = client.json('/api/projects/' + project['id'] + '/environments/production/canvas')
resource = next(item for item in canvas['resources'] if item['id'] == volume['id'])
assert resource['name'] == 'shared uploads' and resource['status'] == 'attached'
assert any(link['kind'] == 'volume-attachment' and link['label'] == '/data' and link['to'] == 'service:' + second['id'] for link in canvas['links'])
print('PASS: standalone volume moves between stopped applications and retains its data', flush=True)
