# SPDX-License-Identifier: GPL-3.0-or-later
"""Exercise the publisher through real rclone against controlled S3 responses."""
from http.server import BaseHTTPRequestHandler, HTTPServer
import os
import shutil
import subprocess
import threading
import unittest
from unittest.mock import patch
from urllib.parse import parse_qs, urlsplit
from xml.etree.ElementTree import Element, SubElement, tostring

import release


@unittest.skipUnless(shutil.which('rclone'), 'requires real rclone with the S3 backend')
class StoredDescriptorTests(unittest.TestCase):
    def setUp(self):
        self.key = 'dem-runtime/nightly/v1.2.3-1-nightly/amd64/release.json'
        self.remote = 'demtest:bucket/' + self.key
        self.objects = {}
        self.errors = {}
        self.requests = []
        fixture = self

        class S3Handler(BaseHTTPRequestHandler):
            def log_message(self, *args):
                pass

            def do_HEAD(self):
                self.respond()

            def do_GET(self):
                self.respond()

            def respond(self):
                url = urlsplit(self.path)
                fixture.requests.append((self.command, url.path))
                status = fixture.errors.get((self.command, url.path))
                if status:
                    body = f'<Error><Code>{"AccessDenied" if status == 403 else "InternalError"}</Code></Error>'
                    self.send_response(status)
                    self.send_header('Content-Length', str(len(body)))
                    self.end_headers()
                    if self.command != 'HEAD':
                        self.wfile.write(body.encode())
                    return
                if url.path == '/bucket':
                    query = parse_qs(url.query)
                    prefix = query.get('prefix', [''])[0]
                    listing = Element('ListBucketResult', xmlns='http://s3.amazonaws.com/doc/2006-03-01/')
                    SubElement(listing, 'Name').text = 'bucket'
                    SubElement(listing, 'Prefix').text = prefix
                    SubElement(listing, 'IsTruncated').text = 'false'
                    directories = set()
                    for key, data in sorted(fixture.objects.items()):
                        if not key.startswith(prefix):
                            continue
                        if query.get('delimiter') == ['/'] and '/' in key[len(prefix):]:
                            directories.add(prefix + key[len(prefix):].split('/')[0] + '/')
                            continue
                        item = SubElement(listing, 'Contents')
                        for name, value in {'Key': key, 'Size': str(len(data)), 'ETag': '"fixture"',
                                            'LastModified': '2026-11-17T00:00:00Z'}.items():
                            SubElement(item, name).text = value
                    for directory in sorted(directories):
                        SubElement(SubElement(listing, 'CommonPrefixes'), 'Prefix').text = directory
                    body = tostring(listing)
                else:
                    body = fixture.objects.get(url.path.removeprefix('/bucket/'))
                    if body is None:
                        self.send_error(404)
                        return
                self.send_response(200)
                self.send_header('Content-Length', str(len(body)))
                self.send_header('Last-Modified', 'Tue, 17 Nov 2026 00:00:00 GMT')
                self.send_header('ETag', '"fixture"')
                self.end_headers()
                if self.command != 'HEAD':
                    self.wfile.write(body)

        server = HTTPServer(('127.0.0.1', 0), S3Handler)
        thread = threading.Thread(target=server.serve_forever)
        thread.start()
        self.addCleanup(server.server_close)
        self.addCleanup(thread.join)
        self.addCleanup(server.shutdown)
        environment = patch.dict(os.environ, {
            'RCLONE_CONFIG': os.devnull,
            'RCLONE_CONFIG_DEMTEST_TYPE': 's3',
            'RCLONE_CONFIG_DEMTEST_PROVIDER': 'Other',
            'RCLONE_CONFIG_DEMTEST_ENDPOINT': f'http://127.0.0.1:{server.server_port}',
            'RCLONE_CONFIG_DEMTEST_ENV_AUTH': 'false',
            'RCLONE_CONFIG_DEMTEST_ACCESS_KEY_ID': '',
            'RCLONE_CONFIG_DEMTEST_SECRET_ACCESS_KEY': '',
            'RCLONE_RETRIES': '1',
            'RCLONE_LOW_LEVEL_RETRIES': '1',
        })
        environment.start()
        self.addCleanup(environment.stop)

    def test_missing_descriptor_in_empty_and_populated_prefixes(self):
        self.assertIsNone(release.read_stored_descriptor(self.remote))
        self.objects.update({self.key + '.old': b'old', self.key + '/nested': b'nested',
                             self.key.replace('release.json', 'runtime.tar.zst'): b'archive'})
        self.assertIsNone(release.read_stored_descriptor(self.remote))

    def test_reads_only_the_exact_existing_descriptor(self):
        body = b'{"signed": "descriptor"}'
        self.objects.update({self.key: body, self.key + '.old': b'old'})
        self.assertEqual(release.read_stored_descriptor(self.remote), body)

    def test_rejects_empty_and_oversized_descriptors_before_download(self):
        for body in (b'', b'x' * (release.METADATA_LIMIT + 1)):
            with self.subTest(size=len(body)):
                self.objects[self.key] = body
                with self.assertRaisesRegex(ValueError, 'protocol limit'):
                    release.read_stored_descriptor(self.remote)
                self.assertNotIn(('GET', '/bucket/' + self.key), self.requests)

    def test_failed_listing_is_not_absence(self):
        for status in (403, 404, 500):
            with self.subTest(status=status):
                self.errors['GET', '/bucket'] = status
                with self.assertRaises(subprocess.CalledProcessError):
                    release.read_stored_descriptor(self.remote)

    def test_failed_descriptor_read_is_not_absence(self):
        self.objects[self.key] = b'{"signed": "descriptor"}'
        for method in ('HEAD', 'GET'):
            with self.subTest(method=method):
                self.errors.clear()
                self.errors[method, '/bucket/' + self.key] = 403
                with self.assertRaises((subprocess.CalledProcessError, ValueError)):
                    release.read_stored_descriptor(self.remote)


if __name__ == '__main__':
    unittest.main()
