const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, 'static/app.js'), 'utf8');
function section(start, end) {
  const first = source.indexOf(start), last = source.indexOf(end, first);
  assert.ok(first >= 0 && last > first, `Missing source section: ${start}`);
  return source.slice(first, last);
}
const listCode = section('async function loadJobs(', '// 通用分页条');
const adminListCode = section('async function loadAdminJobs(', 'function adminJobOps(');
const usersCode = section('async function loadAdminUsers(){', 'function renderAdminUsers(');
const captureCode = section('const adminUsagePermissionFields=', '/* ---------- 任务列表 ---------- */');
const allCapturePermissions = {allowInfo: true, allowDownload: true, allowTranscribe: true};
const selectionCode = section('function selectJob(', "$('#new-task-btn').addEventListener");
const sidebarCode = section('let sidebarLastFocus=null;', "$('#sidebar-toggle').innerHTML");
const pollCode = section('const ACTIVE_POLL_MS=', '/* ---------- 登录 / 退出 / 连接 ---------- */');
const adminNavigationCode = section('function openAdminConsole(', 'const ACTIVE_POLL_MS=');

function harness(code = listCode) {
  const nodes = new Map(), requests = [], rendered = [], notices = [], details = [];
  function node(selector) {
    if (!nodes.has(selector)) nodes.set(selector, {
      value: '', checked: false, disabled: false, children: [], attributes: {}, listeners: {},
      classList: {toggle() {}, add() {}, remove() {}, contains() { return false; }},
      setAttribute(name, value) { this.attributes[name] = value; },
      getAttribute(name) { return this.attributes[name]; },
      addEventListener(name, callback) { this.listeners[name] = callback; },
      focus() { this.focused = true; },
      reset() { this.resetCount = (this.resetCount || 0) + 1; },
      scrollIntoView() {},
    });
    return nodes.get(selector);
  }
  const context = vm.createContext({
    session: {user: {id: 'owner'}}, sessionEpoch: 1, poll: null,
    jobsPending: null, jobDetailPending: null, adminJobsPending: null, adminDetailPending: null,
    jobsLoadId: 0, jobDetailLoadId: 0, adminJobsLoadId: 0, adminUsersLoadId: 0,
    pollRequestedAt: {}, adminStatsHaveActive: false,
    jobsPage: 1, jobsPageSize: 20, jobsTotal: 0, jobsCache: [],
    adminJobsPage: 1, adminJobsPageSize: 20, adminJobsTotal: 0, adminJobsCache: [],
    adminUsers: [], lastAdminJobsSig: '', adminDetailLoadId: 0, adminDetailSig: '', adminMode: false,
    selectedId: null, currentJob: null, currentView: 'new', detailSig: '',
    taskMusic: {jobId: null}, adminOpen: false, videoPreviewURLs: new Map(),
    AbortController, URLSearchParams, setTimeout, clearTimeout, $: node,
    document: {
      hidden: false, listeners: {},
      querySelectorAll() { return []; },
      addEventListener(name, callback) { this.listeners[name] = callback; },
    },
    matchMedia() { return {matches: false}; },
    api(url, options) {
      return new Promise((resolve, reject) => requests.push({url, options, resolve, reject}));
    },
    renderList(jobs) { context.jobsCache = jobs; rendered.push(jobs); },
    renderAdminJobs(jobs) { rendered.push(jobs); },
    renderAdminUsers(users) { rendered.push(users); },
    renderPager() {}, skeletonRows() { return ''; },
    jobSignature(job) { return JSON.stringify(job); },
    jobIsActive(status) { return !['completed','failed','cancelled'].includes(status); },
    jobListSignature(job) { return JSON.stringify(job); },
    jobMediaExpiry(job) { return job?.mediaExpiresAt || null; },
    jobExpired() { return false; },
    jobHasExpiredPreview() { return false; },
    esc(value) { return String(value); },
    toast(message, type) { notices.push({message, type}); },
    enter(value) { context.sessionEpoch++; context.session = value; },
    showView(name) { context.currentView = name; },
    renderDetail() { if (context.currentJob) details.push(context.currentJob); },
    show() {}, stopTaskMusic() {}, setSidebar() {}, closePreview() {}, resetCDKView() {}, resetCapturePermissions() {},
    refreshDailyUsage() {}, uuidv4() { return 'idempotency-key'; },
  });
  vm.runInContext(code, context);
  return {context, node, requests, rendered, notices, details};
}

function job(id) { return {id, status: 'queued', progress: 0}; }
const tick = async () => { await Promise.resolve(); await Promise.resolve(); };

test('旧会话的续期结果不会覆盖新账号，也不会使用新账号重试旧请求', async () => {
  const h = harness(section('let refreshing=null;', '/* ---------- Toast'));
  const requests = [];
  h.context.session.accessToken = 'old';
  h.context.save = value => { h.context.session = value; };
  h.context.fetch = (url, options) => new Promise(resolve => requests.push({url, options, resolve}));
  const mutation = h.context.api('/api/v1/admin/cdks', {method: 'POST'});
  assert.equal(requests[0].options.headers.Authorization, 'Bearer old');
  requests[0].resolve({status: 401, json: async () => ({error: {code: 'AUTH_TOKEN_EXPIRED'}})});
  await new Promise(setImmediate);
  assert.equal(requests[1].url, '/api/v1/auth/refresh');
  const next = {accessToken: 'new', user: {id: 'new'}};
  h.context.enter(next);
  requests[1].resolve({ok: true, json: async () => ({accessToken: 'rotated-old'})});
  await assert.rejects(mutation, /登录已过期/);
  assert.equal(h.context.session, next);
  assert.equal(requests.length, 2);
});

test('续期发现会话失效或 Cookie 已切换账号时，不重试原账号请求', async () => {
  for(const replacement of [null,{accessToken:'different',user:{id:'another-account'}}]){
    const h=harness(section('let refreshing=null;', '/* ---------- Toast'));
    const urls=[];
    h.context.fetch=async url=>{
      urls.push(url);
      if(url==='/api/v1/auth/refresh')return {ok:!!replacement,json:async()=>replacement};
      return {status:401,json:async()=>({error:{code:'AUTH_TOKEN_EXPIRED'}})};
    };
    await assert.rejects(h.context.api('/api/v1/admin/cdks',{method:'POST'}),/登录已过期/);
    assert.equal(h.context.session,replacement);
    assert.deepEqual(urls,['/api/v1/admin/cdks','/api/v1/auth/refresh']);
  }
});

