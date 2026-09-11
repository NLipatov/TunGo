"""Run one real native client against one Linux QEMU guest. CI runners only."""

import argparse
import ctypes
import hashlib
import ipaddress
import json
import os
import platform
import re
import shutil
import signal
import socket
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

SYSTEM = platform.system()
HTTP = urllib.request.build_opener(urllib.request.ProxyHandler({}))
TARGET = 'http://198.18.0.2:8080'


def run(*args, check=True):
    result = subprocess.run(args, capture_output=True, text=True, timeout=45)
    if check and result.returncode:
        raise RuntimeError(f'{args}: {result.stdout}\n{result.stderr}')
    return result.stdout.strip()


def powershell(script):
    return run('pwsh', '-NoProfile', '-NonInteractive', '-Command',
               "$ErrorActionPreference='Stop'; " + script)


def request(url, data=None, timeout=10):
    body = json.dumps(data).encode() if data is not None else None
    req = urllib.request.Request(url, data=body, headers={'Content-Type': 'application/json'})
    try:
        with HTTP.open(req, timeout=timeout) as response:
            return response.read()
    except urllib.error.HTTPError as error:
        raise RuntimeError(f'{url}: {error.read().decode()}') from error


def wait_until(description, action, timeout=90):
    deadline = time.monotonic() + timeout
    last_error = None
    while time.monotonic() < deadline:
        try:
            result = action()
            if result:
                return result
        except (OSError, RuntimeError, ValueError) as error:
            last_error = error
        time.sleep(0.5)
    raise RuntimeError(f'Timed out: {description}; {last_error}')


def snapshot():
    """Capture stable routing fields; omit lifetimes, counters, and ARP caches."""
    state = {'interfaces': sorted(name for _, name in socket.if_nameindex())}
    if SYSTEM == 'Linux':
        fields = ('dst', 'gateway', 'dev', 'table', 'metric', 'prefsrc', 'type', 'scope')
        rows = json.loads(run('ip', '-j', '-4', 'route', 'show', 'table', 'main'))
        state['routes'] = sorted(json.dumps({k: row[k] for k in fields if k in row}, sort_keys=True) for row in rows)
        state['firewall'] = [run('iptables', '-t', table, '-S') for table in ('filter', 'nat', 'mangle')]
    elif SYSTEM == 'Darwin':
        rows = []
        for line in run('netstat', '-rn', '-f', 'inet').splitlines():
            parts = line.split()
            if len(parts) >= 4 and (parts[0] == 'default' or parts[0][0].isdigit()):
                if 'L' not in parts[2] and 'W' not in parts[2]:
                    rows.append(' '.join(parts[:4]))
        state['routes'] = sorted(rows)
    else:
        rows = json.loads(powershell(
            '@(Get-NetRoute -AddressFamily IPv4 | Select-Object DestinationPrefix,NextHop,InterfaceIndex,RouteMetric) '
            '| ConvertTo-Json -Compress'))
        state['routes'] = sorted(json.dumps(row, sort_keys=True) for row in rows)
    return state


class RunnerRoutes:
    """Keep CI control traffic online, leaving both test subnets to TunGo."""

    def __init__(self):
        self.added = []
        self.gateway = None
        self.interface = None

    def install(self):
        if SYSTEM == 'Linux':
            route = json.loads(run('ip', '-j', '-4', 'route', 'get', '1.1.1.1'))[0]
            self.gateway, self.interface = route['gateway'], route['dev']
        elif SYSTEM == 'Darwin':
            output = run('route', '-n', 'get', 'default')
            self.gateway = re.search(r'gateway:\s+(\S+)', output)[1]
        else:
            route = json.loads(powershell(
                'Get-NetRoute -DestinationPrefix 0.0.0.0/0 | Sort-Object RouteMetric | Select-Object -First 1 '
                '-Property NextHop,InterfaceIndex | ConvertTo-Json -Compress'))
            self.gateway, self.interface = route['NextHop'], str(route['InterfaceIndex'])
        excluded = ipaddress.ip_network('198.18.0.0/15')
        for network in ipaddress.ip_network('0.0.0.0/0').address_exclude(excluded):
            # No /1 here: those exact routes belong to TunGo and are asserted below.
            for prefix in network.subnets(new_prefix=2) if network.prefixlen < 2 else [network]:
                prefix = str(prefix)
                if SYSTEM == 'Linux':
                    run('ip', 'route', 'add', prefix, 'via', self.gateway, 'dev', self.interface)
                elif SYSTEM == 'Darwin':
                    run('route', '-n', 'add', '-net', prefix, self.gateway)
                else:
                    powershell(f"New-NetRoute -DestinationPrefix '{prefix}' -NextHop '{self.gateway}' "
                               f'-InterfaceIndex {self.interface} -PolicyStore ActiveStore | Out-Null')
                self.added.append(prefix)

    def remove(self):
        for prefix in reversed(self.added):
            if SYSTEM == 'Linux':
                run('ip', 'route', 'del', prefix, 'via', self.gateway, 'dev', self.interface, check=False)
            elif SYSTEM == 'Darwin':
                run('route', '-n', 'delete', '-net', prefix, self.gateway, check=False)
            else:
                powershell(f"Get-NetRoute -DestinationPrefix '{prefix}' -InterfaceIndex {self.interface} "
                           '-ErrorAction SilentlyContinue | Remove-NetRoute -Confirm:$false')


