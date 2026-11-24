const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, 'static/cdks.js'), 'utf8');
const card = {id: 'card/id', prefix: 'CDK-SAMPLE', taskCount: 3, taskLimit: 8, status: 'redeemed'};

function harness() {
  const elements = new Map(), requests = [], confirmations = [], copied = [];
  const row = {dataset: {cdkId: card.id}};
  const buttons = Object.fromEntries(['quota', 'reset', 'rotate', 'revoke'].map(action => [action, {
    dataset: {cdkAction: action},
    isConnected: true,
    closest(selector) { return selector === '[data-cdk-id]' ? row : this; },
    focus() {},
  }]));
  function node(selector) {
    if (!elements.has(selector)) elements.set(selector, {
      value: '', listeners: {},
      addEventListener(name, listener) { this.listeners[name] = listener; },
      setAttribute() {}, replaceChildren() {}, reset() {}, focus() { this.focused = true; },
      querySelectorAll() { return Object.values(buttons); },
    });
    return elements.get(selector);
  }
  const context = vm.createContext({
    session: {user: {role: 'admin'}}, sessionEpoch: 1, adminOpen: true, adminCurrentView: 'cdks',
    $: node, URLSearchParams, esc: String, fmtDate: String,
    show(selector, visible) { node(selector).visible = visible; },
    renderPager() {}, toast() {},
    navigator: {clipboard: {async writeText(value) { copied.push(value); }}},
    confirmDialog(options) { confirmations.push(options); return context.confirmed; },
    confirmed: true,
    async api(url, options) {
      requests.push({url, method: options.method || 'GET'});
      if (options.method === 'POST') return context.mutation;
      return {items: [card], total: 1};
    },
    mutation: {item: {...card, taskCount: 0}},
  });
  vm.runInContext(source, context);
  context.bindAdminCDKs();
  const list = node('#admin-cdk-list');
  const codes = node('#admin-cdk-codes');
  return {
    context, requests, confirmations, copied, buttons, codes,
    click(action) { return list.listeners.click({target: buttons[action]}); },
    copy() { return node('#admin-cdk-copy').listeners.click(); },
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
  assert.equal(h.codes.value, '');
});

test('轮换返回的密钥可复制保存，关闭前不会被下一次轮换覆盖', async () => {
  const h = harness();
  h.context.mutation = {item: {...card, code: 'CDK-NEW'}};
  await h.context.loadAdminCDKs();
  await h.click('rotate');
  assert.deepEqual(h.requests[1], {url: '/api/v1/admin/cdks/card%2Fid/rotate', method: 'POST'});
  assert.equal(h.keyPanel.visible, true);
  assert.equal(h.codes.value, 'CDK-NEW');
  assert.equal(h.codes.focused, true);
  assert.equal(h.buttons.rotate.disabled, true);
  await h.copy();
  assert.deepEqual(h.copied, ['CDK-NEW']);

  await h.click('rotate');
  assert.equal(h.requests.length, 3);
  h.dismiss();
  assert.equal(h.keyPanel.visible, false);
  assert.equal(h.codes.value, '');
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
  assert.equal(h.codes.value, '');
});
