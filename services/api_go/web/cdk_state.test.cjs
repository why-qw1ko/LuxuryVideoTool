const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, 'static/cdks.js'), 'utf8');
const appSource = fs.readFileSync(path.join(__dirname, 'static/app.js'), 'utf8');
const clipboardStart = appSource.indexOf('async function writeClipboardText(');
const clipboardEnd = appSource.indexOf('async function copyText(', clipboardStart);
assert.ok(clipboardStart >= 0 && clipboardEnd > clipboardStart);
const clipboardCode = appSource.slice(clipboardStart, clipboardEnd);
const card = {id: 'card/id', prefix: 'CDK-SAMPLE', taskCount: 3, taskLimit: 8, status: 'redeemed'};

function harness() {
  const elements = new Map(), requests = [], confirmations = [], copied = [], notices = [];
  const selectionInputs = new Map(), prompts = [];
  const firstCopy = {focus() { this.focused = true; }};
  let clipboardField;
  const row = {dataset: {cdkId: card.id}};
  const buttons = Object.fromEntries(['quota', 'reset', 'rotate', 'revoke', 'delete'].map(action => [action, {
    dataset: {cdkAction: action},
    isConnected: true,
    closest(selector) { return selector === '[data-cdk-id]' ? row : this; },
    focus() {},
  }]));
  const batchButtons = Object.fromEntries(Object.keys(buttons).map(action => [action, {
    dataset: {cdkBatchAction: action}, isConnected: true, closest() { return this; }, focus() {},
  }]));
  function node(selector) {
    if (!elements.has(selector)) elements.set(selector, {
      value: '', innerHTML: '', listeners: {},
      addEventListener(name, listener) { this.listeners[name] = listener; },
      setAttribute() {}, replaceChildren() { this.innerHTML = ''; }, reset() {}, focus() { this.focused = true; },
      insertAdjacentHTML(position, html) { assert.equal(position, 'beforeend'); this.innerHTML += html; },
      querySelector() { return firstCopy; },
      querySelectorAll(query) {
        if (query === '[data-cdk-action]') return Object.values(buttons);
        if (query === '[data-cdk-batch-action]') return Object.values(batchButtons);
        if (query === '[data-cdk-select]') return [...selectionInputs.values()];
        return [];
      },
    });
    return elements.get(selector);
  }
  const context = vm.createContext({
    session: {user: {role: 'admin'}}, sessionEpoch: 1, adminOpen: true, adminCurrentView: 'cdks',
    $: node, URLSearchParams, esc: String, fmtDate: String,
    show(selector, visible) { node(selector).visible = visible; },
    renderPager() {}, toast(message, type) { notices.push({message, type}); },
    navigator: {clipboard: {async writeText(value) { copied.push(value); }}},
    document: {
      activeElement: firstCopy,
      body: {appendChild(field) { clipboardField = field; }},
      createElement(tag) {
        assert.equal(tag, 'textarea');
        return {
          style: {},
          select() { assert.equal(this, clipboardField); this.selected = true; },
          remove() { clipboardField = undefined; },
        };
      },
      execCommand(command) {
        assert.equal(command, 'copy');
        assert.equal(clipboardField.selected, true);
        copied.push(clipboardField.value);
        return true;
      },
    },
    confirmDialog(options) { confirmations.push(options); return context.confirmed; },
    promptDialog(options) { prompts.push(options); return context.promptValue; },
    promptValue: null,
    confirmed: true,
    async api(url, options) {
      requests.push({url, method: options.method || 'GET'});
      if (options.method) return context.mutate(url, options);
      return context.readList(url);
    },
    readList: () => ({items: [card], total: 1}),
    mutate: () => context.mutation,
    mutation: {item: {...card, taskCount: 0}},
  });
  vm.runInContext(clipboardCode + source, context);
  context.bindAdminCDKs();
  const list = node('#admin-cdk-list');
  const codes = node('#admin-cdk-codes');
  const batch = node('#admin-cdk-batch');
  const selectAll = node('#admin-cdk-select-all');
  return {
    context, requests, confirmations, copied, notices, buttons, list, codes, firstCopy, prompts, batchButtons, selectAll,
    click(action) { return list.listeners.click({target: buttons[action]}); },
    clickBatch(action) { return batch.listeners.click({target: batchButtons[action]}); },
    select(id, checked) {
      const input = {dataset: {cdkSelect: id}, checked};
      selectionInputs.set(id, input);
      list.listeners.change({target: input});
    },
    selectPage(checked) { selectAll.listeners.change({target: {checked}}); },
    selected: node('#admin-cdk-selected'),
    generate() { return node('#admin-cdk-form').listeners.submit({preventDefault() {}}); },
    copy(index) {
      const button = {dataset: {copyCdk: String(index)}};
      return codes.listeners.click({target: {closest() { return button; }}});
    },
    copyAll() { return node('#admin-cdk-copy').listeners.click(); },
    copyAllButton: node('#admin-cdk-copy'),
    get clipboardField() { return clipboardField; },
    dismiss() { node('#admin-cdk-dismiss').listeners.click(); },
    dismissButton: node('#admin-cdk-dismiss'),
    keyPanel: node('#admin-cdk-generated'),
  };
}