test('前台只请求列表摘要，点击后单独取详情并忽略以前的任务响应', async () => {
  const h = harness(selectionCode + listCode);
  const list = h.context.loadJobs();
  assert.equal(new URLSearchParams(h.requests[0].url.split('?')[1]).get('view'), 'summary');
  h.requests[0].resolve({jobs: [job('first'), job('second')], total: 2});
  await list;
  assert.equal(h.details.length, 0);
  h.context.selectJob('first');
  h.context.selectJob('second');
  assert.deepEqual(h.requests.slice(1).map(request => request.url), ['/api/v1/jobs/first', '/api/v1/jobs/second']);
  const detail = {...job('second'), result: {files: [{id: 'media'}]}};
  h.requests[2].resolve({job: detail});
  h.requests[1].resolve({job: job('first')});
  await tick();
  assert.deepEqual(h.details, [detail]);
});

test('活跃任务五秒刷新，结态降频，标签隐藏暂停且返回立即同步', async () => {
  const h = harness(pollCode + listCode);
  let now = 60_000, interval = null;
  h.context.Date = {now: () => now};
  h.context.clearInterval = () => { interval = null; };
  h.context.setInterval = (callback, ms) => { assert.equal(ms, 5000); interval = callback; return 1; };
  const initial = h.context.loadJobs();
  h.requests[0].resolve({jobs: [job('active')], total: 1});
  await initial;
  h.context.startJobPolling();
  now += 5000;
  interval();
  assert.equal(h.requests.length, 2);
  h.requests[1].resolve({jobs: [{...job('active'), status: 'completed'}], total: 1});
  await tick();
  now += 5000;
  interval();
  assert.equal(h.requests.length, 2);
  now += 25_000;
  interval();
  assert.equal(h.requests.length, 3);
  h.requests[2].resolve({jobs: [], total: 0});
  await tick();
  h.context.document.hidden = true;
  h.context.document.listeners.visibilitychange();
  assert.equal(interval, null);
  h.context.pollVisibleJobs(true);
  assert.equal(h.requests.length, 3);
  h.context.document.hidden = false;
  h.context.document.listeners.visibilitychange();
  assert.equal(typeof interval, 'function');
  assert.equal(h.requests.length, 4);
  h.requests[3].resolve({jobs: [], total: 0});
  await tick();
});

test('当前页结态详情只在摘要变化或签名过期时自动重新读取', async () => {
  const h = harness(pollCode + listCode + section('function previewExpiry(', '// 微信内置浏览器'));
  const summary = {...job('selected'), status: 'completed', updatedAt: 'original'};
  h.context.selectedId = summary.id;
  h.context.currentJob = {...summary, result: {files: [{previewUrl: 'media?expires=180'}]}};
  h.context.jobsCache = [summary];
  let now = 60_000;
  h.context.Date = {now: () => now};
  h.context.pollVisibleJobs();
  assert.equal(h.requests.length, 1);
  h.requests[0].resolve({jobs: [summary], total: 1});
  await tick();
  assert.equal(h.requests.length, 1);

  now += 30_000;
  h.context.pollVisibleJobs();
  const changed = {...summary, updatedAt: 'changed'};
  h.requests[1].resolve({jobs: [changed], total: 1});
  await tick();
  assert.equal(h.requests[2].url, '/api/v1/jobs/selected');
  h.requests[2].resolve({job: {...h.context.currentJob, updatedAt: changed.updatedAt}});
  await tick();

  now = 180_000;
  h.context.detailSig = 'old rendered media';
  h.context.pollVisibleJobs();
  assert.equal(h.requests[4].url, '/api/v1/jobs/selected');
  h.requests[3].resolve({jobs: [changed], total: 1});
  h.requests[4].resolve({job: {...changed, result: {files: [{previewUrl: 'media?expires=360'}]}}});
  await tick();
  assert.equal(h.context.detailSig, '');
});

test('前台筛选请求乱序返回时，旧结果不会覆盖新筛选', async () => {
  const h = harness();
  const first = h.context.loadJobs();
  h.node('#query').value = '新的关键词';
  h.node('#status').value = 'completed';
  const second = h.context.loadJobs();
  h.requests[1].resolve({jobs: [job('new')], total: 1});
  await second;
  h.requests[0].resolve({jobs: [job('old')], total: 10});
  await first;
  assert.equal(h.rendered.length, 1);
  assert.equal(h.rendered[0][0].id, 'new');
  assert.equal(h.context.jobsTotal, 1);
});

test('退出或切换账号后，未完成的列表请求不会恢复旧界面', async () => {
  const h = harness();
  const pending = h.context.loadJobs();
  h.context.enter(null);
  h.requests[0].resolve({jobs: [job('previous-user')], total: 1});
  await pending;
  assert.equal(h.rendered.length, 0);
  assert.equal(h.context.jobsTotal, 0);
});

test('旧筛选请求失败不干扰新筛选，也不会误退出当前账号', async () => {
  const h = harness();
  const first = h.context.loadJobs();
  h.node('#status').value = 'failed';
  const second = h.context.loadJobs();
  h.requests[1].resolve({jobs: [job('current')], total: 1});
  await second;
  h.requests[0].reject(new Error('登录已过期'));
  await first;
  assert.equal(h.context.session.user.id, 'owner');
  assert.equal(h.notices.length, 0);
});

test('列表总数减少导致当前页越界时，自动请求最后一页', async () => {
  const h = harness();
  h.context.jobsPage = 3;
  const pending = h.context.loadJobs();
  h.requests[0].resolve({jobs: [], total: 2});
  await tick();
  assert.equal(h.context.jobsPage, 1);
  assert.equal(new URLSearchParams(h.requests[1].url.split('?')[1]).get('offset'), '0');
  h.requests[1].resolve({jobs: [job('remaining')], total: 2});
  await pending;
  assert.equal(h.rendered[0][0].id, 'remaining');
});

