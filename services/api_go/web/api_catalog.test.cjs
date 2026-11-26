const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const read = name => fs.readFileSync(path.join(__dirname, 'static', name), 'utf8');
const catalog = read('api-catalog.js');
const app = read('app.js');
const escapeCode = app.split(/\r?\n/).find(line => line.startsWith('function esc(value)'));
const navigation = app.slice(app.indexOf('function adminShowView('), app.indexOf('bindAdminApiCatalog();'));
const operations = [
  {method: 'GET', path: '/health/live', group: '健康检查', summary: '检查进程存活', auth: 'public'},
  {method: 'POST', path: '/api/v1/admin/jobs/{id}/retry', group: '任务管理', summary: '重试任意任务', auth: 'admin', details: '仅限失败或已取消的任务。'},
  {method: 'GET', path: '/api/v1/admin/jobs/{id}', group: '任务管理', summary: '查看任务详情', auth: 'admin'},
];
const afterRequest = () => new Promise(resolve => setImmediate(resolve));

function harness() {
  const nodes = new Map(), requests = [], copied = [], notices = [];
  function node(selector) {
    if (!nodes.has(selector)) {
      nodes.set(selector, {
        textContent: '', value: '', innerHTML: '', attributes: {}, listeners: {},
        classList: {add() {}, remove() {}, toggle() {}},
        addEventListener(name, handler) { this.listeners[name] = handler; },
        setAttribute(name, value) { this.attributes[name] = value; },
        replaceChildren() { this.innerHTML = ''; },
      });
    }
    return nodes.get(selector);
  }
  const context = vm.createContext({
    $: node, AbortController, location: {origin: 'https://capture.example'},
    adminOpen: true, adminCurrentView: 'dashboard', adminDetailLoadId: 0,
    session: {user: {role: 'admin'}}, sessionEpoch: 1, adminMode: false,
    document: {querySelectorAll() { return []; }},
    show(selector, visible) { node(selector).visible = visible; },
    skeletonRows: () => '<div>加载占位</div>',
    icon: () => '<svg></svg>',
    loadAdminStats() {},
    api(url, options) {
      return new Promise((resolve, reject) => requests.push({url, options, resolve, reject}));
    },
    writeClipboardText(text) { copied.push(text); },
    toast(message, type) { notices.push({message, type}); },
  });
  vm.runInContext(escapeCode + '\n' + catalog + '\n' + navigation, context);
  context.bindAdminApiCatalog();
  const search = node('#api-catalog-search'), list = node('#api-catalog-list');
  return {
    context, node, requests, copied, notices, search, list,
    status: node('#api-catalog-status'), retry: node('#api-catalog-retry'),
    open() { context.adminShowView('api-catalog'); },
    type(query) { search.value = query; search.listeners.input(); },
    copy(index, kind) {
      const button = {dataset: {apiCatalogCopy: String(index), apiCatalogCopyKind: kind}};
      return list.listeners.click({target: {closest() { return button; }}});
    },
  };
}