test('取消重置不发请求，确认后仅重置指定密钥的次数', async () => {
  const h = harness();
  await h.context.loadAdminCDKs();
  h.context.confirmed = false;
  await h.click('reset');
  assert.equal(h.confirmations[0].title, '重置已用次数');
  assert.equal(h.requests.length, 1);

  h.context.confirmed = true;
  await h.click('reset');
  assert.deepEqual(h.requests[1], {url: '/api/v1/admin/cdks/card%2Fid/reset', method: 'POST'});
  assert.equal(h.codes.innerHTML, '');
});

test('轮换返回的密钥可复制保存，关闭前不会被下一次轮换覆盖', async () => {
  const h = harness();
  h.context.mutation = {item: {...card, code: 'CDK-NEW'}};
  await h.context.loadAdminCDKs();
  await h.click('rotate');
  assert.deepEqual(h.requests[1], {url: '/api/v1/admin/cdks/card%2Fid/rotate', method: 'POST'});
  assert.equal(h.keyPanel.visible, true);
  assert.match(h.codes.innerHTML, /<code>CDK-NEW<\/code>/);
  assert.equal(h.firstCopy.focused, true);
  assert.equal(h.copyAllButton.visible, false);
  assert.equal(h.buttons.rotate.disabled, true);
  await h.copy(0);
  assert.deepEqual(h.copied, ['CDK-NEW']);

  await h.click('rotate');
  assert.equal(h.requests.length, 3);
  h.dismiss();
  assert.equal(h.keyPanel.visible, false);
  assert.equal(h.codes.innerHTML, '');
  assert.equal(h.buttons.rotate.disabled, false);
});

test('切换登录会话后丢弃旧会话的轮换密钥响应', async () => {
  const h = harness();
  let resolveMutation;
  h.context.mutation = new Promise(resolve => { resolveMutation = resolve; });
  await h.context.loadAdminCDKs();
  const pending = h.click('rotate');
  await Promise.resolve();
  assert.equal(h.requests.length, 2);
  h.context.sessionEpoch++;
  h.context.resetCDKView();
  resolveMutation({item: {...card, code: 'CDK-STALE'}});
  await pending;
  assert.equal(h.keyPanel.visible, false);
  assert.equal(h.codes.innerHTML, '');
});

test('生成的密钥支持逐个复制和批量复制', async () => {
  const h = harness();
  h.context.mutation = {items: [{code: 'CDK-FIRST'}, {code: 'CDK-SECOND'}]};
  await h.generate();
  assert.equal(h.copyAllButton.visible, true);
  assert.equal(h.copyAllButton.focused, true);
  assert.match(h.codes.innerHTML, /<code>CDK-SECOND<\/code>\s+<button[^>]*data-copy-cdk="1"/);
  await h.copy(1);
  await h.copyAll();
  assert.deepEqual(h.copied, ['CDK-SECOND', 'CDK-FIRST\nCDK-SECOND']);
  h.dismiss();
  await h.copyAll();
  assert.equal(h.copied.length, 2);
});