test('后台任务请求乱序返回时，保留最新筛选结果', async () => {
  const h = harness(adminListCode);
  const first = h.context.loadAdminJobs();
  assert.equal(new URLSearchParams(h.requests[0].url.split('?')[1]).get('view'), 'summary');
  h.node('#admin-jobs-user').value = 'user-2';
  const second = h.context.loadAdminJobs();
  h.requests[1].resolve({jobs: [job('new')], total: 1});
  await second;
  h.requests[0].resolve({jobs: [job('old')], total: 8});
  await first;
  assert.equal(h.rendered.length, 1);
  assert.equal(h.rendered[0][0].id, 'new');
  assert.equal(h.context.adminJobsTotal, 1);
});

test('刷新后台用户选项后，保留当前用户筛选', async () => {
  const h = harness(usersCode), select = h.node('#admin-jobs-user');
  // 浏览器重建 select 的选项时会自动选中第一个选项。
  Object.defineProperty(select, 'innerHTML', {set() { this.value = ''; }});
  select.value = 'user-2';
  const pending = h.context.loadAdminUsers();
  h.requests[0].resolve({users: [{id: 'user-1', username: 'one'}, {id: 'user-2', username: 'two'}]});
  await pending;
  assert.equal(select.value, 'user-2');
});

test('用户列表的旧响应不会覆盖更新后的选项', async () => {
  const h = harness(usersCode);
  const first = h.context.loadAdminUsers(), second = h.context.loadAdminUsers();
  h.requests[1].resolve({users: [{id: 'new', username: 'new'}]});
  await second;
  h.requests[0].resolve({users: [{id: 'old', username: 'old'}]});
  await first;
  assert.equal(h.context.adminUsers[0].id, 'new');
  assert.equal(h.rendered.length, 1);
});

test('在搜索结果或后续页创建任务，立即展示新详情并重置筛选', async () => {
  const h = harness(selectionCode + captureCode + listCode);
  h.context.applyCapturePermissions(allCapturePermissions);
  h.context.jobsPage = 4;
  h.node('#query').value = '旧搜索';
  h.node('#status').value = 'completed';
  h.node('#share-text').value = '  https://www.douyin.com/video/123  ';
  const button = h.node('#submit-task');
  const pending = h.node('#capture-form').listeners.submit({preventDefault() {}, submitter: button});
  assert.equal(button.disabled, true);
  assert.equal(JSON.parse(h.requests[0].options.body).shareText, 'https://www.douyin.com/video/123');
  h.requests[0].resolve({job: job('created')});
  await tick();
  assert.equal(h.context.selectedId, 'created');
  assert.equal(h.context.currentJob.id, 'created');
  assert.equal(h.details[0].id, 'created');
  assert.equal(h.context.currentView, 'detail');
  assert.equal(h.context.jobsPage, 1);
  assert.equal(h.node('#query').value, '');
  assert.equal(h.node('#status').value, '');
  const query = new URLSearchParams(h.requests[1].url.split('?')[1]);
  assert.equal(query.get('offset'), '0');
  assert.equal(query.get('q'), '');
  assert.equal(query.get('status'), '');
  h.requests[1].resolve({jobs: [job('created')], total: 1});
  await pending;
  assert.equal(button.disabled, false);
  assert.equal(button.attributes['aria-busy'], 'false');
});

test('空白分享内容不创建任务；无 submitter 的提交可正常使用主按钮', async () => {
  const h = harness(selectionCode + captureCode + listCode);
  h.context.applyCapturePermissions(allCapturePermissions);
  const submit = h.node('#capture-form').listeners.submit;
  h.node('#share-text').value = '   ';
  await submit({preventDefault() {}, submitter: null});
  assert.equal(h.requests.length, 0);
  assert.equal(h.node('#share-text').focused, true);
  h.node('#share-text').value = 'https://v.douyin.com/example';
  const pending = submit({preventDefault() {}, submitter: null});
  assert.equal(h.node('#submit-task').disabled, true);
  h.requests[0].reject(new Error('服务暂不可用'));
  await pending;
  assert.equal(h.node('#submit-task').disabled, false);
  assert.equal(h.node('#capture-feedback').textContent, '服务暂不可用');
});

test('仅解析提交时立即显示进度，失败结果到达后不等待列表刷新就提示', async () => {
  const h = harness(selectionCode + captureCode + listCode);
  h.context.applyCapturePermissions(allCapturePermissions);
  h.node('#info-only').checked = true;
  h.node('#share-text').value = 'https://www.douyin.com/video/123';
  const pending = h.node('#capture-form').listeners.submit({preventDefault() {}});
  assert.equal(h.node('#capture-feedback').textContent, '正在读取作品信息…');
  assert.equal(JSON.parse(h.requests[0].options.body).action, 'info');
  const failed = {...job('failed-info'),status:'failed',error:{message:'读取作品信息超时，请稍后重试'}};
  h.requests[0].resolve({job:failed});
  await tick();
  assert.equal(h.node('#capture-feedback').textContent,'');
  assert.deepEqual(h.notices,[{type:'error',message:failed.error.message}]);
  assert.equal(h.details[0],failed);
  h.requests[1].resolve({jobs:[failed],total:1});
  await pending;
});

test('前台按任务权限隐藏停用选项，只提交当前允许的任务类型', async () => {
  const h = harness(captureCode), visible = new Map();
  h.context.show = (selector, on) => visible.set(selector, on);
  h.context.resetCapturePermissions();
  assert.equal(h.node('#submit-task').disabled, true);
  const info = h.node('#info-only'), transcribe = h.node('#with-transcribe');
  h.context.applyCapturePermissions(allCapturePermissions);
  info.checked = true;
  info.listeners.change();
  h.context.applyCapturePermissions({...allCapturePermissions, allowInfo: false});
  assert.equal(visible.get('#info-only-wrap'), false);
  assert.equal(info.checked, false);
  assert.equal(h.context.captureAction(), 'download');

  h.context.applyCapturePermissions({...allCapturePermissions, allowDownload: false});
  assert.equal(info.checked, true);
  transcribe.checked = true;
  transcribe.listeners.change();
  assert.equal(info.checked, false);
  assert.equal(h.context.captureAction(), 'full');

  h.context.applyCapturePermissions({allowInfo: true, allowDownload: false, allowTranscribe: false});
  assert.equal(visible.get('#with-transcribe-wrap'), false);
  assert.equal(transcribe.checked, false);
  assert.equal(h.context.captureAction(), 'info');

  h.context.applyCapturePermissions({allowInfo: false, allowDownload: false, allowTranscribe: false});
  assert.equal(visible.get('#capture-options'), false);
  assert.equal(h.node('#submit-task').disabled, true);
  assert.match(h.node('#capture-action-hint').textContent, /关闭所有/);
  h.node('#share-text').value = 'https://www.douyin.com/video/123';
  await h.node('#capture-form').listeners.submit({preventDefault() {}});
  assert.equal(h.requests.length, 0);
});