def assert_tunnel_route():
    if SYSTEM == 'Linux':
        routes = json.loads(run('ip', '-j', '-4', 'route', 'show'))
        lower = [r for r in routes if r.get('dst') == '0.0.0.0/1']
        upper = [r for r in routes if r.get('dst') == '128.0.0.0/1']
        target = json.loads(run('ip', '-j', '-4', 'route', 'get', '198.18.0.2'))[0]
        assert lower and upper and lower[0]['dev'] == upper[0]['dev'] == target['dev']
        assert target['dev'].startswith('c_')
    elif SYSTEM == 'Darwin':
        interface = re.search(r'interface:\s+(\S+)', run('route', '-n', 'get', '198.18.0.2'))[1]
        assert interface.startswith('utun'), interface
        routes = run('netstat', '-rn', '-f', 'inet').splitlines()
        for prefix in ('0/1', '128.0/1'):
            assert any(line.split()[0] == prefix and interface in line.split() for line in routes if line.split()), routes
    else:
        interface = powershell(
            "(Find-NetRoute -RemoteIPAddress 198.18.0.2 | Where-Object { $_.PSObject.Properties['InterfaceAlias'] } "
            '| Select-Object -First 1).InterfaceAlias')
        assert interface.startswith('c_'), interface
        for prefix in ('0.0.0.0/1', '128.0.0.0/1'):
            aliases = powershell(f"(Get-NetRoute -DestinationPrefix '{prefix}').InterfaceAlias")
            assert interface in aliases.splitlines(), aliases