test('Clipboard API 不可用时仍通过复制按钮写入剪贴板，失败时显示错误', async () => {
  const h = harness();
  h.context.navigator.clipboard = undefined;
  h.context.mutation = {items: [{code: 'CDK-HTTP'}]};
  await h.generate();
  await h.copy(0);
  assert.deepEqual(h.copied, ['CDK-HTTP']);
  assert.equal(h.clipboardField, undefined);
  assert.deepEqual(h.notices.at(-1), {message: '已复制密钥', type: 'success'});
  h.context.document.execCommand = () => false;
  await h.copy(0);
  assert.equal(h.clipboardField, undefined);
  assert.equal(h.notices.at(-1).type, 'error');
});

test('仅已吊销记录可确认删除，删除末页最后一项后回到上一页', async () => {
  const h = harness();
  await h.context.loadAdminCDKs();
  assert.doesNotMatch(h.list.innerHTML, /data-cdk-action="delete"/);
  await h.click('delete');
  assert.equal(h.requests.length, 1);

  h.context.readList = () => ({items: [{...card, status: 'revoked'}], total: 26});
  await h.context.loadAdminCDKs(2);
  assert.match(h.list.innerHTML, /data-cdk-action="delete"/);
  assert.doesNotMatch(h.list.innerHTML, /data-cdk-action="revoke"/);
  h.context.confirmed = false;
  await h.click('delete');
  assert.equal(h.confirmations[0].title, '删除密钥记录');
  assert.equal(h.requests.length, 2);

  h.context.confirmed = true;
  h.context.mutation = {};
  h.context.readList = url => ({items: url.endsWith('offset=0') ? [card] : [], total: 25});
  await h.click('delete');
  await new Promise(setImmediate);
  assert.deepEqual(h.requests.slice(2), [
    {url: '/api/v1/admin/cdks/card%2Fid/record', method: 'DELETE'},
    {url: '/api/v1/admin/cdks?limit=25&offset=25', method: 'GET'},
    {url: '/api/v1/admin/cdks?limit=25&offset=0', method: 'GET'},
  ]);
});

test('批量修改额度只更新本页选中的未吊销密钥，分页后清空选择', async () => {
  const h = harness();
  const revoked = {...card, id: 'revoked', status: 'revoked'};
  h.context.readList = url => ({items: url.endsWith('offset=0') ? [card, revoked] : [revoked], total: 26});
  const updated = [];
  h.context.mutate = (url, options) => { updated.push({url, body: JSON.parse(options.body)}); return {}; };
  await h.context.loadAdminCDKs();
  assert.match(h.list.innerHTML, /data-cdk-select="card\/id"/);
  h.select(card.id, true);
  assert.equal(h.selectAll.indeterminate, true);
  assert.equal(h.batchButtons.delete.disabled, true);
  h.selectPage(true);
  assert.equal(h.selectAll.checked, true);
  assert.equal(h.batchButtons.delete.disabled, false);
  assert.equal(h.selected.textContent, '已选 2 个');

  await h.clickBatch('quota');
  assert.equal(updated.length, 0);
  assert.match(h.prompts[0].message, /1 个密钥.*另有 1 个所选密钥.*跳过/);
  h.context.promptValue = '15';
  await h.clickBatch('quota');
  await new Promise(setImmediate);
  assert.deepEqual(updated, [{url: '/api/v1/admin/cdks/card%2Fid', body: {taskLimit: 15}}]);
  assert.equal(h.selected.textContent, '已选 1 个');
  assert.equal(h.batchButtons.quota.disabled, true);

  await h.context.loadAdminCDKs(2);
  assert.equal(h.selected.textContent, '已选 0 个');
  assert.equal(h.batchButtons.delete.disabled, true);
});