test('任务权限读取失败可重试，较早的响应不会重新打开已关闭的选项', async () => {
  const h = harness(captureCode), visible = new Map();
  h.context.show = (selector, on) => visible.set(selector, on);
  h.context.resetCapturePermissions();
  const failing = h.context.refreshDailyUsage();
  h.requests[0].reject(new Error('网络错误'));
  await failing;
  assert.equal(visible.get('#capture-usage-retry'), true);
  assert.equal(h.node('#submit-task').disabled, true);

  const old = h.node('#capture-usage-retry').listeners.click();
  const recent = h.context.refreshDailyUsage();
  h.requests[2].resolve({usage: {used: 0, limit: 0, ...allCapturePermissions, allowInfo: false}});
  await recent;
  h.requests[1].resolve({usage: {used: 0, limit: 0, ...allCapturePermissions}});
  await old;
  assert.equal(visible.get('#capture-usage-retry'), false);
  assert.equal(visible.get('#info-only-wrap'), false);
  assert.equal(h.context.captureAction(), 'download');
});

test('后台执行的任务切换到失败时，详情只通知一次', async () => {
  const h = harness(listCode);
  h.context.selectedId = 'job-id';
  h.context.currentJob = {...job('job-id'),status:'resolving'};
  const failed = {...job('job-id'),status:'failed',error:{message:'作品不存在或不可访问'}};
  for(let i=0;i<2;i++){
    const pending=h.context.loadJobDetail('job-id',true);
    h.requests[i].resolve({job:failed});
    await pending;
  }
  assert.deepEqual(h.notices,[{type:'error',message:failed.error.message}]);
});

test('手动重试后立即失败仍会显示失败原因', async () => {
  const h = harness(selectionCode + listCode + section('async function operate(id,op)', '// 签名预览地址'));
  const failed = {...job('job-id'),status:'failed',error:{message:'作品不存在或不可访问'}};
  h.context.selectedId=failed.id;
  h.context.currentJob=failed;
  const retry=h.context.operate(failed.id,'retry');
  h.requests[0].resolve({job:job(failed.id)});
  await tick();
  assert.equal(h.context.currentJob.status,'queued');
  h.requests[1].resolve({jobs:[],total:0});
  h.requests[2].resolve({job:failed});
  await retry;
  assert.equal(h.notices.at(-1).message,failed.error.message);
});

test('搜索按钮与搜索表单一致，跳回第一页后再请求', () => {
  const h = harness(section("$('#search-form').addEventListener", 'async function loadJobs('));
  h.context.loadJobs = () => { h.context.requestedPage = h.context.jobsPage; };
  h.context.jobsPage = 5;
  h.node('#refresh').listeners.click();
  assert.equal(h.context.requestedPage, 1);
});

test('桌面关闭抽屉时保留导航可访问性；手机关闭时禁止聚焦隐藏导航', () => {
  const h = harness();
  const media = {matches: false, addEventListener() {}};
  h.context.matchMedia = () => media;
  h.context.document.activeElement = null;
  h.context.document.contains = () => false;
  h.context.requestAnimationFrame = callback => callback();
  vm.runInContext(sidebarCode, h.context);
  h.context.setSidebar(false);
  assert.equal(h.node('.master-list').attributes['aria-hidden'], 'false');
  assert.equal(h.node('.master-list').inert, false);
  assert.equal(h.node('.admin-sidebar').inert, true);
  media.matches = true;
  h.context.setSidebar(false);
  assert.equal(h.node('.master-list').attributes['aria-hidden'], 'true');
  assert.equal(h.node('.master-list').inert, true);
});

test('管理员进入后台后，顶栏显示返回前台，修改密码由独立菜单打开', () => {
  const h = harness(pollCode);
  const visible = new Map();
  const nav = ['dashboard', 'usage', 'password'].map(name => Object.assign(h.node(name), {dataset: {adminView: name}}));
  h.context.document.querySelectorAll = () => nav;
  h.context.show = (selector, on) => visible.set(selector, on);
  h.context.session.user.role = 'admin';
  h.context.adminCurrentView = 'dashboard';
  h.context.loadAdminStats = () => {};
  h.context.requestAdminUsageSettings = () => { h.context.usageRequests = (h.context.usageRequests || 0) + 1; };
  h.context.selectJob = () => {};
  h.context.loadJobs = () => {};
  vm.runInContext(adminNavigationCode, h.context);

  h.node('#admin-console-btn').listeners.click();
  assert.equal(visible.get('#admin-console'), true);
  assert.equal(visible.get('#admin-back-front'), true);
  assert.equal(visible.get('#logout'), false);
  assert.equal(visible.get('#change-password'), false);

  h.node('password').listeners.click();
  assert.equal(visible.get('#admin-view-password'), true);
  assert.equal(visible.get('#admin-view-dashboard'), false);
  assert.equal(h.node('password').attributes['aria-current'], 'page');

  h.node('#admin-back-front').listeners.click();
  assert.equal(h.node('#admin-password-form').resetCount, 1);
  assert.equal(visible.get('#admin-console'), false);
  assert.equal(visible.get('#logout'), true);
  assert.equal(visible.get('#admin-back-front'), false);
  assert.equal(h.node('#admin-console-btn').focused, true);

  h.node('#settings-link').listeners.click();
  assert.equal(visible.get('#admin-view-usage'), true);
  assert.equal(visible.get('#admin-view-dashboard'), false);
  assert.equal(h.context.usageRequests, 1);
  assert.equal(h.node('#admin-usage-heading').focused, true);
  h.node('#admin-back-front').listeners.click();
  assert.equal(h.node('#settings-link').focused, true);
});


