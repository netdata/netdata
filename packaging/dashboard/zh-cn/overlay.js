/* Display-only Chinese layer for the installed Netdata dashboard. */
(() => {
  'use strict';
  if (window.NetdataZh || !window.NetdataZhDictionary) return;
  const { exact, phrases, prose = Object.create(null), glossary, version } = window.NetdataZhDictionary;
  const folded = new Map(Object.entries(exact).map(([en, zh]) => [en.toLowerCase(), zh]));
  const textRecords = new WeakMap();
  const attributeRecords = new WeakMap();
  const stats = { flushes: 0, translated: 0, version };
  const skip = 'script,style,noscript,code,pre,kbd,textarea,[contenteditable]:not([contenteditable="false"]),[data-netdata-zh-ui]';
  const attributes = ['title', 'aria-label', 'placeholder', 'alt'];
  const units = { s: '秒', sec: '秒', secs: '秒', second: '秒', seconds: '秒', m: '分钟', min: '分钟', mins: '分钟', minute: '分钟', minutes: '分钟', h: '小时', hr: '小时', hour: '小时', hours: '小时', d: '天', day: '天', days: '天', w: '周', week: '周', weeks: '周' };
  Object.assign(units, { month: '个月', months: '个月', year: '年', years: '年' });
  let language = 'zh';
  try { language = localStorage.getItem('netdata.display-language') === 'en' ? 'en' : 'zh'; } catch {}

  function translate(input, proseContext = false) {
    const raw = input.trim();
    const source = raw.replace(/\s+/g, ' ');
    if (!source || !/[a-z]/i.test(source)) return input;
    let result = (proseContext && prose[source]) || exact[source] || folded.get(source.toLowerCase());
    if (!result && source.includes(' · ')) {
      const parts = source.split(' · ');
      const translated = parts.map(part => translate(part, proseContext));
      if (translated.some((s, i) => s !== parts[i])) result = translated.join(' · ');
    }
    if (!result) {
      let match;
      if ((match = source.match(/^(.+): Netdata Agent Console$/))) result = `${match[1]}：${exact['Netdata Agent Console']}`;
      else if ((match = source.match(/^Fleet map with (\d+) filtered nodes and (\d+) displayed items$/))) result = `节点分布图：筛选出 ${match[1]} 个节点，显示 ${match[2]} 项`;
      else if ((match = source.match(/^(\d+) results available\.$/))) result = `有 ${match[1]} 个可用选项。`;
      else if ((match = source.match(/^(\d+) matches?$/))) result = `${match[1]} 个匹配项`;
      else if ((match = source.match(/^App group (.+) file descriptors utilization$/))) result = `应用组 ${match[1]} 文件描述符使用率`;
      else if ((match = source.match(/^Search for "(.+)"$/))) result = `搜索“${match[1]}”`;
      else if ((match = source.match(/^(\d+) Cores$/))) result = `${match[1]} 个核心`;
      else if ((match = source.match(/^No node on the map matches “(.+)”\.$/))) result = `分布图中没有匹配“${match[1]}”的节点。`;
      else if ((match = source.match(/^(\d+) more on the map\. Keep typing to narrow it down\.$/))) result = `分布图中还有 ${match[1]} 个匹配节点，继续输入以缩小范围。`;
      else if ((match = source.match(/^Located (.+), (\d+) of (\d+)$/))) result = `已定位 ${match[1]}，第 ${match[2]} 个，共 ${match[3]} 个`;
      else if ((match = source.match(/^(Sun|Mon|Tue|Wed|Thu|Fri|Sat), (Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec) (\d{1,2}), (\d{4})$/))) result = `${match[4]}年${['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'].indexOf(match[2]) + 1}月${match[3]}日${{ Sun: '周日', Mon: '周一', Tue: '周二', Wed: '周三', Thu: '周四', Fri: '周五', Sat: '周六' }[match[1]]}`;
      else if ((match = source.match(/^(\d+) (seconds?|minutes?|hours?|days?|weeks?|months?|years?) ago$/))) result = `${match[1]}${units[match[2]]}前`;
      else if ((match = source.match(/^(\d+)\/(\d+) timeframes processed$/))) result = `已处理 ${match[1]}/${match[2]} 个时段`;
      else if ((match = source.match(/^next (\d+) timeframes$/))) result = `接下来 ${match[1]} 个时段`;
      else if ((match = source.match(/^([\d.]+)s elapsed$/))) result = `已用时 ${match[1]} 秒`;
      else if ((match = source.match(/^Auto \((\d+) data points\)$/))) result = `自动（${match[1]} 个数据点）`;
      else if ((match = source.match(/^Preview (.+)$/))) result = '预览 ' + match[1];
      else if ((match = source.match(/^The functions below lose accuracy when applied on tiered data, compared to high resolution data\. Your current query is ([\d.]+)% high resolution and ([\d.]+)% tiered data of lower resolution\.$/))) result = `以下函数用于分层降采样数据时会降低精度。当前查询中，高分辨率数据占 ${match[1]}%，较低分辨率的分层数据占 ${match[2]}%。`;
      else if ((match = source.match(/^(\d+) (contexts?|nodes?|dimensions?|instances?) selected \/ (\d+)$/))) result = `已选择 ${match[1]} 个${translate(match[2])} / ${match[3]}`;
      else if (/^(?:\d+(?:\.\d+)?(?:mo|min|ms|[ywdhms])\s*)+$/.test(source)) result = source.replace(/(\d+(?:\.\d+)?)(mo|min|ms|[ywdhms])/g, (_, n, unit) => n + ({ y: '年', mo: '个月', w: '周', d: '天', h: '小时', min: '分钟', m: '分钟', s: '秒', ms: '毫秒' })[unit]);
      else if ((match = source.match(/^on (.+)$/))) result = '位于 ' + match[1];
      else if ((match = source.match(/^([-–—]+\s*)(.+)$/))) result = match[1] + translate(match[2]);
      else if ((match = source.match(/^\[(.+)\]$/))) result = '[' + translate(match[1]) + ']';
      else if ((match = source.match(/^(.+?)[:：]$/))) { const label = translate(match[1]); if (label !== match[1]) result = label + '：'; }
      else if ((match = source.match(/^(.+?), (sorted ascending|sorted descending|sortable, not sorted)$/))) result = translate(match[1]) + '，' + translate(match[2]);
      else if ((match = source.match(/^(.*?)[:：]\s*(.+)$/))) {
        const label = translate(match[1]);
        if (label !== match[1]) result = label + '：' + translate(match[2]);
      }
      else if ((match = source.match(/^every\s+(\d+)\s*(s|sec|seconds?|m|min|minutes?|h|hours?)$/i))) result = `每 ${match[1]} ${units[match[2].toLowerCase()]}`;
      else if ((match = source.match(/^(\d+) of (\d+)$/))) result = `${match[2]} 项中的 ${match[1]} 项`;
      else if ((match = source.match(/^(Selected|Findings|Blast Radius Analysis)\s*[·:]\s*(.*)$/))) result = translate(match[1]) + ' · ' + match[2];
      else if ((match = source.match(/^(Analyzing[…\.]+)\s*(\d+%)$/))) result = '正在分析… ' + match[2];
      else if ((match = source.match(/^(\d+) selected\s*\/\s*(\d+)$/))) result = `已选择 ${match[1]} / ${match[2]}`;
      else if ((match = source.match(/^(Min|Max|Avg|Range) (.+)$/))) result = translate(match[1]) + ' ' + match[2];
      else if ((match = source.match(/^\((\d+) results?\)$/))) result = `（${match[1]} 条结果）`;
      else if ((match = source.match(/^(\d+) results?\)?$/))) result = `${match[1]} 条结果${source.endsWith(')') ? ')' : ''}`;
      else if ((match = source.match(/^(\d+) (states?|contexts?)$/i))) result = `${match[1]} 个${match[2].startsWith('state') ? '状态' : '指标上下文'}`;
      else if ((match = source.match(/^(\d+)\s*(nodes?|systems?|instances?|dimensions?|labels?|charts?|metrics?|alerts?|events?)$/i))) {
        const nouns = { node: '节点', system: '系统', instance: '实例', dimension: '维度', label: '标签', chart: '图表', metric: '指标', alert: '告警', event: '事件' };
        result = `${match[1]} 个${nouns[match[2].toLowerCase().replace(/s$/, '')]}`;
      } else if ((match = source.match(/^(\d+) of (\d+) (dimensions?|nodes?|instances?)$/i))) result = `${match[2]} 个${translate(match[3])}中的 ${match[1]} 个`;
      else if ((match = source.match(/^(\d+)\s*(sec|secs|seconds?|min|mins|minutes?|hr|hours?|days?|weeks?|months?|years?)$/i))) result = `${match[1]} ${units[match[2].toLowerCase()]}`;
      else if ((match = source.match(/^(Last|last)\s+(.+)$/))) { const time = translate(match[2]); if (time !== match[2]) result = '最近 ' + time; }
      else if ((match = source.match(/^(Applications Groups|User Group|User|Systemd Services) (.+)$/))) {
        const prefix = { 'Applications Groups': '应用组', 'User Group': '用户组', User: '用户', 'Systemd Services': 'Systemd 服务' };
        const tail = phrases[match[2]] || translate(match[2]);
        if (tail !== match[2]) result = prefix[match[1]] + tail;
      } else if ((match = source.match(/^(IPv[46]|IP|TCP|UDP|UDPLite) (.+)$/))) {
        const tail = phrases[match[2]] || translate(match[2]);
        if (tail !== match[2]) result = match[1] + ' ' + tail;
      } else if ((match = source.match(/^(Top|Total|Avg|Average) (.+)$/))) {
        // Only translate recognized metric phrases, never arbitrary names or IDs.
        const tail = exact[match[2]] || phrases[match[2]] || folded.get(match[2].toLowerCase());
        if (tail) result = ({ Top: '排名：', Total: '总计：', Avg: '平均：', Average: '平均：' })[match[1]] + tail;
      }
    }
    if (!result && Object.hasOwn(phrases, source)) result = phrases[source];
    if (!result || result === source) return input;
    return input.slice(0, input.indexOf(raw)) + result + input.slice(input.indexOf(raw) + raw.length);
  }

  function excluded(element) { return !element || !!element.closest(skip); }
  // Placeholders are UI text; textarea contents and submitted values stay untouched.
  function textareaAttributes(element) { return element?.tagName === 'TEXTAREA' && !excluded(element.parentElement); }

  function textNode(node) {
    if (excluded(node.parentElement)) return;
    const current = node.nodeValue;
    const old = textRecords.get(node);
    const original = old && (current === old.output || current === old.source) ? old.source : current;
    const alertContext = node.parentElement.closest('[data-testid="alert-details-modal-tabs-container"]');
    const alertCommand = original.trim() === 'command' && alertContext && node.parentElement.parentElement.textContent.includes('$this');
    const alertConnector = original.trim() === 'on' && alertContext && node.parentElement.childElementCount === 1 && node.parentElement.lastElementChild.tagName === 'SPAN';
    const output = language === 'zh' ? (alertCommand ? original.replace('command', '命令') : alertConnector ? original.replace('on', '位于') : translate(original, !!node.parentElement.closest('.dashboard-context-info'))) : original;
    textRecords.set(node, { source: original, output });
    if (output !== current) { node.nodeValue = output; stats.translated++; }
  }

  function elementAttributes(element) {
    if (excluded(element) && !textareaAttributes(element)) return;
    let records = attributeRecords.get(element);
    if (!records) { records = new Map(); attributeRecords.set(element, records); }
    for (const attr of attributes) {
      const current = element.getAttribute(attr);
      if (current === null) { records.delete(attr); continue; }
      const old = records.get(attr);
      const original = old && (current === old.output || current === old.source) ? old.source : current;
      const output = language === 'zh' ? translate(original) : original;
      records.set(attr, { source: original, output });
      if (current !== output) element.setAttribute(attr, output);
    }
  }

  function walk(root) {
    if (!root.isConnected) return;
    if (root.nodeType === Node.TEXT_NODE) { textNode(root); return; }
    if (root.nodeType !== Node.ELEMENT_NODE || (excluded(root) && !textareaAttributes(root))) return;
    elementAttributes(root);
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_ELEMENT | NodeFilter.SHOW_TEXT, {
      acceptNode(node) {
        if (node.nodeType === Node.ELEMENT_NODE) return node.matches(skip) && !textareaAttributes(node) ? NodeFilter.FILTER_REJECT : NodeFilter.FILTER_ACCEPT;
        return NodeFilter.FILTER_ACCEPT;
      },
    });
    let node;
    while ((node = walker.nextNode())) {
      if (node.nodeType === Node.TEXT_NODE) textNode(node);
      else elementAttributes(node);
    }
  }

  const pending = new Set();
  const observe = () => observer.observe(document.documentElement, { subtree: true, childList: true, characterData: true, attributes: true, attributeFilter: attributes });
  function flush() {
    // Disconnect only during synchronous writes, preventing feedback on our own translations.
    observer.disconnect();
    try {
      stats.flushes++;
      const roots = [...pending];
      pending.clear();
      const rootSet = new Set(roots);
      for (const root of roots) {
        let parent = root.parentElement;
        while (parent && !rootSet.has(parent)) parent = parent.parentElement;
        if (!parent) walk(root);
      }
    } finally { observe(); }
  }
  const observer = new MutationObserver(mutations => {
    for (const mutation of mutations) {
      const element = mutation.target.nodeType === Node.ELEMENT_NODE ? mutation.target : mutation.target.parentElement;
      if (excluded(element) && !(mutation.type === 'attributes' && textareaAttributes(element))) continue;
      if (mutation.type === 'childList') {
        for (const node of mutation.addedNodes) {
          if (node.nodeType === Node.TEXT_NODE || node.nodeType === Node.ELEMENT_NODE) pending.add(node);
        }
      } else pending.add(mutation.target);
    }
    // MutationObserver runs at the microtask checkpoint, before rendering. A timer
    // lets React's English text reach several frames on every live update.
    if (pending.size) flush();
  });

  function setLanguage(value) {
    language = value === 'en' ? 'en' : 'zh';
    try { localStorage.setItem('netdata.display-language', language); } catch {}
    document.documentElement.lang = language === 'zh' ? 'zh-CN' : 'en';
    pending.add(document.documentElement);
    flush();
    const button = document.getElementById('netdata-zh-language');
    button.value = language;
    button.setAttribute('aria-label', language === 'zh' ? '显示语言' : 'Display language');
    button.title = language === 'zh' ? '选择显示语言' : 'Choose display language';
  }

  function mount() {
    const style = document.createElement('style');
    style.textContent = `
      #netdata-zh-tools{position:fixed;right:16px;bottom:36px;z-index:10000;display:flex;gap:5px;font:12px/1.5 system-ui,"Microsoft YaHei",sans-serif}
      #netdata-zh-tools button,#netdata-zh-tools select,#netdata-zh-help button{border:1px solid #66727e;background:#17222e;color:#f1f5f9;border-radius:6px;padding:5px 10px;cursor:pointer;font:inherit}
      #netdata-zh-tools button:hover,#netdata-zh-tools select:hover,#netdata-zh-help button:hover{background:#2b4054}
      #netdata-zh-help{position:fixed;inset:8vh 24px auto auto;width:min(560px,calc(100vw - 48px));max-height:76vh;overflow:auto;box-sizing:border-box;background:#17222e;color:#eef2f6;border:1px solid #65788b;border-radius:10px;padding:22px;z-index:10001;box-shadow:0 12px 48px #0008;font:14px/1.8 system-ui,"Microsoft YaHei",sans-serif}
      #netdata-zh-help[hidden]{display:none}#netdata-zh-help h2{margin:0;font-size:19px}#netdata-zh-help h3{font-size:14px;color:#87d7bd;margin:18px 0 4px}#netdata-zh-help p{margin:0}#netdata-zh-help header{display:flex;justify-content:space-between;align-items:center;gap:18px}
      #netdata-zh-help footer{margin-top:18px;color:#b3c1cd;font-size:12px}
      @media(max-width:650px){#netdata-zh-tools{bottom:40px;right:8px}#netdata-zh-tools button,#netdata-zh-tools select{padding:4px 7px}}
    `;
    document.head.append(style);
    const tools = document.createElement('div');
    tools.id = 'netdata-zh-tools';
    tools.setAttribute('data-netdata-zh-ui', '');
    const lang = document.createElement('select');
    lang.id = 'netdata-zh-language';
    for (const [value, label] of [['zh', '中文（简体）'], ['en', 'English']]) {
      const option = document.createElement('option'); option.value = value; option.textContent = label; lang.append(option);
    }
    lang.addEventListener('change', () => setLanguage(lang.value));
    const help = document.createElement('button');
    help.type = 'button'; help.textContent = '?'; help.title = '指标说明'; help.setAttribute('aria-label', '指标说明'); help.setAttribute('aria-controls', 'netdata-zh-help'); help.setAttribute('aria-expanded', 'false');
    const panel = document.createElement('section');
    panel.id = 'netdata-zh-help'; panel.hidden = true; panel.setAttribute('data-netdata-zh-ui', ''); panel.setAttribute('aria-label', 'Netdata 中文指标说明');
    const header = document.createElement('header');
    const title = document.createElement('h2'); title.textContent = 'Netdata 中文指标说明';
    const close = document.createElement('button'); close.type = 'button'; close.textContent = '关闭';
    const toggleHelp = open => { panel.hidden = !open; help.setAttribute('aria-expanded', String(open)); if (open) close.focus(); else help.focus(); };
    help.addEventListener('click', () => toggleHelp(panel.hidden)); close.addEventListener('click', () => toggleHelp(false));
    document.addEventListener('keydown', event => { if (event.key === 'Escape' && !panel.hidden) toggleHelp(false); });
    header.append(title, close); panel.append(header);
    for (const [term, explanation] of glossary) {
      const heading = document.createElement('h3'); heading.textContent = term;
      const paragraph = document.createElement('p'); paragraph.textContent = explanation;
      panel.append(heading, paragraph);
    }
    const footer = document.createElement('footer');
    footer.textContent = '选择 English 可查看英文原文。';
    panel.append(footer); tools.append(lang, help); document.body.append(tools, panel);
    setLanguage(language);
  }
  window.NetdataZh = { translate, setLanguage, stats, get language() { return language; } };
  observe();
  if (document.body) mount(); else document.addEventListener('DOMContentLoaded', mount, { once: true });
})();
