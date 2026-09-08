"""Exercise the regenerated dashboard route with a trusted local TLS fixture.

Runs only after recovery on a disposable host. Does not contact an ACME server.
"""
import importlib.util
import json
import os
import pathlib
import subprocess
import time

if os.environ.get('CLOUDRAIL_DISPOSABLE_TEST') != '1':
    raise SystemExit('Requires an explicitly disposable recovered installation')
os.umask(0o077)
ROOT = pathlib.Path(__file__).resolve().parents[1]
checkpoint = json.loads((ROOT / '.data/host-restore.json').read_text())
if checkpoint['phase'] != 'restored':
    raise SystemExit('Run only after a completed host restore')
spec = importlib.util.spec_from_file_location('host_recovery', ROOT / 'scripts/host-recovery.py')
recovery = importlib.util.module_from_spec(spec)
spec.loader.exec_module(recovery)


def run(*args):
    return subprocess.check_output(args, text=True, stderr=subprocess.PIPE).strip()


directory = ROOT / '.data/dashboard-tls-fixture'
directory.mkdir(mode=0o700)
domain = 'console.cloudrail.test'
name = 'cloudrail-dashboard-recovery-proof'
# Refuse to replace a preexisting dashboard route, even in a disposable fixture.
run('docker', 'exec', 'cloudrail-agent-1', 'test', '!', '-e', '/routes/controlplane.yaml')
try:
    recovery.restore_dashboard_route(domain)
    run('docker', 'cp', 'cloudrail-agent-1:/routes/controlplane.yaml', str(directory / 'controlplane.yaml'))
    run('openssl', 'req', '-x509', '-newkey', 'rsa:2048', '-nodes',
        '-keyout', str(directory / 'key.pem'), '-out', str(directory / 'cert.pem'),
        '-days', '1', '-subj', '/CN=' + domain, '-addext', 'subjectAltName=DNS:' + domain)
    (directory / 'tls.yaml').write_text(json.dumps({'tls': {'certificates': [{
        'certFile': '/lab/cert.pem', 'keyFile': '/lab/key.pem',
    }]}}))
    run('docker', 'run', '--rm', '-d', '--name', name, '--pull', 'never',
        '--network', 'cloudrail_dashboard', '--memory', '128m',
        '--mount', 'type=bind,src=' + str(directory) + ',dst=/lab,readonly',
        '-p', '127.0.0.1:18443:443', 'traefik:v3.5.2',
        '--entrypoints.websecure.address=:443', '--providers.file.directory=/lab', '--log.level=WARN')
    curl = ('curl', '--silent', '--show-error', '--fail', '--noproxy', '*',
            '--max-time', '3', '--cacert', str(directory / 'cert.pem'),
            '--resolve', domain + ':18443:127.0.0.1')
    end = time.monotonic() + 30
    while time.monotonic() < end:
        try:
            status = json.loads(run(*curl, 'https://' + domain + ':18443/auth/status'))
            assert status['configured'] is True
            html = run(*curl, 'https://' + domain + ':18443/')
            assert 'id="root"' in html
            print('PASS regenerated dashboard route: trusted TLS, retained owner status and dashboard HTML on the independent recovered host')
            break
        except subprocess.CalledProcessError:
            time.sleep(.5)
    else:
        raise AssertionError('Regenerated dashboard HTTPS route did not become ready')
finally:
    subprocess.run(['docker', 'rm', '-f', name], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    run('docker', 'exec', 'cloudrail-agent-1', 'rm', '-f', '/routes/controlplane.yaml')
