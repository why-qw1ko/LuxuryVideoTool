const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

// 直接执行网页中的下载代码；使用可控时钟和网络，不依赖浏览器或公网。
const source = fs.readFileSync(path.join(__dirname, 'static/app.js'), 'utf8');
const start = source.indexOf('const ARCHIVE_RETRY_DELAY_MS=');
const end = source.indexOf('async function download(link){', start);
assert.ok(start >= 0 && end > start);
const archiveCode = source.slice(start, end);
const escapeCode = source.split(/\r?\n/).find(line => line.startsWith('function esc(value)'));

function harness() {
  let clock = 100000;
  let nextTimer = 0;
  let request = async () => ({downloadUrl: '/api/v1/archive-download/job-1?expires=999999&signature=test'});
  const timers = new Map(), calls = [], downloads = [], toasts = [], entered = [];
  const button = {disabled: false, attributes: {}, setAttribute(name, value) { this.attributes[name] = value; }};
  const status = {textContent: ''};
  const section = {
    dataset: {archiveDownload: 'job-1'},
    querySelector(selector) { return selector === '[data-op="archive"]' ? button : status; },
  };
  const context = vm.createContext({
    session: {user: {id: 'owner'}},
    location: {origin: 'http://localhost:7788'}, URL, AbortController,
    Date: class extends Date { static now() { return clock; } },
    icon: () => '<svg></svg>',
    toast: (message, type) => toasts.push({message, type}),
    enter(value) {
      vm.runInContext('archiveDownloads.clear()', context);
      context.session = value;
      entered.push(value);
    },
    api: (url, options) => { calls.push({url, options}); return request(url, options); },
    document: {
      querySelectorAll: () => [section],
      body: {appendChild(anchor) { anchor.attached = true; }},
      createElement(tag) {
        assert.equal(tag, 'a');
        return {
          click() {
            assert.equal(this.attached, true);
            downloads.push({href: this.href, name: this.download});
          },
          remove() { this.attached = false; },
        };
      },
    },
    setTimeout(callback, delay) { const id = ++nextTimer; timers.set(id, {at: clock + delay, callback}); return id; },
    clearTimeout(id) { timers.delete(id); },
    fetch() { throw new Error('打包下载不应由网页接收完整文件'); },
  });
  vm.runInContext(escapeCode + '\n' + archiveCode, context);
  return {
    context, button, status, calls, downloads, toasts, entered,
    setRequest(value) { request = value; },
    advance(ms) {
      clock += ms;
      for (const [id, timer] of [...timers]) {
        if (timer.at <= clock) { timers.delete(id); timer.callback(); }
      }
    },
    resetSession(value = null) {
      vm.runInContext('archiveDownloads.clear()', context);
      context.session = value;
    },
  };
}

test('点击立即显示准备状态，重复点击与重新渲染均不会重复请求', async () => {
  const h = harness();
  let resolve;
  h.setRequest(() => new Promise(done => { resolve = done; }));
  const downloading = h.context.downloadArchive('job-1', 'work_images.zip');
  assert.equal(h.button.disabled, true);
  assert.equal(h.button.attributes['aria-busy'], 'true');
  assert.match(h.status.textContent, /正在准备下载/);
  assert.match(h.context.archiveDownloadHTML('job-1'), / disabled/);
  await h.context.downloadArchive('job-1');
  assert.equal(h.calls.length, 1);
  assert.equal(h.calls[0].url, '/api/v1/jobs/job-1/images/archive-download');
  assert.equal(h.calls[0].options.method, 'POST');
  resolve({downloadUrl: '/api/v1/archive-download/job-1?expires=999999&signature=test'});
  await downloading;
  assert.equal(h.downloads.length, 1);
  assert.equal(h.downloads[0].name, 'work_images.zip');
  assert.equal(h.downloads[0].href, 'http://localhost:7788/api/v1/archive-download/job-1?expires=999999&signature=test');
  assert.match(h.status.textContent, /浏览器的下载列表/);
  assert.doesNotMatch(h.status.textContent, /已完成|100%/);
  assert.equal(h.button.attributes['aria-busy'], 'false');
  await h.context.downloadArchive('job-1');
  assert.equal(h.calls.length, 1);
  h.advance(5000);
  assert.equal(h.button.disabled, false);
  assert.match(h.button.innerHTML, /重新下载/);
  h.setRequest(async () => ({downloadUrl: '/api/v1/archive-download/job-1?signature=new'}));
  await h.context.downloadArchive('job-1');
  assert.equal(h.calls.length, 2);
});

test('准备接口失败显示原因并允许重试，不触发下载', async () => {
  const h = harness();
  h.setRequest(async () => { throw new Error('该任务没有可打包的配图'); });
  await h.context.downloadArchive('job-1');
  assert.equal(h.downloads.length, 0);
  assert.equal(h.button.disabled, false);
  assert.match(h.status.textContent, /没有可打包的配图/);
  assert.match(h.context.archiveDownloadHTML('job-1'), /重试打包下载/);
  assert.equal(h.toasts[0].type, 'error');
});

test('准备超时中止请求并允许重试', async () => {
  const h = harness();
  h.setRequest((_, {signal}) => new Promise((resolve, reject) => {
    signal.addEventListener('abort', () => reject(new Error('aborted')), {once: true});
  }));
  const preparing = h.context.downloadArchive('job-1');
  h.advance(30000);
  await preparing;
  assert.equal(h.calls[0].options.signal.aborted, true);
  assert.match(h.status.textContent, /超时/);
  assert.equal(h.button.disabled, false);
  assert.equal(h.downloads.length, 0);
});

