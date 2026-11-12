// SPDX-License-Identifier: GPL-3.0-or-later
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import {prepareLoan, validateLoan} from '../runner/browser-loan.mjs';

const identity = {owner_id: 'owner-a', epoch: '7'};
const browser = {...identity, endpoint: '/tmp/nd-dem-owner-test/browser.sock'};
test('private loan accepts only absolute socket paths and exact fields', () => {
  assert.doesNotThrow(() => validateLoan(browser));
  for (const change of [{epoch:'0'},{epoch:7},{owner_id:''},{endpoint:'ws://example.com:1234/path'},
    {endpoint:'relative.sock'},{endpoint:'/tmp/a/../browser.sock'},{endpoint:'/tmp/browser.sock\0'},
    {endpoint:'/'+'a'.repeat(100)+'.sock'},{extra:true}]) {
    assert.throws(() => validateLoan({...browser,...change}));
  }
});
for (const [name, records, state] of [
  ['no browser requested', {}, 'unused'],
  ['acknowledged', {claim:identity,clean:identity}, 'clean'],
  ['claimed without acknowledgment', {claim:identity}, 'retire'],
  ['orphan acknowledgment', {clean:identity}, 'retire'],
  ['failure wins over acknowledgment', {claim:identity,clean:identity,failed:identity}, 'retire'],
  ['caught failure without browser', {failed:identity}, 'retire'],
  ['stale epoch', {claim:identity,clean:{...identity,epoch:'6'}}, 'retire'],
  ['wrong owner', {claim:identity,clean:{...identity,owner_id:'owner-b'}}, 'retire'],
  ['malformed record', {claim:identity,clean:'not-json'}, 'retire'],
]) {
  test(name, async () => {
    const work=await fs.mkdtemp(path.join(os.tmpdir(),'dem-loan-'));
    const loan=await prepareLoan({modules:'/prepared/node_modules',browser:'/prepared/chrome'},
      {work_dir:work,browser},'/prepared/runner');
    for(const [file,value] of Object.entries(records)) {
      await fs.writeFile(path.join(work,'browser-loan',file+'.json'),typeof value==='string'?value:JSON.stringify(value));
    }
    assert.deepEqual(await loan.disposition(),{...identity,state});
  });
}
