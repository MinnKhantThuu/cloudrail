#!/usr/bin/env python3
"""Real Linux/Docker acceptance. Creates its own project; retains history for review."""
import json
import pathlib
import subprocess
import time
import urllib.error
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parents[1]
ENV = dict(line.split('=', 1) for line in (ROOT / 'deploy/local/.env').read_text().splitlines() if '=' in line)
BASE = 'http://127.0.0.1:8080'
COMPOSE = ['bash', str(ROOT / 'scripts/compose.sh')]

from test_client import Client
client = Client()
def api(path, body=None, token=None, expected=200):
    if token is not None:
        return Client().json(path, body, expected=expected, headers={'Authorization':'Bearer '+token})
    return client.json(path, body, expected=expected)

def state():
    return api('/api/state')

def wait_deployment(identifier, wanted, timeout=220):
    until = time.monotonic() + timeout
    while time.monotonic() < until:
        d = next(d for d in state()['deployments'] if d['id'] == identifier)
        if d['status'] in wanted:
            return d
        if d['status'] in ('active', 'failed', 'superseded'):
            raise AssertionError(('Unexpected terminal state', d['status'], d['error']))
        time.sleep(.35)
    raise AssertionError(('Deployment timed out', identifier))

def assert_route(service, deployment):
    req = urllib.request.Request('http://127.0.0.1:8088/', headers={'Host': service['host']})
    with urllib.request.urlopen(req, timeout=5) as response:
        assert response.status == 200
        assert response.headers.get('X-Cloudrail-Deployment') == deployment['id'], 'Stale route'
        assert 'Hostname:' in response.read().decode(), 'Unexpected application response'

def deploy(service, image, port=80):
    return api('/api/services/' + service['id'] + '/deployments', {'image': image, 'port': port, 'healthPath': '/'}, expected=202)

def main():
    for _ in range(60):
        try:
            urllib.request.urlopen(BASE + '/healthz', timeout=2).close()
            break
        except (OSError, urllib.error.URLError):
            time.sleep(1)
    else:
        raise AssertionError('Control plane did not become healthy')
    client.login()
    api('/api/state', token='incorrect-token', expected=401)
    api('/internal/claim', {}, expected=401)
    print('PASS: admin and agent authentication boundaries', flush=True)

    subprocess.run(['docker', 'pull', 'traefik/whoami:v1.11.0'], check=True, stdout=subprocess.DEVNULL)
    digests = json.loads(subprocess.check_output(['docker', 'image', 'inspect', 'traefik/whoami:v1.11.0', '--format', '{{json .RepoDigests}}']))
    image = digests[0]
    project = api('/api/projects', {'name': 'Acceptance ' + time.strftime('%m%d-%H%M%S')}, expected=201)
    service = api('/api/projects/' + project['id'] + '/services', {'name': 'hello-api'}, expected=201)
    api('/api/services/' + service['id'] + '/deployments', {'image': 'nginx:latest', 'port': 80, 'healthPath': '/'}, expected=400)
    print('PASS: project/service creation and immutable image validation', flush=True)

    first = deploy(service, image)
    wait_deployment(first['id'], {'active'})
    assert_route(service, first)
    print('PASS: pinned image serves real HTTP traffic through Traefik', flush=True)

    bad = deploy(service, image, port=1)
    wait_deployment(bad['id'], {'checking'})
    # The old app must serve while readiness is pending, not only after failure.
    for _ in range(4):
        assert_route(service, first)
        time.sleep(.3)
    failed = wait_deployment(bad['id'], {'failed'})
    assert 'readiness timed out' in failed['error']
    assert failed['logs'], 'Failed candidate logs were not retained'
    assert_route(service, first)
    print('PASS: failed readiness preserves old traffic and captures failure logs', flush=True)

    missing = deploy(service, image.split('@')[0] + '@sha256:' + '0' * 64)
    wait_deployment(missing['id'], {'failed'})
    assert_route(service, first)
    print('PASS: image pull failure preserves active release', flush=True)

    interrupted = deploy(service, image, port=1)
    wait_deployment(interrupted['id'], {'checking'})
    subprocess.run(COMPOSE + ['restart', 'agent'], check=True, stdout=subprocess.DEVNULL)
    assert_route(service, first)
    wait_deployment(interrupted['id'], {'failed'})
    assert_route(service, first)
    names = subprocess.check_output(['docker', 'ps', '-a', '--filter', 'label=cloudrail.deployment=' + interrupted['id'], '--format', '{{.Names}}']).decode().splitlines()
    assert not names, ('Failed candidate was not removed', names)
    print('PASS: agent restart resumes job and cleans failed candidate', flush=True)

    replacement = deploy(service, image)
    wait_deployment(replacement['id'], {'active'})
    assert_route(service, replacement)
    until = time.monotonic() + 20
    while time.monotonic() < until:
        inspection = subprocess.run(['docker', 'inspect', 'cloudrail-app-' + first['id'], '--format', '{{.State.Running}}'], capture_output=True, text=True)
        running = inspection.stdout.strip() if inspection.returncode == 0 else 'false'
        if running == 'false':
            break
        time.sleep(.5)
    assert running == 'false', 'Old container not retired'
    snapshot = state()
    assert next(d for d in snapshot['deployments'] if d['id'] == first['id'])['status'] == 'superseded'
    assert any(e['deploymentId'] == replacement['id'] and e['stage'] == 'active' for e in snapshot['events'])
    print('PASS: healthy replacement activates before old container retires', flush=True)

    subprocess.run(COMPOSE + ['restart', 'server'], check=True, stdout=subprocess.DEVNULL)
    for _ in range(30):
        try:
            snapshot = state()
            break
        except (OSError, urllib.error.URLError):
            time.sleep(1)
    assert next(s for s in snapshot['services'] if s['id'] == service['id'])['activeId'] == replacement['id']
    assert_route(service, replacement)
    print('PASS: API restart preserves history and traffic', flush=True)
    print('Acceptance complete. Project:', project['name'])
    print('Application: http://' + service['host'] + ':8088')
    print('Image:', image)

if __name__ == '__main__':
    main()