test('后台详情在现有轮询中刷新当前任务', () => {
  const h = harness(pollCode);
  h.context.archiveDownloads = {clear() {}};
  h.context.clearInterval = () => {};
  h.context.setInterval = callback => { h.context.pollTick = callback; return 1; };
  h.context.save = value => { h.context.session = value; };
  h.context.show = () => {};
  h.context.selectJob = () => {};
  h.context.loadJobs = () => {};
  h.context.viewAdminJob = (id, refresh) => { h.context.refreshCall = {id, refresh}; };
  h.context.enter({user: {id: 'admin', role: 'user', displayName: 'Admin'}});
  h.context.adminOpen = true;
  h.context.adminCurrentView = 'detail';
  h.context.currentJob = job('task-1');
  h.context.pollTick();
  assert.equal(h.context.refreshCall.id, 'task-1');
  assert.equal(h.context.refreshCall.refresh, true);
});

test('后台详情刷新使用最新响应，状态不变时不重绘媒体', async () => {
  const h = harness(section('async function viewAdminJob(', 'async function operateAdminJob('));
  h.context.adminOpen = true;
  h.context.adminCurrentView = 'detail';
  h.context.currentJob = job('task-1');
  h.context.jobDetailHTML = data => `${data.id}:${data.status}`;
  h.context.syncTaskMusicWithJob = () => {};
  h.context.updateTaskMusicUI = () => {};
  let renders = 0, html = '';
  Object.defineProperty(h.node('#admin-detail'), 'innerHTML', {set(value) { renders++; html = value; }});
  const old = h.context.viewAdminJob('task-1', true);
  const current = h.context.viewAdminJob('task-1', true);
  const completed = {...job('task-1'), status: 'completed'};
  h.requests[1].resolve({job: completed});
  await current;
  h.requests[0].resolve({job: {...job('task-1'), status: 'downloading'}});
  await old;
  assert.equal(h.context.currentJob.status, 'completed');
  assert.equal(html, 'task-1:completed');
  assert.equal(renders, 1);
  const unchanged = h.context.viewAdminJob('task-1', true);
  h.requests[2].resolve({job: completed});
  await unchanged;
  assert.equal(renders, 1);
});

test('离开后台详情后，尚未完成的刷新不会重新打开旧任务', async () => {
  const h = harness(section('async function viewAdminJob(', 'async function operateAdminJob('));
  h.context.adminOpen = true;
  h.context.adminCurrentView = 'detail';
  h.context.currentJob = job('task-1');
  const pending = h.context.viewAdminJob('task-1', true);
  h.context.adminCurrentView = 'users';
  h.requests[0].resolve({job: {...job('task-1'), status: 'completed'}});
  await pending;
  assert.equal(h.context.currentJob.status, 'queued');
  assert.equal(h.context.adminCurrentView, 'users');
});

test('后台详情取消或重试后，立即刷新详情而非只更新隐藏列表', async () => {
  for (const operation of ['cancel', 'retry']) {
    const h = harness(section('async function operate(id,op){', '// 签名预览地址带 expires'));
    h.context.adminMode = true;
    h.context.adminOpen = true;
    h.context.adminCurrentView = 'detail';
    h.context.currentJob = job('task-1');
    h.context.viewAdminJob = async (id, refresh) => { h.context.refreshCall = {id, refresh}; };
    h.context.loadAdminJobs = () => { throw new Error('详情操作不应只刷新隐藏列表'); };
    const pending = h.context.operate('task-1', operation);
    assert.equal(h.requests[0].url, `/api/v1/admin/jobs/task-1/${operation}`);
    h.requests[0].resolve({job: {...job('task-1'), status: 'cancelled'}});
    await pending;
    assert.equal(h.context.refreshCall.id, 'task-1');
    assert.equal(h.context.refreshCall.refresh, true);
  }
});

function videoPreviewHarness() {
  const h = harness(section('function previewExpiry(', '// 微信内置浏览器') +
    section('const videoPreviewURLs=', 'function openGalleryPreview(') +
    section('async function openPreview(', "document.querySelectorAll('[data-preview-nav]').forEach(btn=>{"));
  const video = h.node('#preview-video');
  h.context.document.body = {classList: {add() {}, remove() {}}};
  h.node('#preview-image').removeAttribute = function(name) { delete this[name]; };
  Object.assign(video, {
    paused: true, playCount: 0, duration: 60,
    async play() { this.paused = false; this.playCount++; },
    pause() { this.paused = true; },
    load() { this.currentTime = 0; this.pause(); },
    removeAttribute(name) { delete this[name]; },
  });
  return {...h, video};
}

function signedVideo(signature, secondsUntilExpiry = 3600) {
  const expires = Math.floor(Date.now() / 1000) + secondsUntilExpiry;
  return `/api/v1/media-preview/file%2Fid?expires=${expires}&signature=${signature}`;
}

function videoJob(previewUrl) {
  return {job: {id: 'job/id', result: {files: [{id: 'file/id', previewUrl}]}}};
}

test('视频预览签名过期后，通过对应权限的任务详情更新签名并继续使用原生播放器', async () => {
  for (const inAdmin of [false, true]) {
    const h = videoPreviewHarness();
    h.context.adminMode = inAdmin;
    const fresh = signedVideo('fresh');
    const pending = h.context.openPreview('job/id', 'file/id', 'example.mp4', signedVideo('expired', -1));
    assert.equal(h.requests[0].url, `/api/v1/${inAdmin ? 'admin/' : ''}jobs/job%2Fid`);
    assert.equal(h.requests[0].options.cache, 'no-store');
    h.requests[0].resolve(videoJob(fresh));
    await pending;
    assert.equal(h.video.src, fresh);
    assert.equal(h.video.playCount, 1);
    assert.equal(h.notices.length, 0);
  }
});

