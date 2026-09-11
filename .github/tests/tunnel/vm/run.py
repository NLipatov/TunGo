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
FAMILIES = {
    4: {'target': '198.18.0.2', 'url': 'http://198.18.0.2:8080', 'server': '198.19.0.1', 'nat': '198.18.0.1'},
    6: {'target': 'fd73:7467:6f::2', 'url': 'http://[fd73:7467:6f::2]:8080',
        'server': 'fd73:7467:6f:1::1', 'nat': 'fd73:7467:6f::1'},
}


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
    """Capture stable routing fields; omit lifetimes, counters, and neighbor caches."""
    state = {'interfaces': sorted(name for _, name in socket.if_nameindex())}
    if SYSTEM == 'Linux':
        fields = ('dst', 'gateway', 'dev', 'table', 'metric', 'prefsrc', 'type', 'scope')
        for family in (4, 6):
            rows = json.loads(run('ip', '-j', f'-{family}', 'route', 'show', 'table', 'main'))
            state[f'routes{family}'] = sorted(
                json.dumps({k: row[k] for k in fields if k in row}, sort_keys=True) for row in rows)
            firewall = 'iptables' if family == 4 else 'ip6tables'
            state[f'firewall{family}'] = [run(firewall, '-t', table, '-S') for table in ('filter', 'nat', 'mangle')]
    elif SYSTEM == 'Darwin':
        for family in ('inet', 'inet6'):
            rows = []
            for line in run('netstat', '-rn', '-f', family).splitlines():
                parts = line.split()
                if len(parts) >= 4 and (parts[0] == 'default' or re.match(r'[0-9a-f]', parts[0])):
                    if 'L' not in parts[2] and 'W' not in parts[2]:
                        rows.append(' '.join(parts[:4]))
            state[family] = sorted(rows)
    else:
        rows = json.loads(powershell(
            '@(Get-NetRoute | Select-Object DestinationPrefix,NextHop,InterfaceIndex,RouteMetric) '
            '| ConvertTo-Json -Compress'))
        state['routes'] = sorted(json.dumps(row, sort_keys=True) for row in rows)
    return state


class RunnerRoutes:
    """Keep CI control traffic online, leaving both test subnets to TunGo."""

    def __init__(self):
        self.added = []
        self.gateway = None
        self.interface = None
        self.delete6 = None

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
        # Preserve public IPv6 where the runner already has IPv6 connectivity.
        # The fixture's ULA prefixes remain exclusively routed by TunGo.
        if SYSTEM == 'Linux':
            defaults = json.loads(run('ip', '-j', '-6', 'route', 'show', 'default'))
            if defaults:
                route = min(defaults, key=lambda r: r.get('metric', 0))
                suffix = ['2000::/3', 'via', route['gateway'], 'dev', route['dev']]
                run('ip', '-6', 'route', 'add', *suffix)
                self.delete6 = ['ip', '-6', 'route', 'del', *suffix]
        elif SYSTEM == 'Darwin':
            output = run('route', '-n', 'get', '-inet6', 'default', check=False)
            gateway = re.search(r'gateway:\s+(\S+)', output)
            if gateway:
                suffix = ['-inet6', '2000::/3', gateway[1]]
                run('route', '-n', 'add', *suffix)
                self.delete6 = ['route', '-n', 'delete', *suffix]
        else:
            route = powershell(
                'Get-NetRoute -DestinationPrefix ::/0 -ErrorAction SilentlyContinue | Sort-Object RouteMetric '
                '| Select-Object -First 1 -Property NextHop,InterfaceIndex | ConvertTo-Json -Compress')
            if route:
                route = json.loads(route)
                powershell(f"New-NetRoute -DestinationPrefix '2000::/3' -NextHop '{route['NextHop']}' "
                           f"-InterfaceIndex {route['InterfaceIndex']} -PolicyStore ActiveStore | Out-Null")
                self.delete6 = (f"Get-NetRoute -DestinationPrefix '2000::/3' -InterfaceIndex {route['InterfaceIndex']} "
                                '| Remove-NetRoute -Confirm:$false')

    def remove(self):
        if self.delete6:
            if SYSTEM == 'Windows':
                powershell(self.delete6)
            else:
                run(*self.delete6, check=False)
        for prefix in reversed(self.added):
            if SYSTEM == 'Linux':
                run('ip', 'route', 'del', prefix, 'via', self.gateway, 'dev', self.interface, check=False)
            elif SYSTEM == 'Darwin':
                run('route', '-n', 'delete', '-net', prefix, self.gateway, check=False)
            else:
                powershell(f"Get-NetRoute -DestinationPrefix '{prefix}' -InterfaceIndex {self.interface} "
                           '-ErrorAction SilentlyContinue | Remove-NetRoute -Confirm:$false')


