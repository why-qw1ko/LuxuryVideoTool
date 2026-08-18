const $=s=>document.querySelector(s);
let session=null,poll=null;
let selectedId=null,currentJob=null,jobsCache=[],detailSig='';
let adminMode=false,currentView='new',adminOpen=false,adminCurrentView='dashboard',adminJobsCache=[];
let jobsPage=1,jobsPageSize=20,jobsTotal=0;
let adminJobsPage=1,adminJobsPageSize=20,adminJobsTotal=0;
let taskMusic={jobId:null,source:'',url:'',playing:false};
let statusChart=null,trendChart=null;
let lastStatsSig='',lastAdminJobsSig='';

/* ---------- 主题（仅浅色 / 深色） ---------- */
const THEMES=['light','dark'];
let theme=localStorage.getItem('dc_theme');
if(THEMES.indexOf(theme)<0)theme=matchMedia('(prefers-color-scheme:dark)').matches?'dark':'light';

const actionLabels={info:'仅解析',download:'下载媒体',transcribe:'提取文案',full:'提取内容'};
const statusLabels={queued:'排队中',resolving:'解析信息',downloading:'下载中',extracting:'提取音频',transcribing:'识别中',postprocessing:'生成结果',completed:'已完成',failed:'失败',retry_wait:'等待重试',cancelled:'已取消'};
const statusTone={queued:'muted',resolving:'active',downloading:'active',extracting:'active',transcribing:'active',postprocessing:'active',retry_wait:'warning',completed:'success',failed:'danger',cancelled:'danger'};

/* ---------- 线性图标库（Lucide 风格，内联 SVG，零外部依赖） ---------- */
const ICONS={
  sun:'<circle cx="12" cy="12" r="4"/><path d="M12 2v2"/><path d="M12 20v2"/><path d="m4.93 4.93 1.41 1.41"/><path d="m17.66 17.66 1.41 1.41"/><path d="M2 12h2"/><path d="M20 12h2"/><path d="m6.34 17.66-1.41 1.41"/><path d="m19.07 4.93-1.41 1.41"/>',
  moon:'<path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z"/>',
  plus:'<path d="M5 12h14"/><path d="M12 5v14"/>',
  'refresh-cw':'<path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M8 16H3v5"/>',
  settings:'<path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/>',
  'log-out':'<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" x2="9" y1="12" y2="12"/>',
  search:'<circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/>',
  play:'<polygon points="6 3 20 12 6 21 6 3"/>',
  copy:'<rect width="14" height="14" x="8" y="8" rx="2" ry="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/>',
  check:'<path d="M20 6 9 17l-5-5"/>',
  x:'<path d="M18 6 6 18"/><path d="m6 6 12 12"/>',
  download:'<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" x2="12" y1="15" y2="3"/>',
  trash:'<path d="M3 6h18"/><path d="M19 6v14c0 1-1 2-2 2H7c-1 0-2-1-2-2V6"/><path d="M8 6V4c0-1 1-2 2-2h4c1 0 2 1 2 2v2"/><line x1="10" x2="10" y1="11" y2="17"/><line x1="14" x2="14" y1="11" y2="17"/>',
  'rotate-ccw':'<path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"/><path d="M3 3v5h5"/>',
  'circle-slash':'<circle cx="12" cy="12" r="10"/><line x1="9.15" x2="14.85" y1="9.15" y2="14.85"/>',
  'arrow-up-right':'<path d="M7 7h10v10"/><path d="M7 17 17 7"/>',
  inbox:'<path d="M22 12h-6l-2 3h-4l-2-3H2"/><path d="M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"/>',
  'file-text':'<path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/><path d="M10 9H8"/><path d="M16 13H8"/><path d="M16 17H8"/>',
  'alert-circle':'<circle cx="12" cy="12" r="10"/><line x1="12" x2="12" y1="8" y2="12"/><line x1="12" x2="12.01" y1="16" y2="16"/>',
  'check-circle':'<path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><path d="m9 11 3 3L22 4"/>',
  'alert-triangle':'<path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z"/><path d="M12 9v4"/><path d="M12 17h.01"/>',
  info:'<circle cx="12" cy="12" r="10"/><path d="M12 16v-4"/><path d="M12 8h.01"/>',
  eye:'<path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z"/><circle cx="12" cy="12" r="3"/>',
  'eye-off':'<path d="M9.88 9.88a3 3 0 1 0 4.24 4.24"/><path d="M10.73 5.08A10.43 10.43 0 0 1 12 5c7 0 10 7 10 7a13.16 13.16 0 0 1-1.67 2.68"/><path d="M6.61 6.61A13.526 13.526 0 0 0 2 12s3 7 10 7a9.74 9.74 0 0 0 5.39-1.61"/><line x1="2" x2="22" y1="2" y2="22"/>',
  'volume-2':'<path d="M11 5 6 9H2v6h4l5 4V5Z"/><path d="M15.54 8.46a5 5 0 0 1 0 7.07"/><path d="M19.07 4.93a10 10 0 0 1 0 14.14"/>',
  'volume-x':'<path d="M11 5 6 9H2v6h4l5 4V5Z"/><line x1="22" x2="16" y1="9" y2="15"/><line x1="16" x2="22" y1="9" y2="15"/>',
  music:'<path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/>',
  menu:'<line x1="4" x2="20" y1="6" y2="6"/><line x1="4" x2="20" y1="12" y2="12"/><line x1="4" x2="20" y1="18" y2="18"/>',
  'arrow-up':'<path d="m5 12 7-7 7 7"/><path d="M12 19V5"/>',
  'chevron-left':'<path d="m15 18-6-6 6-6"/>',
  'chevron-right':'<path d="m9 18 6-6-6-6"/>'
};
function icon(name,size=18){
  return `<svg class="ic" width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${ICONS[name]||''}</svg>`;
}

function show(id,on){$(id).classList.toggle('hidden',!on)}
// 网页端安全方案：access token 只保存在内存，refresh token 存服务端 httpOnly Cookie。
// 页面刷新后通过 /api/v1/auth/refresh 用 Cookie 静默恢复会话，JS 不再接触任何长效凭证。
function save(value){session=value}
function normaliseAPI(value){const url=new URL(value);if(!['http:','https:'].includes(url.protocol)||url.username||url.password||url.search||url.hash)throw new Error('API 地址必须是有效的 HTTP/HTTPS 地址');url.pathname=url.pathname.replace(/\/+$/,'')||'/';return url.origin+url.pathname.replace(/\/$/,'')}
/* crypto.randomUUID 仅在安全上下文（HTTPS 或 localhost）可用；内网穿透的 http 地址不可用，需兜底 */
function uuidv4(){
  if(typeof crypto!=='undefined'&&crypto.randomUUID)return crypto.randomUUID();
  if(typeof crypto!=='undefined'&&crypto.getRandomValues){
    const b=crypto.getRandomValues(new Uint8Array(16));
    b[6]=(b[6]&0x0f)|0x40;b[8]=(b[8]&0x3f)|0x80;
    const h=Array.from(b,x=>x.toString(16).padStart(2,'0')).join('');
    return `${h.slice(0,8)}-${h.slice(8,12)}-${h.slice(12,16)}-${h.slice(16,20)}-${h.slice(20)}`;
  }
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g,c=>{const r=Math.random()*16|0;return(c==='x'?r:(r&0x3|0x8)).toString(16)});
}

// 共享的续期 Promise：多个并发 401 只触发一次 refresh，避免 refresh token 轮换时互相把会话刷失效。
let refreshing=null;
async function refreshSession(){
  if(!refreshing){
    refreshing=fetch('/api/v1/auth/refresh',{method:'POST'}).then(async response=>{
      if(response.ok){save(await response.json());return true}
      save(null);return false;
    }).finally(()=>{refreshing=null});
  }
  return refreshing;
}
async function api(path,options={},retry=true){
  options.headers={...(options.headers||{}),'Content-Type':'application/json'};
  if(session?.accessToken)options.headers.Authorization=`Bearer ${session.accessToken}`;
  let response=await fetch(path,options);
  if(response.status===401&&retry){
    if(await refreshSession())return api(path,options,false);
    throw new Error('登录已过期，请重新登录');
  }
  const body=await response.json().catch(()=>({}));
  if(!response.ok)throw new Error(body.error?.message||`请求失败 (${response.status})`);
  return body;
}

/* ---------- Toast 通知 ---------- */
function toast(message,type='info'){
  const wrap=$('#toasts');
  const el=document.createElement('div');
  el.className=`toast toast-${type}`;
  const tic={success:'check-circle',error:'alert-circle',warning:'alert-triangle',info:'info'};
  el.innerHTML=`<span class="toast-ic">${icon(tic[type]||'info',16)}</span><span class="toast-msg">${esc(message)}</span>`;
  const close=document.createElement('button');
  close.type='button';close.className='toast-close';close.innerHTML=icon('x',14);close.setAttribute('aria-label','关闭');
  let timer;
  const dismiss=()=>{el.classList.remove('show');setTimeout(()=>el.remove(),260)};
  close.addEventListener('click',dismiss);
  el.appendChild(close);
  wrap.appendChild(el);
  requestAnimationFrame(()=>el.classList.add('show'));
  timer=setTimeout(dismiss,type==='error'?6000:3200);
  el.addEventListener('mouseenter',()=>clearTimeout(timer));
  el.addEventListener('mouseleave',()=>{clearTimeout(timer);timer=setTimeout(dismiss,600)});
}

/* ---------- 主题切换 ---------- */
function applyTheme(){
  document.documentElement.dataset.theme=theme;
  document.documentElement.style.colorScheme=theme;
  const t=$('#theme-toggle');
  t.innerHTML=theme==='dark'?icon('sun',18):icon('moon',18);
  t.title=theme==='dark'?'切换到浅色主题':'切换到深色主题';
  t.setAttribute('aria-label',t.title);
}
$('#theme-toggle').addEventListener('click',()=>{theme=theme==='dark'?'light':'dark';localStorage.setItem('dc_theme',theme);applyTheme();if(adminOpen&&adminCurrentView==='dashboard'){lastStatsSig='';loadAdminStats()}});
applyTheme();

