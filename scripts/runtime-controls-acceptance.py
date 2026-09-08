#!/usr/bin/env python3
"""Disposable Docker proof for commands, pre-deploy and restart policies."""
import json
import subprocess
import time

from test_client import Client


def wait_deployment(client, identifier, expected, timeout=120):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        item = next(d for d in client.json('/api/state')['deployments'] if d['id'] == identifier)
        if item['status'] == expected:
            return item
        if item['status'] in ('active', 'failed', 'superseded'):
            raise AssertionError(('Unexpected deployment state', item['status'], item['error']))
        time.sleep(.3)
    raise AssertionError(('Deployment timed out', identifier, expected))


def running(identifier):
    result = subprocess.run(['docker', 'inspect', 'cloudrail-app-' + identifier, '--format', '{{.State.Running}}'], text=True, capture_output=True)
    return result.returncode == 0 and result.stdout.strip() == 'true'


def wait_running(identifier, expected, timeout=10):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if running(identifier) == expected:
            return
        time.sleep(.2)
    raise AssertionError(('Container running state did not converge', identifier, expected))


def inspection(identifier):
    return json.loads(subprocess.check_output(['docker', 'inspect', 'cloudrail-app-' + identifier]))[0]


client = Client()
client.login()
subprocess.run(['docker', 'pull', 'alpine:3.23'], check=True, stdout=subprocess.DEVNULL)
image = json.loads(subprocess.check_output(['docker', 'image', 'inspect', 'alpine:3.23', '--format', '{{json .RepoDigests}}']))[0]
project = client.json('/api/projects', {'name': 'Runtime controls ' + time.strftime('%m%d-%H%M%S')}, expected=201)
worker = client.json('/api/projects/' + project['id'] + '/resources', {
    'name': 'controlled-worker', 'environment': 'production', 'sourceType': 'image', 'workloadMode': 'worker',
}, expected=201)

start = 'while true; do echo worker-tick; sleep 30; done'
client.json('/api/services/' + worker['id'] + '/runtime', {
    'startCommand': start, 'preDeployCommand': 'echo migration-ok', 'preDeployTimeoutSeconds': 30,
    'restartPolicy': 'never', 'restartMaxRetries': 0,
}, method='PUT')
first = client.json('/api/services/' + worker['id'] + '/deployments', {'image': image, 'port': 80, 'healthPath': '/'}, expected=202)
wait_deployment(client, first['id'], 'active')
assert running(first['id'])
details = inspection(first['id'])
assert details['Config']['Entrypoint'] == ['/bin/sh', '-lc']
assert details['Config']['Cmd'] == [start]
assert details['HostConfig']['RestartPolicy']['Name'] == 'no'
events = [event['stage'] for event in client.json('/api/state')['events'] if event['deploymentId'] == first['id']]
assert 'predeploy' in events
print('PASS: start override, isolated pre-deploy and Never policy activate', flush=True)

client.json('/api/services/' + worker['id'] + '/runtime', {
    'startCommand': start, 'preDeployCommand': 'echo migration-broke; exit 12', 'preDeployTimeoutSeconds': 30,
    'restartPolicy': 'on-failure', 'restartMaxRetries': 4,
}, method='PUT')
failed = client.json('/api/services/' + worker['id'] + '/deployments', {'image': image, 'port': 80, 'healthPath': '/'}, expected=202)
failed = wait_deployment(client, failed['id'], 'failed')
assert 'migration-broke' in failed['logs'] and running(first['id'])
assert not running(failed['id'])
print('PASS: failed pre-deploy preserves the active release and its logs', flush=True)

client.json('/api/services/' + worker['id'] + '/runtime', {
    'startCommand': start, 'preDeployCommand': 'echo timeout-start; sleep 5', 'preDeployTimeoutSeconds': 1,
    'restartPolicy': 'on-failure', 'restartMaxRetries': 4,
}, method='PUT')
timed = client.json('/api/services/' + worker['id'] + '/deployments', {'image': image, 'port': 80, 'healthPath': '/'}, expected=202)
timed = wait_deployment(client, timed['id'], 'failed')
assert 'timeout-start' in timed['logs'] and running(first['id'])
print('PASS: pre-deploy timeout is bounded and preserves the active release', flush=True)

client.json('/api/services/' + worker['id'] + '/runtime', {
    'startCommand': start, 'preDeployCommand': 'echo ready', 'preDeployTimeoutSeconds': 30,
    'restartPolicy': 'on-failure', 'restartMaxRetries': 4,
}, method='PUT')
second = client.json('/api/services/' + worker['id'] + '/deployments', {'image': image, 'port': 80, 'healthPath': '/'}, expected=202)
wait_deployment(client, second['id'], 'active')
details = inspection(second['id'])
assert details['HostConfig']['RestartPolicy']['Name'] == 'on-failure'
assert details['HostConfig']['RestartPolicy']['MaximumRetryCount'] == 4
wait_running(first['id'], False)

client.json('/api/services/' + worker['id'] + '/runtime', {
    'startCommand': start, 'preDeployCommand': '', 'preDeployTimeoutSeconds': 0,
    'restartPolicy': 'always', 'restartMaxRetries': 0,
}, method='PUT')
third = client.json('/api/services/' + worker['id'] + '/deployments', {'image': image, 'port': 80, 'healthPath': '/'}, expected=202)
wait_deployment(client, third['id'], 'active')
details = inspection(third['id'])
assert details['HostConfig']['RestartPolicy']['Name'] == 'unless-stopped'
wait_running(second['id'], False)
print('PASS: On failure retry limit and Always policy map to the Docker runtime', flush=True)
