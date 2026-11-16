#!/usr/bin/env python3

import contextlib
import io
import os
from pathlib import Path
import runpy
import unittest
from unittest.mock import MagicMock, patch

import requests


SCRIPT = Path(__file__).with_name('netdata-pkgcloud-cleanup.py')
OLD = '2020-01-01T00:00:00.000Z'


def package(name):
    return {
        'name': name,
        'created_at': OLD,
        'package_html_url': f'/html/{name}',
        'destroy_url': f'/api/v1/repos/netdata/netdata-edge/el/9/{name}.rpm',
    }


def response(status, body):
    result = MagicMock()
    result.status_code = status
    result.text = body
    return result


class PackageCloudCleanupTest(unittest.TestCase):
    def run_cleanup(self, names, deletes, dry_run=False):
        pages = [[package(name) for name in names], []]
        listing = MagicMock()
        listing.json.side_effect = pages
        argv = [str(SCRIPT), '-r', 'edge'] + (['-d'] if dry_run else [])
        output = io.StringIO()
        errors = io.StringIO()
        code = 0
        with patch.dict(os.environ, PKGCLOUD_TOKEN='test-token'), \
             patch('sys.argv', argv), \
             patch('requests.get', return_value=listing), \
             patch('requests.delete', side_effect=deletes) as delete, \
             contextlib.redirect_stdout(output), \
             contextlib.redirect_stderr(errors):
            try:
                runpy.run_path(str(SCRIPT), run_name='__main__')
            except SystemExit as e:
                code = e.code
        return code, delete, output.getvalue(), errors.getvalue()

    def test_http_errors_are_reported_and_fail_the_run(self):
        names = ['ok', 'missing', 'broken', 'redirected', 'ok2']
        deletes = [
            response(200, '{}'),
            response(404, '{"error":"not found"}'),
            response(500, '{"error":"internal"}'),
            response(302, ''),
            response(200, '{}'),
        ]
        code, delete, output, errors = self.run_cleanup(names, deletes)
        self.assertEqual(code, 1)
        self.assertEqual(delete.call_count, len(names))
        self.assertEqual(output.count('Package deleted successfully.'), 2)
        self.assertEqual(output.count('Failed deleting package'), 3)
        self.assertIn('HTTP 404', output)
        self.assertIn('HTTP 500', output)
        self.assertIn('HTTP 302', output)
        for call in delete.call_args_list:
            self.assertIs(call.kwargs.get('allow_redirects'), False)
        self.assertIn('3 package(s) could not be deleted', errors)

    def test_request_errors_are_reported_and_the_run_continues(self):
        deletes = [
            requests.exceptions.ConnectionError('connection refused'),
            response(200, '{}'),
        ]
        code, delete, output, errors = self.run_cleanup(['unreachable', 'ok'], deletes)
        self.assertEqual(code, 1)
        self.assertEqual(delete.call_count, 2)
        self.assertEqual(output.count('Package deleted successfully.'), 1)
        self.assertIn('connection refused', output)
        self.assertIn('1 package(s) could not be deleted', errors)

    def test_successful_deletes_exit_zero(self):
        code, delete, output, errors = self.run_cleanup(['a', 'b'], [response(200, '{}'), response(200, '{}')])
        self.assertIn(code, (0, None))
        self.assertEqual(delete.call_count, 2)
        self.assertNotIn('Failed deleting package', output)
        self.assertEqual(errors, '')

    def test_dry_run_sends_no_delete(self):
        code, delete, output, errors = self.run_cleanup(['a'], [], dry_run=True)
        self.assertIn(code, (0, None))
        delete.assert_not_called()
        self.assertIn('DRY_RUN mode', output)


if __name__ == '__main__':
    unittest.main()
