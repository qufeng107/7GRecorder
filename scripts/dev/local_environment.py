#!/usr/bin/env python3
"""Run a fresh local backend plus Vite, with no inherited integration credentials."""
import argparse
import os
from pathlib import Path
import secrets
import signal
import socket
import subprocess
import tempfile
import time
from urllib.request import urlopen

ROOT = Path(__file__).resolve().parents[2]

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--mode', choices=['integration', 'preview'], default='integration')
    parser.add_argument('--test-account', action='store_true', help='Synthetic account for browser tests only')
    args = parser.parse_args()
    port = 4173 if args.mode == 'preview' else 5173
    for value in [18080, port]:
        with socket.socket() as probe:
            # Ignore closed connections in TIME_WAIT, still reject a live listener.
            probe.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
            probe.bind(('127.0.0.1', value))
    base = ROOT / 'data' / 'dev'
    base.mkdir(parents=True, exist_ok=True)
    state = Path(tempfile.mkdtemp(prefix='run-', dir=base))
    os.chmod(state, 0o700)
    for directory in ["db", "temp", "recordings", "upload-sources", "songs", "backups/db"]:
        (state / directory).mkdir(parents=True, exist_ok=True)
    key = state / 'master.key'
    key.write_text(secrets.token_hex(32))
    key.chmod(0o600)
    # An allowlist prevents .bashrc or production env variables reaching the test backend.
    env = {name: os.environ[name] for name in ['PATH', 'HOME', 'GOPATH', 'GOCACHE', 'GOTOOLCHAIN'] if name in os.environ}
    env.update({
        'PATH': str(ROOT / '.node-toolchain/bin') + os.pathsep + os.environ.get('PATH', ''),
        'APP_LISTEN_ADDR': '127.0.0.1:18080', 'APP_PUBLIC_BASE_URL': f'http://127.0.0.1:{port}',
        'DATA_ROOT': str(state), 'SQLITE_PATH': str(state / 'db/app.db'), 'TEMP_ROOT': str(state / 'temp'),
        'MASTER_KEY_PATH': str(key), 'RECORDER_BASE_URL': 'http://127.0.0.1:9',
        'BILIUP_PATH': '/nonexistent/local-test-biliup', 'FFMPEG_PATH': '/nonexistent/local-test-ffmpeg',
        'LOG_LEVEL': 'warning',
    })
    binary = state / '7grecorder'
    subprocess.run(['go', 'build', '-o', str(binary), './cmd/7grecorder'], cwd=ROOT / 'backend', env=env, check=True)
    password = 'local-test-only-password' if args.test_account else secrets.token_urlsafe(18)
    subprocess.run([str(binary), 'admin', 'bootstrap', '--username', 'local-admin', '--password', password], env=env, check=True)
    if args.mode == 'preview':
        subprocess.run(['pnpm', 'build'], cwd=ROOT / 'frontend', env=env, check=True)
    children = []
    def stop(_sig, _frame):
        raise KeyboardInterrupt
    signal.signal(signal.SIGTERM, stop)
    signal.signal(signal.SIGINT, stop)
    try:
        log = (state / 'backend.log').open('w')
        children.append(subprocess.Popen([str(binary), 'serve'], cwd=state, env=env, stdout=log, stderr=subprocess.STDOUT, start_new_session=True))
        for _ in range(100):
            if children[0].poll() is not None:
                raise RuntimeError(f'Backend exited; inspect {state / "backend.log"}')
            try:
                with urlopen('http://127.0.0.1:18080/health/ready', timeout=1) as response:
                    if response.status == 200: break
            except OSError: time.sleep(.1)
        else: raise RuntimeError('Backend readiness timeout')
        print(f'Isolated data: {state}\nURL: http://127.0.0.1:{port}/admin/jobs\nSynthetic login: local-admin / {password}', flush=True)
        children.append(subprocess.Popen(['pnpm', 'preview:test' if args.mode == 'preview' else 'dev'], cwd=ROOT / 'frontend', env=env, start_new_session=True))
        while all(child.poll() is None for child in children): time.sleep(.25)
        raise RuntimeError('A local environment process exited unexpectedly')
    except KeyboardInterrupt:
        pass
    finally:
        for child in reversed(children):
            if child.poll() is None: os.killpg(child.pid, signal.SIGTERM)
        for child in children:
            try: child.wait(timeout=10)
            except subprocess.TimeoutExpired:
                os.killpg(child.pid, signal.SIGKILL)
                child.wait()
        # Keep the isolated DB/logs for diagnosis; never remove user data automatically.

if __name__ == '__main__':
    main()
