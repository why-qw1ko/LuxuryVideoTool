const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, 'static/app-releases.js'), 'utf8');
const release = {id: 'release-1', versionCode: 2, versionName: '1.1', packageName: 'com.buleng.luxuryvideo',
  sha256: 'a'.repeat(64), size: 1024 * 1024, changelog: '修复播放问题', forceUpdate: false,
  downloadUrl: '/api/v1/app/android/releases/release-1/apk', publishedAt: '2026-11-27T08:00:00Z'};
const draft = {...release, id: 'draft-2', versionCode: 3, versionName: '1.2', publishedAt: ''};
const tick = () => new Promise(resolve => setImmediate(resolve));

function harness(data = {draft, release}) {
  const nodes = new Map(), requests = [], confirmations = [], uploads = [];
  function node(selector) {
    if (!nodes.has(selector)) nodes.set(selector, {
      value: '', textContent: '', innerHTML: '', checked: false, disabled: false, files: [], attributes: {}, listeners: {},
      classList: {toggle() {}},
      setAttribute(name, value) { this.attributes[name] = value; },
      removeAttribute(name) { delete this.attributes[name]; },
      addEventListener(name, callback) { this.listeners[name] = callback; },
    });
    return nodes.get(selector);
  }
  class TestXHR {
    constructor() { this.upload = {}; this.headers = {}; uploads.push(this); }
    open(method, url) { this.method = method; this.url = url; }
    setRequestHeader(name, value) { this.headers[name] = value; }
    send(body) { this.body = body; }
    abort() { this.aborted = true; this.onabort?.(); }
    respond(status, data) { this.status = status; this.responseText = JSON.stringify(data); this.onload(); }
  }
  class TestFormData {
    constructor() { this.values = []; }
    append(name, value) { this.values.push([name, value]); }
  }
  const context = vm.createContext({
    $: node, adminOpen: true, adminCurrentView: 'releases', sessionEpoch: 1,
    session: {accessToken: 'token-1', user: {id: 'admin-1', role: 'admin'}},
    AbortController, setTimeout, clearTimeout, XMLHttpRequest: TestXHR, FormData: TestFormData,
    esc: value => String(value ?? '').replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('"', '&quot;'),
    fmtDate: String, show(selector, visible) { node(selector).visible = visible; },
    confirmDialog(options) { confirmations.push(options); return context.confirmed; },
    confirmed: true,
    async refreshSession() { context.session.accessToken = 'token-2'; return true; },
    api(url, options) { return new Promise((resolve, reject) => requests.push({url, options, resolve, reject})); },
  });
  vm.runInContext(source, context);
  context.bindAdminReleases();
  context.renderAdminReleases(data);
  return {
    context, node, requests, confirmations, uploads,
    edit(notes, force = false) {
      node('#admin-release-changelog').value = notes;
      node('#admin-release-force').checked = force;
      node('#admin-release-changelog').listeners.input();
    },
    selectFile(file) { node('#admin-release-apk').files = [file]; node('#admin-release-apk').listeners.change(); },
  };
}

test('后台发版入口与独立资源连接，版本身份只读且文案安全转义', () => {
  const html = fs.readFileSync(path.join(__dirname, 'static/index.html'), 'utf8');
  const app = fs.readFileSync(path.join(__dirname, 'static/app.js'), 'utf8');
  assert.match(html, /data-admin-view="releases"/);
  assert.match(html, /src="\/app-releases\.js" defer/);
  assert.match(html, /href="\/css\/app-releases\.css"/);
  assert.match(app, /name==='releases'.*loadAdminReleases/);
  assert.doesNotMatch(html, /<input[^>]+name="version(?:Code|Name)"/);
  const h = harness({draft: {...draft, versionName: '<img src=x>'}, release});
  assert.match(h.node('#admin-release-draft').innerHTML, /&lt;img src=x>/);
  assert.equal(h.node('#admin-release-download').attributes.href, release.downloadUrl);
  h.context.renderAdminReleases({draft, release: {...release, downloadUrl: 'https://other.example/app.apk'}});
  assert.equal(h.node('#admin-release-download').attributes.href, undefined);
});

