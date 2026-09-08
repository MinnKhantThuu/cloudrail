"""Cookie-authenticated local acceptance helper. Never prints credentials."""
import http.cookiejar
import json
import pathlib
import secrets
import urllib.error
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parents[1]
BASE = 'http://127.0.0.1:8080'
ENV = dict(line.split('=', 1) for line in (ROOT / 'deploy/local/.env').read_text().splitlines() if '=' in line)

class Client:
    def __init__(self):
        self.cookies = http.cookiejar.CookieJar()
        self.opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(self.cookies))

    def json(self, path, body=None, expected=200, method=None, headers=None):
        request = urllib.request.Request(BASE + path, data=json.dumps(body).encode() if body is not None else None,
            method=method, headers={'Content-Type':'application/json', 'X-Cloudrail-Request':'1', **(headers or {})})
        try:
            with self.opener.open(request, timeout=30) as response:
                assert response.status == expected, (path, response.status)
                return json.load(response)
        except urllib.error.HTTPError as error:
            assert error.code == expected, (path, error.code, error.read().decode())
            return json.load(error)

    def login(self):
        status = self.json('/auth/status')
        path = ROOT / '.data/test-owner.json'
        if not status['configured']:
            owner = {'email':'owner@cloudrail.local', 'password':secrets.token_urlsafe(24)}
            path.parent.mkdir(exist_ok=True)
            path.write_text(json.dumps(owner)); path.chmod(0o600)
            self.json('/auth/setup', {**owner, 'passwordConfirmation': owner['password']}, expected=201)
        else:
            if not path.exists():
                raise RuntimeError('Owner is configured. Supply local test credentials in .data/test-owner.json without changing the existing owner.')
            owner = json.loads(path.read_text())
            self.json('/auth/login', owner)
        return owner
