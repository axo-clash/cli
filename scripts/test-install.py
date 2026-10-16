#!/usr/bin/env python3
"""Exercise the real Bash installer with local release assets and mocked downloads."""
import hashlib
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

INSTALLER = Path(__file__).resolve().parents[1] / 'install.sh'


class InstallerTest(unittest.TestCase):
    def install(self, system='Linux', arch='x86_64', checksum='valid', version='v1.2.3', fail=False):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            tools = root / 'tools'
            tools.mkdir()
            destination = root / 'custom bin'
            destination.mkdir()
            binary = destination / 'axo-clash'
            binary.write_bytes(b'previous version')
            payload = b'#!/bin/sh\necho axo-clash v1.2.3\n'
            asset = f"axo-clash_{'darwin' if system == 'Darwin' else 'linux'}_{'arm64' if arch in ('arm64', 'aarch64') else 'amd64'}"
            (root / asset).write_bytes(payload)
            digest = hashlib.sha256(payload).hexdigest() if checksum == 'valid' else '0' * 64
            (root / 'checksums.txt').write_text('' if checksum == 'missing' else f'{digest}  {asset}\n')
            (tools / 'uname').write_text('#!/bin/sh\nif [ "$1" = -s ]; then echo "$TEST_OS"; else echo "$TEST_ARCH"; fi\n')
            (tools / 'curl').write_text('''#!/bin/bash
set -eu
[[ "${FAIL_DOWNLOAD:-}" != 1 ]] || exit 22
url=''
out=''
while [[ $# -gt 0 ]]; do
  case "$1" in
    -o) out="$2"; shift 2 ;;
    --proto|--tlsv1.2|-w) shift 2 ;;
    https://*) url="$1"; shift ;;
    *) shift ;;
  esac
done
if [[ "$url" == */releases/latest ]]; then
  printf 'https://github.com/axo-clash/cli/releases/tag/v1.2.3'
else
  [[ "$url" == */releases/download/v1.2.3/* ]] || exit 22
  cp "$FIXTURES/${url##*/}" "$out"
fi
''')
            for tool in tools.iterdir():
                tool.chmod(0o755)
            env = dict(os.environ, PATH=f'{tools}:{os.environ["PATH"]}', INSTALL_DIR=str(destination),
                       AXO_CLASH_VERSION=version, TEST_OS=system, TEST_ARCH=arch,
                       FIXTURES=str(root), FAIL_DOWNLOAD='1' if fail else '0')
            result = subprocess.run(['bash', str(INSTALLER)], env=env, capture_output=True, text=True)
            success = checksum == 'valid' and system in ('Linux', 'Darwin') and arch in ('x86_64', 'arm64', 'aarch64') and version in ('latest', 'v1.2.3') and not fail
            self.assertEqual(result.returncode == 0, success, result.stdout + result.stderr)
            self.assertEqual(binary.read_bytes(), payload if success else b'previous version')
            if success:
                self.assertTrue(os.access(binary, os.X_OK))
                self.assertIn('v1.2.3 installé', result.stdout)

    def test_supported_platforms(self):
        for system in ('Linux', 'Darwin'):
            for arch in ('x86_64', 'arm64', 'aarch64'):
                with self.subTest(system=system, arch=arch):
                    self.install(system=system, arch=arch)

    def test_latest(self):
        self.install(version='latest')

    def test_rejected_downloads_preserve_existing_binary(self):
        for checksum in ('corrupt', 'missing'):
            with self.subTest(checksum=checksum):
                self.install(checksum=checksum)
        self.install(fail=True)

    def test_unsupported_platform_and_invalid_version(self):
        self.install(system='FreeBSD')
        self.install(arch='i686')
        self.install(version='../invalid')


if __name__ == '__main__':
    unittest.main()
