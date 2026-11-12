// SPDX-License-Identifier: GPL-3.0-or-later
// Run the source Node tests with an installed runtime's exact dependencies,
// under the same whole-tree supervisor used by the browser proofs.
import fs from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath, pathToFileURL} from 'node:url';
import {spawn} from 'node:child_process';
const [installedEntry, work] = process.argv.slice(2);
const runtime = await (await import(pathToFileURL(path.join(path.dirname(installedEntry), 'runtime.mjs')))).runtimePaths();
const tests = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const files = (await fs.readdir(tests)).filter(name => !name.startsWith('.') && name.endsWith('.test.mjs')).map(name => path.join(tests, name));
const child = spawn(process.execPath, ['--test', '--test-reporter=tap', ...files], {stdio: 'inherit',
  env: {...process.env, DEM_TEST_DEPENDENCIES: runtime.dependencies, TMPDIR: work}});
child.once('error', error => { throw error; });
child.once('close', (code, signal) => { process.exitCode = signal ? 1 : code; });