/* ---------- 移动端收缩侧边栏 ---------- */
let sidebarLastFocus=null;
function setSidebar(open){
  const frontSidebar=$('.master-list'),adminSidebar=$('.admin-sidebar');
  const target=adminOpen?adminSidebar:frontSidebar;
  if(open)sidebarLastFocus=document.activeElement;
  if(frontSidebar)frontSidebar.classList.toggle('open',!adminOpen&&open);
  if(adminSidebar)adminSidebar.classList.toggle('open',adminOpen&&open);
  $('#sidebar-backdrop').classList.toggle('hidden',!open);
  $('#sidebar-toggle').setAttribute('aria-expanded',open?'true':'false');
  $('#sidebar-toggle').setAttribute('aria-label',open?'关闭菜单':'打开菜单');
  if(frontSidebar)frontSidebar.setAttribute('aria-hidden',(!open||adminOpen)?'true':'false');
  if(adminSidebar)adminSidebar.setAttribute('aria-hidden',(!open||!adminOpen)?'true':'false');
  if(open&&target){
    requestAnimationFrame(()=>target.querySelector('button,a,input,select,textarea')?.focus());
  }else if(sidebarLastFocus&&document.contains(sidebarLastFocus)){
    sidebarLastFocus.focus();
  }
}
$('#sidebar-toggle').innerHTML=icon('menu',18);
$('#sidebar-toggle').setAttribute('aria-label','打开菜单');
$('#sidebar-toggle').addEventListener('click',()=>{
  const sidebar=adminOpen?$('.admin-sidebar'):$('.master-list');
  setSidebar(!sidebar?.classList.contains('open'));
});
$('#sidebar-backdrop').addEventListener('click',()=>setSidebar(false));

/* ---------- 返回顶部 ---------- */
// PC 应用壳下页面不整体滚动，真正的滚动容器是右侧 .content；移动端仍是 window。
function mainScroller(){
  if(matchMedia('(min-width:961px)').matches&&document.querySelector('.content'))return document.querySelector('.content');
  return window;
}
function updateBackTop(){
  const s=mainScroller();
  const y=s===window?window.scrollY:s.scrollTop;
  $('#back-top').classList.toggle('hidden',y<400);
}
$('#back-top').innerHTML=icon('arrow-up',18);
$('#back-top').addEventListener('click',()=>mainScroller().scrollTo({top:0,behavior:'smooth'}));
window.addEventListener('scroll',updateBackTop,{passive:true});
const __contentEl=document.querySelector('.content');
if(__contentEl)__contentEl.addEventListener('scroll',updateBackTop,{passive:true});

/* ---------- 视图 / 选中 ---------- */
function showView(name){
  currentView=name;
  ['new','detail','settings'].forEach(v=>show('#view-'+v,v===name));
  $('#new-task-btn').classList.toggle('active',name==='new');
  $('#settings-link').classList.toggle('active',name==='settings');
  if(name==='settings')loadProviders();
}
function selectJob(id){
  if(selectedId!==id){
    if(taskMusic.jobId&&taskMusic.jobId!==id)stopTaskMusic();
    selectedId=id;detailSig='';currentJob=id?(jobsCache.find(j=>j.id===id)||null):null;
  }
  document.querySelectorAll('.job-row').forEach(r=>r.classList.toggle('selected',r.dataset.id===id));
  showView(id?'detail':'new');
  if(!id)stopTaskMusic();
  renderDetail();
  if(matchMedia('(max-width:960px)').matches)setSidebar(false);
  if(id&&matchMedia('(max-width:960px)').matches)$('#view-detail').scrollIntoView({behavior:'smooth',block:'start'});
}
$('#new-task-btn').addEventListener('click',()=>selectJob(null));
$('#settings-link').addEventListener('click',e=>{e.preventDefault();selectJob(null);showView('settings')});

/* ---------- 后台管理系统 ---------- */
function openAdminConsole(){
  if(adminOpen){
    toast('已在后台','info');
    return;
  }
  setSidebar(false);
  adminOpen=true;adminMode=false;selectedId=null;currentJob=null;
  lastStatsSig=''; // 重新打开时按当前主题重绘图表，避免沿用旧配色
  show('#app',false);show('#admin-console',true);
  adminShowView('dashboard');
}
function closeAdminConsole(){
  setSidebar(false);
  adminOpen=false;adminMode=false;
  show('#admin-console',false);show('#app',true);
  selectJob(null);
}
function adminShowView(name){
  adminCurrentView=name;
  ['dashboard','jobs','users','detail'].forEach(v=>show('#admin-view-'+v,v===name));
  document.querySelectorAll('.admin-nav [data-admin-view]').forEach(btn=>btn.classList.toggle('active',btn.dataset.adminView===name));
  if(name==='dashboard')loadAdminStats();
  if(name==='jobs'){loadAdminUsers();loadAdminJobs()}
  if(name==='users')loadAdminUsers();
}
$('#admin-console-btn').addEventListener('click',openAdminConsole);
$('#admin-back-front').addEventListener('click',closeAdminConsole);
$('#admin-refresh-stats').addEventListener('click',async e=>{
  const btn=e.currentTarget;
  btn.disabled=true;
  btn.classList.add('loading');
  try{await loadAdminStats()}finally{btn.disabled=false;btn.classList.remove('loading')}
});
$('#admin-detail-back').addEventListener('click',()=>adminShowView('jobs'));
document.querySelectorAll('.admin-nav [data-admin-view]').forEach(btn=>btn.addEventListener('click',()=>{
  adminShowView(btn.dataset.adminView);
  if(matchMedia('(max-width:960px)').matches)setSidebar(false);
}));

function enter(value){
  save(value);
  adminMode=false;currentView='new';adminOpen=false;jobsPage=1;adminJobsPage=1;
  if($('#admin-user-name'))$('#admin-user-name').textContent=value?.user?.displayName||'';
  if($('#admin-user-role'))$('#admin-user-role').textContent=value?.user?.role==='admin'?'管理员':'普通用户';
  if($('#admin-user-avatar'))$('#admin-user-avatar').textContent=(value?.user?.displayName||'?').trim().charAt(0).toUpperCase();
  show('#login',!value);show('#app',!!value);show('#logout',!!value);show('#admin-console',false);
  // 未登录时隐藏左上角菜单按钮：导航抽屉是登录后的功能，登录页点了只会打开空的抽屉。
  // 用 visibility 而非 display，保留其在顶栏网格中的 40px 占位，避免品牌文字被顶位裁剪。
  $('#sidebar-toggle').classList.toggle('nav-hidden',!value);
  document.querySelectorAll('.admin-only').forEach(el=>el.classList.toggle('hidden',value?.user?.role!=='admin'));
  if(value){
    selectJob(null);
    if(value.user?.role==='admin')loadProviders();
    loadJobs();
    poll=setInterval(()=>{
      if(adminOpen){
        if(adminCurrentView==='dashboard')loadAdminStats();
        if(adminCurrentView==='jobs')loadAdminJobs();
      }else loadJobs();
    },5000);
  }else{
    clearInterval(poll);
    stopTaskMusic();
    setSidebar(false); // 退出登录时若抽屉还开着，一并收起
  }
}

/* ---------- 登录 / 退出 / 连接 ---------- */
$('#api-url').value=location.origin;
if($('#current-api'))$('#current-api').textContent=location.origin;
$('#api-form').addEventListener('submit',e=>{e.preventDefault();try{const target=normaliseAPI($('#api-url').value);if(target===location.origin)return toast('当前已连接此 API 服务');location.assign(`${target}/`)}catch(err){toast(err.message,'error')}});
$('#login-form').addEventListener('submit',async e=>{e.preventDefault();try{const data=await api('/api/v1/auth/login',{method:'POST',body:JSON.stringify({username:$('#username').value.trim(),password:$('#password').value,device:{id:localStorage.dc_device||(localStorage.dc_device=uuidv4()),name:navigator.platform||'Web 浏览器',platform:'windows',appVersion:'web-0.1.0'}})},false);$('#password').value='';enter(data)}catch(err){toast(err.message,'error')}});
function bindPasswordToggle(inputId,buttonId){
  const input=$(inputId),btn=$(buttonId);
  if(!input||!btn)return;
  btn.innerHTML=icon('eye',16);
  btn.addEventListener('click',e=>{
    e.preventDefault();e.stopPropagation();
    const show=input.type==='password';
    input.type=show?'text':'password';
    btn.classList.toggle('on',show);
    btn.setAttribute('aria-label',show?'隐藏密码':'显示密码');
    btn.title=show?'隐藏密码':'显示密码';
    btn.innerHTML=icon(show?'eye-off':'eye',16);
    input.focus();
  });
}
bindPasswordToggle('#password','#toggle-password');
$('#logout').addEventListener('click',async()=>{if(!(await confirmDialog({title:'退出登录',message:'确认退出当前账号？',confirmText:'退出',danger:false})))return;try{await api('/api/v1/auth/logout',{method:'POST'})}catch{}enter(null)});

