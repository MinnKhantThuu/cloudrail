#!/usr/bin/env python3
"""Disposable real-Docker proof for route-free background workers."""
import json
import subprocess
import time

from test_client import Client

client = Client()
client.login()


def wait_deployment(identifier, terminal, timeout=220):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        item = next(d for d in client.json('/api/state')['deployments'] if d['id'] == identifier)
        if item['status'] in terminal:
            return item
        if item['status'] in ('active', 'failed', 'superseded'):
            raise AssertionError(('Unexpected deployment state', item['status'], item['error']))
        time.sleep(.35)
    raise AssertionError(('Worker deployment timed out', identifier))


def wait_action(identifier, timeout=40):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        item = next(a for a in client.json('/api/state')['actions'] if a['id'] == identifier)
        if item['status'] == 'done':
            return
        if item['status'] == 'failed':
            raise AssertionError(item['error'])
        time.sleep(.25)
    raise AssertionError(('Worker action timed out', identifier))


def running(identifier):
    result = subprocess.run(
        ['docker', 'inspect', 'cloudrail-app-' + identifier, '--format', '{{.State.Running}}'],
        text=True, capture_output=True,
    )
    return result.returncode == 0 and result.stdout.strip() == 'true'


subprocess.run(['docker', 'pull', 'traefik/whoami:v1.11.0'], check=True, stdout=subprocess.DEVNULL)
image = json.loads(subprocess.check_output(
    ['docker', 'image', 'inspect', 'traefik/whoami:v1.11.0', '--format', '{{json .RepoDigests}}']
))[0]
project = client.json('/api/projects', {'name': 'Worker proof ' + time.strftime('%m%d-%H%M%S')}, expected=201)
worker = client.json('/api/projects/' + project['id'] + '/resources', {
    'name': 'email-queue', 'environment': 'production', 'sourceType': 'image', 'workloadMode': 'worker',
}, expected=201)
assert worker['resourceKind'] == 'service' and worker['workloadMode'] == 'worker'
assert worker['url'] == '', 'Worker unexpectedly received a public URL'

first = client.json('/api/services/' + worker['id'] + '/deployments', {
    'image': image, 'port': 80, 'healthPath': '/',
}, expected=202)
wait_deployment(first['id'], {'active'})
assert running(first['id']), 'Active worker process is not running'
route = subprocess.run(['docker', 'exec', 'cloudrail-agent-1', 'test', '-e', '/routes/' + worker['id'] + '.yaml'])
assert route.returncode != 0, 'Worker received a Traefik route'
print('PASS: long-running worker activates without a public route', flush=True)

missing = client.json('/api/services/' + worker['id'] + '/deployments', {
    'image': image.split('@')[0] + '@sha256:' + '0' * 64, 'port': 80, 'healthPath': '/',
}, expected=202)
wait_deployment(missing['id'], {'failed'})
assert running(first['id']), 'Failed candidate stopped the active worker'
print('PASS: failed worker candidate preserves the active process', flush=True)

stop = client.json('/api/services/' + worker['id'] + '/actions', {'kind': 'stop'}, expected=202)
wait_action(stop['id'])
assert not running(first['id']), 'Stop action left the worker running'
start = client.json('/api/services/' + worker['id'] + '/actions', {'kind': 'start'}, expected=202)
wait_action(start['id'])
assert running(first['id']), 'Start action did not restore the worker'
restart = client.json('/api/services/' + worker['id'] + '/actions', {'kind': 'restart'}, expected=202)
wait_action(restart['id'])
assert running(first['id']), 'Restart action did not restore the worker'
print('PASS: worker start, stop and restart actions preserve route-free lifecycle', flush=True)

subprocess.run(['bash', 'scripts/compose.sh', 'stop', 'agent'], check=True, stdout=subprocess.DEVNULL)
subprocess.run(['docker', 'stop', 'cloudrail-app-' + first['id']], check=True, stdout=subprocess.DEVNULL)
assert not running(first['id']), 'Worker stayed running while the agent was offline'
subprocess.run(['bash', 'scripts/compose.sh', 'start', 'agent'], check=True, stdout=subprocess.DEVNULL)
deadline = time.monotonic() + 30
while time.monotonic() < deadline and not running(first['id']):
    time.sleep(.5)
assert running(first['id']), 'Worker did not recover after agent restart'
route = subprocess.run(['docker', 'exec', 'cloudrail-agent-1', 'test', '-e', '/routes/' + worker['id'] + '.yaml'])
assert route.returncode != 0, 'Reconciliation created a worker route'
print('PASS: worker reconciliation survives agent restart without exposing HTTP', flush=True)
