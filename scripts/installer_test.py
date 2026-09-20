#!/usr/bin/env python3
"""Exercise the real installers against local release/source archives, in temp homes."""
import hashlib
import os
from pathlib import Path
import platform
import shutil
import subprocess
import tarfile
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]
COMMANDS = ('start-remote-shell', 'remote-shell', 'remote-shell-info', 'stop-remote-shell')
SKILLS = ('remote-shell',)
VERSION = (ROOT / 'plugins/bin/VERSION').read_text().strip()
BASH = shutil.which('bash')


class InstallerTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='rs-install-test-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.home = self.root / 'user home'
        self.home.mkdir()
        self.prefix = self.home / '.remote-shell'
        self.assets = self.root / 'release'
        self.assets.mkdir()
        self.env = {k: v for k, v in os.environ.items()
                    if not k.startswith(('REMOTE_SHELL_', 'CODEX_', 'CLAUDE_'))
                    and k not in ('BASH_ENV', 'ENV', 'XDG_CONFIG_HOME')}
        self.env.update(HOME=str(self.home), PATH='/usr/bin:/bin',
                        REMOTE_SHELL_DIST_BASE_URL=self.assets.as_uri())
        self.make_release()

    def make_release(self, *, missing=None, wrong_version=None, os_name=None):
        os_name = os_name or platform.system().lower()
        arch = 'arm64' if platform.machine().lower() in ('aarch64', 'arm64') else 'amd64'
        stem = f'remote-shell-{VERSION}-{os_name}-{arch}'
        package_root = self.root / stem
        package_root.mkdir(exist_ok=True)
        for name in COMMANDS:
            exe = package_root / name
            if name == missing:
                exe.unlink(missing_ok=True)
                continue
            version = 'v0.0.0' if name == wrong_version else VERSION
            exe.write_text(f"#!/bin/sh\nprintf '%s\\n' '{name} {version}'\n")
            exe.chmod(0o755)
        self.archive = self.assets / f'{stem}.tar.gz'
        with tarfile.open(self.archive, 'w:gz') as tar:
            tar.add(package_root, arcname=stem)
        digest = hashlib.sha256(self.archive.read_bytes()).hexdigest()
        self.checksum = f'{digest}  {self.archive.name}\n'
        self.sums = self.assets / 'sha256sums.txt'
        # Also include an unrelated platform: only the selected asset is required.
        self.sums.write_text(self.checksum + 'f' * 64 + '  another-platform.zip\n')

    def run_install(self, *args, success=True, env=None, stdin=False):
        argv = [BASH, '-s', '--'] if stdin else [BASH, str(ROOT / 'install.sh')]
        result = subprocess.run(argv + list(args), cwd=self.root, env=env or self.env,
                                input=(ROOT / 'install.sh').read_text() if stdin else None,
                                text=True, capture_output=True, timeout=30)
        if success:
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        else:
            self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        return result

    def assert_skills(self, directory):
        for skill in SKILLS:
            canonical = ROOT / 'plugins/skills-canonical' / skill
            installed = directory / skill
            originals = {p.relative_to(canonical): p.read_bytes()
                         for p in canonical.rglob('*') if p.is_file()}
            copies = {p.relative_to(installed): p.read_bytes()
                      for p in installed.rglob('*') if p.is_file()}
            self.assertEqual(originals, copies)
            self.assertTrue((installed / 'references').is_dir())

    def assert_binaries(self, prefix=None):
        for name in COMMANDS:
            exe = (prefix or self.prefix) / 'bin' / name
            result = subprocess.run([str(exe), '--version'], capture_output=True, text=True, check=True)
            self.assertEqual(result.stdout.strip(), f'{name} {VERSION}')

    def preserve_existing(self):
        (self.prefix / 'bin').mkdir(parents=True)
        for name in COMMANDS:
            exe = self.prefix / 'bin' / name
            exe.write_text(f"#!/bin/sh\necho '{name} v0.0.0'\n")
            exe.chmod(0o755)
        skill = self.home / '.agents/skills/remote-shell'
        skill.mkdir(parents=True)
        (skill / 'SKILL.md').write_text('local customization')
        return {p: p.read_bytes() for p in self.home.rglob('*') if p.is_file()}

    def assert_preserved(self, files):
        for path, content in files.items():
            self.assertEqual(path.read_bytes(), content, str(path))

    def test_each_client(self):
        paths = {'codex': '.agents/skills', 'opencode': '.config/opencode/skills',
                 'claude-code': '.claude/skills'}
        for client, path in paths.items():
            with self.subTest(client=client):
                child_home = self.root / client
                child_home.mkdir()
                env = dict(self.env, HOME=str(child_home))
                self.run_install('--agent', client, env=env)
                self.assert_skills(child_home / path)
                self.assert_binaries(child_home / '.remote-shell')
                for other in set(paths.values()) - {path}:
                    self.assertFalse((child_home / other).exists())

    def test_all_and_repeat_preserve_configuration(self):
        self.prefix.mkdir()
        config = self.prefix / 'config.toml'
        config.write_text('private configuration sentinel')
        profile = self.home / '.bashrc'
        profile.write_text('# custom shell profile\n')
        unrelated = self.home / '.agents/skills/unrelated/SKILL.md'
        unrelated.parent.mkdir(parents=True)
        unrelated.write_text('other skill')
        self.run_install('--agent', 'all')
        for path in ('.agents/skills', '.config/opencode/skills', '.claude/skills'):
            self.assert_skills(self.home / path)
        before = (self.prefix / 'bin/remote-shell').stat().st_mtime_ns
        result = self.run_install('--agent', 'all')
        self.assertEqual((self.prefix / 'bin/remote-shell').stat().st_mtime_ns, before)
        self.assertFalse((self.prefix / 'backups').exists())
        self.assertEqual(config.read_text(), 'private configuration sentinel')
        self.assertEqual(profile.read_text(), '# custom shell profile\n')
        self.assertEqual(unrelated.read_text(), 'other skill')
        self.assertNotIn(config.read_text(), result.stdout + result.stderr)
        self.assertFalse((self.home / '.agents/skills/.remote-shell-install').exists())

    def test_modified_skill_and_symlink_are_backed_up(self):
        self.run_install('--agent', 'codex')
        directory = self.home / '.agents/skills'
        (directory / 'remote-shell/local-notes.txt').write_text('keep these notes')
        self.run_install('--agent', 'codex')
        self.assert_skills(directory)
        backups = list((self.prefix / 'backups').iterdir())
        self.assertEqual(len(backups), 1)
        backed_up = backups[0] / str(directory).lstrip('/')
        self.assertEqual((backed_up / 'remote-shell/local-notes.txt').read_text(), 'keep these notes')
        target = self.root / 'external skill'
        target.mkdir()
        (target / 'SKILL.md').write_text('external skill sentinel')
        shutil.rmtree(directory / 'remote-shell')
        (directory / 'remote-shell').symlink_to(target, target_is_directory=True)
        self.run_install('--agent', 'codex')
        self.assert_skills(directory)
        self.assertFalse((directory / 'remote-shell').is_symlink())
        backups = list((self.prefix / 'backups').iterdir())
        symlink_backups = [b for b in backups
                           if (b / str(directory).lstrip('/') / 'remote-shell').is_symlink()]
        self.assertEqual(len(symlink_backups), 1)
        self.assertTrue((target / 'SKILL.md').is_file())

    def test_custom_prefix_and_client_config_directories(self):
        prefix = self.root / "custom prefix 'quoted'"
        env = dict(self.env, XDG_CONFIG_HOME=str(self.root / 'xdg config'),
                   CLAUDE_CONFIG_DIR=str(self.root / 'claude config'))
        self.run_install('--agent', 'all', '--prefix', str(prefix), env=env)
        self.assert_binaries(prefix)
        self.assert_skills(self.home / '.agents/skills')
        self.assert_skills(self.root / 'xdg config/opencode/skills')
        self.assert_skills(self.root / 'claude config/skills')
        self.assertFalse(self.prefix.exists())

    def test_auto_detection(self):
        (self.home / '.codex').mkdir()
        (self.home / '.claude').mkdir()
        self.run_install()
        self.assert_skills(self.home / '.agents/skills')
        self.assert_skills(self.home / '.claude/skills')
        self.assertFalse((self.home / '.config/opencode').exists())

    def test_help_and_invalid_arguments_do_not_install(self):
        self.run_install('--help')
        for args in (('--agent', 'unknown'), ('--agent',), ('--prefix', 'relative'), ('--wat',)):
            self.run_install(*args, success=False)
        self.assertFalse(self.prefix.exists())

    def test_no_detected_client_reports_selection_needed(self):
        result = self.run_install(success=False)
        self.assertIn('No client detected', result.stderr)
        self.assertFalse(self.prefix.exists())

    def test_local_source_path_with_spaces(self):
        source = self.root / 'local checkout'
        for relative in ('plugins/skills-canonical', 'plugins/bin', 'examples'):
            shutil.copytree(ROOT / relative, source / relative)
        self.run_install('--agent', 'codex', '--source', str(source))
        self.assert_skills(self.home / '.agents/skills')
        self.assert_binaries()

    def test_binaries_on_path_do_not_skip_selected_prefix(self):
        other = self.root / 'other bin'
        other.mkdir()
        for name in COMMANDS:
            exe = other / name
            exe.write_text(f"#!/bin/sh\necho '{name} {VERSION}'\n")
            exe.chmod(0o755)
        self.run_install('--agent', 'codex', env=dict(self.env, PATH=str(other) + ':' + self.env['PATH']))
        self.assert_binaries()

    def test_checksum_failures_preserve_existing_installation(self):
        existing = self.preserve_existing()
        variants = ('0' * 64 + f'  {self.archive.name}\n',
                    'f' * 64 + '  wrong-file.tar.gz\n',
                    self.checksum + self.checksum)
        for sums in variants:
            with self.subTest(sums=sums[:20]):
                self.sums.write_text(sums)
                self.run_install('--agent', 'codex', success=False)
                self.assert_preserved(existing)
                self.assertFalse((self.prefix / 'backups').exists())

    def test_incomplete_or_wrong_version_package_preserves_installation(self):
        existing = self.preserve_existing()
        self.make_release(missing='stop-remote-shell')
        self.run_install('--agent', 'codex', success=False)
        self.assert_preserved(existing)
        self.make_release(wrong_version='stop-remote-shell')
        self.run_install('--agent', 'codex', success=False)
        self.assert_preserved(existing)

    def test_source_download_from_stdin(self):
        source_archive = self.root / 'source.tar.gz'
        with tarfile.open(source_archive, 'w:gz') as tar:
            for relative in ('plugins/skills-canonical', 'plugins/bin', 'examples'):
                tar.add(ROOT / relative, arcname='remote-shell-main/' + relative)
        env = dict(self.env, REMOTE_SHELL_SOURCE_URL=source_archive.as_uri())
        self.run_install('--agent', 'all', stdin=True, env=env)
        self.assert_binaries()
        self.assert_skills(self.home / '.agents/skills')
        self.assert_skills(self.home / '.config/opencode/skills')
        self.assert_skills(self.home / '.claude/skills')

    def test_failed_downloads_do_not_install(self):
        missing = (self.root / 'missing').as_uri()
        self.run_install('--agent', 'codex', stdin=True,
                         env=dict(self.env, REMOTE_SHELL_SOURCE_URL=missing), success=False)
        self.run_install('--agent', 'codex',
                         env=dict(self.env, REMOTE_SHELL_DIST_BASE_URL=missing), success=False)
        self.assertFalse((self.home / '.agents').exists())
        self.assertFalse((self.prefix / 'bin').exists())

    def test_macos_shasum_fallback(self):
        if not shutil.which('shasum'):
            self.skipTest('shasum not available')
        # Limit PATH to the required portable tools, deliberately excluding sha256sum.
        path = self.root / 'portable tools'
        path.mkdir()
        for name in ('bash', 'sh', 'curl', 'tar', 'ssh', 'dirname', 'cat', 'sed', 'awk',
                     'tr', 'mktemp', 'chmod', 'cp', 'mkdir', 'mv', 'rm', 'diff', 'rmdir', 'shasum', 'gzip'):
            (path / name).symlink_to(shutil.which(name))
        (path / 'uname').write_text('#!/bin/sh\nif [ "$1" = -s ]; then echo Darwin; else echo arm64; fi\n')
        (path / 'uname').chmod(0o755)
        # Stub release executables can run anywhere; label an ARM64 Darwin package.
        stem = f'remote-shell-{VERSION}-darwin-arm64'
        with tarfile.open(self.assets / f'{stem}.tar.gz', 'w:gz') as tar:
            package_root = next(self.root.glob(f'remote-shell-{VERSION}-*'))
            tar.add(package_root, arcname=stem)
        package = self.assets / f'{stem}.tar.gz'
        self.sums.write_text(hashlib.sha256(package.read_bytes()).hexdigest() + f'  {stem}.tar.gz\n')
        self.run_install('--agent', 'codex', env=dict(self.env, PATH=str(path)))
        self.assert_binaries()


if __name__ == '__main__':
    unittest.main(verbosity=2)
