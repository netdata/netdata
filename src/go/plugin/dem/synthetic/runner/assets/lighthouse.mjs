// SPDX-License-Identifier: GPL-3.0-or-later
import fs from 'node:fs/promises';
import {constants} from 'node:fs';
import path from 'node:path';
import {pathToFileURL} from 'node:url';
import protocol from './protocol.cjs';
const {text, event, write} = protocol;
const emit = record => write(3, record);

export function metricsFromLHR(lhr) {
  const metrics = {};
  const score = lhr.categories?.performance?.score;
  if (typeof score === 'number' && Number.isFinite(score) && score >= 0 && score <= 1) metrics.performance = score * 100;
  for (const [field, audit] of Object.entries({fcp_ms: 'first-contentful-paint', lcp_ms: 'largest-contentful-paint', tbt_ms: 'total-blocking-time', si_ms: 'speed-index', cls: 'cumulative-layout-shift'})) {
    const value = lhr.audits?.[audit]?.numericValue;
    if (typeof value === 'number' && Number.isFinite(value) && value >= 0) metrics[field] = value;
  }
  return metrics;
}

let captureEnabled = false;
async function main() {
  const chunks = [];
  for await (const chunk of process.stdin) chunks.push(chunk);
  const request = JSON.parse(Buffer.concat(chunks).toString('utf8'));
  captureEnabled = request.capture;
  const modules = path.join(request.dependencies_path, 'node_modules');
  const {default: lighthouse} = await import(pathToFileURL(path.join(modules, 'lighthouse/core/index.js')));
  const {default: desktop} = await import(pathToFileURL(path.join(modules, 'lighthouse/core/config/lr-desktop-config.js')));
  const {launch} = await import(pathToFileURL(path.join(modules, 'chrome-launcher/dist/index.js')));
  const profile = path.join(request.work_dir, 'tmp', 'lighthouse-profile');
  await fs.mkdir(profile, {recursive: true, mode: 0o700});
  let chrome;
  let result;
  try {
    emit({type: 'event', event: event('audit', {phase: 'start', title: 'Desktop performance audit', status: 'running'})});
    await fs.access(request.browser_path, constants.X_OK);
    chrome = await launch({chromePath: request.browser_path, userDataDir: profile, handleSIGINT: false,
      chromeFlags: ['--headless=new', '--disable-dev-shm-usage']});
    const audit = await lighthouse(request.url, {port: chrome.port, output: request.capture ? ['json', 'html'] : ['json'],
      onlyCategories: ['performance'], maxWaitForLoad: request.timeout_ms, logLevel: 'error'}, desktop);
    if (!audit?.lhr) throw new Error('Lighthouse produced no audit result');
    if (audit.lhr.runtimeError) throw new Error('Lighthouse runtime error: ' + audit.lhr.runtimeError.code + ': ' + audit.lhr.runtimeError.message);
    const artifacts = [];
    if (request.capture && Array.isArray(audit.report) && typeof audit.report[1] === 'string') {
      try {
        await fs.writeFile(path.join(request.work_dir, 'output', 'lighthouse.html'), audit.report[1], {mode: 0o600});
        artifacts.push({id: 'lighthouse-report', kind: 'report', path: 'output/lighthouse.html', mime: 'text/html'});
      } catch (error) {
        emit({type: 'event', event: event('error', {phase: 'capture', status: 'error', message: text('Lighthouse report unavailable: ' + error.message)})});
      }
    }
    result = {status: 'success', metrics: metricsFromLHR(audit.lhr), artifacts,
      capture_state: request.capture ? (artifacts.length ? 'available' : 'unavailable') : 'disabled'};
  } finally {
    if (chrome) await chrome.kill();
  }
  emit({type: 'result', result});
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main().catch(error => {
    emit({type: 'event', event: event('error', {phase: 'audit', status: 'error', message: text(error.message)})});
    emit({type: 'result', result: {status: 'error', error: text(error.message), capture_state: captureEnabled ? 'unavailable' : 'disabled'}});
    process.exitCode = 1;
  });
}
