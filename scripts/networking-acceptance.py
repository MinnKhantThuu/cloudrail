#!/usr/bin/env python3
"""Real-container proof for private DNS and public HTTP route controls."""
import json
import subprocess
import time
import urllib.error
import urllib.request

from test_client import Client


def wait_deployment(client, identifier, timeout=180):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        item = next(value for value in client.json('/api/state')['deployments'] if value['id'] == identifier)
        if item['status'] == 'active':
            return item
        if item['status'] in ('failed', 'superseded'):
            raise AssertionError(item)
        time.sleep(.35)
    raise AssertionError('Timed out waiting for networking deployment ' + identifier)


def inspect(identifier):
    return json.loads(subprocess.check_output(['docker', 'inspect', 'cloudrail-app-' + identifier]))[0]


def route_exists(service_id):
    result = subprocess.run(
        ['docker', 'exec', 'cloudrail-agent-1', 'test', '-e', '/routes/' + service_id + '.yaml'],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    return result.returncode == 0


def wait_route(service_id, expected, timeout=20):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if route_exists(service_id) == expected:
            return
        time.sleep(.25)
    raise AssertionError(('Public route did not converge', service_id, expected))


def public_get(host):
    request = urllib.request.Request('http://127.0.0.1:8088/', headers={'Host': host})
    with urllib.request.urlopen(request, timeout=10) as response:
        return response.status, response.headers.get('X-Cloudrail-Deployment')


def wait_public(host, deployment_id, timeout=20):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            if public_get(host) == (200, deployment_id):
                return
        except urllib.error.HTTPError as error:
            if error.code != 404:
                raise
        time.sleep(.25)
    raise AssertionError(('Public HTTP route did not become ready', host, deployment_id))


client = Client()
client.login()
subprocess.run(['docker', 'pull', 'traefik/whoami:v1.11.0'], check=True, stdout=subprocess.DEVNULL)
image = json.loads(subprocess.check_output(
    ['docker', 'image', 'inspect', 'traefik/whoami:v1.11.0', '--format', '{{json .RepoDigests}}']
))[0]
project = client.json('/api/projects', {'name': 'Networking proof ' + time.strftime('%m%d-%H%M%S')}, expected=201)
service = client.json('/api/projects/' + project['id'] + '/services', {'name': 'private-api'}, expected=201)
private_host = service['settings']['privateHost']
assert private_host.endswith('.internal') and service['settings']['publicEnabled'] is True

queued = client.json('/api/services/' + service['id'] + '/deployments', {
    'image': image, 'port': 80, 'healthPath': '/',
}, expected=202)
active = wait_deployment(client, queued['id'])
details = inspect(active['id'])
environment_network = next(name for name in details['NetworkSettings']['Networks'] if name.startswith('cloudrail-env-'))
assert private_host in details['NetworkSettings']['Networks'][environment_network]['Aliases']

probe_image = subprocess.check_output(
    ['docker', 'inspect', 'cloudrail-postgres-1', '--format', '{{.Config.Image}}'], text=True,
).strip()
resolved = subprocess.run(
    ['docker', 'run', '--rm', '--network', environment_network, probe_image, 'getent', 'hosts', private_host],
    text=True, capture_output=True,
)
assert resolved.returncode == 0 and private_host in resolved.stdout

client.json('/api/projects/' + project['id'] + '/environments', {'name': 'staging'}, expected=201)
staging = client.json('/api/projects/' + project['id'] + '/resources', {
    'name': 'staging-api', 'environment': 'staging', 'sourceType': 'image', 'workloadMode': 'web',
}, expected=201)
staging_queued = client.json('/api/services/' + staging['id'] + '/deployments', {
    'image': image, 'port': 80, 'healthPath': '/',
}, expected=202)
staging_active = wait_deployment(client, staging_queued['id'])
staging_details = inspect(staging_active['id'])
staging_network = next(name for name in staging_details['NetworkSettings']['Networks'] if name.startswith('cloudrail-env-'))
assert staging_network != environment_network
isolated = subprocess.run(
    ['docker', 'run', '--rm', '--network', staging_network, probe_image, 'getent', 'hosts', private_host],
    stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
)
assert isolated.returncode != 0, 'Private DNS leaked across environments'
print('PASS: stable private DNS resolves only inside the owning environment', flush=True)

generated_host = service['host']
wait_route(service['id'], True)
wait_public(generated_host, active['id'])
client.json('/api/services/' + service['id'] + '/networking', {
    'publicEnabled': False, 'targetPort': 80,
}, method='PUT')
wait_route(service['id'], False)
state_service = next(value for value in client.json('/api/state')['services'] if value['id'] == service['id'])
assert state_service['url'] == '' and state_service['settings']['privateHost'] == private_host
canvas = client.json('/api/projects/' + project['id'] + '/environments/production/canvas')
resource = next(value for value in canvas['resources'] if value['id'] == service['id'])
assert resource['privateAddress'] == private_host + ':80' and 'publicAddress' not in resource
print('PASS: internal-only mode removes the public route and keeps the active private service', flush=True)

custom_host = 'networking-proof.localhost'
client.json('/api/services/' + service['id'] + '/domain', {'host': custom_host}, method='PUT')
client.json('/api/services/' + service['id'] + '/networking', {
    'publicEnabled': True, 'targetPort': 80,
}, method='PUT')
wait_route(service['id'], True)
wait_public(custom_host, active['id'])
state_service = next(value for value in client.json('/api/state')['services'] if value['id'] == service['id'])
assert state_service['url'].endswith(custom_host) and state_service['settings']['targetPort'] == 80
assert state_service['settings']['privateHost'] == private_host
print('PASS: generated/custom domain and public route restore preserve the private hostname', flush=True)
