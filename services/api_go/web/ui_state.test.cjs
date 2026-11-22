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
const captureCode = section("$('#capture-form').addEventListener", '/* ---------- 任务列表 ---------- */');
const selectionCode = section('function selectJob(', "$('#new-task-btn').addEventListener");
const sidebarCode = section('let sidebarLastFocus=null;', "$('#sidebar-toggle').innerHTML");

function harness(code = listCode) {
  const nodes = new Map(), requests = [], rendered = [], notices = [], details = [];
  function node(selector) {
    if (!nodes.has(selector)) nodes.set(selector, {
      value: '', checked: false, disabled: false, children: [], attributes: {}, listeners: {},
      classList: {toggle() {}, add() {}, remove() {}, contains() { return false; }},
      setAttribute(name, value) { this.attributes[name] = value; },
      addEventListener(name, callback) { this.listeners[name] = callback; },
      focus() { this.focused = true; },
      scrollIntoView() {},
    });
    return nodes.get(selector);
  }
  const context = vm.createContext({
    session: {user: {id: 'owner'}}, sessionEpoch: 1, poll: null,
    jobsPending: null, adminJobsPending: null, adminDetailPending: null,
    jobsLoadId: 0, adminJobsLoadId: 0, adminUsersLoadId: 0,
    jobsPage: 1, jobsPageSize: 20, jobsTotal: 0, jobsCache: [],
    adminJobsPage: 1, adminJobsPageSize: 20, adminJobsTotal: 0, adminJobsCache: [],
    adminUsers: [], lastAdminJobsSig: '', adminDetailLoadId: 0, adminDetailSig: '', adminMode: false,
    selectedId: null, currentJob: null, detailSig: '',
    taskMusic: {jobId: null}, adminOpen: false,
    URLSearchParams, $: node,
    document: {querySelectorAll() { return []; }},
    matchMedia() { return {matches: false}; },
    api(url, options) {
      return new Promise((resolve, reject) => requests.push({url, options, resolve, reject}));
    },
    renderList(jobs) { context.jobsCache = jobs; rendered.push(jobs); },
    renderAdminJobs(jobs) { rendered.push(jobs); },
    renderAdminUsers(users) { rendered.push(users); },
    renderPager() {}, skeletonRows() { return ''; },
    jobSignature(job) { return JSON.stringify(job); },
    esc(value) { return String(value); },
    toast(message, type) { notices.push({message, type}); },
    enter(value) { context.sessionEpoch++; context.session = value; },
    showView(name) { context.currentView = name; },
    renderDetail() { if (context.currentJob) details.push(context.currentJob); },
    stopTaskMusic() {}, setSidebar() {}, uuidv4() { return 'idempotency-key'; },
  });
  vm.runInContext(code, context);
  return {context, node, requests, rendered, notices, details};
}

function job(id) { return {id, status: 'queued', progress: 0}; }
const tick = async () => { await Promise.resolve(); await Promise.resolve(); };

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


test('后台详情在现有轮询中刷新当前任务', () => {
  const h = harness(section('function enter(value){', '/* ---------- 登录 / 退出 / 连接 ---------- */'));
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

test('视频预览签名过期后，后台走管理员文件接口，前台走普通接口', async () => {
  for (const inAdmin of [false, true]) {
    const h = harness(section('async function openPreview(', '// 画廊点击查看：'));
    h.context.adminMode = inAdmin;
    h.context.previewURL = null;
    h.context.taskMusic = {playing: false};
    h.context.galleryPreview = {items: []};
    h.context.document.body = {classList: {add() {}}};
    h.context.setPreviewNav = () => {};
    h.context.resetPreviewMedia = () => {};
    h.context.previewUsable = () => false;
    h.context.URL = {createObjectURL() { return 'blob:preview'; }, revokeObjectURL() {}};
    h.context.fetch = async (url, options) => {
      h.context.downloadRequest = {url, options};
      return {ok: true, blob: async () => ({})};
    };
    h.context.closePreview = () => { throw new Error('有效文件不应关闭预览'); };
    h.node('#preview-video').classList = {add() {}, remove() {}};
    h.node('#preview-modal').classList = {add() {}, remove() {}};
    h.node('#preview-video').play = async () => {};
    await h.context.openPreview('file/id', 'example.mp4', '/expired');
    assert.equal(h.context.downloadRequest.url, `${inAdmin ? '/api/v1/admin/files' : '/api/v1/files'}/file%2Fid`);
    assert.equal(h.node('#preview-video').src, 'blob:preview');
    assert.equal(h.notices.length, 0);
  }
});


test('任务列表慢请求跨过 5 秒轮询后仍能渲染，完成后恢复下一轮请求', async () => {
  const h = harness(section('function enter(value){', '/* ---------- 登录 / 退出 / 连接 ---------- */') + listCode);
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