test('修改草稿后必须先保存，保存携带所编辑的包身份且请求去重', async () => {
  const h = harness();
  h.edit('新的更新说明', true);
  assert.equal(h.node('#admin-release-publish').disabled, true);
  await h.context.publishAdminRelease();
  assert.equal(h.requests.length, 0);
  const saving = h.context.saveAdminReleaseDraft();
  await h.context.saveAdminReleaseDraft();
  assert.equal(h.requests.length, 1);
  assert.equal(h.requests[0].url, '/api/v1/admin/app/android/release');
  assert.deepEqual(JSON.parse(h.requests[0].options.body), {
    draftId: draft.id, versionCode: 3, versionName: '1.2', changelog: '新的更新说明', forceUpdate: true,
  });
  h.requests[0].resolve({draft: {...draft, changelog: '新的更新说明', forceUpdate: true}, release});
  await saving;
  assert.equal(h.node('#admin-release-publish').disabled, false);
  assert.equal(h.node('#admin-release-draft-fields').disabled, false);
});

test('发布确认只发送审核过的草稿ID，重复点击不会重复发布', async () => {
  const h = harness({draft: {...draft, forceUpdate: true}, release});
  const publishing = h.context.publishAdminRelease();
  await tick();
  await h.context.publishAdminRelease();
  assert.equal(h.confirmations.length, 1);
  assert.equal(h.confirmations[0].danger, true);
  assert.equal(h.requests.length, 1);
  assert.equal(h.requests[0].url, '/api/v1/admin/app/android/publish');
  assert.deepEqual(JSON.parse(h.requests[0].options.body), {draftId: draft.id});
  h.requests[0].resolve({draft, release: {...draft, publishedAt: '2026-11-27T09:00:00Z'}});
  await publishing;
  assert.match(h.node('#admin-release-status').textContent, /已发布/);
});

test('确认期间离开页面或切换账号，不会继续发布或重用新账号权限', async () => {
  for (const change of ['page', 'session']) {
    const h = harness();
    let confirm;
    h.context.confirmDialog = () => new Promise(resolve => { confirm = resolve; });
    const publishing = h.context.publishAdminRelease();
    if (change === 'page') h.context.adminCurrentView = 'jobs';
    else { h.context.sessionEpoch++; h.context.session = {user: {id: 'admin-2', role: 'admin'}}; }
    h.context.resetAdminReleases();
    confirm(true);
    await publishing;
    assert.equal(h.requests.length, 0);
  }
});

test('旧页面响应取消后不能覆盖新页面或已退出账号的状态', async () => {
  const h = harness();
  const loading = h.context.loadAdminReleases();
  h.context.resetAdminReleases();
  h.context.sessionEpoch++;
  assert.equal(h.requests[0].options.signal.aborted, true);
  h.requests[0].resolve({draft, release});
  await loading;
  assert.match(h.node('#admin-release-published').innerHTML, /尚未发布/);
  assert.equal(h.node('#admin-release-policy-fields').disabled, true);
});

test('当前强制策略独立保存，不丢同一草稿尚未保存的编辑', async () => {
  const h = harness();
  h.edit('未保存说明', true);
  h.node('#admin-release-policy').checked = true;
  const saving = h.context.saveAdminReleasePolicy();
  await tick();
  assert.equal(h.confirmations.length, 1);
  assert.deepEqual(JSON.parse(h.requests[0].options.body), {releaseId: release.id, forceUpdate: true});
  h.requests[0].resolve({draft, release: {...release, forceUpdate: true}});
  await saving;
  assert.equal(h.node('#admin-release-changelog').value, '未保存说明');
  assert.equal(h.node('#admin-release-force').checked, true);
  assert.equal(h.node('#admin-release-publish').disabled, true);
  assert.equal(h.node('#admin-release-policy').checked, true);
  assert.equal(h.node('#admin-release-policy-label').textContent, '当前已开启强制更新');
  assert.equal(h.node('#admin-release-policy-save').disabled, true);
});