test('重开同一视频复用未过期签名，轮询提供的新签名不会替换它', async () => {
  const h = videoPreviewHarness(), reused = signedVideo('reused');
  await h.context.openPreview('job/id', 'file/id', 'example.mp4', reused);
  h.context.closePreview();
  const latest = signedVideo('latest');
  h.context.currentJob = videoJob(latest).job;
  await h.context.openPreview('job/id', 'file/id', 'example.mp4', latest);
  assert.equal(h.video.src, reused);
  assert.equal(h.requests.length, 0);
});

test('播放中更新签名后恢复进度和暂停状态，重试失败时退出播放器', async () => {
  for (const paused of [false, true]) {
    const h = videoPreviewHarness();
    await h.context.openPreview('job/id', 'file/id', 'example.mp4', signedVideo('old'));
    h.video.onloadedmetadata();
    h.video.currentTime = 27;
    h.video.paused = paused;
    const retry = h.video.onerror();
    const fresh = signedVideo('fresh');
    h.requests[0].resolve(videoJob(fresh));
    await retry;
    h.video.onloadedmetadata();
    assert.equal(h.video.src, fresh);
    assert.equal(h.video.currentTime, 27);
    assert.equal(h.video.paused, paused);
    assert.equal(h.video.playCount, paused ? 1 : 2);
    await h.video.onerror();
    assert.equal(h.video.src, undefined);
    assert.equal(h.requests.length, 1);
    assert.equal(h.notices[0].type, 'error');
  }
});

test('刷新时服务端返回同一签名，播放器只重试一次', async () => {
  const h = videoPreviewHarness(), url = signedVideo('unchanged');
  await h.context.openPreview('job/id', 'file/id', 'example.mp4', url);
  const retry = h.video.onerror();
  h.requests[0].resolve(videoJob(url));
  await retry;
  assert.equal(h.video.src, url);
  await h.video.onerror();
  assert.equal(h.requests.length, 1);
  assert.equal(h.notices.length, 1);
});

test('已续签的视频自然过期后可再次续签，保留播放位置', async () => {
  const h = videoPreviewHarness();
  let now = Date.now();
  h.context.Date = {now: () => now};
  await h.context.openPreview('job/id', 'file/id', 'example.mp4', signedVideo('first'));
  const firstRetry = h.video.onerror();
  const secondUrl = signedVideo('second', 60);
  h.requests[0].resolve(videoJob(secondUrl));
  await firstRetry;
  h.video.onloadedmetadata();
  h.video.currentTime = 32;
  now += 60_000;
  const secondRetry = h.video.onerror();
  const thirdUrl = signedVideo('third');
  h.requests[1].resolve(videoJob(thirdUrl));
  await secondRetry;
  h.video.onloadedmetadata();
  assert.equal(h.video.src, thirdUrl);
  assert.equal(h.video.currentTime, 32);
  assert.equal(h.video.paused, false);
});

test('签名刷新期间切换预览，旧请求不会覆盖当前视频', async () => {
  const h = videoPreviewHarness();
  const pending = h.context.openPreview('job/id', 'file/id', 'old.mp4', signedVideo('expired', -1));
  const latest = signedVideo('latest');
  await h.context.openPreview('second-job', 'second-file', 'new.mp4', latest);
  h.requests[0].resolve(videoJob(signedVideo('stale-response')));
  await pending;
  assert.equal(h.video.src, latest);
  assert.equal(h.notices.length, 0);
});


test('任务列表慢请求跨过 5 秒轮询后仍能渲染，完成后恢复下一轮请求', async () => {
  const h = harness(pollCode + listCode);
  h.context.archiveDownloads = {clear() {}};
  h.context.clearInterval = () => {};
  h.context.setInterval = (callback, delay) => {
    assert.equal(delay, 5000);
    h.context.pollTick = callback;
    return 1;
  };
  h.context.save = value => { h.context.session = value; };
  h.context.show = () => {};
  h.context.selectJob = () => {};
  h.context.enter({user: {id: 'owner', role: 'user', displayName: 'Owner'}});
  assert.equal(h.requests.length, 1);
  // 模拟第 5 秒的轮询，首个请求到第 6 秒才返回。
  h.context.pollTick();
  assert.equal(h.requests.length, 1);
  h.requests[0].resolve({jobs: [job('slow-result')], total: 1});
  await tick();
  assert.equal(h.rendered.length, 1);
  assert.equal(h.rendered[0][0].id, 'slow-result');
  // 第 10 秒的新一轮能够继续更新，而不是永久跳过。
  h.context.pollTick();
  assert.equal(h.requests.length, 2);
  h.requests[1].resolve({jobs: [job('next-result')], total: 1});
  await tick();
  assert.equal(h.rendered[1][0].id, 'next-result');
});

test('后台列表慢请求跨过多次轮询不会失效，主动刷新可替代旧请求', async () => {
  const h = harness(adminListCode);
  const slow = h.context.loadAdminJobs();
  await h.context.loadAdminJobs(true);
  await h.context.loadAdminJobs(true);
  assert.equal(h.requests.length, 1);
  h.requests[0].resolve({jobs: [job('slow-result')], total: 1});
  await slow;
  assert.equal(h.rendered[0][0].id, 'slow-result');
  const next = h.context.loadAdminJobs(true);
  const forced = h.context.loadAdminJobs();
  assert.equal(h.requests.length, 3);
  h.requests[2].resolve({jobs: [job('forced-result')], total: 1});
  await forced;
  h.requests[1].resolve({jobs: [job('outdated-result')], total: 1});
  await next;
  assert.equal(h.rendered.length, 2);
  assert.equal(h.rendered[1][0].id, 'forced-result');
});

test('前台慢请求期间切换筛选立即请求新结果，不受轮询去重阻挡', async () => {
  const h = harness();
  const slow = h.context.loadJobs();
  await h.context.loadJobs(true);
  h.node('#status').value = 'completed';
  const filtered = h.context.loadJobs();
  assert.equal(h.requests.length, 2);
  h.requests[1].resolve({jobs: [job('filtered')], total: 1});
  await filtered;
  h.requests[0].resolve({jobs: [job('outdated')], total: 1});
  await slow;
  assert.equal(h.rendered.length, 1);
  assert.equal(h.rendered[0][0].id, 'filtered');
});