/* ---------- API Key 设置 ---------- */
async function loadProviders(){try{const data=await api('/api/v1/admin/settings/providers'),p=data.providers;$('#aliyun-key').placeholder=`当前状态：${p.aliyunConfigured?(p.aliyunAvailable?'已配置，可作备用':'已配置，但缺少公网地址'):'未配置'}`;$('#silicon-key').placeholder=`当前状态：${p.siliconFlowConfigured?'已配置，默认使用':'未配置'}`;if($('#asr-model')){$('#asr-model').value=p.asrModel||'FunAudioLLM/SenseVoiceSmall';$('#asr-model').placeholder=`当前模型：${p.asrModel||'FunAudioLLM/SenseVoiceSmall'}`}}catch(err){toast(err.message,'error')}}
$('#provider-form').addEventListener('submit',async e=>{e.preventDefault();const body={};const model=$('#asr-model')?.value.trim();if(model)body.asrModel=model;if($('#clear-aliyun').checked)body.aliyunApiKey='';else if($('#aliyun-key').value.trim())body.aliyunApiKey=$('#aliyun-key').value.trim();if($('#clear-silicon').checked)body.siliconFlowApiKey='';else if($('#silicon-key').value.trim())body.siliconFlowApiKey=$('#silicon-key').value.trim();if(!Object.keys(body).length)return toast('请输入需要保存的 Key、模型，或勾选清除','warning');try{await api('/api/v1/admin/settings/providers',{method:'PUT',body:JSON.stringify(body)});$('#aliyun-key').value=$('#silicon-key').value='';$('#clear-aliyun').checked=$('#clear-silicon').checked=false;toast('配置已安全保存并立即生效','success');await loadProviders()}catch(err){toast(err.message,'error')}});

/* ---------- 新建任务 ---------- */
$('#submit-task').innerHTML=icon('arrow-up-right',14)+' 提取内容';
$('#capture-form').addEventListener('submit',async e=>{e.preventDefault();const button=e.submitter;button.disabled=true;try{const action=$('#info-only').checked?'info':'full';const data=await api('/api/v1/jobs',{method:'POST',headers:{'Idempotency-Key':uuidv4()},body:JSON.stringify({shareText:$('#share-text').value.trim(),action,options:{force:false,keepVideo:action==='full',languageHints:['zh','en'],hotwords:[]}})});$('#share-text').value='';await loadJobs();if(data.job?.id)selectJob(data.job.id);toast('任务已创建，正在处理','success')}catch(err){toast(err.message,'error')}finally{button.disabled=false}});

/* ---------- 任务列表 ---------- */
$('#search-form').addEventListener('submit',e=>{e.preventDefault();jobsPage=1;loadJobs()});
$('#refresh').addEventListener('click',loadJobs);
$('#status').addEventListener('change',()=>{jobsPage=1;loadJobs()});
async function loadJobs(){if(!session)return;try{const q=new URLSearchParams({q:$('#query').value.trim(),status:$('#status').value,limit:String(jobsPageSize),offset:String((jobsPage-1)*jobsPageSize)});const data=await api(`/api/v1/jobs?${q}`);jobsTotal=data.total||0;renderList(data.jobs||[]);if($('#job-total'))$('#job-total').textContent=`${jobsTotal} 个任务`;renderPager($('#job-pager'),jobsPage,Math.max(1,Math.ceil(jobsTotal/jobsPageSize)),p=>{jobsPage=p;loadJobs()})}catch(err){if(err.message.includes('登录'))enter(null);else toast(err.message,'error')}}
// 通用分页条：onPage 接收目标页码；单页时不渲染。
function renderPager(el,page,totalPages,onPage){
  if(!el)return;
  if(totalPages<=1){el.innerHTML='';return}
  el.innerHTML=`<button type="button" class="ghost" data-pager="${page-1}" ${page<=1?'disabled':''}>上一页</button><span class="pager-info">${page} / ${totalPages}</span><button type="button" class="ghost" data-pager="${page+1}" ${page>=totalPages?'disabled':''}>下一页</button>`;
  el.onclick=e=>{
    const btn=e.target.closest('button[data-pager]');
    if(!btn||btn.disabled)return;
    onPage(Number(btn.dataset.pager));
  };
}

/* ---------- 后台仪表盘 ---------- */
let adminUsers=[];
function skeletonRows(count=4){
  return `<div class="skeleton-list" aria-hidden="true">${Array.from({length:count},()=>`<div class="skeleton-row"><span></span><span></span><span></span></div>`).join('')}</div>`;
}
async function loadAdminStats(){
  if(!session)return;
  try{
    if(!$('#admin-stats-cards').children.length)$('#admin-stats-cards').innerHTML=skeletonRows(4);
    document.querySelectorAll('.chart-card').forEach(card=>card.classList.add('loading'));
    const data=await api('/api/v1/admin/stats');
    const stats=data.stats||{};
    // 数据未变化时不重绘，避免轮询导致图表/卡片闪烁
    const sig=JSON.stringify([stats.users,stats.activeUsers,stats.totalJobs,stats.todayJobs,stats.byStatus,stats.byDay]);
    if(sig===lastStatsSig){document.querySelectorAll('.chart-card').forEach(card=>card.classList.remove('loading'));return}
    lastStatsSig=sig;
    renderStatCards(stats);
    renderStatusChart(stats.byStatus||{});
    renderTrendChart(stats.byDay||[]);
    document.querySelectorAll('.chart-card').forEach(card=>card.classList.remove('loading'));
    // 图表高度随布局自适应，绘制完成后校准一次尺寸
    requestAnimationFrame(()=>{if(statusChart)statusChart.resize();if(trendChart)trendChart.resize()});
  }catch(err){document.querySelectorAll('.chart-card').forEach(card=>card.classList.remove('loading'));toast(err.message,'error')}
}
function renderStatCards(stats){
  const cards=[
    {label:'用户总数',value:stats.users??'-'},
    {label:'启用账号',value:stats.activeUsers??'-'},
    {label:'任务总数',value:stats.totalJobs??'-'},
    {label:'今日任务',value:stats.todayJobs??'-'},
  ];
  $('#admin-stats-cards').innerHTML=cards.map(card=>`<div class="stat-card"><span>${esc(card.label)}</span><strong>${esc(card.value)}</strong></div>`).join('');
}
function chartColors(){
  const dark=document.documentElement.dataset.theme==='dark';
  return {text:dark?'#A2A8BB':'#5C6474',axis:dark?'#2A2E38':'#DFE3EA',grid:dark?'#20232B':'#F3F4F6'};
}
function renderStatusChart(byStatus){
  const el=$('#chart-status');
  if(typeof echarts==='undefined'){el.innerHTML='<p class="meta">ECharts 未加载</p>';return}
  if(!statusChart)statusChart=echarts.init(el);
  const labels={queued:'排队中',resolving:'解析信息',downloading:'下载中',extracting:'提取音频',transcribing:'识别中',postprocessing:'生成结果',completed:'已完成',failed:'失败',retry_wait:'等待重试',cancelled:'已取消'};
  const colors={queued:'#9AA1B2',resolving:'#335CFF',downloading:'#3D5AFE',extracting:'#7D94FF',transcribing:'#7D94FF',postprocessing:'#7D94FF',completed:'#2FA96A',failed:'#E5484D',retry_wait:'#E8912D',cancelled:'#5C6474'};
  const data=Object.entries(byStatus).map(([key,value])=>({name:labels[key]||key,value,itemStyle:{color:colors[key]||'#335CFF'}})).filter(d=>d.value>0);
  const c=chartColors();
  statusChart.setOption({
    color:Object.values(colors),
    animation:true,
    animationDuration:260,
    animationEasing:'cubicOut',
    animationDurationUpdate:180,
    animationEasingUpdate:'cubicOut',
    tooltip:{trigger:'item',formatter:'{b}: {c} ({d}%)'},
    legend:{bottom:0,textStyle:{color:c.text}},
    series:[{type:'pie',radius:['38%','64%'],center:['50%','44%'],avoidLabelOverlap:true,itemStyle:{borderColor:document.documentElement.dataset.theme==='dark'?'#17191F':'#FFFFFF',borderWidth:2},label:{color:c.text},data:data.length?data:[{name:'暂无任务',value:1,itemStyle:{color:c.grid}}]}]
  },true);
}
function renderTrendChart(byDay){
  const el=$('#chart-trend');
  if(typeof echarts==='undefined'){el.innerHTML='<p class="meta">ECharts 未加载</p>';return}
  if(!trendChart)trendChart=echarts.init(el);
  const c=chartColors();
  trendChart.setOption({
    animation:true,
    animationDuration:280,
    animationEasing:'cubicOut',
    animationDurationUpdate:180,
    animationEasingUpdate:'cubicOut',
    tooltip:{trigger:'axis'},
    grid:{left:36,right:16,top:24,bottom:28},
    xAxis:{type:'category',data:byDay.map(d=>d.day.slice(5)),axisLine:{lineStyle:{color:c.axis}},axisLabel:{color:c.text}},
    yAxis:{type:'value',minInterval:1,splitLine:{lineStyle:{color:c.axis}},axisLabel:{color:c.text}},
    series:[{name:'任务数',type:'line',smooth:true,symbolSize:6,data:byDay.map(d=>d.count),itemStyle:{color:'#335CFF'},lineStyle:{width:3},areaStyle:{color:{type:'linear',x:0,y:0,x2:0,y2:1,colorStops:[{offset:0,color:'rgba(51,92,255,.28)'},{offset:1,color:'rgba(51,92,255,.02)'}]}}}]
  },true);
}
window.addEventListener('resize',()=>{if(statusChart)statusChart.resize();if(trendChart)trendChart.resize()});

