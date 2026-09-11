"""Fixed-purpose controller on the VM's isolated transport network."""

import json
import os
import platform
import subprocess
import urllib.request
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path

from http_server import CHECKSUM

SERVER = None
BASELINE = None
PROTOCOL = None
LOG = Path('/tmp/server.log')
HTTP = urllib.request.build_opener(urllib.request.ProxyHandler({}))


def run(*args):
    return subprocess.check_output(args, stderr=subprocess.STDOUT, text=True, timeout=30)


def snapshot():
    return {
        'routes': sorted(run('ip', '-4', 'route', 'show').splitlines()),
        'firewall': [run('iptables', '-t', table, '-S') for table in ('filter', 'nat', 'mangle')],
    }


def status():
    # Test the actual target, not just this management service.
    with HTTP.open('http://198.18.0.2:8080/sha256', timeout=5) as response:
        assert response.read() == CHECKSUM, 'HTTP target is not healthy'
    return {'arch': platform.machine(), 'sha256': CHECKSUM.decode().strip(),
            'server_exit': SERVER.poll() if SERVER else None}


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_):
        # Never log request bodies or generated client configuration.
        pass

    def reply(self, data, code=200):
        body = json.dumps(data).encode()
        self.send_response(code)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        try:
            if self.path == '/status':
                self.reply(status())
            elif self.path == '/diagnostics':
                self.reply({'network': snapshot(), 'links': run('ip', '-details', 'link'),
                            'server_log': LOG.read_text() if LOG.exists() else '',
                            'target_log': Path('/tmp/target.log').read_text()})
            else:
                self.reply({'error': 'unknown endpoint'}, 404)
        except Exception as error:
            self.reply({'error': str(error)}, 500)

    def do_POST(self):
        global SERVER, BASELINE, PROTOCOL
        try:
            size = int(self.headers.get('Content-Length', '0'))
            if not 0 < size < 4096:
                raise ValueError('invalid request size')
            data = json.loads(self.rfile.read(size))
            if self.path == '/start':
                if SERVER is not None:
                    raise ValueError('server already started')
                PROTOCOL = data['protocol']
                if PROTOCOL not in ('UDP', 'TCP', 'WS'):
                    raise ValueError('invalid protocol')
                # Only fixture-owned transport addresses are accepted.
                if data['host'] not in ('127.77.0.1', '192.0.2.15'):
                    raise ValueError('invalid transport host')
                env = dict(os.environ, Host=data['host'])
                env.update({f'Enable{p}': str(p == PROTOCOL).lower() for p in ('UDP', 'TCP', 'WS')})
                BASELINE = snapshot()
                directory = Path('/etc/tungo')
                directory.mkdir(mode=0o700, exist_ok=True)
                # Reserved benchmarking ranges avoid runner-private-network collisions.
                settings = {f'{p}Settings': {'IPv4Subnet': '198.19.0.0/24'} for p in ('UDP', 'TCP', 'WS')}
                (directory / 'server_configuration.json').write_text(json.dumps(settings))
                # Generate keys only after boot, so uploaded VM artifacts contain none.
                subprocess.run(['tungo', 's', 'gen'], env=env, check=True,
                               stdout=subprocess.DEVNULL, timeout=30)
                config = json.loads(Path('/etc/tungo/client_configuration.json.1').read_text())
                with LOG.open('w') as output:
                    SERVER = subprocess.Popen(['tungo', 's'], env=env, stdout=output, stderr=output)
                self.reply(config)
            elif self.path == '/stop':
                if SERVER is None:
                    raise ValueError('server not started')
                SERVER.terminate()
                code = SERVER.wait(timeout=30)
                if code != 0:
                    raise ValueError(f'server exit code: {code}')
                after = snapshot()
                if after != BASELINE:
                    raise ValueError(f'server state not restored: before={BASELINE}, after={after}')
                name = f's_{PROTOCOL.lower()}tun0'
                if subprocess.run(['ip', 'link', 'show', name], capture_output=True).returncode == 0:
                    raise ValueError(f'leftover TUN: {name}')
                self.reply({'restored': True})
            else:
                self.reply({'error': 'unknown endpoint'}, 404)
        except Exception as error:
            self.reply({'error': str(error)}, 500)


if __name__ == '__main__':
    HTTPServer(('192.0.2.15', 18080), Handler).serve_forever()
