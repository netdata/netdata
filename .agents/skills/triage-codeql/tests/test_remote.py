"""Exercise repository selection through the actual helper, with Git replaced."""

import os
from pathlib import Path
import subprocess
import unittest

HELPER = Path(__file__).resolve().parents[1] / 'scripts/_lib.sh'


class RemoteTests(unittest.TestCase):
    def parse(self, remote, upstream=True):
        env = {'PATH': os.environ['PATH'], 'TEST_REMOTE': remote,
               'TEST_UPSTREAM': '1' if upstream else '0'}
        return subprocess.run(['bash', '--noprofile', '--norc', '-c', '''
source "$1"
gh_repo_root() { printf /fixture; }
git() {
    if [[ "$*" == *remote.upstream.url && "$TEST_UPSTREAM" == 0 ]]; then return 1; fi
    printf '%s\\n' "$TEST_REMOTE"
}
gh_require_slug
''', 'remote-test', str(HELPER)], env=env, text=True, capture_output=True, timeout=5)

    def test_supported_remote_forms(self):
        for remote in ('git@github.com:example/my.repo.git',
                       'ssh://git@github.com/example/my.repo.git',
                       'https://github.com/example/my.repo.git',
                       'https://x-access-token:TOKEN@github.com/example/my.repo',
                       'https://github.com/example/.github.git'):
            for upstream in (True, False):
                with self.subTest(remote=remote, upstream=upstream):
                    result = self.parse(remote, upstream)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertEqual(result.stdout, 'example/' + ('.github' if '/.github' in remote else 'my.repo'))
                    self.assertEqual(result.stderr, '')

    def test_rejects_spoofed_authority_and_invalid_repository_path(self):
        for remote in ('https://example.invalid/path/@github.com:example/repo.git',
                       'https://example.invalid/@github.com/example/repo.git',
                       'https://example.invalid?@github.com/example/repo.git',
                       'https://example.invalid#@github.com/example/repo.git',
                       'https://github.com.example.invalid/example/repo.git',
                       'https://github.com@evil.example.com/example/repo.git',
                       'git@notgithub.example.com:example/repo.git',
                       'https://github.com/example/repo/extra.git',
                       'https://github.com/example/repo.git?query=1',
                       'https://github.com/example/repo.git#fragment',
                       'https://github.com/../repo.git',
                       'https://github.com/example/..',
                       'https://github.com/example/', 'example/repo'):
            with self.subTest(remote=remote):
                result = self.parse(remote)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(result.stdout, '')
                self.assertNotIn(remote, result.stderr)


if __name__ == '__main__':
    unittest.main()