test('从后台入口加载，按组展示方法、路径、说明和鉴权', async () => {
  const h = harness();
  h.open();
  assert.equal(h.node('#admin-view-api-catalog').visible, true);
  assert.equal(h.node('#admin-view-dashboard').visible, false);
  assert.equal(h.node('#api-catalog-base').textContent, 'https://capture.example');
  assert.equal(h.search.disabled, true);
  assert.equal(h.list.attributes['aria-busy'], 'true');
  assert.equal(h.requests[0].url, '/api/v1/admin/api-catalog');
  assert.equal(h.requests[0].options.cache, 'no-store');
  h.requests[0].resolve({operations});
  await afterRequest();
  assert.equal(h.search.disabled, false);
  assert.equal(h.list.attributes['aria-busy'], 'false');
  assert.equal(h.status.textContent, '共 3 个接口');
  assert.match(h.list.innerHTML, /<h3>健康检查<\/h3>/);
  assert.match(h.list.innerHTML, /<h3>任务管理<\/h3>/);
  assert.match(h.list.innerHTML, /<details[\s\S]*method-post">POST[\s\S]*retry[\s\S]*重试任意任务[\s\S]*管理员 Bearer 访问令牌/);
});

test('显示并复制 PowerShell curl 命令，凭据和任务参数使用占位符', async () => {
  const h = harness();
  const endpoint = '/api/v1/jobs/{id}';
  h.context.session.accessToken = 'CURRENT_SESSION_TOKEN';
  h.open();
  h.requests[0].resolve({operations: [
    {method: 'POST', path: '/api/v1/jobs', group: '用户任务', summary: '创建任务', auth: 'bearer', curlExample: 'create-job'},
    {method: 'GET', path: endpoint, group: '用户任务', summary: '查询结果', auth: 'bearer', curlExample: 'request'},
    {method: 'GET', path: endpoint+'/images/archive', group: '用户任务', summary: '下载 ZIP', auth: 'bearer', curlExample: 'download-zip'},
    {method: 'POST', path: '/api/v1/admin/jobs/{id}/images/archive-download', group: '任务管理', summary: '签发 ZIP 链接', auth: 'admin', curlExample: 'request'},
    {method: 'GET', path: '/api/v1/archive-download/{id}', group: '签名媒体', summary: '下载签名 ZIP', auth: 'signed', curlExample: 'signed-zip'},
    {...operations[0], curlExample: 'unknown'},
  ]});
  await afterRequest();
  assert.match(h.list.innerHTML, /curl 示例（PowerShell）/);
  assert.match(h.list.innerHTML, /<pre><code>[\s\S]*&quot;shareText&quot;: &quot;DOUYIN_URL&quot;/);
  assert.match(h.list.innerHTML, /--url &#39;DOWNLOAD_URL&#39;/);
  assert.doesNotMatch(h.list.innerHTML, /CURRENT_SESSION_TOKEN/);
  assert.equal((h.list.innerHTML.match(/data-api-catalog-copy-kind="curl"/g) || []).length, 5);

  for (const index of [0, 1, 2, 3, 4]) await h.copy(index, 'curl');
  const [create, result, directZip, signZip, signedZip] = h.copied;
  assert.match(create, /'@ \| curl\.exe --request POST/);
  assert.match(create, /--url 'https:\/\/capture\.example\/api\/v1\/jobs'/);
  assert.match(create, /--header 'Authorization: Bearer ACCESS_TOKEN'/);
  assert.match(create, /--header 'Idempotency-Key: REQUEST_KEY'/);
  assert.match(create, /--data-binary '@-'/);
  assert.deepEqual(JSON.parse(create.split("\n'@")[0].slice(3)), {shareText: 'DOUYIN_URL', action: 'download'});
  assert.match(result, /\/jobs\/JOB_ID'/);
  assert.match(directZip, /\/jobs\/JOB_ID\/images\/archive'/);
  assert.match(signZip, /--header 'Authorization: Bearer ADMIN_ACCESS_TOKEN'/);
  assert.match(signedZip, /--url 'DOWNLOAD_URL'/);
  assert.doesNotMatch(signedZip, /--header/);
  for (const command of [directZip, signedZip]) assert.match(command, /--fail --output 'images\.zip'/);
  assert.equal(h.notices.at(-1).message, '已复制命令');
  assert.equal(h.requests.length, 1);
});

test('关键词匹配描述与路径，搜索后仍复制对应的完整接口模板', async () => {
  const h = harness();
  h.open();
  h.requests[0].resolve({operations});
  await afterRequest();

  h.type('pOsT  已取消');
  assert.equal(h.status.textContent, '匹配到 1 个接口');
  assert.doesNotMatch(h.list.innerHTML, /健康检查|查看任务详情/);
  assert.match(h.list.innerHTML, /data-api-catalog-copy="1"/);
  await h.copy(1);
  assert.deepEqual(h.copied, ['https://capture.example/api/v1/admin/jobs/{id}/retry']);
  assert.deepEqual(h.notices, [{message: '已复制接口地址', type: 'success'}]);
  assert.equal(h.requests.length, 1);

  h.type('未收录');
  assert.equal(h.status.textContent, '没有匹配的接口。');
  assert.equal(h.list.innerHTML, '');
});

test('读取失败可重试，离开页面后忽略旧响应', async () => {
  const h = harness();
  h.open();
  h.requests[0].reject(new Error('offline'));
  await afterRequest();
  assert.match(h.status.textContent, /加载失败/);
  assert.equal(h.retry.visible, true);
  const retry = h.retry.listeners.click();
  assert.equal(h.retry.visible, false);
  assert.equal(h.requests.length, 2);
  h.context.adminShowView('dashboard');
  assert.equal(h.requests[1].options.signal.aborted, true);

  h.open();
  h.requests[1].resolve({operations});
  await retry;
  assert.equal(h.list.attributes['aria-busy'], 'true');
  assert.doesNotMatch(h.list.innerHTML, /任务管理/);
  h.requests[2].resolve({operations: []});
  await afterRequest();
  assert.equal(h.status.textContent, '暂无接口。');
});

test('非管理会话无法读取，模板数据作为文本渲染', async () => {
  const h = harness();
  h.context.session.user.role = 'user';
  h.open();
  assert.equal(h.requests.length, 0);
  h.context.session.user.role = 'admin';
  h.open();
  h.requests[0].resolve({operations: [
    {...operations[0], group: '<img>', summary: '<strong>', details: '<script>'},
    {...operations[1], path: '//other.example/path'},
  ]});
  await afterRequest();
  assert.match(h.list.innerHTML, /&lt;img&gt;[\s\S]*&lt;strong&gt;[\s\S]*&lt;script&gt;/);
  assert.doesNotMatch(h.list.innerHTML, /<img>|<script>|other\.example/);
});
