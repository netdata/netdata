// SPDX-License-Identifier: GPL-3.0-or-later
import fs from 'node:fs/promises';
import path from 'node:path';
import {pathToFileURL} from 'node:url';
import {write} from './protocol.cjs';

// The Agent supervises this lifetime separately from each fresh Test process.
// No request script, target credentials or per-attempt environment enters it.
export async function browserOwner(runtime) {
  const first = Promise.withResolvers();
  const stop = Promise.withResolvers();
  let requestedStop = false;
  let inputError;
  let initial;
  process.stdin.setEncoding('utf8');
  const input = (async () => {
    let pending = '';
    for await (const chunk of process.stdin) {
      pending += chunk.toString('utf8');
      if (Buffer.byteLength(pending) > 4 * 1024 * 1024) throw new Error('owner request exceeds transport limit');
      for (let end; (end = pending.indexOf('\n')) !== -1;) {
        const line = pending.slice(0, end);
        pending = pending.slice(end + 1);
        const value = JSON.parse(line);
        if (!initial) { initial = value; first.resolve(value); }
        else {
          if (value.type !== 'stop' || Object.keys(value).length !== 1) throw new Error('invalid browser owner command');
          requestedStop = true;
          stop.resolve();
          return;
        }
      }
    }
    if (!initial || pending.trim()) throw new Error('incomplete browser owner request');
    requestedStop = true;
    stop.resolve();
  })().catch(error => { inputError = error; first.reject(error); requestedStop = true; stop.resolve(); });
  let server;
  let failed = false;
  try {
    const request = await first.promise;
    if (request.version !== 2 || typeof request.owner_id !== 'string' || !/^[a-zA-Z0-9_-]{1,128}$/.test(request.owner_id) ||
        typeof request.work_dir !== 'string' || !path.isAbsolute(request.work_dir) ||
        Object.keys(request).some(key => !['version', 'owner_id', 'work_dir'].includes(key))) {
      throw new Error('invalid browser owner request');
    }
    if (requestedStop) return;
    const home = path.join(request.work_dir, 'home');
    await fs.mkdir(home, {recursive: true, mode: 0o700});
    const {chromium} = await import(pathToFileURL(path.join(runtime.modules, 'playwright/index.mjs')));
    server = await chromium.launchServer({executablePath: runtime.browser, chromiumSandbox: true,
      headless: true, host: '127.0.0.1', env: {PATH: path.dirname(process.execPath) + ':/usr/bin:/bin',
        HOME: home, TMPDIR: home, XDG_CONFIG_HOME: home, XDG_CACHE_HOME: home, LANG: 'C.UTF-8'}});
    server.on('close', () => { if (!requestedStop) failed = true; stop.resolve(); });
    if (!requestedStop) {
      write(1, {type: 'ready', owner_id: request.owner_id, endpoint: server.wsEndpoint()});
      await stop.promise;
    }
  } finally {
    requestedStop = true;
    try { await server?.close(); }
    finally { process.stdin.destroy(); await input; }
  }
  if (inputError || failed) throw new Error('browser owner stopped unexpectedly');
}