test('后台详情慢请求不会被后续轮询饿死，取消重试后的强制刷新可替代旧请求', async () => {
  const h = harness(section('async function viewAdminJob(', 'async function operateAdminJob('));
  h.context.adminOpen = true;
  h.context.adminCurrentView = 'detail';
  h.context.currentJob = job('task-1');
  h.context.jobDetailHTML = data => `${data.id}:${data.status}`;
  h.context.syncTaskMusicWithJob = () => {};
  h.context.updateTaskMusicUI = () => {};
  const slow = h.context.viewAdminJob('task-1', true, true);
  await h.context.viewAdminJob('task-1', true, true);
  await h.context.viewAdminJob('task-1', true, true);
  assert.equal(h.requests.length, 1);
  h.requests[0].resolve({job: {...job('task-1'), status: 'downloading'}});
  await slow;
  assert.equal(h.context.currentJob.status, 'downloading');
  const next = h.context.viewAdminJob('task-1', true, true);
  const forced = h.context.viewAdminJob('task-1', true);
  assert.equal(h.requests.length, 3);
  h.requests[2].resolve({job: {...job('task-1'), status: 'cancelled'}});
  await forced;
  h.requests[1].resolve({job: {...job('task-1'), status: 'transcribing'}});
  await next;
  assert.equal(h.context.currentJob.status, 'cancelled');
});

test('图表颜色跟随当前 CSS 主题 token，状态与趋势保持统一', () => {
  const code = section('function chartColors(){', "window.addEventListener('resize',");
  const h = harness();
  h.context.document.documentElement = {};
  let theme = 'light';
  h.context.getComputedStyle = () => ({getPropertyValue: key => ` ${theme}:${key} `});
  const options = {};
  h.context.echarts = {init: element => ({setOption: value => { options[element.id] = value; }})};
  h.node('#chart-status').id = 'status';
  h.node('#chart-trend').id = 'trend';
  h.context.statusChart = null;
  h.context.trendChart = null;
  vm.runInContext(code, h.context);
  h.context.renderStatusChart({completed: 2, failed: 1});
  h.context.renderTrendChart([{day: '2026-11-22', count: 3}]);
  assert.equal(options.status.series[0].data[0].itemStyle.color, 'light:--success');
  assert.equal(options.status.series[0].data[1].itemStyle.color, 'light:--danger');
  assert.equal(options.status.series[0].itemStyle.borderColor, 'light:--card');
  assert.equal(options.trend.series[0].itemStyle.color, 'light:--primary');
  theme = 'dark';
  h.context.renderStatusChart({completed: 2});
  h.context.renderTrendChart([{day: '2026-11-22', count: 3}]);
  assert.equal(options.status.legend.textStyle.color, 'dark:--text-secondary');
  assert.equal(options.status.series[0].data[0].itemStyle.color, 'dark:--success');
  assert.equal(options.trend.xAxis.axisLine.lineStyle.color, 'dark:--border');
  assert.equal(options.trend.series[0].itemStyle.color, 'dark:--primary');
});