$('#admin-jobs-form').addEventListener('submit',e=>{e.preventDefault();adminJobsPage=1;loadAdminJobs()});
async function loadAdminJobs(){
  if(!session)return;
  const q=new URLSearchParams({q:$('#admin-jobs-q').value.trim(),status:$('#admin-jobs-status').value,userId:$('#admin-jobs-user').value,limit:String(adminJobsPageSize),offset:String((adminJobsPage-1)*adminJobsPageSize)});
  try{
    const root=$('#admin-jobs-list');
    if(root&&!root.children.length)root.innerHTML=skeletonRows(6);
    const data=await api(`/api/v1/admin/jobs?${q}`);
    adminJobsCache=data.jobs||[];
    adminJobsTotal=data.total||0;
    // 数据与筛选条件未变化时不重绘表格，避免轮询闪烁
    const sig=JSON.stringify([String(q),adminJobsTotal,(adminJobsCache||[]).map(jobSignature).join('|')]);
    if(sig===lastAdminJobsSig)return;
    lastAdminJobsSig=sig;
    renderAdminJobs(adminJobsCache);
    if($('#admin-jobs-total'))$('#admin-jobs-total').textContent=`共 ${adminJobsTotal} 个任务`;
    renderPager($('#admin-jobs-pager'),adminJobsPage,Math.max(1,Math.ceil(adminJobsTotal/adminJobsPageSize)),p=>{adminJobsPage=p;loadAdminJobs()});
  }catch(err){toast(err.message,'error')}
}
function adminJobOps(job,wrap=true){
  const terminal=['completed','failed','cancelled'].includes(job.status);
  const ops=[`<button type="button" data-op="view">${icon('search',13)} 查看</button>`];
  if(!terminal)ops.push(`<button type="button" data-op="cancel">${icon('circle-slash',13)} 取消</button>`);
  if(job.status==='failed'||job.status==='cancelled')ops.push(`<button type="button" data-op="retry">${icon('rotate-ccw',13)} 重试</button>`);
  if(terminal)ops.push(`<button type="button" data-op="delete" class="danger">${icon('trash',13)} 删除</button>`);
  return wrap?`<div class="admin-ops-row">${ops.join('')}</div>`:ops.join('');
}
function adminJobCardHTML(job){
  const w=job.work||{};
  const title=stripHashtags(w.title,w.hashtags)||`任务 ${job.id.slice(0,8)}`;
  const tone=statusTone[job.status]||'muted';
  const owner=job.ownerDisplayName||job.ownerUsername||'-';
  return `<article class="admin-card" data-id="${esc(job.id)}">
    <div class="admin-card-head"><strong>${esc(title)}</strong><span class="badge badge-${tone}">${esc(statusLabels[job.status]||job.status)}</span></div>
    <div class="admin-card-meta">
      <span>创建：${fmtShort(job.createdAt)}</span>
      <span>用户：${esc(owner)}</span>
      <span>类型：${esc(actionLabels[job.action]||job.action)}</span>
    </div>
    <div class="admin-card-actions">${adminJobOps(job,false)}</div>
  </article>`;
}
function renderAdminJobs(jobs){
  const root=$('#admin-jobs-list');
  if(!jobs.length){root.innerHTML=`<div class="empty-state">${icon('inbox',36)}<p>暂无任务</p><span>调整筛选条件后重试</span></div>`;return}
  const table=`<div class="admin-table-wrap"><table class="admin-table admin-jobs-table"><thead><tr><th>创建时间</th><th>用户</th><th>任务</th><th>类型</th><th>状态</th><th>操作</th></tr></thead><tbody>`+jobs.map(job=>{
    const w=job.work||{};
    const title=stripHashtags(w.title,w.hashtags)||`任务 ${job.id.slice(0,8)}`;
    const tone=statusTone[job.status]||'muted';
    const owner=job.ownerDisplayName||job.ownerUsername||'-';
    return `<tr data-id="${esc(job.id)}">
      <td class="nowrap">${fmtShort(job.createdAt)}</td>
      <td class="nowrap">${esc(owner)}</td>
      <td class="admin-title">${esc(title)}</td>
      <td>${esc(actionLabels[job.action]||job.action)}</td>
      <td class="nowrap"><span class="dot dot-${tone}"></span>${esc(statusLabels[job.status]||job.status)}</td>
      <td class="admin-ops nowrap">${adminJobOps(job)}</td>
    </tr>`;
  }).join('')+`</tbody></table></div>`;
  root.innerHTML=table+`<div class="admin-card-list">${jobs.map(adminJobCardHTML).join('')}</div>`;
}
$('#admin-view-jobs').addEventListener('click',e=>{
  const btn=e.target.closest('button[data-op]');
  if(!btn)return;
  const row=btn.closest('tr[data-id],.admin-card[data-id]');
  if(!row)return;
  if(btn.dataset.op==='view')viewAdminJob(row.dataset.id);
  else operateAdminJob(row.dataset.id,btn.dataset.op);
});
async function viewAdminJob(id){
  try{
    const data=await api(`/api/v1/admin/jobs/${encodeURIComponent(id)}`);
    adminMode=true;currentJob=data.job;
    $('#admin-detail').innerHTML=jobDetailHTML(data.job);
    syncTaskMusicWithJob(data.job);updateTaskMusicUI();
    adminShowView('detail');
    if(matchMedia('(max-width:960px)').matches)setSidebar(false);
  }catch(err){toast(err.message,'error')}
}
async function operateAdminJob(id,op){
  if(op==='delete'&&!(await confirmDialog({title:'删除任务',message:'确认删除该任务及其文件？此操作不可撤销。',confirmText:'删除'})))return;
  try{
    await api(`/api/v1/admin/jobs/${encodeURIComponent(id)}${op==='delete'?'':`/${op}`}`,{method:op==='delete'?'DELETE':'POST'});
    toast(op==='delete'?'任务已删除':op==='cancel'?'已请求取消':'已重新开始','success');
    if(op==='delete'){
      adminMode=false;
      if(adminJobsPage>1&&adminJobsTotal-1<=(adminJobsPage-1)*adminJobsPageSize)adminJobsPage--;
      adminShowView('jobs');
    }
    else await loadAdminJobs();
  }catch(err){toast(err.message,'error')}
}

/* ---------- 管理后台：用户管理 ---------- */
$('#user-create-form').addEventListener('submit',async e=>{
  e.preventDefault();
  const password=$('#user-password').value;
  if(password.length<12)return toast('密码至少需要 12 位','warning');
  try{
    await api('/api/v1/admin/users',{method:'POST',body:JSON.stringify({username:$('#user-username').value.trim(),displayName:$('#user-display').value.trim(),password,role:$('#user-role').value})});
    e.target.reset();toast('用户已创建','success');await loadAdminUsers();
  }catch(err){toast(err.message,'error')}
});
async function loadAdminUsers(){
  if(!session)return;
  try{
    const root=$('#admin-users-list');
    if(root&&!root.children.length)root.innerHTML=skeletonRows(5);
    const data=await api('/api/v1/admin/users');
    adminUsers=data.users||[];
    renderAdminUsers(adminUsers);
    const select=$('#admin-jobs-user');
    if(select)select.innerHTML=`<option value="">全部用户</option>`+adminUsers.map(u=>`<option value="${esc(u.id)}">${esc(u.displayName||u.username)}</option>`).join('');
  }catch(err){toast(err.message,'error')}
}
function renderAdminUsers(users){
  const root=$('#admin-users-list');
  const table=`<div class="admin-table-wrap"><table class="admin-table"><thead><tr><th>用户名</th><th>显示名</th><th>角色</th><th>状态</th><th>最近登录</th><th>创建时间</th><th>操作</th></tr></thead><tbody>`+users.map(u=>`<tr data-id="${esc(u.id)}">
    <td class="nowrap">${esc(u.username)}</td>
    <td>${esc(u.displayName)}</td>
    <td>${u.role==='admin'?'管理员':'普通用户'}</td>
    <td class="nowrap">${u.isActive?'<span class="dot dot-success"></span>正常':'<span class="dot dot-danger"></span>已禁用'}</td>
    <td class="nowrap">${u.lastLoginAt?fmtDate(u.lastLoginAt):'-'}</td>
    <td class="nowrap">${fmtDate(u.createdAt)}</td>
    <td class="admin-ops nowrap"><div class="admin-ops-row">
      <button type="button" data-op="${u.isActive?'disable':'enable'}">${u.isActive?'禁用':'启用'}</button>
      <button type="button" data-op="password">重置密码</button>
      <button type="button" data-op="sessions">会话</button>
    </div></td>
  </tr>`).join('')+`</tbody></table></div>`;
  const cards=`<div class="admin-card-list">${users.map(u=>`<article class="admin-card" data-id="${esc(u.id)}">
    <div class="admin-card-head"><strong>${esc(u.displayName||u.username)}</strong><span class="badge ${u.isActive?'badge-success':'badge-danger'}">${u.isActive?'正常':'已禁用'}</span></div>
    <div class="admin-card-meta">
      <span>用户名：${esc(u.username)}</span>
      <span>角色：${u.role==='admin'?'管理员':'普通用户'}</span>
      <span>最近登录：${u.lastLoginAt?fmtDate(u.lastLoginAt):'-'}</span>
      <span>创建：${fmtDate(u.createdAt)}</span>
    </div>
    <div class="admin-card-actions">
      <button type="button" data-op="${u.isActive?'disable':'enable'}">${u.isActive?'禁用':'启用'}</button>
      <button type="button" data-op="password">重置密码</button>
      <button type="button" data-op="sessions">会话</button>
    </div>
  </article>`).join('')}</div>`;
  root.innerHTML=table+cards;
}
$('#admin-view-users').addEventListener('click',e=>{
  const btn=e.target.closest('button[data-op]');
  if(!btn)return;
  const row=btn.closest('tr[data-id],.admin-card[data-id]');
  if(!row)return;
  const op=btn.dataset.op;
  if(op==='disable')return setUserActive(row.dataset.id,false);
  if(op==='enable')return setUserActive(row.dataset.id,true);
  if(op==='password')return resetUserPassword(row.dataset.id);
  if(op==='sessions')return showUserSessions(row.dataset.id,row.querySelector('td:nth-child(2)')?.textContent||'');
});
async function setUserActive(id,active){
  if(!active&&!(await confirmDialog({title:'禁用用户',message:'禁用后该用户的所有会话会立即下线且无法登录，确定？',confirmText:'禁用'})))return;
  try{
    await api(`/api/v1/admin/users/${encodeURIComponent(id)}/active`,{method:'PATCH',body:JSON.stringify({active})});
    toast(active?'已启用':'已禁用','success');await loadAdminUsers();
  }catch(err){toast(err.message,'error')}
}
async function resetUserPassword(id){
  const password=await promptDialog({title:'重置密码',message:'该用户的所有会话将立即下线。请输入新密码（至少 12 位）：',placeholder:'新密码',password:true,confirmText:'重置'});
  if(password==null)return;
  if(password.length<12)return toast('密码至少需要 12 位','warning');
  try{
    await api(`/api/v1/admin/users/${encodeURIComponent(id)}/password`,{method:'POST',body:JSON.stringify({password})});
    toast('密码已重置，该用户会话已下线','success');
  }catch(err){toast(err.message,'error')}
}
async function showUserSessions(id,username){
  const host=$('#admin-user-sessions');
  host.dataset.userId=id;host.dataset.userName=username||'';
  host.classList.remove('hidden');
  host.innerHTML='<p class="meta">加载会话…</p>';
  try{
    const data=await api(`/api/v1/admin/users/${encodeURIComponent(id)}/sessions`);
    const sessions=data.sessions||[];
    host.innerHTML=`<div class="admin-head"><div><h3>${esc(username||'用户')} 的会话</h3></div><button type="button" class="ghost" data-close-sessions>收起</button></div>`+
      (sessions.length?sessions.map(s=>`<div class="admin-session"><span>${esc(s.device.name||s.device.platform)}</span><span class="meta">${fmtDate(s.lastUsedAt)} 活跃</span>${s.current?'<span class="badge badge-success">当前</span>':''}<button type="button" class="danger" data-session="${esc(s.id)}">下线</button></div>`).join(''):'<p class="meta">该用户当前没有活跃会话</p>');
  }catch(err){toast(err.message,'error')}
}
$('#admin-user-sessions').addEventListener('click',async e=>{
  const close=e.target.closest('[data-close-sessions]');
  if(close){$('#admin-user-sessions').classList.add('hidden');$('#admin-user-sessions').innerHTML='';return}
  const btn=e.target.closest('button[data-session]');
  if(!btn)return;
  const host=$('#admin-user-sessions');
  try{
    await api(`/api/v1/admin/sessions/${encodeURIComponent(btn.dataset.session)}`,{method:'DELETE'});
    toast('会话已下线','success');
    showUserSessions(host.dataset.userId,host.dataset.userName);
  }catch(err){toast(err.message,'error')}
});