test('线上强制开关显示待保存状态，改回原值不提交也不允许重复保存', () => {
  const h = harness();
  assert.equal(h.node('#admin-release-policy-label').textContent, '当前未开启强制更新');
  assert.equal(h.node('#admin-release-policy-save').disabled, true);
  h.node('#admin-release-policy').checked = true;
  h.node('#admin-release-policy').listeners.change();
  assert.equal(h.node('#admin-release-policy-label').textContent, '开启强制更新（待保存）');
  assert.equal(h.node('#admin-release-policy-save').disabled, false);
  assert.equal(h.requests.length, 0);
  h.node('#admin-release-policy').checked = false;
  h.node('#admin-release-policy').listeners.change();
  assert.equal(h.node('#admin-release-policy-label').textContent, '当前未开启强制更新');
  assert.equal(h.node('#admin-release-policy-save').disabled, true);
  h.context.renderAdminReleases({draft, release: {...release, forceUpdate: true}});
  h.node('#admin-release-policy').checked = false;
  h.node('#admin-release-policy').listeners.change();
  assert.equal(h.node('#admin-release-policy-label').textContent, '关闭强制更新（待保存）');
  assert.equal(h.node('#admin-release-policy-save').disabled, false);
});

test('开关保存失败恢复最后确认的线上状态并保留草稿编辑', async () => {
  for (const originalForce of [false, true]) {
    const h = harness({draft, release: {...release, forceUpdate: originalForce}});
    h.edit('保留未保存说明', true);
    h.node('#admin-release-policy').checked = !originalForce;
    h.node('#admin-release-policy').listeners.change();
    const saving = h.context.saveAdminReleasePolicy();
    await tick();
    assert.equal(h.node('#admin-release-policy-save').disabled, true);
    h.requests[0].reject(new Error('策略保存失败，请重新加载'));
    await saving;
    assert.equal(h.node('#admin-release-policy').checked, originalForce);
    assert.equal(h.node('#admin-release-policy-label').textContent, originalForce ? '当前已开启强制更新' : '当前未开启强制更新');
    assert.equal(h.node('#admin-release-policy-save').disabled, true);
    assert.equal(h.node('#admin-release-changelog').value, '保留未保存说明');
    assert.equal(h.node('#admin-release-force').checked, true);
    assert.match(h.node('#admin-release-status').textContent, /策略保存失败/);
  }
});

test('旧页面策略请求失败不能回滚新页面的开关', async () => {
  const h = harness();
  h.node('#admin-release-policy').checked = true;
  const saving = h.context.saveAdminReleasePolicy();
  await tick();
  h.context.resetAdminReleases();
  h.context.sessionEpoch++;
  h.context.renderAdminReleases({draft, release: {...release, id: 'new-release', forceUpdate: true}});
  h.requests[0].reject(new Error('旧页面请求取消'));
  await saving;
  assert.equal(h.node('#admin-release-policy').checked, true);
  assert.equal(h.node('#admin-release-policy-label').textContent, '当前已开启强制更新');
  assert.equal(h.node('#admin-release-status').textContent, '');
});

test('另一管理员替换草稿时不会把旧包的未保存说明套到新包', async () => {
  const h = harness();
  h.edit('旧包未保存说明', true);
  h.node('#admin-release-policy').checked = true;
  const saving = h.context.saveAdminReleasePolicy();
  await tick();
  h.requests[0].resolve({draft: {...draft, id: 'other-draft', changelog: '新包说明'}, release: {...release, forceUpdate: true}});
  await saving;
  assert.equal(h.node('#admin-release-changelog').value, '新包说明');
  assert.equal(h.node('#admin-release-force').checked, false);
  assert.equal(h.node('#admin-release-publish').disabled, false);
});

