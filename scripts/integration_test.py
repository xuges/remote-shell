#!/usr/bin/env python3
"""Real OpenSSH integration tests. Creates and removes a temporary local account.

Requires Linux, Python 3, Go, OpenSSH server/client, useradd and root or sudo -n.
The SSH server binds only to loopback; no system sshd configuration is modified.
"""
import concurrent.futures
import json
import os
from pathlib import Path
import secrets
import shutil
import signal
import socket
import subprocess
import tempfile
import time

ROOT = Path(__file__).resolve().parents[1]
SUDO = [] if os.geteuid() == 0 else ["sudo", "-n"]


def run(args, **kwargs):
    return subprocess.run(args, check=True, stdout=subprocess.PIPE,
                          stderr=subprocess.PIPE, **kwargs)


def main():
    run(SUDO + ["true"])
    sshd = shutil.which("sshd") or "/usr/sbin/sshd"
    account = "rsht" + secrets.token_hex(4)
    password = secrets.token_urlsafe(24)
    temp = Path(tempfile.mkdtemp(prefix="rsh-test-", dir="/tmp"))
    temp.chmod(0o755)
    env = dict(os.environ, REMOTE_SHELL_DIR=str(temp / "run"))
    env.pop("REMOTE_SHELL_PASSWORD", None)
    daemon_pid = None
    account_created = False
    server = None
    server_log = None
    bins = temp / "bin"
    bins.mkdir()

    def cli(name, *args, input=None, timeout=20, check=True, extra_env=None):
        p = subprocess.run([str(bins / name), *args], input=input,
                           stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                           timeout=timeout, env=extra_env or env)
        if check and p.returncode:
            raise AssertionError(f"{name}: exit {p.returncode}: {p.stderr.decode(errors='replace')}")
        return p

    def start(*auth, check=True):
        nonlocal daemon_pid
        p = cli("start-remote-shell", "-host=127.0.0.1", f"-port={port}",
                f"-user={account}", "-timeout=5s", *auth, check=check)
        if p.returncode == 0:
            daemon_pid = json.loads(cli("remote-shell-info", "-json").stdout)["pid"]
        return p

    def stop():
        nonlocal daemon_pid
        cli("stop-remote-shell")
        daemon_pid = None
        assert not (temp / "run/service.sock").exists()
        assert not (temp / "run/ssh.sock").exists()

    try:
        for name in ["start-remote-shell", "remote-shell", "remote-shell-info", "stop-remote-shell"]:
            run(["go", "build", "-race", "-o", str(bins / name), f"./cmd/{name}"], cwd=ROOT)
        run(["ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", str(temp / "host")])
        run(["ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", str(temp / "identity")])
        run(SUDO + ["useradd", "--no-create-home", "--home-dir", str(temp), "--shell", "/bin/sh", account])
        account_created = True
        run(SUDO + ["chpasswd"], input=f"{account}:{password}\n".encode())
        with socket.socket() as s:
            s.bind(("127.0.0.1", 0))
            port = s.getsockname()[1]
        config = temp / "sshd_config"
        config.write_text(f"""ListenAddress 127.0.0.1
Port {port}
HostKey {temp}/host
PidFile {temp}/sshd.pid
AuthorizedKeysFile {temp}/identity.pub
StrictModes no
PasswordAuthentication yes
KbdInteractiveAuthentication no
UsePAM no
PermitRootLogin no
AllowUsers {account}
PrintMotd no
PrintLastLog no
LogLevel VERBOSE
""")
        # Ubuntu sshd needs this directory even when using an isolated config.
        run(SUDO + ["mkdir", "-p", "/run/sshd"])
        server_log = open(temp / "sshd.log", "wb")
        server = subprocess.Popen(SUDO + [sshd, "-D", "-e", "-f", str(config)],
                                  stdout=server_log, stderr=server_log)
        for _ in range(100):
            if server.poll() is not None:
                raise AssertionError((temp / "sshd.log").read_text())
            try:
                with socket.create_connection(("127.0.0.1", port), timeout=0.1):
                    break
            except OSError:
                time.sleep(0.05)
        else:
            raise AssertionError("test sshd did not listen")

        # Isolate known_hosts so tests never change the developer's SSH files.
        # OpenSSH gets its home from passwd, so supply an ssh wrapper in PATH.
        ssh = shutil.which("ssh")
        wrapper = temp / "ssh"
        wrapper.write_text(f'#!/bin/sh\nexec {ssh} -F /dev/null -o UserKnownHostsFile={temp}/known_hosts "$@"\n')
        wrapper.chmod(0o755)
        env["PATH"] = str(temp) + os.pathsep + env["PATH"]

        assert start("-password=incorrect", check=False).returncode != 0
        assert not (temp / "run/service.sock").exists()
        print("PASS failed password and startup cleanup", flush=True)
        start(f"-password={password}")
        info = json.loads(cli("remote-shell-info", "-json").stdout)
        assert info["connected"] and info["user"] == account
        assert info["os"] == "Linux" and info["os_version"] and info["kernel"]
        assert info["shell"] == "/bin/sh" and info["architecture"]
        assert start(f"-password={password}", check=False).returncode != 0
        print("PASS password login, metadata and duplicate start", flush=True)

        args = ["", "a b", "one'two", '"quoted"', "$(echo unsafe)", "; exit 99", "中文", "a\nb"]
        result = cli("remote-shell", "printf", "%s\\0", *args)
        assert result.stdout == b"\0".join(a.encode() for a in args) + b"\0"
        result = cli("remote-shell", "-c", "printf out; printf err >&2; exit 42", check=False)
        assert (result.returncode, result.stdout, result.stderr) == (42, b"out", b"err")
        assert cli("remote-shell", "-c", "printf hello | tr a-z A-Z").stdout == b"HELLO"
        blob = bytes(range(256)) * 8192
        assert cli("remote-shell", "cat", input=blob).stdout == blob
        assert cli("remote-shell", "cat", input=b"").stdout == b""
        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
            results = list(pool.map(lambda i: cli("remote-shell", "printf", "%s", str(i)).stdout, range(8)))
        assert results == [str(i).encode() for i in range(8)]
        print("PASS quoting, raw shell, exit code, binary stdin/stdout and concurrent commands", flush=True)

        # Observe output before remote process exits: streaming must be live.
        live = subprocess.Popen([str(bins / "remote-shell"), "-c", "printf ready; sleep 2; printf done"],
                                env=env, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        assert live.stdout.read(5) == b"ready" and live.poll() is None
        assert live.communicate(timeout=6)[0] == b"done" and live.returncode == 0

        canceled = subprocess.Popen([str(bins / "remote-shell"), "sleep", "30"], env=env,
                                    stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        time.sleep(0.3)
        canceled.send_signal(signal.SIGINT)
        canceled.communicate(timeout=5)
        assert canceled.returncode != 0
        assert cli("remote-shell", "printf", "still-alive").stdout == b"still-alive"
        stop()
        start(f"-identity={temp}/identity")
        print("PASS streaming, client interruption, stop/restart and key login", flush=True)

        # A crashed daemon can leave a live master behind; restart must close it.
        old_master = int(run(["pgrep", "-P", str(daemon_pid), "ssh"]).stdout.strip())
        os.kill(daemon_pid, signal.SIGKILL)
        daemon_pid = None
        time.sleep(0.2)
        start(f"-identity={temp}/identity")
        time.sleep(0.2)
        old_stat = Path(f"/proc/{old_master}/stat")
        assert not old_stat.exists() or old_stat.read_text().split()[2] == "Z"
        stop()

        env["REMOTE_SHELL_PASSWORD"] = password
        start()
        env.pop("REMOTE_SHELL_PASSWORD")
        environ = Path(f"/proc/{daemon_pid}/environ").read_bytes()
        assert password.encode() not in environ
        print("PASS daemon crash recovery and environment password login", flush=True)

        # Killing the dedicated master simulates an established connection loss.
        master_pid = run(["pgrep", "-P", str(daemon_pid), "ssh"]).stdout.strip().decode()
        os.kill(int(master_pid), signal.SIGKILL)
        time.sleep(0.2)
        disconnected = cli("remote-shell-info", "-json", check=False)
        assert disconnected.returncode == 1 and not json.loads(disconnected.stdout)["connected"]
        assert cli("remote-shell", "true", check=False).returncode == 255
        stop()
        # A stale socket path after a crash must not prevent restarting.
        (temp / "run/service.sock").write_text("stale")
        start(f"-identity={temp}/identity")
        active = subprocess.Popen([str(bins / "remote-shell"), "sleep", "30"], env=env,
                                  stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        time.sleep(0.3)
        stop()
        active.communicate(timeout=5)
        assert active.returncode != 0
        assert cli("remote-shell-info", check=False).returncode != 0

        # A server that accepts TCP but never speaks SSH must time out cleanly.
        with socket.socket() as stalled:
            stalled.bind(("127.0.0.1", 0))
            stalled.listen()
            started = time.monotonic()
            p = cli("start-remote-shell", "-host=127.0.0.1",
                    f"-port={stalled.getsockname()[1]}", "-timeout=500ms", check=False)
            assert p.returncode != 0 and time.monotonic() - started < 5
            assert not (temp / "run/service.sock").exists()
        log = (temp / "run/daemon.log").read_text()
        assert "DATA RACE" not in log, log
        assert password not in log
        print("PASS disconnect reporting, stale socket recovery, active shutdown, timeout, no race diagnostics", flush=True)
    finally:
        if daemon_pid:
            try:
                cli("stop-remote-shell", check=False)
            except Exception:
                try:
                    os.kill(daemon_pid, signal.SIGTERM)
                except ProcessLookupError:
                    pass
        if server is not None:
            if (temp / "sshd.pid").exists():
                subprocess.run(SUDO + ["kill", (temp / "sshd.pid").read_text().strip()], check=False)
            try:
                server.wait(timeout=5)
            except subprocess.TimeoutExpired:
                server.terminate()
        if server_log:
            server_log.close()
        if account_created:
            subprocess.run(SUDO + ["pkill", "-u", account], check=False)
            run(SUDO + ["userdel", account])
        shutil.rmtree(temp)


if __name__ == "__main__":
    main()
