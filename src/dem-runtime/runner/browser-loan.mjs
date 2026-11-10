// SPDX-License-Identifier: GPL-3.0-or-later
import fs from 'node:fs/promises';
import path from 'node:path';

export function validateLoan(loan) {
  if (!loan || typeof loan !== 'object' || Array.isArray(loan) ||
      Object.keys(loan).sort().join(',') !== 'endpoint,epoch,owner_id' ||
      typeof loan.owner_id !== 'string' || !/^[a-zA-Z0-9_-]{1,128}$/.test(loan.owner_id) ||
      typeof loan.epoch !== 'string' || !/^[1-9][0-9]{0,19}$/.test(loan.epoch)) {
    throw new Error('invalid browser loan');
  }
  const endpoint = new URL(loan.endpoint);
  if (endpoint.protocol !== 'ws:' || endpoint.hostname !== '127.0.0.1' || !endpoint.port ||
      endpoint.username || endpoint.password || endpoint.search || endpoint.hash || endpoint.pathname === '/') {
    throw new Error('invalid browser owner endpoint');
  }
}

// Worker records are private attempt evidence, not public captures or history.
// The adapter reads them after the CLI exits; the Agent still joins its whole tree.
export async function prepareLoan(runtime, request, here) {
  const {owner_id, epoch, endpoint} = request.browser;
  const identity = {owner_id, epoch};
  const directory = path.join(request.work_dir, 'browser-loan');
  await fs.mkdir(directory, {mode: 0o700});
  const shim = path.join(directory, 'test.mjs');
  const api = path.join(runtime.modules, '@playwright/test/index.mjs');
  const implementation = path.join(here, 'managed-browser.mjs');
  // Absolute paths work in both native ESM and Test's CommonJS transformation.
  await fs.writeFile(shim, `import * as api from ${JSON.stringify(api)};
import fs from 'node:fs/promises';
import path from 'node:path';
import {managedTest} from ${JSON.stringify(implementation)};
export * from ${JSON.stringify(api)};
const identity=${JSON.stringify(identity)};
const record=name=>fs.writeFile(path.join(${JSON.stringify(directory)},name+'.json'),JSON.stringify(identity),{flag:'wx',mode:0o600});
export const test=managedTest(api,{endpoint:${JSON.stringify(endpoint)},
launchOptions:${JSON.stringify({executablePath:runtime.browser,chromiumSandbox:true})},
claim:()=>record('claim'),cleaned:()=>record('clean'),failed:()=>record('failed')});
export default test;
`, {mode: 0o600});
  return {
    shim,
    async disposition() {
      const records = {};
      try {
        for (const name of ['claim', 'clean', 'failed']) {
          try {
            const raw = await fs.readFile(path.join(directory, name + '.json'), 'utf8');
            if (raw.length > 1024) throw new Error('invalid browser loan record');
            const value = JSON.parse(raw);
            if (Object.keys(value).sort().join(',') !== 'epoch,owner_id' || value.owner_id !== owner_id || value.epoch !== epoch) {
              throw new Error('mismatched browser loan record');
            }
            records[name] = true;
          } catch (error) { if (error.code !== 'ENOENT') throw error; }
        }
      } catch { return {...identity, state: 'retire'}; }
      const state = records.failed ? 'retire' : records.claim ? (records.clean ? 'clean' : 'retire') : (records.clean ? 'retire' : 'unused');
      return {...identity, state};
    },
  };
}