/* ---------- 渲染 ---------- */
function esc(value){return String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))}
function stripHashtags(title,hashtags){let t=String(title||'');(hashtags||[]).forEach(h=>{if(h)t=t.split('#'+h).join(' ')});const out=t.replace(/[#＃]\S*/g,'').replace(/\s{2,}/g,' ').replace(/^\s+|\s+$/g,'');return out||t.trim()}
function fmtDuration(ms){if(!ms)return'';const s=Math.max(1,Math.round(ms/1000)),h=Math.floor(s/3600),m=Math.floor(s%3600/60),ss=s%60;return h?`${h}:${String(m).padStart(2,'0')}:${String(ss).padStart(2,'0')}`:`${m}:${String(ss).padStart(2,'0')}`}
function fmtDate(iso){if(!iso)return'';const d=new Date(iso);return isNaN(d)?'':d.toLocaleString('zh-CN',{year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',hour12:false})}
function fmtShort(iso){if(!iso)return'';const d=new Date(iso);if(isNaN(d))return'';return `${String(d.getMonth()+1).padStart(2,'0')}-${String(d.getDate()).padStart(2,'0')} ${String(d.getHours()).padStart(2,'0')}:${String(d.getMinutes()).padStart(2,'0')}`}
function fileLabel(f){
  if(f.kind==='result_text'||f.name==='result.txt')return'下载文案 (txt)';
  if(f.kind==='result_markdown'||f.name==='result.md')return'下载文案 (md)';
  if(f.kind==='result_meta'||f.name==='meta.json')return'下载元数据';
  if(f.kind==='animated')return'下载动图';
  if(f.kind==='image')return'下载配图';
  if(f.kind==='video'||(f.mimeType||'').startsWith('video/'))return'下载无水印视频';
  return`下载 ${f.name}`;
}
function jobSignature(job){
  // previewUrl/expiresAt 每次轮询都会变化（预览签名按需生成），不算入签名，否则每 5s 重渲染导致动图/图片闪烁。
  const result=job.result||null;
  const stable=result?{...result,files:(result.files||[]).map(f=>({id:f.id,kind:f.kind,name:f.name,mimeType:f.mimeType,sizeBytes:f.sizeBytes}))}:null;
  return [job.status,job.progress,job.updatedAt,job.error?.code,JSON.stringify(stable)].join('|');
}

function jobRowHTML(job){
  const w=job.work||{};
  const title=stripHashtags(w.title,w.hashtags)||`任务 ${job.id.slice(0,8)}`;
  const tone=statusTone[job.status]||'muted';
  const meta=[];
  if(w.authorName)meta.push(w.authorName);
  if(w.durationMs)meta.push(fmtDuration(w.durationMs));
  if(meta.length)meta.push(actionLabels[job.action]||'');
  return `<div class="row-top"><span class="row-title">${esc(title)}</span><span class="row-time">${fmtShort(job.createdAt)}</span></div>
  <div class="row-meta"><span class="dot dot-${tone}"></span>${esc(statusLabels[job.status]||job.status)}${meta.length?' · '+esc(meta.join(' · ')):''}</div>`;
}

function renderList(jobs){
  jobsCache=jobs;
  const root=$('#job-list');
  const empty=root.querySelector(':scope > .empty-state');
  if(!jobs.length){
    if(!empty)root.innerHTML=`<div class="empty-state">${icon('inbox',36)}<p>暂无任务</p><span>粘贴抖音链接开始使用</span></div>`;
    return;
  }
  if(empty)empty.remove();
  const seen=new Set(jobs.map(j=>j.id));
  [...root.querySelectorAll('.job-row')].forEach(row=>{if(!seen.has(row.dataset.id))row.remove()});
  const frag=document.createDocumentFragment();
  jobs.forEach(job=>{
    let row=root.querySelector(`.job-row[data-id="${CSS.escape(job.id)}"]`);
    if(row&&row.dataset.sig===jobSignature(job))return;
    if(row)row.remove();
    row=document.createElement('div');
    row.className='job-row';row.dataset.id=job.id;row.dataset.sig=jobSignature(job);
    if(job.id===selectedId)row.classList.add('selected');
    row.innerHTML=jobRowHTML(job);
    frag.appendChild(row);
  });
  root.appendChild(frag);
  jobs.forEach((job,index)=>{
    const row=root.querySelector(`.job-row[data-id="${CSS.escape(job.id)}"]`);
    if(!row)return;
    const target=root.children[index];
    if(target&&target!==row)root.insertBefore(row,target);
  });
  if(selectedId){
    const fresh=jobs.find(j=>j.id===selectedId);
    if(fresh)currentJob=fresh;
    renderDetail();
  }
}

function jobDetailHTML(job){
  const w=job.work||{};
  const title=stripHashtags(w.title,w.hashtags)||`任务 ${job.id.slice(0,8)}`;
  const terminal=['completed','failed','cancelled'].includes(job.status);
  const text=job.result?.normalizedText||job.result?.text||'';
  const files=job.result?.files||[];
  // 真实视频才提供"预览视频"；动图（kind=animated）在画廊内联展示，不走视频播放器。
  const videoFile=files.find(f=>f.kind==='video'||((f.mimeType||'').startsWith('video/')&&f.kind!=='animated'));
  const tone=statusTone[job.status]||'muted';
  const badges=[`<span class="badge badge-${tone}">${esc(statusLabels[job.status]||job.status)}</span>`,`<span class="badge badge-action">${esc(actionLabels[job.action]||job.action)}</span>`];
  const noteImgs=w.images||[];
  if(w.type==='note')badges.push(noteImgs.length>0&&noteImgs.every(img=>img.animatedUrl)?'<span class="badge badge-note">动图作品</span>':'<span class="badge badge-note">图文作品</span>');

  const metas=[];
  // 作者/分辨率仅在解析完成后显示；为空显示 "-"（未解析的任务不显示这些行）。
  if(w.douyinWorkId){
    metas.push(`<span>作者：${esc(w.authorName||'-')}</span>`);
    metas.push(`<span>分辨率：${esc((w.width&&w.height)?`${w.width}×${w.height}`:'-')}</span>`);
  }
  if(w.durationMs)metas.push(`<span>时长 ${fmtDuration(w.durationMs)}</span>`);
  if(w.publishedAt)metas.push(`<span>发布 ${fmtDate(w.publishedAt)}</span>`);
  const tags=(w.hashtags||[]).map(t=>`<span class="tag">#${esc(t)}</span>`).join('');
  const cover=w.coverUrl?`<img class="detail-cover" src="${esc(w.coverUrl)}" alt="" loading="lazy" decoding="async" referrerpolicy="no-referrer" onerror="this.style.display='none'">`:'';

  const error=job.error?`<div class="error-box">${icon('alert-circle',14)}<div><strong>${esc(job.error.code)}</strong><p>${esc(job.error.message)}</p></div></div>`:'';
  const progressPct=Math.max(0,Math.min(100,Math.round(Number(job.progress)||0)));
  const progressBlock=!terminal?`<div class="progress-row"><div class="progress"><i style="width:${progressPct}%"></i></div><span>${progressPct}%</span></div>`:'';
  const statusText=job.statusMessage||(['completed'].includes(job.status)?'处理完成':'');

  const isNote=w.type==='note';
  const linkFiles=isNote?files.filter(f=>f.kind!=='image'&&f.kind!=='animated'):files;
  const fileHref=f=>adminMode?`/api/v1/admin/files/${encodeURIComponent(f.id)}`:`/api/v1/files/${encodeURIComponent(f.id)}`;
  // data-preview 携带同源签名流式地址：视频/配图下载直接用 <a download> 流式落盘，不再 fetch 整文件缓冲。
  const fileLinks=linkFiles.map(f=>`<a class="btn download" href="${fileHref(f)}" data-file="${esc(f.id)}" data-name="${esc(f.name)}" data-preview="${esc(f.previewUrl||'')}">${icon('download',14)} ${esc(fileLabel(f))}</a>`);
  const viewOps=[],taskOps=[];
  if(videoFile)viewOps.push(`<button type="button" class="btn preview-btn" data-op="preview" data-fid="${esc(videoFile.id)}" data-name="${esc(videoFile.name)}" data-preview="${esc(videoFile.previewUrl||'')}">${icon('play',14)} 预览视频</button>`);
  if(!terminal)taskOps.push(`<button type="button" data-op="cancel">${icon('circle-slash',14)} 取消</button>`);
  if(job.status==='failed'||job.status==='cancelled')taskOps.push(`<button type="button" data-op="retry">${icon('rotate-ccw',14)} 重试</button>`);
  if(terminal)taskOps.push(`<button type="button" data-op="delete" class="danger">${icon('trash',14)} 删除</button>`);
  const actionGroups=[viewOps,fileLinks,taskOps].filter(group=>group.length).map(group=>`<div class="action-group">${group.join('')}</div>`).join('');

  const canonical=w.canonicalUrl?`<div class="canonical-row"><a href="${esc(w.canonicalUrl)}" target="_blank" rel="noopener">${icon('arrow-up-right',14)} 查看原作品</a></div>`:'';
  const transcriptHeading=isNote?'图文配文':'视频文案';
  const transcript=text?`<section class="detail-section"><div class="section-head"><h3>${icon('file-text',14)} ${transcriptHeading}</h3><button type="button" class="copy-btn" data-copy title="复制全部文案">${icon('copy',13)} 复制</button></div><div class="result">${esc(text)}</div></section>`:'';

  // 图文/动图：图片画廊。动图缩略图直接用动态本体内联动（点开是全屏查看器，非视频播放器）。
  const mediaFiles=files.filter(f=>f.kind==='image'||f.kind==='animated');
  const noteImages=w.images||[];
  // 按文件名前缀（image-NN）匹配配图，避免某张图下载失败被跳过导致数组下标错位、文件张冠李戴。
  const mediaFileForImage=i=>mediaFiles.find(f=>(f.name||'').indexOf(`image-${String(i+1).padStart(2,'0')}`)===0)||null;
  const galleryItems=noteImages.map((img,i)=>{
    const file=mediaFileForImage(i);
    const cdnSrc=img.animatedUrl||img.url;
    // 签名预览地址 24h 有效：可用时优先（跨区域稳定），过期则退回 CDN 原址，避免 404 黑框。
    const viewerSrc=(file&&file.previewUrl&&previewUsable(file.previewUrl))?file.previewUrl:cdnSrc;
    const dl=file?`<a class="gallery-dl" href="${fileHref(file)}" data-file data-name="${esc(file.name||'image')}" data-preview="${esc(file.previewUrl||'')}" title="下载">${icon('download',14)}</a>`:'';
    const badge=`<span class="gallery-badge">${img.animatedUrl?'动图':'图片'}</span>`;
    // 回退链在错误发生时现查（不烘焙渲染时的判断），避免页面停留超过 24h 后签名地址过期、
    // 或客户端时钟偏差把新鲜地址误判为过期时，图片/动图永久无法恢复。mediaFallback 每项只尝试一次。
    const preview=file&&file.previewUrl?file.previewUrl:'';
    // 图片缩略图优先用本地已下载文件的签名地址（同源、稳定、不绕外网 CDN），CDN 原图仅作错误兜底；
    // 签名过期时直接走 CDN，并把本地地址留给兜底链，避免页面停留久了缩略图永久 404。
    const useLocal=!!preview&&previewUsable(preview);
    const thumbSrc=useLocal?preview:img.url;
    // 安卓微信 X5 内核需要 h5 模式才在页面内渲染视频；poster 用静态图兜底自动播放被拦时的黑屏。
    const x5Attrs=isWeChat()?' x5-video-player-type="h5" x5-playsinline=""':'';
    const media=img.animatedUrl
      ?`<video class="gallery-anim" src="${esc(viewerSrc)}" poster="${esc(img.url)}" autoplay muted loop playsinline webkit-playsinline preload="metadata"${x5Attrs}${viewerSrc===cdnSrc?` data-preview="${esc(preview)}"`:` data-fallback="${esc(cdnSrc)}"`} onerror="mediaFallback(this)"></video>`
      :`<img src="${esc(thumbSrc)}" alt="" loading="lazy" decoding="async" referrerpolicy="no-referrer"${useLocal?` data-fallback="${esc(img.url)}"`:` data-preview="${esc(preview)}"`} onerror="mediaFallback(this)">`;
    const foot=(badge||dl)?`<div class="gallery-foot">${badge}${dl}</div>`:'';
    return `<figure class="gallery-item" data-preview="${esc(viewerSrc)}" data-fallback="${esc(cdnSrc)}" data-poster="${esc(img.url)}" data-animated="${img.animatedUrl?'1':'0'}" data-title="${esc((stripHashtags(w.title,w.hashtags)||'预览').slice(0,40))}"><div class="gallery-media">${media}</div>${foot}</figure>`;
  }).join('');
  const noteGallery=isNote&&noteImages.length?`<div class="note-gallery">${galleryItems}</div>`:'';
  // 管理员详情里打包下载走用户自己的接口会因所有权校验失败，故仅在普通用户视图展示。
  const zipLink=(!adminMode&&isNote&&mediaFiles.length&&job.status==='completed')?`<a class="btn" href="/api/v1/jobs/${encodeURIComponent(job.id)}/images/archive" data-file data-name="${esc((job.work&&job.work.douyinWorkId)||job.id)}_images.zip">${icon('download',14)} 打包下载全部</a>`:'';

  const musicFile=files.find(f=>f.kind==='music');
  const musicPlayUrl=musicFile&&musicFile.previewUrl&&previewUsable(musicFile.previewUrl)?musicFile.previewUrl:w.musicUrl;
  const musicRow=isNote&&w.musicUrl?`<div class="music-row"><span class="music-info">${icon('music',13)} <span>${esc(w.musicTitle||'背景音乐')}${w.musicArtist?' · '+esc(w.musicArtist):''}</span></span><button type="button" class="music-toggle" data-op="music" data-music="${esc(w.musicUrl)}" data-play-url="${esc(musicPlayUrl||'')}" data-fallback="${esc(w.musicUrl)}">${icon('play',13)} 播放音乐</button></div>`:'';
  const hasMedia=cover||w.authorName||metas.length;
  const mediaBlock=isNote
    ?(noteGallery||zipLink||musicRow?`<div class="detail-media no-cover">${noteGallery}${musicRow}${metas.length?`<div class="detail-meta">${metas.join('')}</div>`:''}${canonical}${zipLink?`<div class="action-group">${zipLink}</div>`:''}</div>`:'')
    :(hasMedia?`<div class="detail-media${cover?'':' no-cover'}">${cover}<div class="detail-side">${metas.length?`<div class="detail-meta">${metas.join('')}</div>`:''}${canonical}</div></div>`:'');

  const expiringFiles=files.filter(f=>f.expiresAt);
  const mediaExpiry=expiringFiles.reduce((e,f)=>!e||f.expiresAt<e?f.expiresAt:e,null);
  const retentionNote=mediaExpiry?`<p class="retention-note">${icon('info',13)} 媒体文件保留至 ${fmtDate(mediaExpiry)}，届时自动清理；删除本任务会同时删除媒体</p>`:'';

  return `<article class="job-detail" data-job="${esc(job.id)}">
    <div class="detail-head"><div><h2>${esc(title)}</h2><div class="badges">${badges.join('')}</div></div><span class="job-time" title="${esc(new Date(job.createdAt).toLocaleString())}">${fmtDate(job.createdAt)}</span></div>
    ${mediaBlock}
    ${tags?`<div class="tags">${tags}</div>`:''}
    ${actionGroups?`<div class="actions">${actionGroups}</div>`:''}
    ${retentionNote}
    ${error}
    ${progressBlock}
    ${statusText?`<p class="status-msg">${esc(statusText)}</p>`:''}
    ${transcript}
  </article>`;
}

function renderDetail(){
  if(!currentJob)return;
  const sig=jobSignature(currentJob);
  if(sig!==detailSig){
    detailSig=sig;
    $('#detail').innerHTML=jobDetailHTML(currentJob);
  }
  syncTaskMusicWithJob(currentJob);
  updateTaskMusicUI();
}

function handleContentClick(e){
  const row=e.target.closest('.job-row');
  if(row){selectJob(row.dataset.id);return}
  const gItem=e.target.closest('.gallery-item');
  if(gItem&&!e.target.closest('.gallery-dl')){openGalleryPreview(gItem);return}
  const btn=e.target.closest('[data-op],a[data-file],[data-copy]');
  if(!btn)return;
  if(btn.dataset.op==='music'){e.preventDefault();e.stopPropagation();toggleTaskMusic(btn.closest('[data-job]')?.dataset.job,btn.dataset.music,btn.dataset.playUrl||btn.dataset.music,btn.dataset.fallback||'');return}
  if(btn.dataset.op==='preview'){e.preventDefault();openPreview(btn.dataset.fid,btn.dataset.name,btn.dataset.preview);return}
  if(btn.hasAttribute('data-file')){e.preventDefault();download(btn);return}
  if(btn.hasAttribute('data-copy')){e.preventDefault();e.stopPropagation();copyText(btn);return}
  if(btn.dataset.op){const job=btn.closest('[data-job]');if(job)operate(job.dataset.job,btn.dataset.op)}
}
$('#app').addEventListener('click',handleContentClick);
$('#admin-console').addEventListener('click',handleContentClick);

/* ---------- 自定义确认弹窗 ---------- */
function confirmDialog(opts={}){
  return new Promise(resolve=>{
    const modal=$('#confirm-modal'),ok=$('#confirm-ok'),cancel=$('#confirm-cancel');
    $('#confirm-title').textContent=opts.title||'确认操作';
    $('#confirm-message').textContent=opts.message||'';
    ok.textContent=opts.confirmText||'确认';
    ok.className=opts.danger===false?'btn-primary':'btn-danger';
    const close=v=>{modal.classList.add('hidden');document.body.classList.remove('modal-open');resolve(v)};
    ok.onclick=()=>close(true);
    cancel.onclick=()=>close(false);
    modal.querySelectorAll('[data-close]').forEach(b=>b.onclick=()=>close(false));
    modal.classList.remove('hidden');document.body.classList.add('modal-open');
    cancel.focus();
  });
}

/* 带输入框的弹窗（用于重置密码等）；取消返回 null，确定返回输入值 */
function promptDialog(opts={}){
  return new Promise(resolve=>{
    const modal=$('#confirm-modal'),ok=$('#confirm-ok'),cancel=$('#confirm-cancel');
    $('#confirm-title').textContent=opts.title||'输入';
    const message=$('#confirm-message');
    message.innerHTML='';
    if(opts.message)message.appendChild(document.createTextNode(opts.message));
    const input=document.createElement('input');
    input.type=opts.password?'password':'text';
    input.placeholder=opts.placeholder||'';
    input.maxLength=1024;
    input.autocomplete='new-password';
    input.className='prompt-input';
    message.appendChild(input);
    ok.textContent=opts.confirmText||'确定';
    ok.className='btn-primary';
    const close=v=>{modal.classList.add('hidden');document.body.classList.remove('modal-open');resolve(v)};
    ok.onclick=()=>close(input.value);
    cancel.onclick=()=>close(null);
    modal.querySelectorAll('[data-close]').forEach(b=>b.onclick=()=>close(null));
    modal.classList.remove('hidden');document.body.classList.add('modal-open');
    input.focus();
    input.addEventListener('keydown',e=>{if(e.key==='Enter'){e.preventDefault();ok.click()}});
  });
}

async function operate(id,op){
  if(op==='delete'&&!(await confirmDialog({title:'删除任务',message:'确认删除该任务及其文件？此操作不可撤销。',confirmText:'删除'})))return;
  const base=adminMode?`/api/v1/admin/jobs/${encodeURIComponent(id)}`:`/api/v1/jobs/${encodeURIComponent(id)}`;
  try{
    await api(base+(op==='delete'?'':`/${op}`),{method:op==='delete'?'DELETE':'POST'});
    if(adminMode){
      toast(op==='delete'?'任务已删除':op==='cancel'?'已请求取消':'已重新开始','success');
      if(op==='delete'){
        adminMode=false;
        if(adminJobsPage>1&&adminJobsTotal-1<=(adminJobsPage-1)*adminJobsPageSize)adminJobsPage--;
        adminShowView('jobs');
      }
      else await loadAdminJobs();
      return;
    }
    await loadJobs();
    if(op==='delete'&&selectedId===id)selectJob(null);
    if(op==='delete'&&jobsPage>1&&jobsTotal<=(jobsPage-1)*jobsPageSize){jobsPage--;loadJobs()}
    if(op==='delete')toast('任务已删除','success');
    if(op==='cancel')toast('已请求取消');
    if(op==='retry')toast('已重新开始');
  }catch(err){toast(err.message,'error')}
}
// 签名预览地址带 expires（Unix 秒）；页面停留超过有效期后点击应回退 fetch+鉴权，避免拿到已过期的地址。
// 客户端时钟可能领先服务端（校时错误/双系统），加 1 小时容差，避免把刚签发的地址误判为已过期、
// 从而退回可能被地域/防盗链拦截的 CDN 原址。
const PREVIEW_SKEW_MS=60*60*1000;
function previewUsable(url){
  if(!url)return false;
  const m=/[?&]expires=(\d+)/.exec(url);
  if(!m)return true;
  return Number(m[1])*1000+PREVIEW_SKEW_MS>Date.now();
}
// 微信内置浏览器（iOS WKWebView / 安卓 X5）对视频自动播放和页面内渲染有额外限制，需要单独适配。
function isWeChat(){return /MicroMessenger/i.test(navigator.userAgent||'')}
// 媒体加载失败兜底：按 data-preview（同源签名地址）→ data-fallback（CDN 原址）依次尝试，
// 每项只试一次（试完清除该属性），全部失败则隐藏元素。判断发生在错误时刻而非渲染时刻。
function mediaFallback(el){
  const preview=el.dataset.preview;
  if(preview){el.dataset.preview='';el.src=preview;return;}
  const fallback=el.dataset.fallback;
  if(fallback){el.dataset.fallback='';el.src=fallback;return;}
  el.style.display='none';
}
async function download(link){
  // 媒体文件有同源签名流式地址：直接用 <a download>，浏览器流式写入磁盘、立即开始，不用 fetch 整文件。
  const preview=previewUsable(link.dataset.preview)?link.dataset.preview:'';
  if(preview){
    const a=document.createElement('a');a.href=preview;a.download=link.dataset.name||'download';a.rel='noopener';document.body.appendChild(a);a.click();a.remove();
    return;
  }
  try{const response=await fetch(link.href,{headers:{Authorization:`Bearer ${session.accessToken}`}});if(!response.ok)throw new Error('下载失败');const blob=await response.blob(),url=URL.createObjectURL(blob),a=document.createElement('a');a.href=url;a.download=link.dataset.name;a.click();setTimeout(()=>URL.revokeObjectURL(url),30000)}catch(err){toast(err.message,'error')}
}
async function copyText(btn){
  const section=btn.closest('.detail-section')||btn.closest('.result-details');
  const text=(section&&section.querySelector('.result')||{}).textContent||'';
  try{await navigator.clipboard.writeText(text)}
  catch{const ta=document.createElement('textarea');ta.value=text;ta.style.position='fixed';ta.style.opacity='0';document.body.appendChild(ta);ta.select();document.execCommand('copy');ta.remove()}
  btn.innerHTML=icon('check',13)+' 已复制';btn.classList.add('copied');
  setTimeout(()=>{btn.innerHTML=icon('copy',13)+' 复制';btn.classList.remove('copied')},1400);
}

/* ---------- 当前任务音乐 ---------- */
function taskAudio(){return $('#task-music-audio')}
function updateTaskMusicUI(){
  document.querySelectorAll('.music-toggle').forEach(btn=>{
    const jobId=btn.closest('[data-job]')?.dataset.job;
    const playing=taskMusic.playing&&taskMusic.jobId===jobId&&taskMusic.source===btn.dataset.music;
    btn.classList.toggle('playing',playing);
    btn.setAttribute('aria-pressed',playing?'true':'false');
    btn.innerHTML=icon(playing?'volume-x':'play',13)+(playing?' 停止音乐':' 播放音乐');
  });
}
function stopTaskMusic(){
  const audio=taskAudio();
  if(audio){
    audio.pause();
    audio.currentTime=0;
  }
  taskMusic.playing=false;
  updateTaskMusicUI();
}
function resumeTaskMusic(){
  if(!taskMusic.jobId||!taskMusic.url)return;
  const audio=taskAudio();
  if(!audio)return;
  audio.src=taskMusic.url;
  audio.loop=true;
  audio.play().then(()=>{taskMusic.playing=true;updateTaskMusicUI()}).catch(()=>{taskMusic.playing=false;updateTaskMusicUI()});
}
function syncTaskMusicWithJob(job){
  const source=job?.work?.musicUrl||'';
  if(!job||!source){
    if(taskMusic.playing)stopTaskMusic();
    taskMusic={jobId:job?.id||null,source:'',url:'',playing:false};
    return;
  }
  const files=(job.Result&&job.Result.files)||[];
  const musicFile=Array.isArray(files)?files.find(f=>f.kind==='music')||null:null;
  const url=musicFile&&musicFile.previewUrl&&previewUsable(musicFile.previewUrl)?musicFile.previewUrl:source;
  if(taskMusic.jobId!==job.id){
    if(taskMusic.playing)stopTaskMusic();
    taskMusic={jobId:job.id,source,url,playing:false};
  }else if(taskMusic.source!==source){
    stopTaskMusic();
    taskMusic={jobId:job.id,source,url,playing:false};
  }else if(!taskMusic.playing){
    taskMusic.url=url;
  }
}
async function toggleTaskMusic(jobId,source,playUrl,fallbackUrl){
  if(!jobId||!source)return;
  const audio=taskAudio();
  if(!audio)return;
  const url=playUrl||source;
  if(taskMusic.jobId!==jobId||taskMusic.source!==source){
    stopTaskMusic();
    taskMusic={jobId,source,url,playing:false};
  }
  if(taskMusic.playing){
    stopTaskMusic();
    return;
  }
  try{
    audio.src=url;
    audio.loop=true;
    await audio.play();
    taskMusic.url=url;
    taskMusic.playing=true;
    updateTaskMusicUI();
  }catch(err){
    if(fallbackUrl&&fallbackUrl!==url){
      try{
        audio.src=fallbackUrl;
        audio.loop=true;
        await audio.play();
        taskMusic.url=fallbackUrl;
        taskMusic.playing=true;
        updateTaskMusicUI();
        return;
      }catch{}
    }
    taskMusic.playing=false;
    updateTaskMusicUI();
    toast('音乐播放失败，请稍后重试','error');
  }
}

/* ---------- 视频预览 / 图文查看 ---------- */
let previewURL=null,galleryPreview={items:[],index:0,touchX:null};
let previewMusicSuspended=false;
let previewSoundOn=false;
let previewGestureHandler=null;
function setPreviewNav(show){
  document.querySelectorAll('[data-preview-nav]').forEach(btn=>btn.classList.toggle('hidden',!show));
}
function resetPreviewMedia(){
  const video=$('#preview-video'),img=$('#preview-image');
  if(previewGestureHandler){
    video.removeEventListener('click',previewGestureHandler);
    video.removeEventListener('touchstart',previewGestureHandler);
    previewGestureHandler=null;
  }
  video.pause();video.currentTime=0;video.removeAttribute('src');video.removeAttribute('poster');video.removeAttribute('x5-video-player-type');video.load();video.classList.add('hidden');
  img.classList.add('hidden');img.removeAttribute('src');
}
function openGalleryPreview(item){
  const nodes=[...item.closest('.note-gallery').querySelectorAll('.gallery-item')];
  const items=nodes.map(el=>({
    src:el.dataset.preview,
    animated:el.dataset.animated,
    title:el.dataset.title,
    fallback:el.dataset.fallback,
    poster:el.dataset.poster
  }));
  galleryPreview.items=items;
  galleryPreview.index=Math.max(0,nodes.indexOf(item));
  renderGalleryPreview();
}
function moveGalleryPreview(delta){
  if(galleryPreview.items.length<2||$('#preview-modal').classList.contains('hidden'))return;
  const target=galleryPreview.index+delta;
  if(target<0||target>=galleryPreview.items.length)return; // 两端不循环
  galleryPreview.index=target;
  renderGalleryPreview();
}
function renderGalleryPreview(){
  const item=galleryPreview.items[galleryPreview.index];
  if(!item)return;
  const modal=$('#preview-modal'),video=$('#preview-video'),img=$('#preview-image'),soundBtn=$('#preview-sound-toggle');
  modal.classList.remove('hidden');
  modal.classList.toggle('preview-animated-mode',item.animated==='1');
  modal.classList.toggle('preview-image-mode',item.animated!=='1');
  modal.classList.remove('preview-video-mode');
  document.body.classList.add('modal-open');
  resetPreviewMedia();
  let src=item.src;
  if(!previewUsable(src)&&item.fallback)src=item.fallback;
  const count=galleryPreview.items.length>1?` ${galleryPreview.index+1}/${galleryPreview.items.length}`:'';
  $('#preview-title').textContent=(item.title||'预览')+count;
  setPreviewNav(galleryPreview.items.length>1);
  // 两端禁用对应方向的按钮：第一张不能左、最后一张不能右
  const prevBtn=document.querySelector('.preview-prev'),nextBtn=document.querySelector('.preview-next');
  if(galleryPreview.items.length>1){
    prevBtn.disabled=galleryPreview.index===0;
    nextBtn.disabled=galleryPreview.index===galleryPreview.items.length-1;
  }
  // 动图点击即自动播放：动图静音播放，同时自动播放背景音乐；「原声」按钮按需开启原声。
  previewSoundOn=false;
  if(item.animated==='1'){
    // 从有声动图切回时先恢复背景音乐
    if(previewMusicSuspended){previewMusicSuspended=false;resumeTaskMusic()}
    video.classList.remove('hidden');
    video.controls=false;video.loop=true;video.playsInline=true;video.muted=true;
    // 安卓微信 X5 内核需要 h5 模式才在页面内渲染视频，否则黑屏或弹独立播放器
    if(isWeChat())video.setAttribute('x5-video-player-type','h5');else video.removeAttribute('x5-video-player-type');
    soundBtn.classList.remove('hidden');
    setPreviewSoundUI(false);
    if(item.poster)video.poster=item.poster;
    video.src=src;video.load();
    const autoplay=()=>{video.play().catch(()=>{})};
    autoplay();
    // 资源未就绪时等 canplay 再补一次，保证点击后一定自动播放
    video.oncanplay=()=>{if(video.paused)autoplay();video.oncanplay=null};
    // 微信等浏览器可能拦截自动播放：poster 已兜底画面，首次点击/触摸视频时补播
    previewGestureHandler=()=>{
      if(!video.paused){
        video.removeEventListener('click',previewGestureHandler);
        video.removeEventListener('touchstart',previewGestureHandler);
        return;
      }
      // 播放成功后再移除监听，避免资源未就绪时首次手势无效、之后无法再补播
      video.play().then(()=>{
        video.removeEventListener('click',previewGestureHandler);
        video.removeEventListener('touchstart',previewGestureHandler);
      }).catch(()=>{});
    };
    video.addEventListener('click',previewGestureHandler);
    video.addEventListener('touchstart',previewGestureHandler,{passive:true});
    // 点击动图自动播放背景音乐（作品没有音乐则不响，原声开启时不打断）
    if(taskMusic.jobId&&taskMusic.url&&!taskMusic.playing&&!previewMusicSuspended)resumeTaskMusic();
  }else{
    if(previewMusicSuspended){previewMusicSuspended=false;resumeTaskMusic()}
    soundBtn.classList.add('hidden');
    img.classList.remove('hidden');img.decoding='async';img.src=src;
  }
}
function setPreviewSoundUI(on){
  const btn=$('#preview-sound-toggle');
  btn.classList.toggle('on',on);
  btn.setAttribute('aria-pressed',on?'true':'false');
  btn.innerHTML=icon(on?'volume-2':'volume-x',15)+' 原声';
}
function togglePreviewSound(){
  const video=$('#preview-video');
  const next=!previewSoundOn;
  previewSoundOn=next;
  setPreviewSoundUI(next);
  if(previewSoundOn){
    if(taskMusic.playing){stopTaskMusic();previewMusicSuspended=true}
    video.muted=false;
    video.play().catch(()=>{});
  }else{
    video.muted=true;
    if(previewMusicSuspended){previewMusicSuspended=false;resumeTaskMusic()}
  }
}
$('#preview-sound-toggle').addEventListener('click',e=>{e.preventDefault();e.stopPropagation();togglePreviewSound()});
async function openPreview(fileId,name,previewUrl){
  const modal=$('#preview-modal'),video=$('#preview-video'),img=$('#preview-image');
  if(taskMusic.playing)stopTaskMusic();
  galleryPreview={items:[],index:0,touchX:null};
  modal.classList.add('preview-video-mode');
  modal.classList.remove('preview-image-mode','preview-animated-mode');
  previewSoundOn=false;
  $('#preview-sound-toggle').classList.add('hidden');
  setPreviewNav(false);
  $('#preview-title').textContent=name||'视频预览';
  modal.classList.remove('hidden');
  document.body.classList.add('modal-open');
  resetPreviewMedia();
  video.classList.remove('hidden');
  video.controls=true;video.muted=false;video.loop=false;video.playsInline=true;
  if(previewURL){URL.revokeObjectURL(previewURL);previewURL=null}
  // 优先用同源签名地址直接流式播放（支持 Range，秒开），不再把整个视频 fetch 进内存再播。
  if(previewUsable(previewUrl)){
    video.src=previewUrl;
    video.play().catch(()=>{});
    return;
  }
  try{
    const r=await fetch(`/api/v1/files/${encodeURIComponent(fileId)}`,{headers:{Authorization:`Bearer ${session.accessToken}`}});
    if(!r.ok)throw new Error('视频加载失败');
    const blob=await r.blob();
    if(previewURL)URL.revokeObjectURL(previewURL);
    previewURL=URL.createObjectURL(blob);
    video.src=previewURL;
    video.play().catch(()=>{});
  }catch(err){closePreview();toast(err.message,'error')}
}
// 画廊点击查看：动图点击即自动播放，并自动播放背景音乐；「原声」按钮切换动图原声。
function openMediaPreview(src,animated,title,fallback,poster){
  galleryPreview={items:[{src,animated,title,fallback,poster}],index:0,touchX:null};
  renderGalleryPreview();
}
function closePreview(){
  const modal=$('#preview-modal');modal.classList.add('hidden');document.body.classList.remove('modal-open');
  modal.classList.remove('preview-video-mode','preview-image-mode','preview-animated-mode');
  resetPreviewMedia();
  galleryPreview={items:[],index:0,touchX:null};
  setPreviewNav(false);
  if(previewURL){URL.revokeObjectURL(previewURL);previewURL=null}
  previewSoundOn=false;
  $('#preview-sound-toggle').classList.add('hidden');
  if(previewMusicSuspended){previewMusicSuspended=false;resumeTaskMusic()}
}
document.querySelectorAll('[data-preview-nav]').forEach(btn=>{
  btn.innerHTML=icon(btn.dataset.previewNav==='-1'?'chevron-left':'chevron-right',22);
  btn.addEventListener('click',e=>{e.preventDefault();e.stopPropagation();moveGalleryPreview(Number(btn.dataset.previewNav))});
});
$('#preview-modal').addEventListener('touchstart',e=>{
  if(galleryPreview.items.length<2)return;
  galleryPreview.touchX=e.touches[0]?.clientX??null;
},{passive:true});
$('#preview-modal').addEventListener('touchend',e=>{
  if(galleryPreview.items.length<2||galleryPreview.touchX==null)return;
  const endX=e.changedTouches[0]?.clientX??galleryPreview.touchX;
  const dx=endX-galleryPreview.touchX;
  galleryPreview.touchX=null;
  if(Math.abs(dx)>48)moveGalleryPreview(dx<0?1:-1);
},{passive:true});
$('#preview-modal').addEventListener('click',e=>{if(e.target.closest('[data-close]'))closePreview()});
document.addEventListener('keydown',e=>{
  if(!$('#preview-modal').classList.contains('hidden')){
    if(e.key==='Escape'){closePreview();return}
    if(e.key==='ArrowLeft'){moveGalleryPreview(-1);return}
    if(e.key==='ArrowRight'){moveGalleryPreview(1);return}
  }
  if(e.key!=='Escape')return;
  if(!$('#sidebar-backdrop').classList.contains('hidden')){setSidebar(false);return}
  if(!$('#confirm-modal').classList.contains('hidden')){$('#confirm-cancel').click()}
});

// 启动时用 httpOnly Cookie 静默恢复会话；失效则回到登录页。
(async()=>{try{enter(await api('/api/v1/auth/refresh',{method:'POST'},false))}catch{enter(null)}})();