def assert_tunnel_route(family):
    address = FAMILIES[family]['target']
    prefixes = ('0.0.0.0/1', '128.0.0.0/1') if family == 4 else ('::/1', '8000::/1')
    if SYSTEM == 'Linux':
        routes = json.loads(run('ip', '-j', f'-{family}', 'route', 'show'))
        target = json.loads(run('ip', '-j', f'-{family}', 'route', 'get', address))[0]
        assert target['dev'].startswith('c_'), target
        for prefix in prefixes:
            assert any(r.get('dst') == prefix and r.get('dev') == target['dev'] for r in routes), routes
    elif SYSTEM == 'Darwin':
        family_arg = ['-inet6'] if family == 6 else []
        interface = re.search(r'interface:\s+(\S+)', run('route', '-n', 'get', *family_arg, address))[1]
        assert interface.startswith('utun'), interface
        routes = run('netstat', '-rn', '-f', 'inet6' if family == 6 else 'inet').splitlines()
        for prefix in (('0/1', '128.0/1') if family == 4 else prefixes):
            assert any(line.split()[0] == prefix and interface in line.split() for line in routes if line.split()), routes
    else:
        interface = powershell(
            f"(Find-NetRoute -RemoteIPAddress '{address}' | Where-Object {{ $_.PSObject.Properties['InterfaceAlias'] }} "
            '| Select-Object -First 1).InterfaceAlias')
        assert interface.startswith('c_'), interface
        for prefix in prefixes:
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
    endpoint = '127.0.0.1' if SYSTEM == 'Windows' else '192.168.250.15'
    control = 'http://' + ('127.0.0.1' if SYSTEM == 'Windows' else '192.168.250.15') + ':18080'
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
        network = 'vmnet-host,id=transport,start-address=192.168.250.1,end-address=192.168.250.254,subnet-mask=255.255.255.0'
    elif SYSTEM == 'Linux':
        network = 'tap,id=transport,ifname=tungo-vm0,script=no,downscript=no'
    else:
        network = ('user,id=transport,net=192.168.250.0/24,restrict=on,'
                   'hostfwd=tcp:127.0.0.1:18080-192.168.250.15:18080,'
                   'hostfwd=tcp:127.0.0.1:8080-192.168.250.15:8080,'
                   'hostfwd=udp:127.0.0.1:9090-192.168.250.15:9090,'
                   'hostfwd=tcp:127.0.0.1:1010-192.168.250.15:1010')
    command += ['-netdev', network, '-device', 'virtio-net-pci,netdev=transport']
    routes = RunnerRoutes()
    vm = client = None
    config_path = Path(os.environ.get('ProgramData', 'C:/ProgramData')) / 'TunGo/client_configuration.json' if SYSTEM == 'Windows' else Path('/etc/tungo/client_configuration.json')
    assert not config_path.exists(), 'Refusing to overwrite existing TunGo configuration'
    config_written = False
    tap_created = False

    def control_call(path, data=None):
        return json.loads(request(control + path, data, timeout=45))

    def no_bypass():
        control_call('/status')
        for family, addresses in FAMILIES.items():
            try:
                request(addresses['url'] + '/sha256', timeout=2)
            except (OSError, RuntimeError):
                continue
            raise AssertionError(f'IPv{family} target reachable without TunGo')

    def save_state(name):
        state = snapshot()
        (args.artifacts / f'{name}.json').write_text(json.dumps(state, indent=2))
        return state

    try:
        if SYSTEM == 'Linux':
            run('ip', 'tuntap', 'add', 'dev', 'tungo-vm0', 'mode', 'tap')
            tap_created = True
            run('ip', 'addr', 'add', '192.168.250.1/24', 'dev', 'tungo-vm0')
            run('ip', 'link', 'set', 'tungo-vm0', 'up')
        with (args.artifacts / 'vm.log').open('w') as log:
            vm = subprocess.Popen(command, stdout=log, stderr=log)
        def ready():
            assert vm.poll() is None, f'VM exited: {vm.returncode}'
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
            for family, addresses in FAMILIES.items():
                print(f'Checking IPv{family} traffic', flush=True)
                def connected():
                    assert client.poll() is None, f'Client exited: {client.returncode}'
                    return request(addresses['url'] + '/sha256', timeout=2)
                wait_until(f'tunneled IPv{family} HTTP', connected)
                assert_tunnel_route(family)
                if SYSTEM == 'Windows':
                    run('ping', f'-{family}', '-n', '3', '-w', '2000', addresses['server'])
                elif SYSTEM == 'Darwin':
                    run('ping6' if family == 6 else 'ping', '-n', '-c', '3', addresses['server'])
                else:
                    run('ping', f'-{family}', '-n', '-c', '3', addresses['server'])
                payload = request(addresses['url'] + '/payload', timeout=60)
                assert len(payload) == 4 * 1024 * 1024, f'IPv{family} payload truncated'
                assert hashlib.sha256(payload).hexdigest() == info['sha256'], f'IPv{family} checksum mismatch'
                assert request(addresses['url'] + '/peer').decode().strip() == addresses['nat'], f'IPv{family} MASQUERADE missing'
            save_state(f'client-connected-{cycle}')
            stop_client(client)
            client = None
            wait_until('client routes and interfaces restored', lambda: snapshot() == baseline, timeout=30)
            save_state(f'client-after-{cycle}')
            no_bypass()
        assert control_call('/stop', {})['restored']
        no_bypass()
        print('PASS: dual-stack IPv4/IPv6 TUN, routes, 4 MiB checksum, NAT, client restart, graceful cleanup', flush=True)
    except BaseException:
        # Include diagnostics in job logs as well as downloadable artifacts.
        # Neither runtime logs nor these snapshots contain generated private keys.
        try:
            print('Client failure state:', json.dumps(save_state('client-failure')), flush=True)
            print('Server diagnostics:', json.dumps(control_call('/diagnostics')), flush=True)
        except Exception as error:
            print(f'Failure snapshot error: {error}', flush=True)
        for path in sorted(args.artifacts.glob('*.log')):
            print(f'{path.name}:\n{path.read_text(errors="replace")[-20000:]}', flush=True)
        raise
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
        if tap_created:
            run('ip', 'link', 'delete', 'tungo-vm0')


if __name__ == '__main__':
    main()
