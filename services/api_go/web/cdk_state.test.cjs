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
  const firstCopy = {focus() { this.focused = true; }};
  let clipboardField;
  const row = {dataset: {cdkId: card.id}};
  const buttons = Object.fromEntries(['quota', 'reset', 'rotate', 'revoke'].map(action => [action, {
    dataset: {cdkAction: action},
    isConnected: true,
    closest(selector) { return selector === '[data-cdk-id]' ? row : this; },
    focus() {},
  }]));
  function node(selector) {
    if (!elements.has(selector)) elements.set(selector, {
      value: '', innerHTML: '', listeners: {},
      addEventListener(name, listener) { this.listeners[name] = listener; },
      setAttribute() {}, replaceChildren() { this.innerHTML = ''; }, reset() {}, focus() { this.focused = true; },
      querySelector() { return firstCopy; },
      querySelectorAll() { return Object.values(buttons); },
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
    confirmed: true,
    async api(url, options) {
      requests.push({url, method: options.method || 'GET'});
      if (options.method === 'POST') return context.mutation;
      return {items: [card], total: 1};
    },
    mutation: {item: {...card, taskCount: 0}},
  });
  vm.runInContext(clipboardCode + source, context);
  context.bindAdminCDKs();
  const list = node('#admin-cdk-list');
  const codes = node('#admin-cdk-codes');
  return {
    context, requests, confirmations, copied, notices, buttons, codes, firstCopy,
    click(action) { return list.listeners.click({target: buttons[action]}); },
    generate() { return node('#admin-cdk-form').listeners.submit({preventDefault() {}}); },
    copy(index) {
      const button = {dataset: {copyCdk: String(index)}};
      return codes.listeners.click({target: {closest() { return button; }}});
    },
    copyAll() { return node('#admin-cdk-copy').listeners.click(); },
    copyAllButton: node('#admin-cdk-copy'),
    get clipboardField() { return clipboardField; },
    dismiss() { node('#admin-cdk-dismiss').listeners.click(); },
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