def stop_client(process):
    """Exercise the application's shutdown handler, including on Windows."""
    if process.poll() is not None:
        raise RuntimeError(f'Client exited before shutdown: {process.returncode}')
    if SYSTEM == 'Windows':
        # Go maps CTRL_BREAK_EVENT to os.Interrupt. TerminateProcess skips cleanup.
        process.send_signal(signal.CTRL_BREAK_EVENT)
    else:
        process.send_signal(signal.SIGTERM)
    if process.wait(timeout=45) != 0:
        raise RuntimeError(f'Client did not exit cleanly: {process.returncode}')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--protocol', choices=['UDP', 'TCP', 'WS'], required=True)
    parser.add_argument('--client-arch', choices=['amd64', 'arm64'], required=True)
    parser.add_argument('--server-arch', choices=['amd64', 'arm64'], required=True)
    parser.add_argument('--image-dir', type=Path, required=True)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--artifacts', type=Path, required=True)
    args = parser.parse_args()
    if os.environ.get('GITHUB_ACTIONS') != 'true':
        raise RuntimeError('This fixture changes host routes: run it only on disposable GitHub Actions runners')
    arch = {'x86_64': 'amd64', 'AMD64': 'amd64', 'aarch64': 'arm64', 'arm64': 'arm64', 'ARM64': 'arm64'}[platform.machine()]
    assert arch == args.client_arch, f'Client runner architecture {arch} != {args.client_arch}'
    if SYSTEM == 'Windows':
        assert ctypes.windll.shell32.IsUserAnAdmin(), 'Administrator required'
        # Actions may launch Python without a console; the child needs one for Ctrl+Break.
        if not ctypes.windll.kernel32.GetConsoleWindow():
            ctypes.windll.kernel32.AllocConsole()
    else:
        assert os.geteuid() == 0, 'Root required'
    args.artifacts.mkdir(parents=True, exist_ok=True)
    endpoint = '192.0.2.15' if SYSTEM == 'Darwin' else '127.77.0.1'
    control = 'http://' + ('192.0.2.15' if SYSTEM == 'Darwin' else '127.0.0.1') + ':18080'
    qemu_arch = 'x86_64' if args.server_arch == 'amd64' else 'aarch64'
    qemu = shutil.which(f'qemu-system-{qemu_arch}')
    assert qemu, f'Missing QEMU for {args.server_arch}'
    command = [qemu, '-accel', 'tcg', '-m', '1024', '-smp', '2', '-display', 'none', '-monitor', 'none',
               '-no-reboot', '-kernel', str(args.image_dir / 'kernel'),
               '-initrd', str(args.image_dir / 'initramfs.gz'), '-serial', 'stdio']
    console = 'ttyS0' if args.server_arch == 'amd64' else 'ttyAMA0'
    command += ['-append', f'console={console} rdinit=/init panic=1']
    command += ['-machine', 'q35' if args.server_arch == 'amd64' else 'virt',
                '-cpu', 'max' if args.server_arch == 'amd64' else 'cortex-a72']
    if SYSTEM == 'Darwin':
        network = 'vmnet-host,id=transport,start-address=192.0.2.1,end-address=192.0.2.254,subnet-mask=255.255.255.0'
    else:
        network = ('user,id=transport,net=192.0.2.0/24,restrict=on,'
                   'hostfwd=tcp:127.0.0.1:18080-192.0.2.15:18080,'
                   'hostfwd=tcp:127.77.0.1:8080-192.0.2.15:8080,'
                   'hostfwd=udp:127.77.0.1:9090-192.0.2.15:9090,'
                   'hostfwd=tcp:127.77.0.1:1010-192.0.2.15:1010')
    command += ['-netdev', network, '-device', 'virtio-net-pci,netdev=transport']
    routes = RunnerRoutes()
    vm = client = None
    config_path = Path(os.environ.get('ProgramData', 'C:/ProgramData')) / 'TunGo/client_configuration.json' if SYSTEM == 'Windows' else Path('/etc/tungo/client_configuration.json')
    assert not config_path.exists(), 'Refusing to overwrite existing TunGo configuration'
    config_written = False

    def control_call(path, data=None):
        return json.loads(request(control + path, data, timeout=45))

    def no_bypass():
        control_call('/status')
        try:
            request(TARGET + '/sha256', timeout=2)
        except (OSError, RuntimeError):
            return
        raise AssertionError('Target reachable without TunGo')

    def save_state(name):
        state = snapshot()
        (args.artifacts / f'{name}.json').write_text(json.dumps(state, indent=2))
        return state

    try:
        with (args.artifacts / 'vm.log').open('w') as log:
            vm = subprocess.Popen(command, stdout=log, stderr=log)
        def ready():
            if vm.poll() is not None:
                raise RuntimeError(f'VM exited: {vm.returncode}')
            return control_call('/status')
        info = wait_until('Linux guest boot', ready, timeout=240)
        expected_guest = 'x86_64' if args.server_arch == 'amd64' else 'aarch64'
        assert info['arch'] == expected_guest, info
        print(f'Native {SYSTEM}/{arch} -> Linux/{info["arch"]} (QEMU TCG), {args.protocol}', flush=True)
        routes.install()
        baseline = save_state('client-before')
        no_bypass()
        config = control_call('/start', {'protocol': args.protocol, 'host': endpoint})
        config_path.parent.mkdir(parents=True, exist_ok=True)
        with config_path.open('x') as output:
            config_written = True
            if SYSTEM != 'Windows':
                os.fchmod(output.fileno(), 0o600)
            json.dump(config, output)
        for cycle in (1, 2):
            print(f'Connection cycle {cycle}', flush=True)
            with (args.artifacts / f'client-{cycle}.log').open('w') as log:
                options = {'creationflags': subprocess.CREATE_NEW_PROCESS_GROUP} if SYSTEM == 'Windows' else {}
                client = subprocess.Popen([str(args.binary.resolve()), 'c'], stdout=log, stderr=log, **options)
            def connected():
                assert client.poll() is None, f'Client exited: {client.returncode}'
                return request(TARGET + '/sha256', timeout=2)
            wait_until('tunneled HTTP', connected)
            assert_tunnel_route()
            ping_args = ['-n', '3', '-w', '2000'] if SYSTEM == 'Windows' else ['-n', '-c', '3']
            run('ping', *ping_args, '198.19.0.1')
            payload = request(TARGET + '/payload', timeout=60)
            assert len(payload) == 4 * 1024 * 1024, 'Payload truncated'
            assert hashlib.sha256(payload).hexdigest() == info['sha256'], 'Checksum mismatch'
            assert request(TARGET + '/peer').decode().strip() == '198.18.0.1', 'MASQUERADE missing'
            save_state(f'client-connected-{cycle}')
            stop_client(client)
            client = None
            wait_until('client routes and interfaces restored', lambda: snapshot() == baseline, timeout=30)
            save_state(f'client-after-{cycle}')
            no_bypass()
        assert control_call('/stop', {})['restored']
        no_bypass()
        print('PASS: TUN, routes, 4 MiB checksum, NAT, client restart, graceful cleanup', flush=True)
    finally:
        if client is not None and client.poll() is None:
            try:
                stop_client(client)
            except Exception:
                client.kill()
                client.wait(timeout=10)
        try:
            save_state('client-final')
            (args.artifacts / 'server.json').write_text(json.dumps(control_call('/diagnostics'), indent=2))
        except Exception as error:
            print(f'Diagnostics error: {error}', file=sys.stderr)
        if vm is not None and vm.poll() is None:
            vm.terminate()
            try:
                vm.wait(timeout=10)
            except subprocess.TimeoutExpired:
                vm.kill()
                vm.wait(timeout=10)
        if config_written:
            config_path.unlink(missing_ok=True)
        routes.remove()


if __name__ == '__main__':
    main()