test('共享续期请求未响应时，准备状态仍按总时限结束', async () => {
  const h = harness();
  h.setRequest(() => new Promise(() => {}));
  const preparing = h.context.downloadArchive('job-1');
  h.advance(30000);
  await preparing;
  assert.match(h.status.textContent, /超时/);
  assert.equal(h.button.disabled, false);
  assert.equal(h.downloads.length, 0);
});

test('拒绝外部地址、其他任务地址或缺失下载地址', async () => {
  for (const downloadUrl of ['https://example.test/api/v1/archive-download/job-1', '/api/v1/archive-download/other-job', undefined]) {
    const h = harness();
    h.setRequest(async () => ({downloadUrl}));
    await h.context.downloadArchive('job-1');
    assert.equal(h.downloads.length, 0);
    assert.equal(h.button.disabled, false);
    assert.equal(h.toasts.length, 1);
  }
});

test('准备期间退出或切换账号，不再启动原账号下载', async () => {
  for (const nextSession of [null, {user: {id: 'another-owner'}}]) {
    const h = harness();
    let resolve;
    h.setRequest(() => new Promise(done => { resolve = done; }));
    const preparing = h.context.downloadArchive('job-1');
    h.resetSession(nextSession);
    resolve({downloadUrl: '/api/v1/archive-download/job-1'});
    await preparing;
    assert.equal(h.downloads.length, 0);
  }
});

test('续期失效时退出旧页面并提示登录，不遗留准备中的任务', async () => {
  const h = harness();
  h.setRequest(async () => {
    h.context.session = null;
    throw new Error('登录已过期，请重新登录');
  });
  await h.context.downloadArchive('job-1');
  assert.equal(h.entered.length, 1);
  assert.equal(h.entered[0], null);
  assert.equal(h.context.archiveDownloadView('job-1').locked, false);
  assert.equal(h.downloads.length, 0);
  assert.match(h.toasts[0].message, /登录已过期/);
});

test('未登录时提示登录，不请求或下载', async () => {
  const h = harness();
  h.resetSession();
  await h.context.downloadArchive('job-1');
  assert.equal(h.calls.length, 0);
  assert.equal(h.downloads.length, 0);
  assert.match(h.toasts[0].message, /登录/);
});

test('任务编号、文件名和错误消息正确转义', async () => {
  const h = harness();
  const html = h.context.archiveDownloadHTML('job"<', 'work"<.zip');
  assert.match(html, /data-archive-download="job&quot;&lt;"/);
  assert.match(html, /data-name="work&quot;&lt;\.zip"/);
  h.setRequest(async () => { throw new Error('<img src=x onerror=alert(1)>'); });
  await h.context.downloadArchive('job-1');
  assert.doesNotMatch(h.context.archiveDownloadHTML('job-1'), /<img/);
});

test('图集按类型与序号核对保存数量，缺失动图不会被历史静图冒充', () => {
  const h = harness();
  Object.assign(h.context, {
    adminMode: false, statusLabels: {completed: '已完成'}, statusTone: {completed: 'success'}, actionLabels: {},
    stripHashtags: title => title || '', jobMediaExpiry: () => null, jobExpired: () => false,
    fmtDate: date => date || '', fmtDuration: () => '', fileLabel: file => file.name,
    previewUsable: () => false, isWeChat: () => false,
  });
  const detailStart = source.indexOf('function jobDetailHTML(job){');
  const detailEnd = source.indexOf('function renderDetail(){', detailStart);
  assert.ok(detailStart >= 0 && detailEnd > detailStart);
  vm.runInContext(source.slice(detailStart, detailEnd), h.context);
  const job = {
    id: 'job-1', status: 'completed', action: 'download',
    work: {type: 'note', title: '混合图集', images: [
      {url: 'https://example.test/1.jpg'},
      {url: 'https://example.test/2.jpg', animatedUrl: 'https://example.test/2.mp4'},
    ]},
    result: {files: [{kind: 'image', name: 'image-01.webp'}, {kind: 'image', name: 'image-02.jpg'}]},
  };
  let html = h.context.jobDetailHTML(job);
  assert.match(html, /已保存 1\/2 项媒体/);
  assert.match(html, /动图 · 未保存/);
  assert.match(html, /打包下载已保存的 1 项/);
  job.result.files[1] = {kind: 'animated', name: 'image-02.mp4'};
  html = h.context.jobDetailHTML(job);
  assert.match(html, /已保存 2\/2 项媒体/);
  assert.doesNotMatch(html, /动图 · 未保存/);
  assert.match(html, />[^<>]*打包下载全部<\/button>/);
  job.action = 'info';
  job.result.files = [];
  html = h.context.jobDetailHTML(job);
  assert.doesNotMatch(html, /已保存 0\/2/);
  job.action = 'download';
  job.work.images = Array.from({length: 100}, (_, i) => ({url: `https://example.test/${i + 1}.jpg`}));
  job.result.files = [{kind: 'image', name: 'image-100.jpg'}];
  html = h.context.jobDetailHTML(job);
  assert.match(html, /已保存 1\/100 项媒体/);
  assert.match(html, /打包下载已保存的 1 项/);
});
