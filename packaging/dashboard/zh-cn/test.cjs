const assert = require('node:assert/strict');
const path = require('node:path');
const { chromium } = require('playwright-core');
(async () => {
  const browser = await chromium.launch({ channel: process.env.CHROME_CHANNEL || 'chrome', headless: true });
  try {
    const page = await browser.newPage();
    await page.route('http://netdata.test/', route => route.fulfill({ body: '<html><body></body></html>', contentType: 'text/html' }));
    await page.goto('http://netdata.test/');
    await page.setContent('<html><head><title>Metrics</title></head><body><h1>Metrics</h1><p id="cpu">Total CPU utilization</p><input value="Search" placeholder="Search"><code>system.cpu</code><div id="chart">some 10</div><a href="/Nodes">Nodes</a><div id="live"></div></body></html>');
    await page.addScriptTag({ path: path.join(__dirname, 'dictionary.js') });
    await page.addScriptTag({ path: path.join(__dirname, 'overlay.js') });
    await page.waitForFunction(() => document.querySelector('h1').textContent === '指标');
    assert.equal(await page.locator('#netdata-zh-language').evaluate(el => el.tagName), 'SELECT', 'display language has explicit options');
    assert.equal(await page.locator('#netdata-zh-language option:checked').textContent(), '中文（简体）');
    assert.deepEqual(await page.locator('#netdata-zh-language option').allTextContents(), ['中文（简体）', 'English']);
    await page.evaluate(() => { document.title = 'example-host: Netdata Agent Console'; });
    await page.waitForFunction(() => document.title === 'example-host：Netdata 采集代理控制台', undefined, { timeout: 1000 });
    assert.equal(await page.locator('#cpu').textContent(), 'CPU 总使用率');
    assert.equal(await page.locator('input').inputValue(), 'Search');
    assert.equal(await page.locator('input').getAttribute('placeholder'), '搜索');
    assert.equal(await page.locator('code').textContent(), 'system.cpu');
    assert.equal(await page.locator('a').getAttribute('href'), '/Nodes');
    const englishFrames = await page.evaluate(async () => {
      const live = document.querySelector('#live');
      const misses = [];
      for (let i = 0; i < 12; i++) {
        live.textContent = i % 2 ? 'Total Disk Writes' : 'Total Disk Reads';
        await new Promise(requestAnimationFrame);
        if (/Total Disk/.test(live.textContent)) misses.push(live.textContent);
      }
      return misses;
    });
    assert.deepEqual(englishFrames, [], 'live English updates must be translated before the next paint');
    assert.match(await page.locator('#chart').textContent(), /部分.*10/);
    await page.evaluate(() => { document.querySelector('#live').textContent = 'Total Disk Reads'; });
    await page.waitForFunction(() => document.querySelector('#live').textContent === '磁盘总读取速率');
    await page.evaluate(() => { document.querySelector('#live').firstChild.nodeValue = 'Total Disk Writes'; });
    await page.waitForFunction(() => document.querySelector('#live').textContent === '磁盘总写入速率');
    const tipMutations = await page.evaluate(async () => {
      const cpu = document.querySelector('#cpu');
      let count = 0;
      const observer = new MutationObserver(records => { count += records.length; });
      observer.observe(cpu, { attributes: true, attributeFilter: ['title'] });
      cpu.firstChild.nodeValue = 'Total CPU utilization';
      await new Promise(requestAnimationFrame);
      observer.disconnect();
      return count;
    });
    assert.equal(tipMutations, 0, 'live updates must not create extra tooltip mutations');
    assert.equal(await page.locator('#cpu').getAttribute('title'), null, 'Chinese labels must not generate an extra English-original tooltip');
    await page.locator('#netdata-zh-language').selectOption('en');
    assert.equal(await page.title(), 'example-host: Netdata Agent Console', 'dynamic head text restores when switching to English');
    assert.equal(await page.locator('h1').textContent(), 'Metrics');
    assert.equal(await page.locator('#live').textContent(), 'Total Disk Writes');
    assert.equal(await page.locator('input').getAttribute('placeholder'), 'Search');
    await page.locator('#netdata-zh-language').selectOption('zh');
    await page.waitForFunction(() => document.querySelector('h1').textContent === '指标');
    await page.getByRole('button', { name: '指标说明', exact: true }).click();
    assert.match(await page.locator('#netdata-zh-help').innerText(), /PSI/);
    assert.equal(await page.evaluate(() => window.NetdataZh.translate('system.cpu')), 'system.cpu');
    assert.equal(await page.evaluate(() => window.NetdataZh.translate('https://example.com/Nodes')), 'https://example.com/Nodes');
    assert.equal(await page.evaluate(() => window.NetdataZh.translate('HTTP Response Time')), 'HTTP 响应时间');
    assert.equal(await page.evaluate(() => window.NetdataZh.translate('3 dimensions')), '3 个维度');
    assert.equal(await page.evaluate(() => window.NetdataZh.translate('every 3s')), '每 3 秒');
    for (const [source, expected] of [
      ['No access', '无访问权限'],
      ['Manage alerts', '管理告警'],
      ['Waiting analysis…', '等待分析…'],
      ['2/10 timeframes processed · Waiting for data: #5, #6 · Analyzing: #3, #4 · Additional timeframes queued · 3s elapsed', '已处理 2/10 个时段 · 等待数据：#5, #6 · 正在分析：#3, #4 · 更多时段正在排队 · 已用时 3 秒'],
      ['More · next 10 timeframes', '更多 · 接下来 10 个时段'],
      ['0 contexts selected / 12', '已选择 0 个指标上下文 / 12'],
      ['29d 21h 12min 40s', '29天 21小时 12分钟 40秒'],
      ['  Manage\n  alerts  ', '  管理告警  '],
      ['Wed, Nov 11, 2026', '2026年11月11日周三'],
      ['14 days ago', '14天前'],
      ['Unseen', '尚未连接'],
      ['1mo 1d 23h 44min 44s', '1个月 1天 23小时 44分钟 44秒'],
      ['Fleet map with 12 filtered nodes and 4 displayed items', '节点分布图：筛选出 12 个节点，显示 4 项'],
      ['Search for "system.cpu"', '搜索“system.cpu”'],
      ['10 results available.', '有 10 个可用选项。'],
      ['0 matches', '0 个匹配项'],
      ['App group alarm-notify file descriptors utilization', '应用组 alarm-notify 文件描述符使用率'],
      ['2 Cores', '2 个核心'],
      ['No node on the map matches “example-host”.', '分布图中没有匹配“example-host”的节点。'],
      ['3 more on the map. Keep typing to narrow it down.', '分布图中还有 3 个匹配节点，继续输入以缩小范围。'],
      ['Located example-host, 2 of 3', '已定位 example-host，第 2 个，共 3 个'],
    ]) assert.equal(await page.evaluate(s => window.NetdataZh.translate(s), source), expected, source);
    for (const technicalName of ['constructor', '__proto__', 'toString', 'hasOwnProperty']) {
      assert.equal(await page.evaluate(name => window.NetdataZh.translate(name), technicalName), technicalName, 'unknown technical names stay intact');
    }
    assert.equal(await page.evaluate(() => window.NetdataZh.translate('technical · A')), 'technical · A', 'compound text must not enable chart prose translations');
    await page.evaluate(() => {
      const host = document.createElement('div');
      host.innerHTML = '<span id="technical-command">command</span><span id="technical-on">on</span><div data-testid="alert-details-modal-tabs-container"><p>More details about <span>$this</span> <span id="alert-command">command</span></p><span id="alert-connector">on <span>example-host</span></span></div>';
      document.body.append(host);
    });
    await page.waitForFunction(() => document.querySelector('#alert-command').textContent === '命令', undefined, { timeout: 1000 });
    assert.equal(await page.locator('#technical-command').textContent(), 'command', 'technical command label stays intact outside alert explanation');
    assert.equal(await page.locator('#alert-connector').textContent(), '位于 example-host');
    assert.equal(await page.locator('#technical-on').textContent(), 'on', 'technical status stays intact outside alert explanation');
    await page.evaluate(() => {
      const info = document.createElement('div');
      info.className = 'dashboard-context-info'; info.id = 'info-prose'; info.textContent = 'and';
      document.body.append(info);
    });
    await page.waitForFunction(() => document.querySelector('#info-prose').textContent === '和');
    await page.locator('#netdata-zh-language').selectOption('en');
    assert.equal(await page.locator('#info-prose').textContent(), 'and');
    await page.locator('#netdata-zh-language').selectOption('zh');
    await page.evaluate(() => {
      const textarea = document.createElement('textarea');
      textarea.id = 'help-input'; textarea.placeholder = 'Ask anything about Netdata...'; textarea.value = 'system.cpu: Search';
      document.body.appendChild(textarea);
    });
    await page.waitForFunction(() => document.querySelector('#help-input').placeholder === '询问任何关于 Netdata 的问题…', undefined, { timeout: 1000 });
    assert.equal(await page.locator('#help-input').inputValue(), 'system.cpu: Search', 'translating help placeholders must not modify submitted input');
    await page.evaluate(() => { document.querySelector('#help-input').placeholder = 'Type your message...'; });
    await page.waitForFunction(() => document.querySelector('#help-input').placeholder === '输入消息…', undefined, { timeout: 1000 });
    await page.locator('#netdata-zh-language').selectOption('en');
    assert.equal(await page.locator('#help-input').getAttribute('placeholder'), 'Type your message...');
    assert.equal(await page.locator('#help-input').inputValue(), 'system.cpu: Search');
    await page.locator('#netdata-zh-language').selectOption('zh');
    const before = await page.evaluate(() => window.NetdataZh.stats.flushes);
    await page.waitForTimeout(500);
    assert.equal(await page.evaluate(() => window.NetdataZh.stats.flushes), before, 'observer must settle');
    const early = await browser.newPage();
    early.on('pageerror', error => console.error('Early bootstrap:', error.message));
    const fs = require('node:fs');
    await early.route('http://netdata.test/slow.js', async route => {
      await new Promise(resolve => setTimeout(resolve, 350));
      await route.fulfill({ body: '', contentType: 'application/javascript' });
    });
    const bootstrap = ['dictionary.js', 'overlay.js'].map(name => fs.readFileSync(path.join(__dirname, name), 'utf8')).join('\n');
    await early.route('http://netdata.test/early', route => route.fulfill({ contentType: 'text/html; charset=utf-8', body: '<html><head><meta charset="utf-8"><script>' + bootstrap + '</script><script defer src="/slow.js"></script></head><body><h1>Metrics</h1><script>requestAnimationFrame(() => { window.firstFrame = { text: document.querySelector("h1").textContent, ready: document.readyState }; });</script></body></html>' }));
    await early.goto('http://netdata.test/early');
    assert.equal(await early.evaluate(() => window.firstFrame.text), '指标', 'parser-time text must be Chinese before DOMContentLoaded: ' + JSON.stringify(await early.evaluate(() => ({ frame: window.firstFrame, stats: window.NetdataZh?.stats }))));
    assert.equal(await early.locator('#netdata-zh-tools').count(), 1, 'early bootstrap mounts tools once');
    await early.close();
    console.log('PASS: Chinese, dynamic updates, English restoration, input/ID protection, glossary, observer convergence');
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
