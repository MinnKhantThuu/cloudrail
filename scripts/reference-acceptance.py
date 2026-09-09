#!/usr/bin/env python3
"""Real-container proof for typed variables and live service references."""
import json
import subprocess
import time

from test_client import Client


def wait_deployment(client, identifier, timeout=180):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        deployment = next(item for item in client.json('/api/state')['deployments'] if item['id'] == identifier)
        if deployment['status'] == 'active':
            return deployment
        if deployment['status'] in ('failed', 'superseded'):
            raise AssertionError(deployment)
        time.sleep(.35)
    raise AssertionError('Timed out waiting for referenced deployment ' + identifier)


def container_env(identifier):
    details = json.loads(subprocess.check_output(['docker', 'inspect', 'cloudrail-app-' + identifier]))[0]
    return dict(item.split('=', 1) for item in details['Config']['Env'] if '=' in item)


client = Client()
client.login()
subprocess.run(['docker', 'pull', 'traefik/whoami:v1.11.0'], check=True, stdout=subprocess.DEVNULL)
image = json.loads(subprocess.check_output(
    ['docker', 'image', 'inspect', 'traefik/whoami:v1.11.0', '--format', '{{json .RepoDigests}}']
))[0]
project = client.json('/api/projects', {'name': 'Reference proof ' + time.strftime('%m%d-%H%M%S')}, expected=201)
backend = client.json('/api/projects/' + project['id'] + '/services', {'name': 'backend'}, expected=201)
frontend = client.json('/api/projects/' + project['id'] + '/services', {'name': 'frontend'}, expected=201)

client.json('/api/services/' + backend['id'] + '/variables/API_ORIGIN', {
    'kind': 'plain', 'value': 'http://backend.internal',
}, method='PUT')
client.json('/api/services/' + backend['id'] + '/variables/INTERNAL_SECRET', {
    'kind': 'secret', 'value': 'reference-secret-must-stay-hidden',
}, method='PUT')
metadata = client.json('/api/services/' + backend['id'] + '/variables')
encoded = json.dumps(metadata)
assert 'reference-secret-must-stay-hidden' not in encoded
assert next(item for item in metadata['variables'] if item['name'] == 'API_ORIGIN')['value'] == 'http://backend.internal'

client.json('/api/services/' + frontend['id'] + '/variables/API_URL', {
    'kind': 'reference', 'targetServiceId': backend['id'], 'targetVariable': 'API_ORIGIN',
}, method='PUT')
reference = next(item for item in client.json('/api/services/' + frontend['id'] + '/variables')['variables'] if item['name'] == 'API_URL')
assert reference['kind'] == 'reference' and reference['targetServiceName'] == 'backend'
canvas = client.json('/api/projects/' + project['id'] + '/environments/production/canvas')
assert any(item['kind'] == 'variable-reference' and item['label'] == 'API_URL → API_ORIGIN' for item in canvas['links'])


def deploy(key):
    queued = client.json('/api/services/' + frontend['id'] + '/deployments', {
        'image': image, 'port': 80, 'healthPath': '/',
    }, expected=202, headers={'Idempotency-Key': key})
    return wait_deployment(client, queued['id'])


first = deploy('reference-runtime-first')
assert container_env(first['id'])['API_URL'] == 'http://backend.internal'
client.json('/api/services/' + backend['id'] + '/variables/API_ORIGIN', {
    'kind': 'plain', 'value': 'http://backend-v2.internal',
}, method='PUT')
second = deploy('reference-runtime-second')
assert container_env(second['id'])['API_URL'] == 'http://backend-v2.internal'

client.json('/api/services/' + backend['id'] + '/variables/API_ORIGIN/rename', {'name': 'INTERNAL_ORIGIN'})
reference = next(item for item in client.json('/api/services/' + frontend['id'] + '/variables')['variables'] if item['name'] == 'API_URL')
assert reference['targetVariable'] == 'INTERNAL_ORIGIN'
third = deploy('reference-runtime-third')
assert container_env(third['id'])['API_URL'] == 'http://backend-v2.internal'

blocked = client.json('/api/services/' + backend['id'] + '/variables/INTERNAL_ORIGIN', expected=400, method='DELETE')
assert 'referenced by another service' in blocked['error']
cycle = client.json('/api/services/' + backend['id'] + '/variables/LOOP', {
    'kind': 'reference', 'targetServiceId': frontend['id'], 'targetVariable': 'API_URL',
}, expected=400, method='PUT')
assert 'cycle' in cycle['error']

client.json('/api/projects/' + project['id'] + '/environments', {'name': 'staging'}, expected=201)
staging = client.json('/api/projects/' + project['id'] + '/services', {'name': 'staging-api', 'environment': 'staging'}, expected=201)
client.json('/api/services/' + staging['id'] + '/variables/API_ORIGIN', {'kind': 'plain', 'value': 'http://staging.internal'}, method='PUT')
cross = client.json('/api/services/' + frontend['id'] + '/variables/CROSS_ENV', {
    'kind': 'reference', 'targetServiceId': staging['id'], 'targetVariable': 'API_ORIGIN',
}, expected=400, method='PUT')
assert 'same project and environment' in cross['error']
print('PASS: typed variables stay safe and canvas references resolve updates and renames into real containers', flush=True)
