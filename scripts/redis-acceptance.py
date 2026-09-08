#!/usr/bin/env python3
"""Disposable Docker proof for the versioned Redis data template."""
import json
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
catalog = client.json('/api/templates')
redis_template = next(item for item in catalog if item['key'] == 'redis')
assert redis_template == {
    'key': 'redis', 'version': '8.2.2', 'name': 'Redis',
    'description': 'Cache, queue and key-value data with append-only persistence',
    'port': 6379, 'mountPath': '/data', 'memoryMB': 128,
}
assert 'password' not in json.dumps(catalog).lower()

project = client.json('/api/projects', {'name': 'Redis template ' + time.strftime('%m%d-%H%M%S')}, expected=201)
cache = client.json('/api/projects/' + project['id'] + '/databases', {
    'name': 'cache', 'environment': 'production', 'template': 'redis',
}, expected=201)
assert cache['template'] == 'redis' and cache['templateVersion'] == '8.2.2'
assert cache['settings']['kind'] == 'redis' and cache['settings']['mountPath'] == '/data'
assert 'redis://default:' not in json.dumps(cache)


def active():
    state = client.json('/api/state')
    service = next(item for item in state['services'] if item['id'] == cache['id'])
    deployments = [item for item in state['deployments'] if item['serviceId'] == cache['id']]
    if deployments and deployments[0]['status'] == 'failed':
        raise AssertionError(deployments[0]['error'])
    if not service['activeId']:
        return None
    return next(item for item in deployments if item['id'] == service['activeId'])


deployment = wait(active, 'Redis deployment')
container = 'cloudrail-app-' + deployment['id']


def inspect():
    return json.loads(subprocess.check_output(['docker', 'inspect', container]))[0]


details = inspect()
assert details['State']['Health']['Status'] == 'healthy'
assert details['HostConfig']['PortBindings'] in (None, {})
assert details['HostConfig']['RestartPolicy']['Name'] == 'unless-stopped'
networks = details['NetworkSettings']['Networks']
assert len(networks) == 1 and next(iter(networks)).startswith('cloudrail-env-')
assert 'db-' + cache['id'] in next(iter(networks.values()))['Aliases']
mount = next(item for item in details['Mounts'] if item['Destination'] == '/data')
assert mount['Name'] == cache['settings']['volumeName']

names = client.json('/api/services/' + cache['id'] + '/variables')['names']
assert names == ['REDISHOST', 'REDISPORT', 'REDISUSER', 'REDIS_PASSWORD', 'REDIS_URL']
subprocess.run(['docker', 'exec', container, 'sh', '-lc', 'REDISCLI_AUTH="$REDIS_PASSWORD" redis-cli SET cloudrail-proof persistent'], check=True, stdout=subprocess.DEVNULL)

app = client.json('/api/projects/' + project['id'] + '/services', {'name': 'api'}, expected=201)
client.json('/api/services/' + cache['id'] + '/bindings', {'targetServiceId': app['id'], 'variableName': 'REDIS_URL'})
assert 'REDIS_URL' in client.json('/api/services/' + app['id'] + '/variables')['names']
canvas = client.json('/api/projects/' + project['id'] + '/environments/production/canvas')
resource = next(item for item in canvas['resources'] if item['id'] == cache['id'])
assert resource['template'] == 'redis' and resource['templateVersion'] == '8.2.2'
assert resource['privateAddress'] == 'db-' + cache['id'] + ':6379'
assert any(link['kind'] == 'variable-reference' and link['label'] == 'REDIS_URL → REDIS_URL' for link in canvas['links'])
print('PASS: Redis is a private versioned canvas template with hidden generated credentials', flush=True)

subprocess.run(['docker', 'stop', 'cloudrail-agent-1'], check=True, stdout=subprocess.DEVNULL)
subprocess.run(['docker', 'stop', container], check=True, stdout=subprocess.DEVNULL)
subprocess.run(['docker', 'start', 'cloudrail-agent-1'], check=True, stdout=subprocess.DEVNULL)


def recovered():
    try:
        state = inspect()['State']
        return state['Running'] and state['Health']['Status'] == 'healthy'
    except (subprocess.CalledProcessError, KeyError):
        return False


wait(recovered, 'Redis recovery after agent restart')
value = subprocess.check_output(['docker', 'exec', container, 'sh', '-lc', 'REDISCLI_AUTH="$REDIS_PASSWORD" redis-cli GET cloudrail-proof']).decode().strip()
assert value == 'persistent'
print('PASS: Redis append-only volume data survives container stop and agent recovery', flush=True)
