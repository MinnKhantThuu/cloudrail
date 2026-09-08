"""Generate the dashboard route from the installation's retained domain."""
import json
import pathlib
import re
import sys


def write_dashboard_route(path, domain):
    labels = domain.split('.')
    if len(domain) > 253 or len(labels) < 2 or any(
        not re.fullmatch(r'[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?', label) for label in labels
    ):
        raise ValueError('Use a lowercase dashboard DNS hostname')
    route = {'http': {
        'routers': {'cloudrail-dashboard': {
            'rule': 'Host(`' + domain + '`)', 'entryPoints': ['websecure'],
            'service': 'cloudrail-dashboard', 'tls': {'certResolver': 'letsencrypt'},
        }},
        'services': {'cloudrail-dashboard': {'loadBalancer': {'servers': [{'url': 'http://server:8080'}]}}},
    }}
    path = pathlib.Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(route))
    path.chmod(0o600)


if __name__ == '__main__':
    if len(sys.argv) != 3:
        raise SystemExit('usage: public_route.py DOMAIN OUTPUT_PATH')
    write_dashboard_route(sys.argv[2], sys.argv[1])