test('画廊只加载静态缩略图，原图和动图保留给点击预览', () => {
  const h = harness(section('function esc(value)', 'function renderList(') +
    section('function jobDetailHTML(', 'function renderDetail('));
  Object.assign(h.context, {
    actionLabels: {}, statusLabels: {}, statusTone: {},
    icon: name => name, previewUsable: Boolean, archiveDownloadHTML: () => '',
  });
  const images = [
    {url: 'https://cdn.example.test/raw.jpg'},
    {url: 'https://cdn.example.test/poster.jpg', animatedUrl: 'https://cdn.example.test/live.mp4'},
    {url: 'https://cdn.example.test/unsaved.jpg'},
  ];
  const files = [
    {id: 'photo', kind: 'image', name: 'image-01.jpg', previewUrl: '/preview/photo', thumbnailUrl: '/thumb/photo.webp'},
    {id: 'live', kind: 'animated', name: 'image-02.mp4', previewUrl: '/preview/live', thumbnailUrl: '/thumb/live.webp'},
  ];
  const data = {...job('note'), action: 'download', status: 'completed',
    work: {type: 'note', title: '图文', images}, result: {files}};
  const html = h.context.jobDetailHTML(data);

  assert.deepEqual([...html.matchAll(/<img src="([^"]+)"[^>]*loading="lazy"/g)].map(match => match[1]),
    ['/thumb/photo.webp', '/thumb/live.webp']);
  assert.doesNotMatch(html, /<video|src="https:\/\/cdn\.example\.test/);
  assert.match(html, /data-preview="\/preview\/photo"/);
  assert.match(html, /data-preview="\/preview\/live"/);
  assert.match(html, /data-preview="https:\/\/cdn\.example\.test\/unsaved\.jpg"/);
  assert.match(html, /<button[^>]+aria-label="播放动图 2"/);
  assert.match(html, /<button[^>]+aria-label="查看原图 3"/);
  assert.match(html, /href="\/api\/v1\/files\/live"/);

  const refreshed = {...data, result: {files: files.map(file => ({...file, thumbnailUrl: file.thumbnailUrl + '?new'}))}};
  const unavailable = {...data, result: {files: files.map(({thumbnailUrl, ...file}) => file)}};
  assert.equal(h.context.jobSignature(data), h.context.jobSignature(refreshed));
  assert.notEqual(h.context.jobSignature(data), h.context.jobSignature(unavailable));
  assert.doesNotMatch(h.context.jobDetailHTML(unavailable), /<img|<video/);
});

test('视频解析使用同源签名封面，已下载视频优先使用本地缩略图', () => {
  const h = harness(section('function esc(value)', 'function renderList(') +
    section('function jobDetailHTML(', 'function renderDetail('));
  Object.assign(h.context, {
    actionLabels: {}, statusLabels: {}, statusTone: {},
    icon: name => name, previewUsable: Boolean,
  });
  const coverPreviewUrl = '/api/v1/job-cover/video?signature=preview';
  const coverUrl = 'https://p3.douyinpic.com/video.jpg';
  const thumbnailUrl = '/api/v1/media-preview/file/thumbnail?signature=thumbnail';
  const data = {...job('video'), action: 'info', status: 'completed',
    work: {type: 'video', title: '视频', coverPreviewUrl, coverUrl}, result: {files: []}};
  assert.ok(h.context.jobDetailHTML(data).includes(`class="detail-cover" src="${coverPreviewUrl}" data-cover-fallback="${coverUrl}"`));

  data.result.files.push({id: 'file', kind: 'video', name: 'video.mp4', thumbnailUrl});
  assert.ok(h.context.jobDetailHTML(data).includes(`class="detail-cover" src="${thumbnailUrl}" data-cover-fallback="${coverPreviewUrl}"`));
  const layoutClasses = new Set();
  const image = {
    src: thumbnailUrl, parentElement: {classList: layoutClasses}, dataset: {coverFallback: coverPreviewUrl, coverSource: coverUrl},
    remove() { this.removed = true; },
  };
  h.context.retryVideoCover(image);
  assert.equal(image.src, coverPreviewUrl);
  h.context.retryVideoCover(image);
  assert.equal(image.src, coverUrl);
  h.context.retryVideoCover(image);
  assert.equal(image.removed, true);
  assert.equal(layoutClasses.has('no-cover'), true);
});

test('点开画廊时优先使用当前任务轮询得到的新签名', () => {
  const h = harness(section('function openGalleryPreview(', 'function moveGalleryPreview('));
  h.context.galleryPreview = {};
  h.context.previewUsable = Boolean;
  h.context.renderGalleryPreview = () => {};
  h.context.currentJob = {id: 'note', result: {files: [
    {id: 'live', previewUrl: '/new/video', thumbnailUrl: '/new/poster'},
  ]}};
  const item = {
    dataset: {fileId: 'live', preview: '/old/video', poster: '/old/poster', fallback: '/cdn/video'},
    closest(selector) {
      return selector === '[data-job]' ? {dataset: {job: 'note'}} : {querySelectorAll: () => [item]};
    },
  };
  h.context.openGalleryPreview(item);
  assert.equal(h.context.galleryPreview.items[0].src, '/new/video');
  assert.equal(h.context.galleryPreview.items[0].poster, '/new/poster');
  h.context.currentJob.id = 'other';
  h.context.openGalleryPreview(item);
  assert.equal(h.context.galleryPreview.items[0].src, '/old/video');
});


test('后台使用设置只保存任务额度和权限开关，保存期间阻止重复提交', async () => {
  const h = harness(captureCode);
  h.context.session.user.role = 'admin';
  h.context.adminOpen = true;
  const form = h.node('#admin-usage-form'), fields = h.node('#admin-usage-fields');
  const settings = {
    dailyTaskLimit: 10,
    allowInfo: true, allowDownload: false, allowTranscribe: true,
  };
  const inputs = Object.fromEntries(Object.keys(settings).map(name => [name, {
    value: '', checked: false, get valueAsNumber() { return Number(this.value); },
  }]));
  form.elements = {namedItem: name => inputs[name]};
  form.reportValidity = () => true;
  const load = h.context.requestAdminUsageSettings();
  assert.equal(fields.disabled, true);
  assert.equal(h.requests[0].url, '/api/v1/admin/settings/usage');
  h.requests[0].resolve({settings});
  await load;
  assert.equal(fields.disabled, false);
  assert.equal(inputs.dailyTaskLimit.value, '10');
  assert.equal(inputs.allowInfo.checked, true);
  assert.equal(inputs.allowDownload.checked, false);

  inputs.dailyTaskLimit.value = '0';
  inputs.allowDownload.checked = true;
  const event = {preventDefault() {}, currentTarget: form};
  const save = form.listeners.submit(event);
  form.listeners.submit(event);
  assert.equal(h.requests.length, 2);
  assert.equal(h.requests[1].options.method, 'PUT');
  const next = {...settings, dailyTaskLimit: 0, allowDownload: true};
  assert.deepEqual(JSON.parse(h.requests[1].options.body), next);
  h.requests[1].resolve({settings: next});
  await save;
  assert.equal(h.node('#admin-usage-status').textContent, '使用设置已保存');
  assert.equal(fields.disabled, false);
});

test('后台使用设置加载失败时保持禁用，非管理员不会提交修改', async () => {
  const h = harness(captureCode);
  h.context.session.user.role = 'admin';
  h.context.adminOpen = true;
  const pending = h.context.requestAdminUsageSettings();
  h.requests[0].reject(new Error('加载错误'));
  await pending;
  assert.equal(h.node('#admin-usage-fields').disabled, true);
  assert.equal(h.node('#admin-usage-refresh').disabled, false);
  assert.equal(h.node('#admin-usage-status').textContent, '加载错误');
  h.context.session.user.role = 'user';
  await h.context.requestAdminUsageSettings({dailyTaskLimit: 0});
  assert.equal(h.requests.length, 1);
});

test('单用户额度区分继承和不限，拒绝非法数值并阻止同一用户并发保存', async () => {
  const h = harness(section('const pendingUserTaskLimits=', 'async function showUserSessions('));
  h.context.session.user.role = 'admin';
  h.context.adminOpen = true;
  h.context.adminUsers = [{id: 'user/1', displayName: '一号用户', dailyTaskLimitOverride: 8}];
  let reloads = 0;
  h.context.loadAdminUsers = async () => { reloads++; };
  for (const [raw, value] of [['', null], ['0', 0], ['12', 12]]) {
    h.context.promptDialog = async opts => {
      assert.equal(opts.numberInput.value, 8);
      assert.equal(opts.numberInput.max, 100000);
      return raw;
    };
    const save = h.context.setUserDailyTaskLimit('user/1');
    await tick();
    await h.context.setUserDailyTaskLimit('user/1');
    assert.equal(h.requests.length, reloads + 1);
    const request = h.requests.at(-1);
    assert.equal(request.url, '/api/v1/admin/users/user%2F1/daily-task-limit');
    assert.equal(request.options.method, 'PUT');
    assert.deepEqual(JSON.parse(request.options.body), {dailyTaskLimit: value});
    request.resolve({});
    await save;
  }
  h.context.promptDialog = async () => '100001';
  await h.context.setUserDailyTaskLimit('user/1');
  h.context.promptDialog = async () => null;
  await h.context.setUserDailyTaskLimit('user/1');
  assert.equal(h.requests.length, 3);
  assert.equal(reloads, 3);
});