test('批量轮换部分失败后保留所有成功生成的密钥，并保留失败项的选择', async () => {
  const h = harness();
  const failed = {...card, id: 'no-update', prefix: 'CDK-FAILED'};
  const other = {...card, id: 'other', prefix: 'CDK-OTHER'};
  h.context.readList = () => ({items: [card, failed, other], total: 3});
  h.context.mutate = url => {
    if (url.includes(failed.id)) throw new Error('无法轮换');
    return {item: {code: url.includes(other.id) ? 'CDK-SECOND' : 'CDK-FIRST'}};
  };
  await h.context.loadAdminCDKs();
  h.selectPage(true);
  await h.clickBatch('rotate');
  await new Promise(setImmediate);
  assert.equal(h.confirmations.length, 1);
  assert.match(h.notices.at(-1).message, /成功 2 个，失败 1 个.*CDK-FAILED.*无法轮换/);
  assert.equal(h.notices.at(-1).type, 'warning');
  assert.equal(h.selected.textContent, '已选 1 个');
  assert.equal(h.batchButtons.rotate.disabled, true);
  assert.equal(h.buttons.rotate.disabled, true);
  assert.match(h.codes.innerHTML, /原密钥 CDK-SAMPLE…<\/div><code>CDK-FIRST<\/code>/);
  assert.match(h.codes.innerHTML, /原密钥 CDK-OTHER…<\/div><code>CDK-SECOND<\/code>/);
  await h.copyAll();
  assert.deepEqual(h.copied, ['CDK-FIRST\nCDK-SECOND']);
  h.dismiss();
  assert.equal(h.batchButtons.rotate.disabled, false);
});

test('批量轮换时逐个显示新密钥，切换管理页面后仍可复制', async () => {
  const h = harness();
  const other = {...card, id: 'other', prefix: 'CDK-OTHER'};
  h.context.readList = () => ({items: [card, other], total: 2});
  let finishSecond, startSecond;
  const secondStarted = new Promise(resolve => { startSecond = resolve; });
  const secondResponse = new Promise(resolve => { finishSecond = resolve; });
  h.context.mutate = url => {
    if (!url.includes(other.id)) return {item: {code: 'CDK-FIRST'}};
    startSecond();
    return secondResponse;
  };
  await h.context.loadAdminCDKs();
  h.selectPage(true);
  const pending = h.clickBatch('rotate');
  await secondStarted;
  assert.equal(h.keyPanel.visible, true);
  assert.match(h.codes.innerHTML, /原密钥 CDK-SAMPLE…<\/div><code>CDK-FIRST<\/code>/);
  assert.equal(h.dismissButton.disabled, true);
  h.dismiss();
  assert.equal(h.keyPanel.visible, true);
  await h.copy(0);
  assert.deepEqual(h.copied, ['CDK-FIRST']);

  h.context.adminCurrentView = 'jobs';
  finishSecond({item: {code: 'CDK-SECOND'}});
  await pending;
  assert.equal(h.dismissButton.disabled, false);
  h.context.adminCurrentView = 'cdks';
  await h.context.loadAdminCDKs();
  await h.copyAll();
  assert.deepEqual(h.copied, ['CDK-FIRST', 'CDK-FIRST\nCDK-SECOND']);
});

test('批量操作期间切换登录会话，不再请求剩余密钥', async () => {
  const h = harness();
  const items = [card, {...card, id: 'other'}, {...card, id: 'remaining'}];
  h.context.readList = () => ({items, total: items.length});
  let resolveMutation;
  const request = new Promise(resolve => { resolveMutation = resolve; });
  h.context.mutate = url => url.endsWith('/other/rotate') ? request : {item: {code: 'CDK-FIRST'}};
  await h.context.loadAdminCDKs();
  h.selectPage(true);
  const pending = h.clickBatch('rotate');
  await new Promise(setImmediate);
  assert.equal(h.requests.length, 3);
  assert.match(h.codes.innerHTML, /CDK-FIRST/);
  h.context.sessionEpoch++;
  h.context.resetCDKView();
  resolveMutation({item: {code: 'CDK-STALE'}});
  await pending;
  assert.equal(h.requests.length, 3);
  assert.equal(h.keyPanel.visible, false);
  assert.equal(h.codes.innerHTML, '');
});
