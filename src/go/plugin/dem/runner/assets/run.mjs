// SPDX-License-Identifier: GPL-3.0-or-later
import fs from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {spawn} from 'node:child_process';
import {createInterface} from 'node:readline';
import protocol from './protocol.cjs';
const {text, event, write} = protocol;
const here = path.dirname(fileURLToPath(import.meta.url));
const emit = record => write(1, record);
let request;

function absolute(value, name) {
  if (typeof value !== 'string' || !path.isAbsolute(value)) throw new Error(name + ' must be an absolute path');
}
function validate(r) {
  if (!r || r.version !== 1 || !['journey', 'lighthouse'].includes(r.kind)) throw new Error('unsupported runner request');
  for (const key of ['dependencies_path', 'browser_path', 'work_dir']) absolute(r[key], key);
  if (typeof r.run_id !== 'string' || !r.run_id) throw new Error('run_id is required');
  if (!Number.isSafeInteger(r.timeout_ms) || r.timeout_ms <= 0) throw new Error('timeout_ms must be positive');
  if (typeof r.capture !== 'boolean') throw new Error('capture must be boolean');
  if (r.kind === 'journey') {
    if (Boolean(r.script) === Boolean(r.script_path)) throw new Error('set exactly one of script and script_path');
    if (r.script && typeof r.script !== 'string') throw new Error('script must be source text');
    if (r.script_path) {
      absolute(r.script_path, 'script_path');
      if (!/\.(?:[cm]?[jt]s)$/.test(r.script_path)) throw new Error('script_path must be a JS or TS file');
    }
  } else if (typeof r.url !== 'string' || !/^https?:$/.test(new URL(r.url).protocol)) throw new Error('url must use HTTP or HTTPS');
  for (const [name, value] of Object.entries(r.secrets || {})) {
    if (!/^DEM_SECRET_[A-Za-z0-9_]+$/.test(name) || typeof value !== 'string' || value.includes('\0')) throw new Error('invalid controlled secret environment entry');
  }
}
async function main() {
  if (Number(process.versions.node.split('.')[0]) < 24) throw new Error('DEM prepared runtime requires Node 24 or newer');
  const chunks = [];
  for await (const chunk of process.stdin) chunks.push(chunk);
  request = JSON.parse(Buffer.concat(chunks).toString('utf8'));
  validate(request);
  protocol.configureSecrets(request.secrets);
  const r = request;
  for (const dir of ['output', 'tmp', 'home', 'cache']) await fs.mkdir(path.join(r.work_dir, dir), {recursive: true, mode: 0o700});
  const env = {
    PATH: path.dirname(process.execPath) + ':/usr/bin:/bin', HOME: path.join(r.work_dir, 'home'),
    TMPDIR: path.join(r.work_dir, 'tmp'), XDG_CONFIG_HOME: path.join(r.work_dir, 'home', '.config'),
    XDG_CACHE_HOME: path.join(r.work_dir, 'cache'), PWTEST_CACHE_DIR: path.join(r.work_dir, 'cache', 'transform'),
    CI: '1', NO_COLOR: '1', LANG: 'C.UTF-8', ...r.secrets,
  };
  let args;
  if (r.kind === 'journey') {
    let entry = r.script_path;
    if (!entry) {
      const input = path.join(r.work_dir, 'input');
      await fs.mkdir(input, {mode: 0o700});
      await fs.writeFile(path.join(input, 'package.json'), '{"type":"module"}\n', {mode: 0o600});
      entry = path.join(input, 'journey.spec.ts');
      await fs.writeFile(entry, r.script, {mode: 0o600});
    }
    const modules = path.join(r.dependencies_path, 'node_modules');
    const config = {
      testDir: path.dirname(entry), workers: 1, retries: 0, repeatEach: 1, fullyParallel: false,
      forbidOnly: true, timeout: r.timeout_ms, globalTimeout: r.timeout_ms,
      respectGitIgnore: false, outputDir: path.join(r.work_dir, 'output'),
      reporter: [[path.join(here, 'reporter.cjs'), {workDir: r.work_dir, capture: r.capture}]],
      projects: [{name: 'chromium', use: {browserName: 'chromium', headless: true,
        launchOptions: {executablePath: r.browser_path, chromiumSandbox: true},
        screenshot: r.capture ? 'only-on-failure' : 'off', trace: 'off', video: 'off'}}],
    };
    const configPath = path.join(r.work_dir, 'playwright.config.cjs');
    const escaped = entry.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    await fs.writeFile(configPath, 'module.exports = Object.assign(' + JSON.stringify(config) + ', {testMatch: new RegExp(' + JSON.stringify('^' + escaped + '$') + ')});\n', {mode: 0o600});
    const tsconfig = path.join(r.work_dir, 'tsconfig.json');
    await fs.writeFile(tsconfig, JSON.stringify({compilerOptions: {allowJs: true, baseUrl: r.dependencies_path, paths: {
      '@playwright/test': [path.join(modules, '@playwright/test/index.mjs')],
      'playwright/test': [path.join(modules, 'playwright/test.mjs')],
      'playwright': [path.join(modules, 'playwright/index.mjs')],
    }}}), {mode: 0o600});
    args = [path.join(modules, 'playwright/cli.js'), 'test', '--config', configPath, '--tsconfig', tsconfig, '--pass-with-no-tests'];
  } else {
    args = [path.join(here, 'lighthouse.mjs')];
  }
  const child = spawn(process.execPath, args, {cwd: r.work_dir, env, stdio: [r.kind === 'lighthouse' ? 'pipe' : 'ignore', 'pipe', 'pipe', 'pipe']});
  if (r.kind === 'lighthouse') child.stdin.end(JSON.stringify({...r, secrets: undefined}));
  let terminal;
  let protocolError;
  for (const [kind, stream] of [['stdout', child.stdout], ['stderr', child.stderr]]) {
    const safe = protocol.secretStream(message => emit({type: 'event', event: event(kind, {phase: 'cli', message})}));
    stream.on('data', chunk => safe.write(chunk));
    stream.on('end', () => safe.end());
  }
  const lines = createInterface({input: child.stdio[3], crlfDelay: Infinity});
  lines.on('line', line => {
    try {
      const record = JSON.parse(line);
      if (record.type === 'event' && record.event) emit(record);
      else if (record.type === 'result' && record.result && !terminal) terminal = record.result;
      else throw new Error('invalid or duplicate reporter record');
    } catch (error) { protocolError = text(error.message); }
  });
  const ended = new Promise(resolve => lines.once('close', resolve));
  const exit = await new Promise((resolve, reject) => { child.once('error', reject); child.once('close', (code, signal) => resolve({code, signal})); });
  await ended;
  if (protocolError || !terminal) throw new Error(protocolError || 'Runner exited without a terminal result (' + (exit.signal || exit.code) + ')');
  if (terminal.status === 'success' && exit.code !== 0) throw new Error('Runner exited unsuccessfully after a successful report');
  emit({type: 'result', result: terminal});
}
main().catch(error => {
  emit({type: 'event', event: event('error', {phase: 'execution', status: 'error', message: text(error.message)})});
  emit({type: 'result', result: {status: 'error', error: text(error.message), capture_state: request?.capture ? 'unavailable' : 'disabled'}});
  process.exitCode = 1;
});
