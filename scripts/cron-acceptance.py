#!/usr/bin/env python3
"""Disposable real-Docker proof for scheduled one-shot cron workloads."""
import json
import subprocess
import time

from test_client import Client


def sql(statement):
    subprocess.run(
        ['docker', 'exec', 'cloudrail-postgres-1', 'psql', '-U', 'cloudrail', '-d', 'cloudrail', '-v', 'ON_ERROR_STOP=1', '-c', statement],
        check=True, stdout=subprocess.DEVNULL,
    )


def wait_deployment(client, identifier, timeout=180):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        item = next(d for d in client.json('/api/state')['deployments'] if d['id'] == identifier)
        if item['status'] == 'active':
            return item
        if item['status'] in ('failed', 'superseded'):
            raise AssertionError(('Cron deployment failed', item['status'], item['error']))
        time.sleep(.3)
    raise AssertionError(('Cron deployment timed out', identifier))


def wait_run(client, identifier=None, service=None, timeout=45):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        runs = client.json('/api/state')['cronRuns']
        item = next((run for run in runs if (identifier and run['id'] == identifier) or (service and run['serviceId'] == service)), None)
        if item and item['status'] in ('succeeded', 'failed'):
            return item
        time.sleep(.3)
    raise AssertionError(('Cron run timed out', identifier or service))


client = Client()
client.login()
subprocess.run(['docker', 'pull', 'hello-world:latest'], check=True, stdout=subprocess.DEVNULL)
image = json.loads(subprocess.check_output(
    ['docker', 'image', 'inspect', 'hello-world:latest', '--format', '{{json .RepoDigests}}']
))[0]
project = client.json('/api/projects', {'name': 'Cron proof ' + time.strftime('%m%d-%H%M%S')}, expected=201)
cron = client.json('/api/projects/' + project['id'] + '/resources', {
    'name': 'scheduled-cleanup', 'environment': 'production', 'sourceType': 'image', 'workloadMode': 'cron',
}, expected=201)
client.json('/api/services/' + cron['id'] + '/cron', {'schedule': '* * * * *'}, method='PUT')
deployment = client.json('/api/services/' + cron['id'] + '/deployments', {
    'image': image, 'port': 80, 'healthPath': '/',
}, expected=202)
wait_deployment(client, deployment['id'])
route = subprocess.run(['docker', 'exec', 'cloudrail-agent-1', 'test', '-e', '/routes/' + cron['id'] + '.yaml'])
assert route.returncode != 0, 'Cron deployment received a public route'

sql("UPDATE services SET cron_next_run=now()-interval '1 second' WHERE id='" + cron['id'] + "'")
normal = wait_run(client, service=cron['id'])
assert normal['status'] == 'succeeded' and normal.get('exitCode') == 0
assert 'Hello from Docker' in normal['logs']
assert not any(run['serviceId'] == cron['id'] and run['status'] in ('queued', 'running') for run in client.json('/api/state')['cronRuns'])
print('PASS: due UTC cron executes once, captures logs and has no route', flush=True)

recovery_id = 'feedfacefeedfacefeedface'
subprocess.run(['bash', 'scripts/compose.sh', 'stop', 'agent'], check=True, stdout=subprocess.DEVNULL)
sql("INSERT INTO cron_runs(id,service_id,deployment_id,scheduled_for,status) VALUES('" + recovery_id + "','" + cron['id'] + "','" + deployment['id'] + "',now(),'running')")
subprocess.run([
    'docker', 'create', '--name', 'cloudrail-cron-' + recovery_id,
    '--label', 'cloudrail.managed=true', '--label', 'cloudrail.cron-run=' + recovery_id,
    '--label', 'cloudrail.deployment=' + deployment['id'], '--label', 'cloudrail.service=' + cron['id'], image,
], check=True, stdout=subprocess.DEVNULL)
subprocess.run(['docker', 'start', '-a', 'cloudrail-cron-' + recovery_id], check=True, stdout=subprocess.DEVNULL)
subprocess.run(['bash', 'scripts/compose.sh', 'start', 'agent'], check=True, stdout=subprocess.DEVNULL)
recovered = wait_run(client, identifier=recovery_id)
assert recovered['status'] == 'succeeded' and recovered.get('exitCode') == 0
assert 'Hello from Docker' in recovered['logs']
assert subprocess.run(['docker', 'inspect', 'cloudrail-cron-' + recovery_id], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode != 0
print('PASS: agent restart resumes the same exited cron container and records one result', flush=True)