test('关闭强制更新无需重复确认，取消开启不会调用政策接口', async () => {
  const h = harness({draft, release: {...release, forceUpdate: true}});
  h.node('#admin-release-policy').checked = false;
  const disabling = h.context.saveAdminReleasePolicy();
  assert.equal(h.confirmations.length, 0);
  h.requests[0].resolve({draft, release});
  await disabling;
  assert.equal(h.node('#admin-release-policy').checked, false);
  assert.equal(h.node('#admin-release-policy-label').textContent, '当前未开启强制更新');
  h.context.confirmed = false;
  h.node('#admin-release-policy').checked = true;
  await h.context.saveAdminReleasePolicy();
  assert.equal(h.requests.length, 1);
  assert.equal(h.node('#admin-release-policy').checked, false);
  assert.equal(h.node('#admin-release-policy-label').textContent, '当前未开启强制更新');
  assert.equal(h.node('#admin-release-policy-save').disabled, true);
});

test('APK 使用 multipart 上传及真实进度，不添加 JSON Content-Type', async () => {
  const h = harness({draft: null, release});
  const file = {name: 'app-release.apk', size: 1024};
  h.selectFile(file);
  const upload = h.context.uploadAdminRelease();
  assert.equal(h.uploads.length, 1);
  const xhr = h.uploads[0];
  assert.equal(xhr.url, '/api/v1/admin/app/android/apk');
  assert.deepEqual(xhr.headers, {Authorization: 'Bearer token-1'});
  assert.deepEqual(xhr.body.values, [['apk', file]]);
  xhr.upload.onprogress({lengthComputable: true, loaded: 512, total: 1024});
  assert.equal(h.node('#admin-release-progress').value, 50);
  xhr.respond(200, {draft, release});
  await upload;
  assert.equal(h.node('#admin-release-progress').value, 100);
  assert.match(h.node('#admin-release-status').textContent, /生成草稿/);
});

test('上传校验拒绝超限或非 APK，现有草稿被替换前可以取消', async () => {
  const h = harness();
  for (const file of [{name: 'app.zip', size: 10}, {name: 'big.apk', size: 256 * 1024 * 1024 + 1}]) {
    h.selectFile(file);
    await h.context.uploadAdminRelease();
  }
  assert.equal(h.uploads.length, 0);
  h.context.confirmed = false;
  h.selectFile({name: 'app.apk', size: 1024});
  await h.context.uploadAdminRelease();
  assert.equal(h.confirmations.length, 1);
  assert.equal(h.uploads.length, 0);
});

test('上传遇到401仅同会话续期重试，账号变化后不重传旧文件', async () => {
  for (const changed of [false, true]) {
    const h = harness({draft: null, release});
    h.selectFile({name: 'app.apk', size: 1024});
    h.context.refreshSession = async () => {
      if (changed) { h.context.sessionEpoch++; h.context.resetAdminReleases(); }
      else h.context.session.accessToken = 'token-2';
      return true;
    };
    const uploading = h.context.uploadAdminRelease();
    h.uploads[0].respond(401, {error: {code: 'AUTH_TOKEN_EXPIRED'}});
    await tick();
    if (!changed) {
      assert.equal(h.uploads.length, 2);
      assert.equal(h.uploads[1].headers.Authorization, 'Bearer token-2');
      h.uploads[1].respond(200, {draft, release});
    } else assert.equal(h.uploads.length, 1);
    await uploading;
  }
});

test('上传中离开取消请求，失败或草稿冲突保留编辑供重试', async () => {
  const h = harness({draft: null, release});
  h.selectFile({name: 'app.apk', size: 1024});
  const uploading = h.context.uploadAdminRelease();
  h.context.resetAdminReleases();
  await uploading;
  assert.equal(h.uploads[0].aborted, true);
  h.context.renderAdminReleases({draft, release});
  h.edit('需要保留的编辑');
  const saving = h.context.saveAdminReleaseDraft();
  h.requests[0].reject(new Error('草稿已被替换，请重新加载'));
  await saving;
  assert.equal(h.node('#admin-release-changelog').value, '需要保留的编辑');
  assert.equal(h.node('#admin-release-publish').disabled, true);
  assert.match(h.node('#admin-release-status').textContent, /重新加载/);
});
