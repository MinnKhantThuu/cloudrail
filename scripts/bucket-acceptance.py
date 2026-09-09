#!/usr/bin/env python3
"""Disposable MinIO proof for bucket creation, binding, rotation and revoke."""
import atexit
import json
import secrets
import subprocess
import time

from test_client import Client


MINIO_IMAGE = 'minio/minio:RELEASE.2025-09-07T16-13-09Z'
MC_IMAGE = 'minio/mc:RELEASE.2025-08-13T08-35-41Z'
suffix = secrets.token_hex(4)
minio_container = 'cloudrail-bucket-proof-' + suffix
minio_volume = 'cloudrail-bucket-proof-' + suffix


def cleanup():
    subprocess.run(['docker', 'rm', '-f', minio_container], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    subprocess.run(['docker', 'volume', 'rm', '-f', minio_volume], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


atexit.register(cleanup)


def wait(check, label, timeout=150):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        value = check()
        if value:
            return value
        time.sleep(.4)
    raise AssertionError('Timed out: ' + label)


def mc(network, user, password, command, expected=0):
    result = subprocess.run([
        'docker', 'run', '--rm', '--network', network,
        '-e', 'PROOF_USER=' + user, '-e', 'PROOF_PASSWORD=' + password,
        '--entrypoint', '/bin/sh', MC_IMAGE, '-lc',
        'mc alias set proof http://' + minio_container + ':9000 "$PROOF_USER" "$PROOF_PASSWORD" >/dev/null && ' + command,
    ], stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
    assert result.returncode == expected, (command, result.returncode)
    return result.stdout.decode().strip()


def start_minio(network, user, password):
    subprocess.run([
        'docker', 'run', '-d', '--name', minio_container, '--network', network,
        '-e', 'MINIO_ROOT_USER=' + user, '-e', 'MINIO_ROOT_PASSWORD=' + password,
        '-v', minio_volume + ':/data', MINIO_IMAGE, 'server', '/data',
    ], check=True, stdout=subprocess.DEVNULL)

    def ready():
        try:
            mc(network, user, password, 'mc ls proof >/dev/null')
            return True
        except AssertionError:
            return False

    wait(ready, 'MinIO readiness')


def active(client, service):
    state = client.json('/api/state')
    current = next(item for item in state['services'] if item['id'] == service['id'])
    deployments = [item for item in state['deployments'] if item['serviceId'] == service['id']]
    if deployments and deployments[0]['status'] == 'failed':
        raise AssertionError(deployments[0]['error'])
    return next((item for item in deployments if item['id'] == current['activeId']), None)


def changed_active(client, service, previous):
    item = active(client, service)
    return item if item and item['id'] != previous else None


client = Client()
client.login()
subprocess.run(['docker', 'pull', MINIO_IMAGE], check=True, stdout=subprocess.DEVNULL)
subprocess.run(['docker', 'pull', MC_IMAGE], check=True, stdout=subprocess.DEVNULL)
mc_digest = json.loads(subprocess.check_output(['docker', 'image', 'inspect', MC_IMAGE, '--format', '{{json .RepoDigests}}']))[0]
project = client.json('/api/projects', {'name': 'Bucket proof ' + suffix}, expected=201)
worker = client.json('/api/projects/' + project['id'] + '/resources', {
    'name': 'object-client', 'environment': 'production', 'sourceType': 'image', 'workloadMode': 'worker',
}, expected=201)
client.json('/api/services/' + worker['id'] + '/runtime', {
    'startCommand': 'while true; do sleep 30; done', 'preDeployCommand': '', 'preDeployTimeoutSeconds': 0,
    'restartPolicy': 'always', 'restartMaxRetries': 0,
}, method='PUT')

first_user = 'initial-' + suffix
first_password = secrets.token_urlsafe(24)
remote_name = 'uploads-' + suffix
bucket = client.json('/api/projects/' + project['id'] + '/buckets', {
    'name': 'uploads', 'environment': 'production', 'endpoint': 'http://' + minio_container + ':9000',
    'region': 'us-east-1', 'bucketName': remote_name, 'accessKeyId': first_user,
    'secretAccessKey': first_password, 'forcePathStyle': True,
}, expected=201)
assert bucket['credentialVersion'] == 1
assert first_user not in json.dumps(bucket) and first_password not in json.dumps(bucket)
client.json('/api/buckets/' + bucket['id'] + '/bindings/' + worker['id'], {'variablePrefix': 'UPLOADS'}, method='PUT')
names = client.json('/api/services/' + worker['id'] + '/variables')['names']
assert all(name in names for name in (
    'UPLOADS_BUCKET', 'UPLOADS_ENDPOINT', 'UPLOADS_REGION', 'UPLOADS_ACCESS_KEY_ID',
    'UPLOADS_SECRET_ACCESS_KEY', 'UPLOADS_FORCE_PATH_STYLE',
))
client.json('/api/services/' + worker['id'] + '/deployments', {'image': mc_digest, 'port': 80, 'healthPath': '/'}, expected=202)
first_deployment = wait(lambda: active(client, worker), 'bucket client deployment')
network = worker['settings']['network']
start_minio(network, first_user, first_password)
mc(network, first_user, first_password, 'mc mb --ignore-existing proof/' + remote_name + ' >/dev/null')
first_container = 'cloudrail-app-' + first_deployment['id']
value = subprocess.check_output([
    'docker', 'exec', first_container, '/bin/sh', '-lc',
    'mc alias set live "$UPLOADS_ENDPOINT" "$UPLOADS_ACCESS_KEY_ID" "$UPLOADS_SECRET_ACCESS_KEY" >/dev/null && printf portable-object | mc pipe live/"$UPLOADS_BUCKET"/proof.txt >/dev/null && mc cat live/"$UPLOADS_BUCKET"/proof.txt',
]).decode().strip()
assert value == 'portable-object'
canvas = client.json('/api/projects/' + project['id'] + '/environments/production/canvas')
resource = next(item for item in canvas['resources'] if item['id'] == bucket['id'])
assert resource['kind'] == 'bucket' and resource['status'] == 'connected' and resource['credentialVersion'] == 1
assert any(link['kind'] == 'bucket-binding' and link['label'] == 'UPLOADS_*' for link in canvas['links'])
print('PASS: bucket credentials stay hidden and a bound application uploads/downloads through MinIO', flush=True)

second_user = 'rotated-' + suffix
second_password = secrets.token_urlsafe(24)
subprocess.run(['docker', 'rm', '-f', minio_container], check=True, stdout=subprocess.DEVNULL)
start_minio(network, second_user, second_password)
old = subprocess.run([
    'docker', 'run', '--rm', '--network', network,
    '-e', 'PROOF_USER=' + first_user, '-e', 'PROOF_PASSWORD=' + first_password,
    '--entrypoint', '/bin/sh', MC_IMAGE, '-lc',
    'mc alias set old http://' + minio_container + ':9000 "$PROOF_USER" "$PROOF_PASSWORD" >/dev/null && mc stat old/' + remote_name + '/proof.txt >/dev/null',
], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
assert old.returncode != 0
rotated = client.json('/api/buckets/' + bucket['id'] + '/credentials', {
    'accessKeyId': second_user, 'secretAccessKey': second_password,
}, method='PUT')
assert rotated['credentialVersion'] == 2
assert second_user not in json.dumps(rotated) and second_password not in json.dumps(rotated)
client.json('/api/services/' + worker['id'] + '/deployments', {'image': mc_digest, 'port': 80, 'healthPath': '/'}, expected=202)
second_deployment = wait(lambda: changed_active(client, worker, first_deployment['id']), 'rotated bucket client deployment')
second_container = 'cloudrail-app-' + second_deployment['id']
value = subprocess.check_output([
    'docker', 'exec', second_container, '/bin/sh', '-lc',
    'mc alias set live "$UPLOADS_ENDPOINT" "$UPLOADS_ACCESS_KEY_ID" "$UPLOADS_SECRET_ACCESS_KEY" >/dev/null && mc cat live/"$UPLOADS_BUCKET"/proof.txt',
]).decode().strip()
assert value == 'portable-object'
print('PASS: credential rotation invalidates the old login and updates the next deployment', flush=True)

client.json('/api/buckets/' + bucket['id'] + '/bindings/' + worker['id'], method='DELETE')
names = client.json('/api/services/' + worker['id'] + '/variables')['names']
assert not any(name.startswith('UPLOADS_') for name in names)
client.json('/api/services/' + worker['id'] + '/deployments', {'image': mc_digest, 'port': 80, 'healthPath': '/'}, expected=202)
third_deployment = wait(lambda: changed_active(client, worker, second_deployment['id']), 'revoked bucket client deployment')
subprocess.run([
    'docker', 'exec', 'cloudrail-app-' + third_deployment['id'], '/bin/sh', '-lc',
    'test -z "${UPLOADS_SECRET_ACCESS_KEY+x}"',
], check=True)
canvas = client.json('/api/projects/' + project['id'] + '/environments/production/canvas')
resource = next(item for item in canvas['resources'] if item['id'] == bucket['id'])
assert resource['status'] == 'available' and not any(link['kind'] == 'bucket-binding' for link in canvas['links'])
print('PASS: disconnect removes managed variables and the real canvas binding', flush=True)
