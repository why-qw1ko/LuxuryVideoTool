const $=s=>document.querySelector(s);
let session=null,poll=null;
let selectedId=null,currentJob=null,jobsCache=[],detailSig='';
let adminMode=false,currentView='new',adminOpen=false,adminCurrentView='dashboard',adminJobsCache=[];
let jobsPage=1,jobsPageSize=20,jobsTotal=0;
let adminJobsPage=1,adminJobsPageSize=20,adminJobsTotal=0;
let taskMusic={jobId:null,source:'',url:'',playing:false};
let videoPreviewFailure=null;
let statusChart=null,trendChart=null;
let lastStatsSig='',lastAdminJobsSig='';
let sessionEpoch=0,jobsLoadId=0,jobDetailLoadId=0,adminJobsLoadId=0,adminUsersLoadId=0,adminDetailLoadId=0;
let adminDetailSig='';
let jobsPending=null,jobDetailPending=null,adminJobsPending=null,adminDetailPending=null;
let pollRequestedAt={},adminStatsHaveActive=false;

/* ---------- 主题（仅浅色 / 深色） ---------- */
const THEMES=['light','dark'];
let theme=localStorage.getItem('dc_theme');
if(THEMES.indexOf(theme)<0)theme=matchMedia('(prefers-color-scheme:dark)').matches?'dark':'light';

const actionLabels={info:'仅解析基本信息',download:'下载媒体',transcribe:'提取文案',full:'提取内容'};
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
  image:'<rect width="18" height="18" x="3" y="3" rx="2"/><circle cx="8.5" cy="8.5" r="1.5"/><path d="m21 15-5-5L5 21"/>',
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
function normaliseAPI(value){
  const url=new URL(value);
  if(!['http:','https:'].includes(url.protocol)||url.username||url.password||url.search||url.hash)throw new Error('请输入有效的 HTTP 或 HTTPS 服务地址');
  return url.origin+url.pathname.replace(/\/+$/,'');
}
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
async function readWebSession(){
  const controller=new AbortController();
  const timeout=setTimeout(()=>controller.abort(),8000);
  try{
    const response=await fetch('/api/v1/auth/refresh',{method:'POST',signal:controller.signal});
    return response.ok?await response.json():null;
  }finally{clearTimeout(timeout)}
}
async function refreshSession(){
  const epoch=sessionEpoch;
  if(refreshing?.epoch!==epoch){
    const request={epoch};
    const userId=session?.user?.id;
    request.promise=readWebSession().then(value=>{
      if(epoch!==sessionEpoch)return false;
      if(!value||value.user?.id!==userId){
        enter(value);
        return false;
      }
      save(value);
      return true;
    }).finally(()=>{if(refreshing===request)refreshing=null});
    refreshing=request;
  }
  return refreshing.promise;
}
async function api(path,options={},retry=true){
  const epoch=sessionEpoch;
  const headers={...(options.headers||{}),'Content-Type':'application/json'};
  if(session?.accessToken)headers.Authorization=`Bearer ${session.accessToken}`;
  const response=await fetch(path,{...options,headers});
  const body=await response.json().catch(()=>({}));
  if(response.status===401&&retry&&body.error?.code!=='AUTH_INVALID_CREDENTIALS'){
    if(epoch===sessionEpoch&&await refreshSession()&&epoch===sessionEpoch)return api(path,options,false);
    throw new Error('登录已过期，请重新登录');
  }
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
const sidebarMedia=matchMedia('(max-width:960px)');
function syncSidebarAccessibility(){
  const frontSidebar=$('.master-list'),adminSidebar=$('.admin-sidebar');
  [frontSidebar,adminSidebar].forEach((sidebar,index)=>{
    if(!sidebar)return;
    const hidden=!session||(index===0?adminOpen:!adminOpen)||(sidebarMedia.matches&&!sidebar.classList.contains('open'));
    sidebar.setAttribute('aria-hidden',String(hidden));
    sidebar.inert=hidden;
  });
}
function setSidebar(open){
  const frontSidebar=$('.master-list'),adminSidebar=$('.admin-sidebar');
  const target=adminOpen?adminSidebar:frontSidebar;
  open=!!open&&sidebarMedia.matches&&!!session;
  if(open)sidebarLastFocus=document.activeElement;
  if(frontSidebar)frontSidebar.classList.toggle('open',!adminOpen&&open);
  if(adminSidebar)adminSidebar.classList.toggle('open',adminOpen&&open);
  $('#sidebar-backdrop').classList.toggle('hidden',!open);
  $('#sidebar-toggle').setAttribute('aria-expanded',open?'true':'false');
  $('#sidebar-toggle').setAttribute('aria-label',open?'关闭菜单':'打开菜单');
  syncSidebarAccessibility();
  if(open&&target){
    requestAnimationFrame(()=>target.querySelector('button,a,input,select,textarea')?.focus());
  }else if(sidebarLastFocus&&document.contains(sidebarLastFocus)){
    sidebarLastFocus.focus();
    sidebarLastFocus=null;
  }
}
sidebarMedia.addEventListener('change',()=>setSidebar(false));
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
  ['new','detail'].forEach(v=>show('#view-'+v,v===name));
  $('#new-task-btn').classList.toggle('active',name==='new');
  if(name==='new'&&!document.hidden)void refreshDailyUsage();
}
function selectJob(id,job=null){
  if(selectedId!==id||job){
    if(taskMusic.jobId&&taskMusic.jobId!==id)stopTaskMusic();
    jobDetailLoadId++;
    selectedId=id;detailSig='';currentJob=id?job:null;
  }
  document.querySelectorAll('.job-row').forEach(r=>{
    const selected=r.dataset.id===id;
    r.classList.toggle('selected',selected);
    r.setAttribute('aria-current',String(selected));
  });
  showView(id?'detail':'new');
  if(!id)stopTaskMusic();
  if(id&&currentJob)renderDetail();
  if(id&&!currentJob)$('#detail').innerHTML=skeletonRows();
  if(id&&!job)loadJobDetail(id);
  if(matchMedia('(max-width:960px)').matches)setSidebar(false);
  if(id&&matchMedia('(max-width:960px)').matches)$('#view-detail').scrollIntoView({behavior:'smooth',block:'start'});
}
$('#new-task-btn').addEventListener('click',()=>selectJob(null));

/* ---------- 后台管理系统 ---------- */
function openAdminConsole(view='dashboard'){
  if(session?.user?.role!=='admin')return;
  if(adminOpen)return;
  adminReturnFocus=view==='usage'?(matchMedia('(max-width:960px)').matches?'#sidebar-toggle':'#settings-link'):'#admin-console-btn';
  stopTaskMusic();
  adminOpen=true;adminMode=false;selectedId=null;currentJob=null;
  setSidebar(false);
  lastStatsSig=''; // 重新打开时按当前主题重绘图表，避免沿用旧配色
  show('#app',false);show('#admin-console',true);
  syncHeaderActions();
  adminShowView(view);
  $(view==='usage'?'#admin-usage-heading':'#admin-back-front').focus();
}
let adminReturnFocus='#admin-console-btn';
function closeAdminConsole(){
  resetAdminPasswordForm();
  resetProviderSettings();
  resetAdminApiCatalog();
  closeAdminUserSessions();
  closeAdminUserMenu();
  adminOpen=false;adminMode=false;
  setSidebar(false);
  show('#admin-console',false);show('#app',true);
  syncHeaderActions();
  selectJob(null);
  $(adminReturnFocus).focus();
  if(!document.hidden)loadJobs(true);
}
function adminShowView(name){
  adminDetailLoadId++;
  if(name!=='detail'){adminMode=false;adminDetailSig=''}
  if(adminCurrentView==='password'&&name!=='password')resetAdminPasswordForm();
  if(adminCurrentView==='providers'&&name!=='providers')resetProviderSettings();
  if(adminCurrentView==='api-catalog'&&name!=='api-catalog')resetAdminApiCatalog();
  if(adminCurrentView==='users'&&name!=='users'){closeAdminUserSessions();closeAdminUserMenu()}
  adminCurrentView=name;
  ['dashboard','jobs','users','cdks','usage','providers','api-catalog','password','detail'].forEach(v=>show('#admin-view-'+v,v===name));
  document.querySelectorAll('.admin-nav [data-admin-view]').forEach(btn=>{
    const current=btn.dataset.adminView===name;
    btn.classList.toggle('active',current);
    btn.setAttribute('aria-current',current?'page':'false');
  });
  if(name==='dashboard')loadAdminStats(true);
  if(name==='jobs'){loadAdminUsers();loadAdminJobs()}
  if(name==='users')loadAdminUsers();
  if(name==='cdks')void loadAdminCDKs();
  if(name==='usage')requestAdminUsageSettings();
  if(name==='providers')void loadProviders();
  if(name==='api-catalog')void loadAdminApiCatalog();
}
bindAdminApiCatalog();
$('#admin-console-btn').addEventListener('click',()=>openAdminConsole());
$('#settings-link').addEventListener('click',()=>openAdminConsole('usage'));
$('#admin-back-front').addEventListener('click',closeAdminConsole);
$('#admin-refresh-stats').addEventListener('click',async e=>{
  const btn=e.currentTarget;
  btn.disabled=true;
  btn.classList.add('loading');
  try{await loadAdminStats(true)}finally{btn.disabled=false;btn.classList.remove('loading')}
});
$('#admin-detail-back').addEventListener('click',()=>adminShowView('jobs'));
document.querySelectorAll('.admin-nav [data-admin-view]').forEach(btn=>btn.addEventListener('click',()=>{
  adminShowView(btn.dataset.adminView);
  if(matchMedia('(max-width:960px)').matches)setSidebar(false);
}));
$('#admin-password-form').addEventListener('submit',async e=>{
  e.preventDefault();
  if(!adminOpen||adminCurrentView!=='password'||session?.user?.role!=='admin'||adminPasswordPendingEpoch===sessionEpoch)return;
  const form=e.currentTarget;
  const epoch=sessionEpoch;
  const values=Object.fromEntries(new FormData(form));
  adminPasswordPendingEpoch=epoch;
  $('#admin-password-submit').disabled=true;
  try{
    if(await saveOwnPassword(values)&&epoch===sessionEpoch)form.reset();
  }finally{
    if(adminPasswordPendingEpoch===epoch)adminPasswordPendingEpoch=null;
    if(epoch===sessionEpoch)$('#admin-password-submit').disabled=false;
  }
});

const ACTIVE_POLL_MS=5000,IDLE_POLL_MS=30000;
function jobIsActive(status){return !['completed','failed','cancelled'].includes(status)}
function jobHasExpiredPreview(job,now=Date.now()){
  const urls=[job?.work?.coverPreviewUrl,...(job?.result?.files||[]).flatMap(file=>[file.previewUrl,file.thumbnailUrl])];
  return urls.some(url=>{
    const expires=previewExpiry(url);
    return expires!==null&&expires<=now;
  });
}
function pollVisibleJobs(force=false){
  if(!session||document.hidden)return;
  if(currentJob&&jobExpired(currentJob)){
    if(adminOpen&&adminCurrentView==='detail')renderAdminDetail(currentJob);
    else if(!adminOpen&&currentJob.id===selectedId)renderDetail();
  }
  const now=Date.now();
  const due=(key,active)=>force||active||now-(pollRequestedAt[key]??0)>=IDLE_POLL_MS;
  if(adminOpen){
    if(adminCurrentView==='dashboard'&&due('stats',adminStatsHaveActive))loadAdminStats();
    if(adminCurrentView==='jobs'&&due('adminJobs',adminJobsCache.some(job=>jobIsActive(job.status))))loadAdminJobs(true);
    if(adminCurrentView==='detail'&&currentJob?.id&&due('adminDetail',jobIsActive(currentJob.status)))viewAdminJob(currentJob.id,true,true);
    return;
  }
  if(due('jobs',jobsCache.some(job=>jobIsActive(job.status))))loadJobs(true);
  if(currentView==='new'&&due('usage',false))void refreshDailyUsage();
  if(!selectedId)return;
  const active=jobIsActive(currentJob?.status);
  if(!due('detail',active))return;
  const hasSummary=jobsCache.some(job=>job.id===selectedId);
  if(force||active||!hasSummary||jobHasExpiredPreview(currentJob,now))loadJobDetail(selectedId,true);
}
function startJobPolling(){
  clearInterval(poll);
  poll=session&&!document.hidden?setInterval(pollVisibleJobs,ACTIVE_POLL_MS):null;
}
document.addEventListener('visibilitychange',()=>{
  startJobPolling();
  if(!document.hidden)pollVisibleJobs(true);
});

let adminPasswordPendingEpoch=null;
function resetAdminPasswordForm(){
  $('#admin-password-form').reset();
  $('#admin-password-submit').disabled=adminPasswordPendingEpoch===sessionEpoch;
}
function syncHeaderActions(){
  const isAdmin=session?.user?.role==='admin';
  show('#admin-console-btn',isAdmin&&!adminOpen);
  show('#admin-back-front',isAdmin&&adminOpen);
  show('#change-password',!!session&&!isAdmin&&!session.user?.isCdk);
  show('#logout',!!session&&!adminOpen);
}
function enter(value){
  dismissActionDialog?.();
  clearInterval(poll);
  sessionEpoch++;jobsLoadId++;jobDetailLoadId++;adminJobsLoadId++;adminUsersLoadId++;adminDetailLoadId++;
  adminDetailSig='';jobsPending=null;jobDetailPending=null;adminJobsPending=null;adminDetailPending=null;
  pollRequestedAt={};adminStatsHaveActive=false;
  $('#capture-feedback').textContent='';
  $('#daily-usage')?.classList.add('hidden');
  resetCapturePermissions();
  jobsCache=[];adminJobsCache=[];adminUsers=[];selectedId=null;currentJob=null;detailSig='';
  lastStatsSig='';lastAdminJobsSig='';
  adminStatsPending=null;
  archiveDownloads.clear();
  videoPreviewURLs.clear();
  videoPreviewFailure=null;
  if(!$('#preview-modal').classList.contains('hidden')){
    previewMusicSuspended=false;
    closePreview();
  }
  save(value);
  adminMode=false;currentView='new';adminOpen=false;jobsPage=1;adminJobsPage=1;
  setSidebar(false);
  if($('#admin-user-name'))$('#admin-user-name').textContent=value?.user?.displayName||'';
  if($('#admin-user-role'))$('#admin-user-role').textContent=value?.user?.role==='admin'?'管理员':'普通用户';
  if($('#admin-user-avatar'))$('#admin-user-avatar').textContent=(value?.user?.displayName||'?').trim().charAt(0).toUpperCase();
  show('#login',!value);show('#app',!!value);show('#admin-console',false);
  resetAdminPasswordForm();
  resetProviderSettings();
  resetCDKView();
  resetAdminJobManagement();
  resetAdminApiCatalog();
  closeAdminUserSessions();
  closeAdminUserMenu();
  $('#admin-users-q').value='';
  $('#admin-users-role').value='';
  $('#admin-users-active').value='';
  $('#admin-jobs-user').innerHTML='<option value="">全部用户</option>';
  syncHeaderActions();
  // 未登录时隐藏左上角菜单按钮：导航抽屉是登录后的功能，登录页点了只会打开空的抽屉。
  // 用 visibility 而非 display，保留其在顶栏网格中的 40px 占位，避免品牌文字被顶位裁剪。
  $('#sidebar-toggle').classList.toggle('nav-hidden',!value);
  document.querySelectorAll('.admin-only').forEach(el=>el.classList.toggle('hidden',value?.user?.role!=='admin'));
  if(value){
    selectJob(null);
    if(!document.hidden)loadJobs();
    startJobPolling();
  }else{
    clearInterval(poll);
    stopTaskMusic();
    setSidebar(false); // 退出登录时若抽屉还开着，一并收起
  }
}

/* ---------- 登录 / 退出 / 连接 ---------- */
$('#api-url').value=location.origin;
if($('#current-api')){$('#current-api').textContent=location.origin;$('.foot-api').title=location.origin;}
$('#api-form').addEventListener('submit',e=>{e.preventDefault();try{const target=normaliseAPI($('#api-url').value);if(target===location.origin)return toast('当前已打开此服务');location.assign(`${target}/`)}catch(err){toast(err.message,'error')}});
function loginDevice(){
  return {id:localStorage.dc_device||(localStorage.dc_device=uuidv4()),name:navigator.platform||'Web 浏览器',platform:'windows',appVersion:'web-0.1.0'};
}
function bindPasswordToggle(inputId,buttonId,credential='密码'){
  const input=$(inputId),btn=$(buttonId);
  if(!input||!btn)return;
  btn.innerHTML=icon('eye',16);
  btn.addEventListener('click',e=>{
    e.preventDefault();e.stopPropagation();
    const show=input.type==='password';
    input.type=show?'text':'password';
    btn.classList.toggle('on',show);
    btn.title=`${show?'隐藏':'显示'}${credential}`;
    btn.setAttribute('aria-label',btn.title);
    btn.innerHTML=icon(show?'eye-off':'eye',16);
    input.focus();
  });
}
bindPasswordToggle('#password','#toggle-password');
bindPasswordToggle('#login-cdk','#toggle-cdk','密钥');
bindLoginForms();
bindAdminCDKs();
$('#change-password').addEventListener('click',changeOwnPassword);
$('#logout').addEventListener('click',logout);
$('#admin-logout').addEventListener('click',()=>{setSidebar(false);void logout()});
let logoutPendingEpoch=null;
async function logout(){
  const epoch=sessionEpoch;
  if(!session||logoutPendingEpoch===epoch)return;
  logoutPendingEpoch=epoch;
  try{
    const message=session.user?.isCdk?'额度用完前可使用当前 CDK 密钥重新登录。确认退出？':'确认退出当前账号？';
    if(!(await confirmDialog({title:'退出登录',message,confirmText:'退出',danger:false}))||epoch!==sessionEpoch)return;
    if(refreshing?.epoch===epoch)await refreshing.promise.catch(()=>false);
    if(epoch!==sessionEpoch||!session)return;
    try{await api('/api/v1/auth/logout',{method:'POST'})}catch{}
    if(epoch===sessionEpoch)enter(null);
  }finally{
    if(logoutPendingEpoch===epoch)logoutPendingEpoch=null;
  }
}

function passwordChangeDialog(){
  const wrap=document.createElement('div');
  wrap.className='password-change-grid';
  const fields=[
    {key:'currentPassword',label:'当前密码',autocomplete:'current-password'},
    {key:'newPassword',label:'新密码（6 至 12 位）',autocomplete:'new-password'},
    {key:'confirmPassword',label:'确认新密码',autocomplete:'new-password'}
  ];
  const inputs={};
  fields.forEach(field=>{
    const label=document.createElement('label');
    label.appendChild(document.createTextNode(field.label));
    const box=document.createElement('span');
    box.className='password-wrap';
    const input=document.createElement('input');
    input.type='password';
    if(field.key==='currentPassword')input.maxLength=1024;
    else input.pattern='.{6,12}';
    input.required=true;
    input.autocomplete=field.autocomplete;
    const toggle=document.createElement('button');
    toggle.type='button';
    toggle.className='password-eye';
    toggle.setAttribute('aria-label','显示密码');
    toggle.title='显示密码';
    toggle.innerHTML=icon('eye',16);
    toggle.addEventListener('click',e=>{
      e.preventDefault();e.stopPropagation();
      const show=input.type==='password';
      input.type=show?'text':'password';
      toggle.classList.toggle('on',show);
      toggle.setAttribute('aria-label',show?'隐藏密码':'显示密码');
      toggle.title=show?'隐藏密码':'显示密码';
      toggle.innerHTML=icon(show?'eye-off':'eye',16);
      input.focus();
    });
    box.appendChild(input);
    box.appendChild(toggle);
    label.appendChild(box);
    wrap.appendChild(label);
    inputs[field.key]=input;
  });
  return openActionDialog({
    title:'修改密码',message:'保存后，其他设备将退出登录，当前设备保持登录。',confirmText:'保存',
    fields:wrap,initialFocus:inputs.currentPassword,
    collect:()=>{
      if(Object.values(inputs).every(input=>input.reportValidity())){
        return {currentPassword:inputs.currentPassword.value,newPassword:inputs.newPassword.value,confirmPassword:inputs.confirmPassword.value};
      }
    }
  });
}
async function changeOwnPassword(){
  const values=await passwordChangeDialog();
  if(values)await saveOwnPassword(values);
}
function validNewPassword(value){
  const length=Array.from(value).length;
  return length>=6&&length<=12;
}
async function saveOwnPassword(values){
  if(!values.currentPassword){toast('请输入当前密码','warning');return false}
  if(!validNewPassword(values.newPassword)){toast('新密码须为 6 至 12 位','warning');return false}
  if(values.newPassword!==values.confirmPassword){toast('两次输入的新密码不一致','warning');return false}
  const epoch=sessionEpoch;
  try{
    await api('/api/v1/auth/password',{method:'POST',body:JSON.stringify({currentPassword:values.currentPassword,newPassword:values.newPassword})});
    if(epoch!==sessionEpoch)return false;
    toast('密码已修改，其他设备会话已下线','success');
    return true;
  }catch(err){if(epoch===sessionEpoch)toast(err.message,'error');return false}
}

/* ---------- 转写服务设置 ---------- */
const asrProviderNames={siliconflow:'硅基流动',aliyun:'阿里云',deepseek:'DeepSeek'};
const siliconModels=['FunAudioLLM/SenseVoiceSmall','TeleAI/TeleSpeechASR'];
const providersPath='/api/v1/admin/settings/providers';
let providerSettings=null,providerOperation=null;

function resetProviderSettings(){
  providerSettings=null;
  providerOperation=null;
  $('#provider-form').reset();
  $('#provider-fields').disabled=true;
  $('#providers-refresh').disabled=false;
  document.querySelectorAll('[data-provider-result]').forEach(el=>el.textContent='');
}

function providerStatus(text, tone, target){
  if(target.textContent!==text)target.textContent=text;
  target.classList.toggle('failed',tone==='error');
  target.classList.toggle('success',tone==='success');
}

function enabledProviderOptions(preferred){
  const select=$('#default-asr-provider');
  const ids=[...document.querySelectorAll('#provider-form input[name="enabledProviders"]:checked')].map(input=>input.value);
  const selected=preferred??select.value;
  select.innerHTML='<option value="">自动选择可用服务</option>'+ids.map(id=>`<option value="${esc(id)}">${esc(asrProviderNames[id])}</option>`).join('');
  select.value=ids.includes(selected)?selected:'';
}

function syncSiliconModel(){
  const custom=$('#asr-model').value==='custom';
  show('#asr-model-custom-wrap',custom);
  $('#asr-model-custom').disabled=!custom;
  $('#asr-model-custom').required=custom;
}

function renderProviderSettings(settings, applyDraft){
  providerSettings=settings;
  const effective=asrProviderNames[settings.effectiveDefaultProvider];
  const configured=asrProviderNames[settings.defaultProvider];
  $('#provider-policy-note').textContent=effective?`当前默认服务为「${effective}」。`:
    configured?`默认服务「${configured}」不可用。可重新配置，或更改默认服务。`:'当前没有可用的默认服务。';
  for(const card of document.querySelectorAll('#provider-form [data-provider]')){
    const item=settings.items.find(provider=>provider.id===card.dataset.provider);
    if(!item)continue;
    const badge=card.querySelector('[data-provider-status]');
    badge.textContent=item.available?'已配置':item.configured?'缺少公网 HTTPS 地址':'未配置';
    badge.className='badge badge-'+(item.available?'success':item.configured?'warning':'muted');
    card.querySelector('[data-provider-test]').disabled=!item.available;
    card.querySelector('[data-provider-clear]').disabled=!item.configured;
    card.querySelector('input[type=password]').placeholder=item.configured?'输入新 Key 以替换':'输入 API Key';
    if(applyDraft)card.querySelector('input[role=switch]').checked=item.enabled;
  }
  if(applyDraft){
    $('#provider-form').elements.namedItem('allowUserSelection').checked=settings.allowUserSelection;
    enabledProviderOptions(settings.defaultProvider);
    const model=settings.items.find(provider=>provider.id==='siliconflow')?.model||siliconModels[0];
    $('#asr-model').value=siliconModels.includes(model)?model:'custom';
    $('#asr-model-custom').value=siliconModels.includes(model)?'':model;
    syncSiliconModel();
  }
}

async function runProviderOperation(action, card, options, trigger){
  if(providerOperation||!adminOpen||adminCurrentView!=='providers'||session?.user?.role!=='admin')return;
  const request={epoch:sessionEpoch};
  providerOperation=request;
  const active=()=>providerOperation===request&&request.epoch===sessionEpoch&&adminOpen&&adminCurrentView==='providers';
  const id=card?.dataset.provider;
  const result=card?.querySelector('[data-provider-result]')||$('#provider-status');
  const fields=$('#provider-fields'),refresh=$('#providers-refresh');
  try{
    if(action==='clear'){
      const confirmed=await confirmDialog({
        title:'清除 API Key',
        message:`清除 ${asrProviderNames[id]} 的 API Key？该服务将停止接收新任务，直至重新配置。`,
        confirmText:'清除 Key',
      });
      if(!active()||!confirmed){if(active())trigger?.focus();return}
    }
    fields.disabled=true;
    refresh.disabled=true;
    providerStatus({load:'正在加载配置…',save:'正在保存配置…',test:'正在测试连接…',clear:'正在清除 Key…'}[action],'',result);
    const endpoint=action==='test'?`${providersPath}/${id}/test`:action==='clear'?`${providersPath}/${id}/key`:providersPath;
    const response=await api(endpoint,options);
    if(!active())return;
    if(action==='test'){
      providerStatus(`连接正常 · ${response.durationMs} ms`,'success',result);
      return;
    }
    renderProviderSettings(response.providers,action!=='clear');
    if(action==='clear'){
      card.querySelector('input[type=password]').value='';
      providerStatus('API Key 已清除','success',result);
    }else if(action==='save'){
      document.querySelectorAll('#provider-form input[type=password]').forEach(input=>input.value='');
      document.querySelectorAll('[data-provider-result]').forEach(el=>providerStatus('','',el));
      providerStatus('转写设置已保存','success',result);
    }else{
      providerStatus('','',result);
    }
  }catch(err){
    if(active())providerStatus(err.message,'error',result);
  }finally{
    if(active()){
      providerOperation=null;
      fields.disabled=!providerSettings;
      refresh.disabled=false;
      if(trigger?.isConnected){
        if(!trigger.disabled)trigger.focus();
        else if(action==='clear')card.querySelector('input[type=password]').focus();
      }
    }
  }
}
function loadProviders(){
  return runProviderOperation('load',null,{cache:'no-store'});
}
$('#providers-refresh').addEventListener('click',loadProviders);
$('#asr-model').addEventListener('change',syncSiliconModel);
$('#provider-form').addEventListener('change',e=>{
  if(e.target.name==='enabledProviders')enabledProviderOptions();
});
$('#provider-form').addEventListener('click',e=>{
  const button=e.target.closest('[data-provider-test],[data-provider-clear]');
  if(!button)return;
  const card=button.closest('[data-provider]');
  if(button.hasAttribute('data-provider-test'))return runProviderOperation('test',card,{method:'POST'},button);
  return runProviderOperation('clear',card,{method:'DELETE'},button);
});
$('#provider-form').addEventListener('submit',e=>{
  e.preventDefault();
  if(!providerSettings||!e.currentTarget.reportValidity())return;
  const data=new FormData(e.currentTarget);
  const body={
    defaultProvider:data.get('defaultProvider')||'',
    allowUserSelection:data.has('allowUserSelection'),
    enabledProviders:data.getAll('enabledProviders'),
  };
  const model=$('#asr-model').value==='custom'?$('#asr-model-custom').value.trim():$('#asr-model').value;
  if(!model){
    $('#asr-model-custom').focus();
    providerStatus('请输入自定义模型 ID','error',$('#provider-status'));
    return;
  }
  if(model!==providerSettings.items.find(item=>item.id==='siliconflow')?.model)body.asrModel=model;
  for(const name of ['siliconFlowApiKey','aliyunApiKey','deepseekApiKey']){
    const key=data.get(name)?.trim();
    if(key)body[name]=key;
  }
  return runProviderOperation('save',null,{method:'PUT',body:JSON.stringify(body)},e.submitter);
});

/* ---------- 管理后台：使用设置 ---------- */
const adminUsagePermissionFields=['allowInfo','allowDownload','allowTranscribe'];
let adminUsagePending=null;

async function requestAdminUsageSettings(body){
  const epoch=sessionEpoch;
  if(!adminOpen||session?.user?.role!=='admin'||adminUsagePending?.epoch===epoch)return;
  const request={epoch};
  adminUsagePending=request;
  const form=$('#admin-usage-form'),fields=$('#admin-usage-fields'),refresh=$('#admin-usage-refresh');
  const status=$('#admin-usage-status'),saving=body!==undefined;
  const current=()=>adminUsagePending===request&&epoch===sessionEpoch;
  fields.disabled=true;
  refresh.disabled=true;
  form.setAttribute('aria-busy','true');
  status.classList.remove('failed');
  status.textContent=saving?'正在保存设置…':'正在加载设置…';
  try{
    const options=saving?{method:'PUT',body:JSON.stringify(body)}:{cache:'no-store'};
    const {settings}=await api('/api/v1/admin/settings/usage',options);
    if(!current())return;
    form.elements.namedItem('dailyTaskLimit').value=String(settings.dailyTaskLimit);
    for(const name of adminUsagePermissionFields)form.elements.namedItem(name).checked=settings[name];
    if(saving){
      dailyUsageRequestId++;
      applyCapturePermissions(settings);
    }
    fields.disabled=false;
    status.textContent=saving?'使用设置已保存':'';
  }catch(err){
    if(!current())return;
    fields.disabled=!saving;
    status.classList.add('failed');
    status.textContent=err.message;
  }finally{
    if(current()){
      adminUsagePending=null;
      refresh.disabled=false;
      form.setAttribute('aria-busy','false');
    }
  }
}
$('#admin-usage-refresh').addEventListener('click',()=>requestAdminUsageSettings());
$('#admin-usage-form').addEventListener('submit',e=>{
  e.preventDefault();
  if($('#admin-usage-fields').disabled)return;
  const form=e.currentTarget;
  if(!form.reportValidity())return;
  const body={dailyTaskLimit:form.elements.namedItem('dailyTaskLimit').valueAsNumber};
  for(const name of adminUsagePermissionFields)body[name]=form.elements.namedItem(name).checked;
  return requestAdminUsageSettings(body);
});

/* ---------- 新建任务 ---------- */
let dailyUsageRequestId=0;
let capturePermissions=null,captureUsageError=false,captureASR=null,captureCDKExhausted=false;
const captureModes=[['download','allowDownload'],['info','allowInfo'],['full','allowTranscribe']];
function renderCaptureProvider(action){
  const canChoose=action==='full'&&captureASR?.allowUserSelection===true&&captureASR.providers.length>0;
  show('#capture-asr-provider-wrap',canChoose);
  const status=$('#capture-asr-status');
  const unavailable=capturePermissions?.allowTranscribe===false&&captureASR?.transcribePermitted;
  const defaultLabel=action==='full'&&!canChoose&&captureASR?.defaultProvider&&asrProviderNames[captureASR.defaultProvider];
  const message=unavailable?(action?'当前没有可用的转写服务。':''):(defaultLabel?`转写服务商：${defaultLabel}`:'');
  if(status.textContent!==message)status.textContent=message;
  show('#capture-asr-status',!!message);
}
function updateCaptureProvider(asr,transcribePermitted){
  const ids=(asr?.providers||[]).map(item=>item.id).filter(id=>Object.hasOwn(asrProviderNames,id));
  const select=$('#capture-asr-provider'),value=select.value;
  const next=asr?{providers:ids,defaultProvider:asr.defaultProvider,allowUserSelection:asr.allowUserSelection,transcribePermitted}:null;
  if(JSON.stringify(next)!==JSON.stringify(captureASR)){
    const label=asrProviderNames[asr?.defaultProvider];
    const placeholder=label?`跟随默认设置（${esc(label)}）`:'请选择转写服务商';
    select.innerHTML=`<option value=""${label?'':' disabled'}>${placeholder}</option>`+ids.map(id=>`<option value="${esc(id)}">${esc(asrProviderNames[id])}</option>`).join('');
    select.value=asr?.allowUserSelection&&ids.includes(value)?value:'';
  }
  captureASR=next;
}
function captureAction(){
  return captureModes.find(([mode,permission])=>capturePermissions?.[permission]&&$(`#capture-mode-${mode}`).checked)?.[0]||null;
}
function renderCaptureAction(){
  const action=captureAction(),button=$('#submit-task');
  const needsProvider=action==='full'&&captureASR&&!captureASR.defaultProvider&&!$('#capture-asr-provider').value;
  const hints={info:'仅获取作品信息。',full:'下载媒体并转写视频口播。',download:'下载媒体。图文作品保留原配文。'};
  const anyAllowed=capturePermissions&&adminUsagePermissionFields.some(name=>capturePermissions[name]);
  const hint=$('#capture-action-hint');
  const message=captureCDKExhausted?'CDK 任务次数已用完，请联系管理员。':needsProvider?'请选择转写服务商。':action?hints[action]:!capturePermissions?
    (captureUsageError?'无法加载任务权限。':'正在读取任务权限…'):
    (anyAllowed?'请选择任务类型。':captureASR?.transcribePermitted?'当前没有可用的转写服务。':'管理员已关闭所有新建任务类型。');
  if(hint.textContent!==message)hint.textContent=message;
  button.disabled=!action||needsProvider||captureCDKExhausted||button.getAttribute('aria-busy')==='true';
  renderCaptureProvider(action);
}
function applyCapturePermissions(policy){
  if(!policy||policy.asr)updateCaptureProvider(policy?.asr,policy?.allowTranscribe);
  else if(captureASR)captureASR.transcribePermitted=policy.allowTranscribe;
  const hasProvider=!captureASR||!!captureASR.defaultProvider||(captureASR.allowUserSelection&&captureASR.providers.length>0);
  if(policy)policy={...policy,allowTranscribe:policy.allowTranscribe&&hasProvider};
  capturePermissions=policy;
  const selected=captureAction()||captureModes.find(([,permission])=>policy?.[permission])?.[0];
  show('#capture-options',!!selected);
  for(const [mode,permission] of captureModes){
    const input=$(`#capture-mode-${mode}`),allowed=!!policy?.[permission];
    show(`#capture-mode-${mode}-wrap`,allowed);
    input.disabled=!allowed;
    input.checked=mode===selected;
  }
  renderCaptureAction();
}
function resetCapturePermissions(){
  $('#submit-task').setAttribute('aria-busy','false');
  captureUsageError=false;
  captureCDKExhausted=false;
  show('#capture-usage-retry',false);
  applyCapturePermissions(null);
}
async function refreshDailyUsage(){
  const label=$('#daily-usage');
  if(!session||!label)return;
  const requestId=++dailyUsageRequestId,epoch=sessionEpoch;
  const current=()=>requestId===dailyUsageRequestId&&epoch===sessionEpoch&&!!session;
  pollRequestedAt.usage=Date.now();
  try{
    const {usage}=await api('/api/v1/jobs/usage');
    if(!current())return;
    if(adminUsagePermissionFields.some(name=>typeof usage?.[name]!=='boolean'))throw new Error('任务权限数据不可用');
    captureUsageError=false;
    show('#capture-usage-retry',false);
    const {used,limit,cdk}=usage;
    captureCDKExhausted=!!cdk&&cdk.used>=cdk.limit;
    applyCapturePermissions(usage);
    const daily=limit>0?`今日剩余 ${Math.max(0,limit-used)} / ${limit} 次 · 北京时间零点重置`:`今日已用 ${used} 次 · 不限次数`;
    const lifetime=cdk?`CDK 剩余 ${Math.max(0,cdk.limit-cdk.used)} / ${cdk.limit} 次`:'';
    label.textContent=lifetime&&limit>0?`${lifetime} · ${daily}`:lifetime||daily;
    label.classList.remove('hidden');
  }catch{
    if(!current())return;
    label.classList.add('hidden');
    captureUsageError=true;
    show('#capture-usage-retry',true);
    renderCaptureAction();
  }
}

$('#capture-usage-retry').addEventListener('click',refreshDailyUsage);
$('#capture-asr-provider').addEventListener('change',renderCaptureAction);
for(const [mode] of captureModes){
  $(`#capture-mode-${mode}`).addEventListener('change',renderCaptureAction);
}
$('#capture-form').addEventListener('submit',async e=>{
  e.preventDefault();
  const button=e.submitter||$('#submit-task');
  const action=captureAction();
  if(button.disabled||!action)return;
  const shareText=$('#share-text').value.trim();
  if(!shareText){$('#share-text').focus();return toast('请先粘贴抖音分享文本或作品链接','warning')}
  const epoch=sessionEpoch;
  const feedback=$('#capture-feedback');
  feedback.classList.remove('failed');
  feedback.textContent=action==='info'?'正在读取作品信息…':'正在提交任务…';
  button.disabled=true;button.setAttribute('aria-busy','true');
  try{
    const options={force:false,keepVideo:action==='full',languageHints:['zh','en'],hotwords:[]};
    const provider=$('#capture-asr-provider').value;
    if(action==='full'&&captureASR?.allowUserSelection&&captureASR.providers.includes(provider))options.asrProvider=provider;
    const data=await api('/api/v1/jobs',{method:'POST',headers:{'Idempotency-Key':uuidv4()},body:JSON.stringify({shareText,action,options})});
    if(epoch!==sessionEpoch||!session)return;
    feedback.textContent='';
    $('#share-text').value='';
    jobsPage=1;$('#query').value='';$('#status').value='';
    const job=data.job;
    if(job?.id)selectJob(job.id,job);
    if(job?.status==='failed')toast(job.error?.message||job.statusMessage||'解析失败','error');
    else if(job?.status==='completed')toast('作品信息解析完成','success');
    else if(job?.id)toast('任务已创建，正在处理','success');
    await loadJobs();
  }catch(err){
    if(epoch===sessionEpoch){
      feedback.textContent=err.message;
      feedback.classList.add('failed');
      void refreshDailyUsage();
    }
  }
  finally{
    if(epoch===sessionEpoch){
      button.setAttribute('aria-busy','false');
      renderCaptureAction();
    }
  }
});

/* ---------- 任务列表 ---------- */
$('#search-form').addEventListener('submit',e=>{e.preventDefault();jobsPage=1;loadJobs()});
$('#refresh').addEventListener('click',()=>{jobsPage=1;loadJobs()});
$('#status').addEventListener('change',()=>{jobsPage=1;loadJobs()});
async function loadJobs(polling=false){
  if(!session)return;
  const q=new URLSearchParams({view:'summary',q:$('#query').value.trim(),status:$('#status').value,limit:String(jobsPageSize),offset:String((jobsPage-1)*jobsPageSize)});
  if(polling&&jobsPending?.key===String(q)&&jobsPending.epoch===sessionEpoch)return;
  const requestId=++jobsLoadId,epoch=sessionEpoch,pending={key:String(q),epoch};
  jobsPending=pending;
  pollRequestedAt.jobs=Date.now();
  try{
    const data=await api(`/api/v1/jobs?${q}`);
    if(requestId!==jobsLoadId||epoch!==sessionEpoch||!session)return;
    jobsTotal=data.total||0;
    const totalPages=Math.max(1,Math.ceil(jobsTotal/jobsPageSize));
    if(jobsPage>totalPages){jobsPage=totalPages;return loadJobs()}
    const jobs=data.jobs||[];
    renderList(jobs);
    const selected=jobs.find(job=>job.id===selectedId);
    if(!document.hidden&&!adminOpen&&currentJob&&selected){
      if(selected.updatedAt!==currentJob.updatedAt||selected.status!==currentJob.status||jobMediaExpiry(selected)!==jobMediaExpiry(currentJob)){
        void loadJobDetail(selected.id,true);
      }else if(jobExpired(currentJob))renderDetail();
    }
    if($('#job-total'))$('#job-total').textContent=`${jobsTotal} 个任务`;
    renderPager($('#job-pager'),jobsPage,totalPages,p=>{jobsPage=p;loadJobs()});
  }catch(err){
    if(requestId!==jobsLoadId||epoch!==sessionEpoch)return;
    if(err.message.includes('登录'))enter(null);else toast(err.message,'error');
  }finally{if(jobsPending===pending)jobsPending=null}
}
async function loadJobDetail(id,polling=false){
  if(!session||adminOpen||selectedId!==id)return;
  if(polling&&jobDetailPending?.id===id&&jobDetailPending.epoch===sessionEpoch)return;
  const requestId=++jobDetailLoadId,epoch=sessionEpoch,pending={id,epoch};
  jobDetailPending=pending;
  pollRequestedAt.detail=Date.now();
  const active=()=>requestId===jobDetailLoadId&&epoch===sessionEpoch&&!adminOpen&&selectedId===id;
  try{
    const data=await api(`/api/v1/jobs/${encodeURIComponent(id)}`);
    if(!active()||!session)return;
    if(jobHasExpiredPreview(currentJob)&&!jobHasExpiredPreview(data.job))detailSig='';
    const justFailed=currentJob?.status&&jobIsActive(currentJob.status)&&data.job.status==='failed';
    currentJob=data.job;
    renderDetail();
    if(justFailed)toast(currentJob.error?.message||currentJob.statusMessage||'任务处理失败','error');
  }catch(err){
    if(!active())return;
    if(err.message.includes('登录')){enter(null);return}
    if(!currentJob)$('#detail').innerHTML=`<div class="error-box" role="status">${esc(err.message)}</div>`;
    if(!polling)toast(err.message,'error');
  }finally{if(jobDetailPending===pending)jobDetailPending=null}
}
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
// echarts.min.js 约 1MB，只有管理员仪表盘用到，改为打开仪表盘时才动态加载，避免拖慢登录页。
let echartsPromise=null;
function loadEcharts(){
  if(typeof echarts!=='undefined')return Promise.resolve();
  if(!echartsPromise)echartsPromise=new Promise((resolve,reject)=>{
    const s=document.createElement('script');
    s.src='/vendor/echarts.min.js';
    s.onload=()=>resolve();
    s.onerror=()=>{echartsPromise=null;reject(new Error('图表库加载失败'))};
    document.head.appendChild(s);
  });
  return echartsPromise;
}
function skeletonRows(count=4){
  return `<div class="skeleton-list" aria-hidden="true">${Array.from({length:count},()=>`<div class="skeleton-row"><span></span><span></span><span></span></div>`).join('')}</div>`;
}
let adminStatsPending=null;
const overviewTime=new Intl.DateTimeFormat('zh-CN',{timeZone:'Asia/Shanghai',hour12:false,hour:'2-digit',minute:'2-digit'});

async function loadAdminStats(force=false){
  if(!session||!adminOpen||adminCurrentView!=='dashboard'||adminStatsPending)return;
  const request={epoch:sessionEpoch};
  adminStatsPending=request;
  const current=()=>adminStatsPending===request&&request.epoch===sessionEpoch&&adminOpen&&adminCurrentView==='dashboard';
  const now=Date.now(),status=$('#admin-stats-status');
  pollRequestedAt.stats=now;
  const updateProviders=force||now-(pollRequestedAt.overviewProviders??0)>=IDLE_POLL_MS;
  if(updateProviders){
    pollRequestedAt.overviewProviders=now;
    void api(providersPath,{cache:'no-store'}).then(({providers})=>{
      if(request.epoch!==sessionEpoch||!adminOpen||adminCurrentView!=='dashboard')return;
      const ready=providers.items.filter(item=>item.enabled&&item.available).length;
      const defaultName=asrProviderNames[providers.effectiveDefaultProvider];
      $('#overview-provider-summary').textContent=ready?`${ready} 项可用 · ${defaultName||'默认服务不可用'}`:'暂无可用服务';
    }).catch(()=>{
      if(request.epoch===sessionEpoch&&adminOpen&&adminCurrentView==='dashboard')$('#overview-provider-summary').textContent='读取失败';
    });
  }
  if(!lastStatsSig){
    $('#admin-stats-cards').innerHTML=skeletonRows(4);
    document.querySelectorAll('.chart-card').forEach(card=>card.classList.add('loading'));
  }
  try{
    const {stats}=await api('/api/v1/admin/stats',{cache:'no-store'});
    if(!current())return;
    const {byStatus={},byDay=[]}=stats;
    adminStatsHaveActive=Object.entries(byStatus).some(([state,count])=>count>0&&jobIsActive(state));
    const sig=JSON.stringify([stats.users,stats.activeUsers,stats.totalJobs,stats.todayJobs,byStatus,byDay]);
    const fresh=sig!==lastStatsSig;
    if(fresh)renderStatCards(stats);
    providerStatus(`北京时间 ${overviewTime.format(new Date())} 更新`,'',status);
    if(fresh){
      await loadEcharts().catch(()=>null);
      if(!current())return;
      lastStatsSig=sig;
      renderStatusChart(byStatus);
      renderTrendChart(byDay);
      requestAnimationFrame(()=>{statusChart?.resize();trendChart?.resize()});
    }
  }catch(err){
    if(current())providerStatus(err.message,'error',status);
  }finally{
    if(adminStatsPending===request){
      adminStatsPending=null;
      document.querySelectorAll('.chart-card').forEach(card=>card.classList.remove('loading'));
    }
  }
}

function renderStatCards(stats){
  const counts=stats.byStatus||{};
  const processing=['resolving','downloading','extracting','transcribing','postprocessing'].reduce((sum,state)=>sum+(counts[state]||0),0);
  const queued=counts.queued||0,retry=counts.retry_wait||0,failed=counts.failed||0,completed=counts.completed||0;
  const cards=[
    {label:'今日新建',value:stats.todayJobs,detail:`累计 ${stats.totalJobs} 个任务`},
    {label:'待处理任务',value:queued+processing+retry,detail:`${queued} 个排队 · ${processing} 个处理中`},
    {label:'已完成任务',value:completed,detail:completed+failed?`成功率 ${Math.round(completed/(completed+failed)*100)}%（完成或失败）`:'暂无完成或失败任务'},
    {label:'已启用账号',value:stats.activeUsers,detail:`共 ${stats.users} 个账号`},
  ];
  $('#admin-stats-cards').innerHTML=cards.map(({label,value,detail})=>`<div class="stat-card"><span>${esc(label)}</span><strong>${esc(value??'—')}</strong><small>${esc(detail)}</small></div>`).join('');
  $('#overview-queued').textContent=String(queued);
  $('#overview-processing').textContent=String(processing);
  $('#overview-retry-wait').textContent=String(retry);
  $('#overview-failed').textContent=String(failed);
  $('#overview-active-users').textContent=`${stats.activeUsers} / ${stats.users}`;
}

$('#admin-view-dashboard').addEventListener('click',e=>{
  const button=e.target.closest('[data-overview-view],[data-overview-status]');
  if(!button)return;
  const status=button.dataset.overviewStatus;
  const view=status?'jobs':button.dataset.overviewView;
  if(status){
    $('#admin-jobs-q').value='';
    $('#admin-jobs-user').value='';
    $('#admin-jobs-status').value=status;
    adminJobsPage=1;
  }
  adminShowView(view);
  const heading=$(`#admin-view-${view} h2`);
  heading?.setAttribute('tabindex','-1');
  heading?.focus();
});

function chartColors(){
  const style=getComputedStyle(document.documentElement);
  const token=name=>style.getPropertyValue(name).trim();
  return {text:token('--text-secondary'),axis:token('--border'),grid:token('--input'),card:token('--card'),muted:token('--text-tertiary'),primary:token('--primary'),primary2:token('--primary-2'),primaryLight:token('--primary-light'),accent:token('--accent-2'),success:token('--success'),danger:token('--danger'),warning:token('--warning')};
}
function renderStatusChart(byStatus){
  const el=$('#chart-status');
  if(typeof echarts==='undefined'){el.innerHTML='<p class="meta">ECharts 未加载</p>';return}
  if(!statusChart)statusChart=echarts.init(el);
  const labels={queued:'排队中',resolving:'解析信息',downloading:'下载中',extracting:'提取音频',transcribing:'识别中',postprocessing:'生成结果',completed:'已完成',failed:'失败',retry_wait:'等待重试',cancelled:'已取消'};
  const c=chartColors();
  const colors={queued:c.muted,resolving:c.primary,downloading:c.primary2,extracting:c.accent,transcribing:c.primary2,postprocessing:c.accent,completed:c.success,failed:c.danger,retry_wait:c.warning,cancelled:c.text};
  const data=Object.entries(byStatus).map(([key,value])=>({name:labels[key]||key,value,itemStyle:{color:colors[key]||c.primary}})).filter(d=>d.value>0);
  const hasTasks=data.length>0;
  statusChart.setOption({
    color:Object.values(colors),
    animation:true,
    animationDuration:260,
    animationEasing:'cubicOut',
    animationDurationUpdate:180,
    animationEasingUpdate:'cubicOut',
    title:{show:!hasTasks,text:'暂无任务',left:'center',top:'center',textStyle:{color:c.muted,fontSize:14,fontWeight:400}},
    tooltip:{show:hasTasks,trigger:'item',formatter:'{b}: {c} ({d}%)'},
    legend:{show:hasTasks,bottom:0,textStyle:{color:c.text}},
    series:[{type:'pie',radius:['38%','64%'],center:['50%',hasTasks?'44%':'50%'],avoidLabelOverlap:true,silent:!hasTasks,itemStyle:{borderColor:c.card,borderWidth:2},label:{show:hasTasks,color:c.text},labelLine:{show:hasTasks},data:hasTasks?data:[{value:1,itemStyle:{color:c.grid}}]}]
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
    series:[{name:'任务数',type:'line',smooth:true,symbolSize:6,data:byDay.map(d=>d.count),itemStyle:{color:c.primary},lineStyle:{width:3},areaStyle:{color:{type:'linear',x:0,y:0,x2:0,y2:1,colorStops:[{offset:0,color:c.primaryLight},{offset:1,color:c.card}]}}}]
  },true);
}
window.addEventListener('resize',()=>{if(statusChart)statusChart.resize();if(trendChart)trendChart.resize()});

const adminJobsSelected=new Set();
const adminJobsBatchActions={
  cancel:{label:'取消',message:'将向系统请求取消这些任务。'},
  retry:{label:'重试',message:'这些任务将重新进入处理队列。'},
  delete:{label:'删除',message:'这些任务及其文件将永久删除。'}
};
let adminJobsListKey='',adminJobsSearchTimer=null,adminJobsMutation=null;

function adminJobsQuery(){
  return String(new URLSearchParams({
    view:'summary',q:$('#admin-jobs-q').value.trim(),status:$('#admin-jobs-status').value,
    userId:$('#admin-jobs-user').value,limit:String(adminJobsPageSize),offset:String((adminJobsPage-1)*adminJobsPageSize)
  }));
}
function viewingAdminJobs(){
  return adminOpen&&adminCurrentView==='jobs'&&session?.user?.role==='admin';
}
function adminJobAllows(job,op){
  if(op==='cancel')return jobIsActive(job.status);
  if(op==='retry')return ['failed','cancelled'].includes(job.status);
  return op==='delete'&&!jobIsActive(job.status);
}
function syncAdminJobsSelection(){
  const list=$('#admin-jobs-list'),all=$('#admin-jobs-select-all');
  const ready=!!adminJobsListKey&&adminJobsListKey===adminJobsQuery();
  const locked=!ready||!!adminJobsMutation?.running;
  show('#admin-jobs-batch',ready&&adminJobsCache.length>0);
  all.checked=adminJobsSelected.size>0&&adminJobsSelected.size===adminJobsCache.length;
  all.indeterminate=adminJobsSelected.size>0&&!all.checked;
  all.disabled=locked||!adminJobsCache.length;
  $('#admin-jobs-selected').textContent=`已选 ${adminJobsSelected.size} 项`;
  const selected=adminJobsCache.filter(job=>adminJobsSelected.has(job.id));
  for(const button of document.querySelectorAll('#admin-jobs-batch [data-admin-job-batch]')){
    button.disabled=locked||!selected.some(job=>adminJobAllows(job,button.dataset.adminJobBatch));
  }
  for(const input of list.querySelectorAll('[data-admin-job-select]')){
    input.disabled=locked;
    input.checked=adminJobsSelected.has(input.value);
  }
  for(const button of list.querySelectorAll('button[data-op]'))button.disabled=locked;
  syncAdminJobDetailActions();
}
function clearAdminJobsList(loading=false){
  adminJobsCache=[];
  adminJobsListKey='';lastAdminJobsSig='';
  adminJobsSelected.clear();
  const list=$('#admin-jobs-list');
  list.innerHTML=loading?skeletonRows(6):'';
  list.setAttribute('aria-busy',String(loading));
  $('#admin-jobs-total').textContent='';
  $('#admin-jobs-pager').replaceChildren();
  syncAdminJobsSelection();
}
function resetAdminJobManagement(){
  clearTimeout(adminJobsSearchTimer);
  adminJobsSearchTimer=null;adminJobsMutation=null;adminJobsTotal=0;
  $('#admin-jobs-batch-status').textContent='';
  $('#admin-jobs-form').reset();
  clearAdminJobsList();
}
function filterAdminJobs(debounce=false){
  if(!viewingAdminJobs()||adminJobsQuery()===adminJobsListKey)return;
  if(!adminJobsMutation)$('#admin-jobs-batch-status').textContent='';
  adminJobsPage=1;
  clearTimeout(adminJobsSearchTimer);
  if(!debounce){void loadAdminJobs();return}
  const epoch=sessionEpoch;
  clearAdminJobsList(true);
  adminJobsSearchTimer=setTimeout(()=>{
    adminJobsSearchTimer=null;
    if(epoch===sessionEpoch&&viewingAdminJobs())void loadAdminJobs();
  },250);
}
$('#admin-jobs-form').addEventListener('submit',e=>{
  e.preventDefault();
  filterAdminJobs();
});
$('#admin-jobs-q').addEventListener('input',e=>{if(!e.isComposing)filterAdminJobs(true)});
$('#admin-jobs-q').addEventListener('compositionend',()=>filterAdminJobs(true));
$('#admin-jobs-status').addEventListener('change',()=>filterAdminJobs());
$('#admin-jobs-user').addEventListener('change',()=>filterAdminJobs());

async function loadAdminJobs(polling=false){
  if(!session||(polling&&adminJobsSearchTimer!==null))return;
  clearTimeout(adminJobsSearchTimer);
  adminJobsSearchTimer=null;
  const key=adminJobsQuery();
  if(polling&&adminJobsPending?.key===key&&adminJobsPending.epoch===sessionEpoch)return;
  const requestId=++adminJobsLoadId,epoch=sessionEpoch,pending={key,epoch};
  const current=()=>requestId===adminJobsLoadId&&epoch===sessionEpoch&&session&&adminJobsSearchTimer===null&&key===adminJobsQuery();
  adminJobsPending=pending;
  pollRequestedAt.adminJobs=Date.now();
  if(key!==adminJobsListKey)clearAdminJobsList(true);
  const root=$('#admin-jobs-list');
  try{
    const data=await api(`/api/v1/admin/jobs?${key}`);
    if(!current())return;
    adminJobsCache=data.jobs||[];
    adminJobsTotal=data.total||0;
    const totalPages=Math.max(1,Math.ceil(adminJobsTotal/adminJobsPageSize));
    if(adminJobsPage>totalPages){adminJobsPage=totalPages;return loadAdminJobs()}
    adminJobsListKey=key;
    const visible=new Set(adminJobsCache.map(job=>job.id));
    for(const id of adminJobsSelected)if(!visible.has(id))adminJobsSelected.delete(id);
    root.setAttribute('aria-busy','false');
    // 数据与筛选条件未变化时不重绘表格，避免轮询闪烁
    const sig=JSON.stringify([key,adminJobsTotal,adminJobsCache.map(jobListSignature)]);
    if(sig!==lastAdminJobsSig){
      lastAdminJobsSig=sig;
      renderAdminJobs(adminJobsCache);
      $('#admin-jobs-total').textContent=`共 ${adminJobsTotal} 个任务`;
      renderPager($('#admin-jobs-pager'),adminJobsPage,totalPages,p=>{adminJobsPage=p;void loadAdminJobs()});
    }
    syncAdminJobsSelection();
  }catch(err){
    if(!current())return;
    root.setAttribute('aria-busy','false');
    if(key!==adminJobsListKey)root.innerHTML=`<div class="error-box" role="status">${esc(err.message)}</div>`;
    if(!polling)toast(err.message,'error');
  }
  finally{if(adminJobsPending===pending)adminJobsPending=null}
}
function adminJobOps(job,wrap=true){
  const ops=[`<button type="button" data-op="view">${icon('search',13)} 查看</button>`];
  if(adminJobAllows(job,'cancel'))ops.push(`<button type="button" data-op="cancel">${icon('circle-slash',13)} 取消</button>`);
  if(adminJobAllows(job,'retry'))ops.push(`<button type="button" data-op="retry">${icon('rotate-ccw',13)} 重试</button>`);
  if(adminJobAllows(job,'delete'))ops.push(`<button type="button" data-op="delete" class="danger">${icon('trash',13)} 删除</button>`);
  return wrap?`<div class="admin-ops-row">${ops.join('')}</div>`:ops.join('');
}
function adminJobCheckbox(job,title){
  return `<input type="checkbox" class="admin-selection-checkbox" data-admin-job-select value="${esc(job.id)}" aria-label="选择任务 ${esc(title)}">`;
}
function adminJobExpiryBadge(job){
  return jobExpired(job)?'<span class="badge badge-warning">内容已过期</span>':'';
}
function adminJobCardHTML(job){
  const w=job.work||{};
  const title=stripHashtags(w.title,w.hashtags)||`任务 ${job.id.slice(0,8)}`;
  const tone=statusTone[job.status]||'muted';
  const owner=job.ownerDisplayName||job.ownerUsername||'-';
  return `<article class="admin-card" data-id="${esc(job.id)}">
    <div class="admin-card-head"><strong>${esc(title)}</strong><div class="admin-job-status"><span class="badge badge-${tone}">${esc(statusLabels[job.status]||job.status)}</span>${adminJobExpiryBadge(job)}</div></div>
    <div class="admin-card-meta">
      <span>创建：${fmtShort(job.createdAt)}</span>
      <span>用户：${esc(owner)}</span>
      <span>类型：${esc(actionLabels[job.action]||job.action)}</span>
    </div>
    <div class="admin-card-actions"><label class="admin-job-selection">${adminJobCheckbox(job,title)} 选择</label>${adminJobOps(job,false)}</div>
  </article>`;
}
function renderAdminJobs(jobs){
  const root=$('#admin-jobs-list');
  if(!jobs.length){root.innerHTML=`<div class="empty-state">${icon('inbox',36)}<p>暂无任务</p><span>调整筛选条件后重试</span></div>`;return}
  const table=`<div class="admin-table-wrap"><table class="admin-table admin-jobs-table"><thead><tr><th class="admin-selection-cell" scope="col">选择</th><th>创建时间</th><th>用户</th><th>任务</th><th>类型</th><th>状态</th><th>操作</th></tr></thead><tbody>`+jobs.map(job=>{
    const w=job.work||{};
    const title=stripHashtags(w.title,w.hashtags)||`任务 ${job.id.slice(0,8)}`;
    const tone=statusTone[job.status]||'muted';
    const owner=job.ownerDisplayName||job.ownerUsername||'-';
    return `<tr data-id="${esc(job.id)}">
      <td class="admin-selection-cell">${adminJobCheckbox(job,title)}</td>
      <td class="nowrap">${fmtShort(job.createdAt)}</td>
      <td class="nowrap">${esc(owner)}</td>
      <td class="admin-title">${esc(title)}</td>
      <td>${esc(actionLabels[job.action]||job.action)}</td>
      <td><div class="admin-job-status"><span><span class="dot dot-${tone}"></span>${esc(statusLabels[job.status]||job.status)}</span>${adminJobExpiryBadge(job)}</div></td>
      <td class="admin-ops nowrap">${adminJobOps(job)}</td>
    </tr>`;
  }).join('')+`</tbody></table></div>`;
  root.innerHTML=table+`<div class="admin-card-list">${jobs.map(adminJobCardHTML).join('')}</div>`;
}
$('#admin-view-jobs').addEventListener('click',e=>{
  const batch=e.target.closest('button[data-admin-job-batch]');
  if(batch&&!batch.disabled){void operateAdminJobsBatch(batch.dataset.adminJobBatch);return}
  const btn=e.target.closest('button[data-op]');
  if(!btn||btn.disabled)return;
  const row=btn.closest('tr[data-id],.admin-card[data-id]');
  if(!row)return;
  if(btn.dataset.op==='view')viewAdminJob(row.dataset.id);
  else operateAdminJob(row.dataset.id,btn.dataset.op);
});
$('#admin-jobs-select-all').addEventListener('change',e=>{
  if(e.target.disabled)return;
  adminJobsSelected.clear();
  if(e.target.checked)for(const job of adminJobsCache)adminJobsSelected.add(job.id);
  syncAdminJobsSelection();
});
$('#admin-jobs-list').addEventListener('change',e=>{
  const input=e.target.closest('[data-admin-job-select]');
  if(!input||input.disabled||!adminJobsCache.some(job=>job.id===input.value))return;
  if(input.checked)adminJobsSelected.add(input.value);
  else adminJobsSelected.delete(input.value);
  syncAdminJobsSelection();
});
async function viewAdminJob(id,refresh=false,polling=false){
  if(!session||!adminOpen||(!refresh&&adminJobsMutation?.running))return;
  if(polling&&adminDetailPending?.id===id&&adminDetailPending.epoch===sessionEpoch)return;
  const requestId=++adminDetailLoadId,epoch=sessionEpoch,pending={id,epoch};
  adminDetailPending=pending;
  pollRequestedAt.adminDetail=Date.now();
  try{
    const data=await api(`/api/v1/admin/jobs/${encodeURIComponent(id)}`);
    if(requestId!==adminDetailLoadId||epoch!==sessionEpoch||!session||!adminOpen)return;
    if(refresh&&(adminCurrentView!=='detail'||currentJob?.id!==id))return;
    if(!refresh&&adminJobsMutation?.running)return;
    const renewedExpiredPreview=refresh&&jobHasExpiredPreview(currentJob)&&!jobHasExpiredPreview(data.job);
    renderAdminDetail(data.job,renewedExpiredPreview);
    if(!refresh){
      adminShowView('detail');
      if(matchMedia('(max-width:960px)').matches)setSidebar(false);
    }
  }catch(err){if(requestId===adminDetailLoadId&&epoch===sessionEpoch&&adminOpen)toast(err.message,'error')}
  finally{if(adminDetailPending===pending)adminDetailPending=null}
}
function renderAdminDetail(job,renewedExpiredPreview=false){
  adminMode=true;currentJob=job;
  const sig=job.id+'|'+jobSignature(job);
  if(sig!==adminDetailSig||renewedExpiredPreview){
    adminDetailSig=sig;
    $('#admin-detail').innerHTML=jobDetailHTML(job);
    syncAdminJobDetailActions();
  }
  syncDetailMedia(job);
}
function syncAdminJobDetailActions(){
  const disabled=!!adminJobsMutation?.running;
  for(const button of document.querySelectorAll('#admin-detail button:is([data-op="cancel"],[data-op="retry"],[data-op="delete"])'))button.disabled=disabled;
}
function sendAdminJobOperation(id,op){
  return api(`/api/v1/admin/jobs/${encodeURIComponent(id)}${op==='delete'?'':`/${op}`}`,{method:op==='delete'?'DELETE':'POST'});
}
async function operateAdminJobsBatch(op){
  const action=adminJobsBatchActions[op],key=adminJobsListKey;
  if(!action||adminJobsMutation||!viewingAdminJobs()||key!==adminJobsQuery())return;
  const jobs=adminJobsCache.filter(job=>adminJobsSelected.has(job.id)&&adminJobAllows(job,op));
  if(!jobs.length)return;
  const skipped=adminJobsSelected.size-jobs.length;
  const note=skipped?`另有 ${skipped} 个所选任务当前不支持${action.label}，将跳过。`:'';
  const epoch=sessionEpoch,request={running:false},opener=document.activeElement;
  const current=()=>adminJobsMutation===request&&epoch===sessionEpoch;
  adminJobsMutation=request;
  try{
    const confirmed=await confirmDialog({
      title:`批量${action.label}任务`,message:`将${action.label} ${jobs.length} 个任务。${action.message}${note}`,
      confirmText:`批量${action.label}`,danger:op!=='retry'
    });
    if(!confirmed||!current()||!viewingAdminJobs()||key!==adminJobsQuery())return;
    request.running=true;
    syncAdminJobsSelection();
    const status=$('#admin-jobs-batch-status');
    status.textContent=`正在批量${action.label} ${jobs.length} 个任务…`;
    let succeeded=0,failed=0,firstError='';
    for(const job of jobs){
      if(!current())return;
      try{
        await sendAdminJobOperation(job.id,op);
        if(!current())return;
        succeeded++;
        if(key===adminJobsListKey)adminJobsSelected.delete(job.id);
      }catch(err){
        failed++;
        if(!firstError)firstError=err.message;
      }
    }
    if(!current())return;
    status.textContent=`批量${action.label}完成：成功 ${succeeded} 个，失败 ${failed} 个${skipped?`，跳过 ${skipped} 个`:''}。${firstError?`首条失败原因：${firstError}`:''}`;
    if(viewingAdminJobs()&&adminJobsSearchTimer===null)await loadAdminJobs();
  }finally{
    if(current()){
      adminJobsMutation=null;
      syncAdminJobsSelection();
      if(request.running&&viewingAdminJobs()&&document.activeElement===document.body){
        const focus=opener?.isConnected&&!opener.disabled?opener:$('#admin-jobs-q');
        focus.focus({preventScroll:true});
      }
    }
  }
}
async function operateAdminJob(id,op){
  const viewingDetail=()=>adminOpen&&adminMode&&adminCurrentView==='detail'&&currentJob?.id===id&&session?.user?.role==='admin';
  const fromDetail=viewingDetail(),key=adminJobsListKey;
  const available=fromDetail?viewingDetail:()=>viewingAdminJobs()&&key===adminJobsQuery();
  const job=fromDetail?currentJob:adminJobsCache.find(item=>item.id===id);
  if(adminJobsMutation||!available()||!job||!adminJobAllows(job,op))return;
  const epoch=sessionEpoch,request={running:false};
  const current=()=>adminJobsMutation===request&&epoch===sessionEpoch;
  adminJobsMutation=request;
  try{
    if(op==='delete'&&!(await confirmDialog({title:'删除任务',message:'确认删除该任务及其文件？此操作不可撤销。',confirmText:'删除'})))return;
    if(!current()||!available())return;
    request.running=true;
    syncAdminJobsSelection();
    await sendAdminJobOperation(id,op);
    if(!current()||(!viewingDetail()&&!viewingAdminJobs()))return;
    toast(op==='delete'?'任务已删除':op==='cancel'?'已请求取消':'已重新开始','success');
    if(viewingDetail()){
      if(op==='delete')adminShowView('jobs');
      else await viewAdminJob(id,true);
    }else if(adminJobsSearchTimer===null)await loadAdminJobs();
  }catch(err){
    if(current()&&(viewingDetail()||viewingAdminJobs()))toast(err.message,'error');
  }finally{
    if(current()){
      adminJobsMutation=null;
      syncAdminJobsSelection();
    }
  }
}

/* ---------- 管理后台：用户管理 ---------- */
$('#admin-user-create').addEventListener('click',openAdminUserCreate);
function openAdminUserCreate(){
  const epoch=sessionEpoch;
  const current=()=>epoch===sessionEpoch&&adminOpen&&adminCurrentView==='users'&&session?.user?.role==='admin';
  if(!current())return;
  const form=$('#admin-user-create-template').content.firstElementChild.cloneNode(true);
  const fields=form.querySelector('fieldset'),error=form.querySelector('.admin-user-create-error');
  const {username,displayName,password,role}=form.elements;
  form.addEventListener('submit',event=>{event.preventDefault();$('#confirm-ok').click()});
  return openActionDialog({
    title:'创建用户',confirmText:'创建用户',fields:form,initialFocus:username,
    collect(){
      if(!current())return;
      error.textContent='';
      username.value=username.value.trim();
      displayName.value=displayName.value.trim();
      if(!form.reportValidity())return;
      if(!validNewPassword(password.value)){
        error.textContent='密码须为 6 至 12 位。';
        password.focus();
        return;
      }
      return {username:username.value,displayName:displayName.value,password:password.value,role:role.value};
    },
    async submit(value){
      fields.disabled=true;
      const controller=new AbortController();
      let timer;
      const timeout=new Promise((_,reject)=>{
        timer=setTimeout(()=>{
          controller.abort();
          reject(new Error('创建请求超时'));
        },15000);
      });
      try{
        await Promise.race([
          api('/api/v1/admin/users',{method:'POST',body:JSON.stringify(value),signal:controller.signal}),
          timeout,
        ]);
        if(current()){
          toast('用户已创建','success');
          void loadAdminUsers();
        }
        return true;
      }catch(err){
        if(!current())return true;
        if(controller.signal.aborted){
          error.textContent='创建请求超时，结果尚未确认。请关闭弹窗，在用户列表中核对账号是否已创建，再重新提交。';
          void loadAdminUsers();
        }else{
          error.textContent=err.message;
        }
        return false;
      }finally{
        clearTimeout(timer);
        fields.disabled=false;
      }
    },
  });
}
$('#admin-users-q').addEventListener('input',()=>renderAdminUsers(filterAdminUsers(adminUsers)));
$('#admin-users-role').addEventListener('change',()=>renderAdminUsers(filterAdminUsers(adminUsers)));
$('#admin-users-active').addEventListener('change',()=>renderAdminUsers(filterAdminUsers(adminUsers)));

async function loadAdminUsers(){
  if(!session)return;
  const requestId=++adminUsersLoadId,epoch=sessionEpoch;
  try{
    const root=$('#admin-users-list');
    if(root&&!root.children.length)root.innerHTML=skeletonRows(5);
    const data=await api('/api/v1/admin/users?includeDeletedJobOwners=1');
    if(requestId!==adminUsersLoadId||epoch!==sessionEpoch||!session)return;
    updateAdminUsers(data.users||[],data.deletedJobOwners||[]);
  }catch(err){if(requestId===adminUsersLoadId&&epoch===sessionEpoch)toast(err.message,'error')}
}
function updateAdminUsers(users,deletedJobOwners){
  const deletedIds=new Set(deletedJobOwners?.map(owner=>owner.id));
  adminUsers=deletedIds.size?users.filter(user=>!deletedIds.has(user.id)):users;
  renderAdminUsers(filterAdminUsers(adminUsers));
  if(!deletedJobOwners)return;
  const select=$('#admin-jobs-user'),selected=select.value;
  const options=(owners,includeUsername)=>owners.map(owner=>{
    const name=owner.displayName||owner.username;
    const label=includeUsername&&name!==owner.username?`${name}（${owner.username}）`:name;
    return `<option value="${esc(owner.id)}">${esc(label)}</option>`;
  }).join('');
  select.innerHTML='<option value="">全部用户</option>'+options(adminUsers,false)+
    (deletedJobOwners.length?`<optgroup label="已删除账号">${options(deletedJobOwners,true)}</optgroup>`:'');
  const hasSelection=deletedIds.has(selected)||adminUsers.some(owner=>owner.id===selected);
  select.value=hasSelection?selected:'';
  if(select.value!==selected){
    adminJobsPage=1;
    if(adminOpen&&adminCurrentView==='jobs')void loadAdminJobs();
  }
}
function filterAdminUsers(users){
  const query=$('#admin-users-q').value.trim().toLowerCase();
  const role=$('#admin-users-role').value,active=$('#admin-users-active').value;
  return users.filter(user=>(
    (!query||[user.username,user.displayName].some(value=>(value||'').toLowerCase().includes(query)))&&
    (!role||user.role===role)&&
    (!active||Boolean(user.isActive)===(active==='active'))
  ));
}
function taskLimitLabel(value){
  if(value==null)return '继承全局';
  return value===0?'不限':`${value} 次/天`;
}
function renderAdminUsers(users){
  closeAdminUserMenu();
  const root=$('#admin-users-list');
  if(!users.length){
    root.innerHTML='<p class="meta" role="status">没有匹配的用户。</p>';
    return;
  }
  const table=`<div class="admin-table-wrap"><table class="admin-table"><thead><tr><th>用户名</th><th>显示名</th><th>角色</th><th>状态</th><th>最近登录</th><th>创建时间</th><th>每日额度</th><th>操作</th></tr></thead><tbody>`+users.map(u=>`<tr data-id="${esc(u.id)}">
    <td class="nowrap">${esc(u.username)}</td>
    <td>${esc(u.displayName)}</td>
    <td>${u.role==='admin'?'管理员':'普通用户'}</td>
    <td class="nowrap">${u.isActive?'<span class="dot dot-success"></span>正常':'<span class="dot dot-danger"></span>已禁用'}</td>
    <td class="nowrap">${u.lastLoginAt?fmtDate(u.lastLoginAt):'-'}</td>
    <td class="nowrap">${fmtDate(u.createdAt)}</td>
    <td class="nowrap">${esc(taskLimitLabel(u.dailyTaskLimitOverride))}</td>
    <td class="admin-ops nowrap"><div class="admin-ops-row">${adminUserOps(u)}</div></td>
  </tr>`).join('')+`</tbody></table></div>`;
  const cards=`<div class="admin-card-list">${users.map(u=>`<article class="admin-card" data-id="${esc(u.id)}">
    <div class="admin-card-head"><strong>${esc(u.displayName||u.username)}</strong><span class="badge ${u.isActive?'badge-success':'badge-danger'}">${u.isActive?'正常':'已禁用'}</span></div>
    <div class="admin-card-meta">
      <span>用户名：${esc(u.username)}</span>
      <span>角色：${u.role==='admin'?'管理员':'普通用户'}</span>
      <span>最近登录：${u.lastLoginAt?fmtDate(u.lastLoginAt):'-'}</span>
      <span>创建：${fmtDate(u.createdAt)}</span>
      <span>每日额度：${esc(taskLimitLabel(u.dailyTaskLimitOverride))}</span>
    </div>
    <div class="admin-card-actions">${adminUserOps(u)}</div>
  </article>`).join('')}</div>`;
  root.innerHTML=table+cards;
}
function adminUserOps(user){
  return `
      <button type="button" data-op="view-user-jobs">查看任务</button>
      <button type="button" data-op="sessions">会话</button>
      <button type="button" data-op="daily-task-limit" aria-haspopup="dialog">设置额度</button>
      <button type="button" data-op="more-user" aria-haspopup="menu" aria-expanded="false" aria-controls="admin-user-menu" aria-label="更多操作 ${esc(user.username)}">更多</button>`;
}
$('#admin-view-users').addEventListener('click',e=>{
  const btn=e.target.closest('button[data-op]');
  if(!btn)return;
  const row=btn.closest('tr[data-id],.admin-card[data-id]');
  if(!row)return;
  const user=adminUsers.find(item=>item.id===row.dataset.id);
  if(!user)return;
  const op=btn.dataset.op;
  if(op==='view-user-jobs')return viewAdminUserJobs(user.id);
  if(op==='daily-task-limit')return setUserDailyTaskLimit(user.id);
  if(op==='sessions')return showUserSessions(user.id,user.displayName||user.username);
  if(op==='more-user')return toggleAdminUserMenu(user,btn);
});
function viewAdminUserJobs(id){
  $('#admin-jobs-user').value=id;
  $('#admin-jobs-q').value='';
  $('#admin-jobs-status').value='';
  adminJobsPage=1;
  adminShowView('jobs');
  $('#admin-jobs-q').focus();
}

let adminUserMenuAnchor=null;
function closeAdminUserMenu(restoreFocus=false){
  const anchor=adminUserMenuAnchor;
  if(!anchor)return;
  adminUserMenuAnchor=null;
  const menu=$('#admin-user-menu');
  menu.classList.add('hidden');
  delete menu.dataset.userId;
  menu.replaceChildren();
  anchor.setAttribute('aria-expanded','false');
  if(restoreFocus&&anchor.isConnected)anchor.focus();
}
function toggleAdminUserMenu(user,anchor){
  if(!adminOpen||adminCurrentView!=='users'||session?.user?.role!=='admin')return;
  if(anchor===adminUserMenuAnchor){closeAdminUserMenu(true);return}
  closeAdminUserMenu();
  adminUserMenuAnchor=anchor;
  anchor.setAttribute('aria-expanded','true');
  const menu=$('#admin-user-menu'),own=user.id===session.user.id;
  menu.dataset.userId=user.id;
  menu.innerHTML=`<button type="button" role="menuitem" tabindex="-1" data-user-menu-op="password">重置密码</button>
    <button type="button" role="menuitem" tabindex="-1" data-user-menu-op="${user.isActive?'disable':'enable'}">${user.isActive?'禁用':'启用'}</button>
    ${own?'':`<button type="button" role="menuitem" tabindex="-1" data-user-menu-op="delete-user" class="danger" aria-label="删除账号 ${esc(user.username)}">删除</button>`}`;
  menu.classList.remove('hidden');
  const rect=anchor.getBoundingClientRect(),{offsetWidth:width,offsetHeight:height}=menu;
  menu.style.left=`${Math.max(8,Math.min(rect.right-width,innerWidth-width-8))}px`;
  const top=rect.bottom+height+12<=innerHeight?rect.bottom+4:rect.top-height-4;
  menu.style.top=`${Math.max(8,top)}px`;
  menu.firstElementChild.focus({preventScroll:true});
}
$('#admin-user-menu').addEventListener('click',event=>{
  const button=event.target.closest('button[data-user-menu-op]');
  if(!adminUserMenuAnchor||!button)return;
  const id=$('#admin-user-menu').dataset.userId,anchor=adminUserMenuAnchor;
  closeAdminUserMenu(true);
  const op=button.dataset.userMenuOp;
  if(op==='password')return resetUserPassword(id);
  if(op==='delete-user')return deleteAdminUser(id,anchor);
  if(op==='disable'||op==='enable')return setUserActive(id,op==='enable');
});
$('#admin-user-menu').addEventListener('keydown',event=>{
  if(event.key==='Escape'||event.key==='Tab'){
    if(event.key==='Escape')event.preventDefault();
    closeAdminUserMenu(true);
    return;
  }
  const keys=['ArrowDown','ArrowUp','Home','End'];
  if(!keys.includes(event.key))return;
  event.preventDefault();
  const items=[...event.currentTarget.querySelectorAll('button')];
  const position=items.indexOf(document.activeElement);
  const next={ArrowDown:(position+1)%items.length,ArrowUp:(position+items.length-1)%items.length,Home:0,End:items.length-1};
  items[next[event.key]]?.focus();
});
document.addEventListener('click',event=>{
  if(adminUserMenuAnchor&&!adminUserMenuAnchor.contains(event.target)&&!$('#admin-user-menu').contains(event.target))closeAdminUserMenu();
});
document.addEventListener('scroll',()=>closeAdminUserMenu(),true);
window.addEventListener('resize',()=>closeAdminUserMenu());

const pendingUserDeletions=new Set();
async function deleteAdminUser(id,button){
  const user=adminUsers.find(item=>item.id===id),epoch=sessionEpoch,key=`${epoch}:${id}`;
  const current=()=>epoch===sessionEpoch&&session?.user?.role==='admin';
  const viewingUsers=()=>adminOpen&&adminCurrentView==='users';
  if(!current()||!viewingUsers()||!user||session.user.id===id||pendingUserDeletions.has(key))return;
  pendingUserDeletions.add(key);
  try{
    const confirmed=await confirmDialog({
      title:'删除用户',
      message:`账号「${user.username}」的所有会话将立即下线，之后无法登录。历史任务会保留。`,
      confirmText:'删除'
    });
    if(!confirmed||!current()||!viewingUsers()||!adminUsers.some(item=>item.id===id))return;
    if(button){
      button.disabled=true;
      button.setAttribute('aria-busy','true');
    }
    await api(`/api/v1/admin/users/${encodeURIComponent(id)}`,{method:'DELETE'});
    if(!current())return;
    // 丢弃删除前发起的读取，随后用当前筛选重新加载。
    adminUsersLoadId++;adminJobsLoadId++;adminJobsPending=null;
    lastAdminJobsSig='';lastStatsSig='';adminStatsPending=null;
    if($('#admin-user-sessions').dataset.userId===id)closeAdminUserSessions();
    // 保留当前任务筛选，等待服务器确认该账号是否还有历史任务。
    updateAdminUsers(adminUsers.filter(item=>item.id!==id));
    if(adminOpen){
      toast('用户已删除','success');
      void loadAdminUsers();
      if(adminCurrentView==='jobs')void loadAdminJobs();
      if(adminCurrentView==='dashboard')void loadAdminStats();
    }
  }catch(err){
    if(current()&&adminOpen)toast(err.message,'error');
  }finally{
    pendingUserDeletions.delete(key);
    if(button?.isConnected){
      button.disabled=false;
      button.setAttribute('aria-busy','false');
    }
  }
}
async function setUserActive(id,active){
  if(!active&&!(await confirmDialog({title:'禁用用户',message:'禁用后该用户的所有会话会立即下线且无法登录，确定？',confirmText:'禁用'})))return;
  try{
    await api(`/api/v1/admin/users/${encodeURIComponent(id)}/active`,{method:'PATCH',body:JSON.stringify({active})});
    toast(active?'已启用':'已禁用','success');await loadAdminUsers();
  }catch(err){toast(err.message,'error')}
}
async function resetUserPassword(id){
  const password=await promptDialog({title:'重置密码',message:'新密码须为 6 至 12 位。该用户的所有会话将立即下线。',placeholder:'新密码',password:true,confirmText:'重置'});
  if(password==null)return;
  if(!validNewPassword(password))return toast('密码须为 6 至 12 位','warning');
  try{
    await api(`/api/v1/admin/users/${encodeURIComponent(id)}/password`,{method:'POST',body:JSON.stringify({password})});
    toast('密码已重置，该用户会话已下线','success');
  }catch(err){toast(err.message,'error')}
}
const pendingUserTaskLimits=new Set();
async function setUserDailyTaskLimit(id){
  const user=adminUsers.find(item=>item.id===id),epoch=sessionEpoch,key=`${epoch}:${id}`;
  if(!user||!adminOpen||session?.user?.role!=='admin'||pendingUserTaskLimits.has(key))return;
  pendingUserTaskLimits.add(key);
  try{
    const raw=await promptDialog({
      title:`${user.displayName||user.username} 的每日额度`,
      message:'留空继承全局额度，输入 0 表示不限。',
      placeholder:'每日新建任务数',
      numberInput:{value:user.dailyTaskLimitOverride,max:100000},
      confirmText:'保存'
    });
    if(raw==null||epoch!==sessionEpoch||!adminOpen)return;
    const limit=raw===''?null:Number(raw);
    if(limit!==null&&(!Number.isInteger(limit)||limit<0||limit>100000))return toast('请输入 0 到 100000 之间的整数','warning');
    await api(`/api/v1/admin/users/${encodeURIComponent(id)}/daily-task-limit`,{method:'PUT',body:JSON.stringify({dailyTaskLimit:limit})});
    if(epoch!==sessionEpoch||!adminOpen)return;
    toast('用户额度已保存','success');
    await loadAdminUsers();
  }catch(err){
    if(epoch===sessionEpoch&&adminOpen)toast(err.message,'error');
  }finally{
    pendingUserTaskLimits.delete(key);
  }
}
let adminUserSessionsPanelId=0,adminUserSessionsRequestId=0;
function closeAdminUserSessions(){
  adminUserSessionsPanelId++;
  const host=$('#admin-user-sessions');
  delete host.dataset.userId;
  delete host.dataset.userName;
  host.classList.add('hidden');
  host.replaceChildren();
}
async function showUserSessions(id,username){
  const host=$('#admin-user-sessions');
  adminUserSessionsPanelId++;
  host.dataset.userId=id;host.dataset.userName=username||'';
  host.classList.remove('hidden');
  return loadAdminUserSessions();
}
async function loadAdminUserSessions(){
  const host=$('#admin-user-sessions');
  const {userId:id,userName:username}=host.dataset;
  const panelId=adminUserSessionsPanelId,requestId=++adminUserSessionsRequestId,epoch=sessionEpoch;
  const current=()=>panelId===adminUserSessionsPanelId&&requestId===adminUserSessionsRequestId&&epoch===sessionEpoch;
  host.innerHTML='<p class="meta">加载会话…</p>';
  try{
    const data=await api(`/api/v1/admin/users/${encodeURIComponent(id)}/sessions`);
    if(!current())return;
    const sessions=data.sessions||[];
    host.innerHTML=`<div class="admin-head"><div><h3>${esc(username||'用户')} 的会话</h3></div><button type="button" class="ghost" data-close-sessions>收起</button></div>`+
      (sessions.length?sessions.map(s=>`<div class="admin-session"><span>${esc(s.device.name||s.device.platform)}</span><span class="meta">${fmtDate(s.lastUsedAt)} 活跃</span>${s.current?'<span class="badge badge-success">当前</span>':''}<button type="button" class="danger" data-session="${esc(s.id)}">下线</button></div>`).join(''):'<p class="meta">该用户当前没有活跃会话</p>');
  }catch(err){if(current())toast(err.message,'error')}
}
$('#admin-user-sessions').addEventListener('click',async e=>{
  const close=e.target.closest('[data-close-sessions]');
  if(close){closeAdminUserSessions();return}
  const btn=e.target.closest('button[data-session]');
  if(!btn)return;
  const host=$('#admin-user-sessions');
  const {userId}=host.dataset,panelId=adminUserSessionsPanelId,epoch=sessionEpoch;
  const current=()=>panelId===adminUserSessionsPanelId&&epoch===sessionEpoch;
  try{
    await api(`/api/v1/admin/sessions/${encodeURIComponent(btn.dataset.session)}`,{method:'DELETE'});
    if(current())toast('会话已下线','success');
    if(epoch===sessionEpoch&&host.dataset.userId===userId)void loadAdminUserSessions();
  }catch(err){if(current())toast(err.message,'error')}
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
// 服务端保留到期时间，清理文件后仍能识别已过期的任务。
function jobMediaExpiry(job){
  if(job?.mediaExpiresAt)return job.mediaExpiresAt;
  const files=job?.result?.files||[];
  return files.filter(f=>f.expiresAt&&f.kind!=='asr_source').reduce((e,f)=>!e||f.expiresAt<e?f.expiresAt:e,null);
}
function jobExpired(job){const expiry=jobMediaExpiry(job);return !!expiry&&new Date(expiry).getTime()<=Date.now()}
function jobListSignature(job){return JSON.stringify([job,jobExpired(job)])}
function jobSignature(job){
  // 预览与缩略图的签名地址不参与比较，避免轮询重绘；只记录是否已有缩略图。
  // 过期标记是稳定布尔值，算入签名：任务在页面停留期间过期时，轮询能自动刷新列表行与详情。
  const result=job.result||null;
  const stable=result?{...result,files:(result.files||[]).map(f=>({id:f.id,kind:f.kind,name:f.name,mimeType:f.mimeType,sizeBytes:f.sizeBytes,hasThumbnail:!!f.thumbnailUrl}))}:null;
  return [job.status,job.progress,job.updatedAt,job.error?.code,jobExpired(job),JSON.stringify(stable)].join('|');
}

function jobRowHTML(job){
  const w=job.work||{};
  const title=stripHashtags(w.title,w.hashtags)||`任务 ${job.id.slice(0,8)}`;
  const tone=statusTone[job.status]||'muted';
  const meta=[];
  if(w.authorName)meta.push(w.authorName);
  if(w.durationMs)meta.push(fmtDuration(w.durationMs));
  if(meta.length)meta.push(actionLabels[job.action]||'');
  if(jobExpired(job))meta.push('已过期');
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
    if(row&&row.dataset.sig===jobListSignature(job))return;
    if(row)row.remove();
    row=document.createElement('div');
    row.className='job-row';row.dataset.id=job.id;row.dataset.sig=jobListSignature(job);
    row.tabIndex=0;row.setAttribute('role','button');
    row.setAttribute('aria-current',String(job.id===selectedId));
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
}

function jobDetailHTML(job){
  const w=job.work||{};
  const title=stripHashtags(w.title,w.hashtags)||`任务 ${job.id.slice(0,8)}`;
  const terminal=['completed','failed','cancelled'].includes(job.status);
  const text=job.result?.normalizedText||job.result?.text||'';
  const files=job.result?.files||[];
  // 普通视频使用独立预览入口，动图从画廊打开。
  const videoFile=files.find(f=>f.kind==='video'||((f.mimeType||'').startsWith('video/')&&f.kind!=='animated'));
  const tone=statusTone[job.status]||'muted';
  const badges=[`<span class="badge badge-${tone}">${esc(statusLabels[job.status]||job.status)}</span>`,`<span class="badge badge-action">${esc(actionLabels[job.action]||job.action)}</span>`];
  const serviceNames={siliconflow_sensevoice:'硅基流动',aliyun_paraformer:'阿里云',deepseek_asr:'DeepSeek'};
  const transcriptionService=serviceNames[job.result?.asrProvider]||(job.asrProvider&&asrProviderNames[job.asrProvider]);
  if(transcriptionService&&w.type!=='note')badges.push(`<span class="badge badge-muted" title="转写服务">${esc(transcriptionService)}</span>`);
  const noteImgs=w.images||[];
  if(w.type==='note')badges.push(`<span class="badge badge-note">${noteImgs.length>0&&noteImgs.every(img=>img.animatedUrl)?'动图作品':noteImgs.some(img=>img.animatedUrl)?'含动图':'图文作品'}</span>`);

  const mediaExpiry=jobMediaExpiry(job);
  const expired=jobExpired(job);
  const expiredNotice=expired?`<div class="detail-notice expired-box" role="status">${icon('alert-circle',16)}<span>内容已过期</span></div>`:'';

  const metas=[];
  // 作者/分辨率仅在解析完成后显示；为空显示 "-"（未解析的任务不显示这些行）。
  if(w.douyinWorkId){
    metas.push(`<span>作者：${esc(w.authorName||'-')}</span>`);
    metas.push(`<span>分辨率：${esc((w.width&&w.height)?`${w.width}×${w.height}`:'-')}</span>`);
  }
  if(w.durationMs)metas.push(`<span>时长 ${fmtDuration(w.durationMs)}</span>`);
  if(w.publishedAt)metas.push(`<span>发布 ${fmtDate(w.publishedAt)}</span>`);
  const tags=(w.hashtags||[]).map(t=>`<span class="tag">#${esc(t)}</span>`).join('');
  const signedCovers=[videoFile?.thumbnailUrl,w.coverPreviewUrl].filter(previewUsable);
  const coverSources=[...new Set(w.coverUrl?[...signedCovers,w.coverUrl]:signedCovers)];
  const coverImage=coverSources.length?`<img class="detail-cover" src="${esc(coverSources[0])}" data-cover-fallback="${esc(coverSources[1])}" data-cover-source="${esc(coverSources[2])}" alt="" loading="lazy" decoding="async" referrerpolicy="no-referrer" onerror="retryVideoCover(this)">`:'';
  const previewAttrs=videoFile&&job.action!=='info'?`data-op="preview" aria-haspopup="dialog" data-fid="${esc(videoFile.id)}" data-name="${esc(videoFile.name)}" data-preview="${esc(videoFile.previewUrl||'')}"`:'';
  const previewFailed=previewAttrs&&videoPreviewFailure?.jobId===job.id&&videoPreviewFailure.fileId===videoFile.id;
  const cover=coverImage&&previewAttrs&&!expired?`<button type="button" class="detail-cover-button" ${previewAttrs} aria-label="播放视频">${coverImage}<span class="detail-cover-play">${icon('play',24)}</span></button>`:coverImage;

  const error=job.error?`<div class="error-box">${icon('alert-circle',14)}<div><strong>${esc(job.error.code)}</strong><p>${esc(job.error.message)}</p></div></div>`:'';
  const progressPct=Math.max(0,Math.min(100,Math.round(Number(job.progress)||0)));
  const progressBlock=!terminal?`<div class="progress-row"><div class="progress"><i style="width:${progressPct}%"></i></div><span>${progressPct}%</span></div>`:'';
  const statusText=job.error&&job.error.message===job.statusMessage?'':job.statusMessage||(job.status==='completed'?'处理完成':'');

  const isNote=w.type==='note';
  const linkFiles=isNote?files.filter(f=>f.kind!=='image'&&f.kind!=='animated'):files;
  const fileHref=f=>adminMode?`/api/v1/admin/files/${encodeURIComponent(f.id)}`:`/api/v1/files/${encodeURIComponent(f.id)}`;
  // data-preview 携带同源签名流式地址：视频/配图下载直接用 <a download> 流式落盘，不再 fetch 整文件缓冲。
  const fileLinks=linkFiles.map(f=>`<a class="btn download" href="${fileHref(f)}" data-file="${esc(f.id)}" data-name="${esc(f.name)}" data-preview="${esc(f.previewUrl||'')}">${icon('download',14)} ${esc(fileLabel(f))}</a>`);
  const viewOps=[],taskOps=[];
  if(previewAttrs)viewOps.push(`<button type="button" class="btn preview-btn" ${previewAttrs}>${icon('play',14)} 预览视频</button>`);
  if(!terminal)taskOps.push(`<button type="button" data-op="cancel">${icon('circle-slash',14)} 取消</button>`);
  if(job.status==='failed'||job.status==='cancelled')taskOps.push(`<button type="button" data-op="retry">${icon('rotate-ccw',14)} 重试</button>`);
  if(terminal)taskOps.push(`<button type="button" data-op="delete" class="danger">${icon('trash',14)} 删除</button>`);
  const actionGroups=[viewOps,fileLinks,taskOps].filter(group=>group.length).map(group=>`<div class="action-group">${group.join('')}</div>`).join('');

  const canonical=w.canonicalUrl?`<div class="canonical-row"><a href="${esc(w.canonicalUrl)}" target="_blank" rel="noopener">${icon('arrow-up-right',14)} 查看原作品</a></div>`:'';
  const transcriptHeading=isNote?'图文配文':'视频文案';
  const transcript=text?`<section class="detail-section"><div class="section-head"><h3>${icon('file-text',14)} ${transcriptHeading}</h3><button type="button" class="copy-btn" data-copy title="复制全部文案">${icon('copy',13)} 复制</button></div><div class="result">${esc(text)}</div></section>`:'';

  // 画廊只加载静态缩略图，原图和动图在打开预览时加载。
  const mediaFiles=files.filter(f=>f.kind==='image'||f.kind==='animated');
  const noteImages=w.images||[];
  // 按媒体类型和文件名前缀（image-NN）匹配，避免历史静态文件被当作动图、下载失败导致下标错位。
  const mediaFileForImage=(img,i)=>mediaFiles.find(f=>f.kind===(img.animatedUrl?'animated':'image')&&(f.name||'').indexOf(`image-${String(i+1).padStart(2,'0')}.`)===0)||null;
  const savedMediaCount=noteImages.filter((img,i)=>mediaFileForImage(img,i)).length;
  const showMediaSummary=isNote&&job.status==='completed'&&job.action!=='info'&&noteImages.length>0;
  const mediaSummary=showMediaSummary?`<p class="status-msg">已保存 ${savedMediaCount}/${noteImages.length} 项媒体${savedMediaCount<noteImages.length?'；未保存的媒体不会包含在压缩包中。':'。'}</p>`:'';
  const galleryItems=noteImages.map((img,i)=>{
    const file=mediaFileForImage(img,i);
    const animated=!!img.animatedUrl;
    const cdnSrc=img.animatedUrl||img.url;
    const viewerSrc=previewUsable(file?.previewUrl)?file.previewUrl:cdnSrc;
    const thumbnail=file?.thumbnailUrl||'';
    const dl=file?`<a class="gallery-dl" href="${fileHref(file)}" data-file data-name="${esc(file.name||'image')}" data-preview="${esc(file.previewUrl||'')}" title="下载">${icon('download',14)}</a>`:'';
    const badge=`<span class="gallery-badge">${animated?'动图':'图片'}${showMediaSummary&&!file?' · 未保存':''}</span>`;
    const thumb=thumbnail?`<img src="${esc(thumbnail)}" alt="" loading="lazy" decoding="async" onerror="this.remove()">`:'';
    const indicator=animated?`<span class="gallery-play">${icon('play',18)}</span>`:`<span class="gallery-placeholder">${icon('image',24)}</span>`;
    return `<figure class="gallery-item" data-file-id="${esc(file?.id)}" data-preview="${esc(viewerSrc)}" data-fallback="${esc(cdnSrc)}" data-poster="${esc(thumbnail||img.url)}" data-animated="${animated?'1':'0'}" data-title="${esc((stripHashtags(w.title,w.hashtags)||'预览').slice(0,40))}"><button type="button" class="gallery-media" aria-haspopup="dialog" aria-label="${animated?'播放动图':'查看原图'} ${i+1}">${indicator}${thumb}</button><div class="gallery-foot">${badge}${dl}</div></figure>`;
  }).join('');
  const noteGallery=isNote&&noteImages.length?`<div class="note-gallery">${galleryItems}</div>`:'';
  const zipLink=(isNote&&mediaFiles.length&&job.status==='completed')?archiveDownloadHTML(job.id,`${w.douyinWorkId||job.id}_images.zip`,savedMediaCount<noteImages.length?`打包下载已保存的 ${savedMediaCount} 项`:'打包下载全部'):'';

  const musicFile=files.find(f=>f.kind==='music');
  const musicPlayUrl=musicFile&&musicFile.previewUrl&&previewUsable(musicFile.previewUrl)?musicFile.previewUrl:w.musicUrl;
  const musicRow=isNote&&w.musicUrl?`<div class="music-row"><span class="music-info">${icon('music',13)} <span>${esc(w.musicTitle||'背景音乐')}${w.musicArtist?' · '+esc(w.musicArtist):''}</span></span><button type="button" class="music-toggle" data-op="music" data-music="${esc(w.musicUrl)}" data-play-url="${esc(musicPlayUrl||'')}" data-fallback="${esc(w.musicUrl)}">${icon('play',13)} 播放音乐</button></div>`:'';
  const hasMedia=cover||w.authorName||metas.length;
  const mediaBlock=isNote
    ?(noteGallery||zipLink||musicRow?`<div class="detail-media no-cover">${noteGallery}${mediaSummary}${musicRow}${metas.length?`<div class="detail-meta">${metas.join('')}</div>`:''}${canonical}${zipLink}</div>`:'')
    :(hasMedia?`<div class="detail-media${cover?'':' no-cover'}">${cover}<div class="detail-side">${metas.length?`<div class="detail-meta">${metas.join('')}</div>`:''}${canonical}</div></div>`:'');

  const retentionNote=mediaExpiry?`<p class="retention-note">${icon('info',13)} 媒体文件保留至 ${fmtDate(mediaExpiry)}，到期后自动清理。删除任务也会删除媒体文件。</p>`:'';

  return `<article class="job-detail" data-job="${esc(job.id)}">
    <div class="detail-head"><div><h2>${esc(title)}</h2><div class="badges">${badges.join('')}</div></div><span class="job-time" title="${esc(new Date(job.createdAt).toLocaleString())}">${fmtDate(job.createdAt)}</span></div>
    ${expiredNotice}
    ${mediaBlock}
    ${tags?`<div class="tags">${tags}</div>`:''}
    ${actionGroups?`<div class="actions">${actionGroups}</div>`:''}
    ${previewAttrs?`<div class="detail-notice${previewFailed?'':' hidden'}" data-video-notice="${esc(videoFile.id)}" role="status">${icon('info',16)}<span>视频暂时无法播放，可稍后重试。</span></div>`:''}
    ${retentionNote}
    ${error}
    ${progressBlock}
    ${statusText?`<p class="status-msg">${esc(statusText)}</p>`:''}
    ${transcript}
  </article>`;
}

function retryVideoCover(image){
  const next=image.dataset.coverFallback;
  if(next){
    image.dataset.coverFallback=image.dataset.coverSource||'';
    delete image.dataset.coverSource;
    image.src=next;
    return;
  }
  const cover=image.parentElement?.classList.contains('detail-cover-button')?image.parentElement:image;
  cover.parentElement?.classList.add('no-cover');
  cover.remove();
}

function renderDetail(){
  if(!currentJob)return;
  const sig=jobSignature(currentJob);
  if(sig!==detailSig){
    detailSig=sig;
    $('#detail').innerHTML=jobDetailHTML(currentJob);
  }
  syncDetailMedia(currentJob);
}

function syncDetailMedia(job){
  syncTaskMusicWithJob(job);
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
  if(btn.dataset.op==='preview'){
    e.preventDefault();
    const detail=btn.closest('[data-job]');
    openPreview(detail?.dataset.job,btn.dataset.fid,btn.dataset.name,btn.dataset.preview,btn);
    return;
  }
  if(btn.dataset.op==='archive'){e.preventDefault();downloadArchive(btn.closest('[data-archive-download]').dataset.archiveDownload,btn.dataset.name,adminMode);return}
  if(btn.hasAttribute('data-file')){e.preventDefault();download(btn);return}
  if(btn.hasAttribute('data-copy')){e.preventDefault();e.stopPropagation();copyText(btn);return}
  if(btn.dataset.op){const job=btn.closest('[data-job]');if(job)operate(job.dataset.job,btn.dataset.op)}
}
$('#job-list').addEventListener('keydown',e=>{
  const row=e.target.closest('.job-row');
  if(row&&(e.key==='Enter'||e.key===' ')){e.preventDefault();selectJob(row.dataset.id)}
});
$('#app').addEventListener('click',handleContentClick);
$('#admin-console').addEventListener('click',handleContentClick);

/* ---------- 自定义确认弹窗 ---------- */
let dismissActionDialog=null;
function openActionDialog({title,message,confirmText,danger,fields,initialFocus,collect,submit,cancelValue=null}){
  dismissActionDialog?.();
  return new Promise(resolve=>{
    const modal=$('#confirm-modal'),ok=$('#confirm-ok'),cancel=$('#confirm-cancel');
    const messageNode=$('#confirm-message'),fieldsNode=$('#confirm-fields'),iconNode=$('#confirm-icon');
    const opener=document.activeElement;
    const closeButtons=modal.querySelectorAll('[data-close]');
    const closeButton=modal.querySelector('.modal-close');
    modal.classList.toggle('confirm-simple',!fields);
    $('#confirm-title').textContent=title||'确认操作';
    messageNode.textContent=message||'';
    messageNode.classList.toggle('hidden',!message);
    if(fields)fieldsNode.appendChild(fields);
    iconNode.innerHTML=fields?'':icon(danger?'alert-triangle':'info',18);
    iconNode.classList.toggle('hidden',!!fields);
    iconNode.classList.toggle('confirm-icon-warning',!!danger);
    ok.textContent=confirmText||'确认';
    ok.className=danger?'btn-danger':'btn-primary';
    let pending=false;
    const updatePending=value=>{
      pending=value;
      ok.disabled=cancel.disabled=closeButton.disabled=value;
      modal.setAttribute('aria-busy',String(value));
    };
    const close=value=>{
      if(dismissActionDialog!==dismiss)return;
      dismissActionDialog=null;
      updatePending(false);
      modal.classList.add('hidden');
      document.body.classList.remove('modal-open');
      fieldsNode.replaceChildren();
      modal.onkeydown=ok.onclick=cancel.onclick=null;
      closeButtons.forEach(button=>button.onclick=null);
      if(opener?.isConnected)opener.focus({preventScroll:true});
      resolve(value);
    };
    const dismiss=()=>close(cancelValue);
    dismissActionDialog=dismiss;
    ok.onclick=async()=>{
      if(pending)return;
      const value=collect();
      if(value===undefined)return;
      if(!submit){close(value);return}
      updatePending(true);
      modal.focus();
      try{if(await submit(value))close(value)}
      finally{
        if(dismissActionDialog===dismiss){
          updatePending(false);
          (initialFocus||ok).focus();
        }
      }
    };
    const cancelIfIdle=()=>{if(!pending)dismiss()};
    cancel.onclick=cancelIfIdle;
    closeButtons.forEach(button=>button.onclick=cancelIfIdle);
    modal.onkeydown=e=>{
      if(e.isComposing)return;
      if(e.key==='Escape'){
        e.preventDefault();e.stopPropagation();cancelIfIdle();
      }else if(e.key==='Enter'&&e.target.tagName==='INPUT'){
        e.preventDefault();ok.click();
      }else if(e.key==='Tab'){
        if(pending){e.preventDefault();return}
        const controls=modal.querySelectorAll('button:not(:disabled),input:not(:disabled),select:not(:disabled)');
        const first=controls[0],last=controls[controls.length-1];
        if(e.shiftKey?document.activeElement===first:document.activeElement===last){
          e.preventDefault();(e.shiftKey?last:first).focus();
        }
      }
    };
    modal.tabIndex=-1;
    modal.classList.remove('hidden');document.body.classList.add('modal-open');
    (initialFocus||cancel).focus();
  });
}

function confirmDialog(opts={}){
  return openActionDialog({...opts,danger:opts.danger!==false,collect:()=>true,cancelValue:false});
}

/* 带输入框的弹窗（用于重置密码等）；取消返回 null，确定返回输入值 */
function promptDialog(opts={}){
  const input=document.createElement('input');
  input.type=opts.password?'password':opts.numberInput?'number':'text';
  if(opts.numberInput){
    input.min=String(opts.numberInput.min??0);
    input.max=String(opts.numberInput.max);
    input.step='1';
    input.value=opts.numberInput.value??'';
    input.inputMode='numeric';
  }
  input.placeholder=opts.placeholder||'';
  input.setAttribute('aria-label',opts.placeholder||opts.title||'输入');
  if(opts.password)input.pattern='.{6,12}';
  else input.maxLength=1024;
  input.required=!!opts.password||!!opts.numberInput?.required;
  input.autocomplete=opts.password?'new-password':'off';
  input.className='prompt-input';
  return openActionDialog({
    title:opts.title||'输入',message:opts.message,confirmText:opts.confirmText||'确定',
    fields:input,initialFocus:input,
    collect:()=>{if(input.reportValidity())return input.value}
  });
}

async function operate(id,op){
  if(adminMode)return operateAdminJob(id,op);
  if(op==='delete'&&!(await confirmDialog({title:'删除任务',message:'确认删除该任务及其文件？此操作不可撤销。',confirmText:'删除'})))return;
  const epoch=sessionEpoch;
  const base=`/api/v1/jobs/${encodeURIComponent(id)}`;
  try{
    const data=await api(base+(op==='delete'?'':`/${op}`),{method:op==='delete'?'DELETE':'POST'});
    if(epoch!==sessionEpoch||!session)return;
    if(op==='delete'&&selectedId===id)selectJob(null);
    if(op==='retry'){
      if(selectedId===id&&data?.job?.id===id)selectJob(id,data.job);
      if(data?.job?.status==='failed')toast(data.job.error?.message||data.job.statusMessage||'任务处理失败','error');
      else toast('已重新开始');
    }
    await Promise.all([loadJobs(),op!=='delete'&&selectedId===id?loadJobDetail(id):null]);
    if(op==='delete'&&jobsPage>1&&jobsTotal<=(jobsPage-1)*jobsPageSize){jobsPage--;loadJobs()}
    if(op==='delete')toast('任务已删除','success');
    if(op==='cancel')toast('已请求取消');
  }catch(err){toast(err.message,'error')}
}
// 签名预览地址带 expires（Unix 秒）。
function previewExpiry(url){
  const match=/[?&]expires=(\d+)/.exec(url);
  return match?Number(match[1])*1000:null;
}
// 对有原址回退的媒体容许客户端时钟偏差，避免因误判而过早改用 CDN。
const PREVIEW_SKEW_MS=60*60*1000;
function previewUsable(url){
  if(!url)return false;
  const expiry=previewExpiry(url);
  return expiry===null||expiry+PREVIEW_SKEW_MS>Date.now();
}
// 微信内置浏览器（iOS WKWebView / 安卓 X5）对视频自动播放和页面内渲染有额外限制，需要单独适配。
function isWeChat(){return /MicroMessenger/i.test(navigator.userAgent||'')}
// 打包下载交给浏览器接收，页面只报告准备和移交状态；下载百分比以浏览器下载列表为准。
const ARCHIVE_RETRY_DELAY_MS=5000;
const archiveDownloads=new Map();
function archiveDownloadKey(jobId){return `${session?.user?.id||''}:${jobId}`}
function archiveDownloadView(jobId,label='打包下载全部'){
  const state=archiveDownloads.get(archiveDownloadKey(jobId));
  const busy=state?.phase==='preparing';
  const submitted=state?.phase==='submitted';
  const locked=busy||(submitted&&Date.now()<state.retryAt);
  const buttonLabel=busy?'正在准备下载…':submitted?(locked?'下载请求已提交':'重新下载'):state?.phase==='error'?'重试打包下载':label;
  const message=busy?'正在准备下载，请稍候…':submitted?'已交给浏览器下载，请在浏览器的下载列表查看进度、速度和剩余时间。':state?.message||'下载进度会显示在浏览器的下载列表中。';
  return {busy,locked,label:buttonLabel,message};
}
function archiveDownloadHTML(jobId,name,label='打包下载全部'){
  const view=archiveDownloadView(jobId,label);
  return `<div data-archive-download="${esc(jobId)}" data-archive-label="${esc(label)}"><div class="action-group"><button type="button" class="btn" data-op="archive" data-name="${esc(name||jobId+'_images.zip')}" aria-busy="${view.busy}"${view.locked?' disabled':''}>${icon('download',14)} ${esc(view.label)}</button></div><p class="status-msg" role="status" aria-live="polite" data-archive-status>${esc(view.message)}</p></div>`;
}
function updateArchiveDownloadUI(){
  document.querySelectorAll('[data-archive-download]').forEach(section=>{
    const view=archiveDownloadView(section.dataset.archiveDownload,section.dataset.archiveLabel);
    const button=section.querySelector('[data-op="archive"]');
    button.disabled=view.locked;
    button.setAttribute('aria-busy',String(view.busy));
    button.innerHTML=icon('download',14)+' '+esc(view.label);
    section.querySelector('[data-archive-status]').textContent=view.message;
  });
}
function startNativeDownload(url,name){
  const a=document.createElement('a');
  a.href=url;a.download=name||'download';a.rel='noopener';
  document.body.appendChild(a);a.click();a.remove();
}
function archiveDownloadSessionCurrent(key,state,userId){
  if(archiveDownloads.get(key)!==state)return false;
  if(session?.user?.id===userId)return true;
  if(!session){enter(null);toast('登录已过期，请重新登录','error')}
  return false;
}
async function downloadArchive(jobId,name,inAdmin=false){
  if(archiveDownloadView(jobId).locked)return;
  const userId=session?.user?.id;
  if(!userId){toast('登录已过期，请重新登录','error');return}
  const key=archiveDownloadKey(jobId),state={phase:'preparing'};
  archiveDownloads.set(key,state);
  updateArchiveDownloadUI();
  const controller=new AbortController();
  let timer;
  // 续期请求由 api 共享，不能只依赖单个 fetch 的 signal 限时。
  const timeout=new Promise((_,reject)=>{timer=setTimeout(()=>{controller.abort();reject(new Error('准备下载超时'))},30000)});
  try{
    const path=`/api/v1/${inAdmin?'admin/':''}jobs/${encodeURIComponent(jobId)}/images/archive-download`;
    const result=await Promise.race([api(path,{method:'POST',signal:controller.signal}),timeout]);
    // 准备期间退出或切换账号时，不再触发原账号的下载。
    if(!archiveDownloadSessionCurrent(key,state,userId))return;
    if(typeof result.downloadUrl!=='string')throw new Error('未能取得下载地址，请重试');
    const target=new URL(result.downloadUrl,location.origin);
    if(target.origin!==location.origin||target.pathname!==`/api/v1/archive-download/${encodeURIComponent(jobId)}`)throw new Error('下载地址无效，请重试');
    startNativeDownload(target.href,name||`${jobId}_images.zip`);
    state.phase='submitted';state.retryAt=Date.now()+ARCHIVE_RETRY_DELAY_MS;
    updateArchiveDownloadUI();
    setTimeout(()=>{if(archiveDownloads.get(key)===state)updateArchiveDownloadUI()},ARCHIVE_RETRY_DELAY_MS);
  }catch(err){
    if(!archiveDownloadSessionCurrent(key,state,userId))return;
    state.phase='error';
    state.message=controller.signal.aborted?'准备下载超时，请重试。':`准备下载失败：${err.message||'请重试'}`;
    updateArchiveDownloadUI();
    toast(state.message,'error');
  }finally{clearTimeout(timer)}
}
async function download(link){
  // 媒体文件有同源签名流式地址：直接用 <a download>，浏览器流式写入磁盘、立即开始，不用 fetch 整文件。
  const preview=previewUsable(link.dataset.preview)?link.dataset.preview:'';
  if(preview){
    startNativeDownload(preview,link.dataset.name);
    return;
  }
  try{const response=await fetch(link.href,{headers:{Authorization:`Bearer ${session.accessToken}`}});if(!response.ok)throw new Error('下载失败');const blob=await response.blob(),url=URL.createObjectURL(blob),a=document.createElement('a');a.href=url;a.download=link.dataset.name;a.click();setTimeout(()=>URL.revokeObjectURL(url),30000)}catch(err){toast(err.message,'error')}
}
async function writeClipboardText(text){
  try{
    await navigator.clipboard.writeText(text);
  }catch{
    // 兼容 Clipboard API 不可用的 HTTP 页面。
    const focused=document.activeElement,field=document.createElement('textarea');
    field.value=text;
    field.readOnly=true;
    field.tabIndex=-1;
    field.style.cssText='position:fixed;top:0;left:0;opacity:0;pointer-events:none';
    document.body.appendChild(field);
    try{
      field.select();
      if(!document.execCommand('copy'))throw new Error('复制失败');
    }finally{
      field.remove();
      focused?.focus({preventScroll:true});
    }
  }
}
async function copyText(btn){
  const section=btn.closest('.detail-section')||btn.closest('.result-details');
  const text=(section&&section.querySelector('.result')||{}).textContent||'';
  try{await writeClipboardText(text)}
  catch{toast('复制失败，请检查浏览器的剪贴板权限','error');return}
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
function playPreviewMusic(){
  if(previewMusicSuspended){
    previewMusicSuspended=false;
    resumeTaskMusic();
    return;
  }
  if(taskMusic.jobId&&taskMusic.url&&!taskMusic.playing)resumeTaskMusic();
}
function syncTaskMusicWithJob(job){
  const source=job?.work?.musicUrl||'';
  if(!job||!source){
    if(taskMusic.playing)stopTaskMusic();
    taskMusic={jobId:job?.id||null,source:'',url:'',playing:false};
    return;
  }
  const files=(job.result&&job.result.files)||[];
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
const videoPreviewURLs=new Map();
let videoPreviewRequest=0,galleryPreview={items:[],index:0,touchX:null};
let previewReturnFocus=null;
let previewMusicSuspended=false;
let previewSoundOn=false;
let previewGestureHandler=null;
function setPreviewNav(show){
  document.querySelectorAll('[data-preview-nav]').forEach(btn=>btn.classList.toggle('hidden',!show));
}
function resetPreviewMedia(){
  videoPreviewRequest++;
  const video=$('#preview-video'),img=$('#preview-image');
  if(previewGestureHandler){
    video.removeEventListener('click',previewGestureHandler);
    video.removeEventListener('touchstart',previewGestureHandler);
    previewGestureHandler=null;
  }
  video.onerror=null;video.oncanplay=null;video.onloadedmetadata=null;img.onerror=null;
  video.pause();video.currentTime=0;video.removeAttribute('src');video.removeAttribute('poster');video.removeAttribute('x5-video-player-type');video.load();video.classList.add('hidden');
  img.classList.add('hidden');img.removeAttribute('src');
}
function openGalleryPreview(item){
  const jobId=item.closest('[data-job]')?.dataset.job;
  const nodes=[...item.closest('.note-gallery').querySelectorAll('.gallery-item')];
  // 轮询更新的数据比卡片上的旧签名更新，预览时优先复用。
  const files=currentJob&&currentJob.id===jobId?currentJob.result?.files||[]:[];
  const items=nodes.map(el=>{
    const file=files.find(f=>f.id===el.dataset.fileId);
    return {
      src:previewUsable(file?.previewUrl)?file.previewUrl:el.dataset.preview,
      animated:el.dataset.animated,
      title:el.dataset.title,
      fallback:el.dataset.fallback,
      poster:file?.thumbnailUrl||el.dataset.poster
    };
  });
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
  const fallback=item.fallback&&item.fallback!==src?item.fallback:'';
  const count=galleryPreview.items.length>1?` ${galleryPreview.index+1}/${galleryPreview.items.length}`:'';
  $('#preview-title').textContent=(item.title||'预览')+count;
  setPreviewNav(galleryPreview.items.length>1);
  // 两端禁用对应方向的按钮：第一张不能左、最后一张不能右
  const prevBtn=document.querySelector('.preview-prev'),nextBtn=document.querySelector('.preview-next');
  if(galleryPreview.items.length>1){
    prevBtn.disabled=galleryPreview.index===0;
    nextBtn.disabled=galleryPreview.index===galleryPreview.items.length-1;
  }
  // 图文预览点击即播放：动图静音播放，普通图片也自动播放背景音乐；「原声」按钮按需开启动图原声。
  previewSoundOn=false;
  if(item.animated==='1'){
    video.classList.remove('hidden');
    video.controls=false;video.loop=true;video.playsInline=true;video.muted=true;
    // 安卓微信 X5 内核需要 h5 模式才在页面内渲染视频，否则黑屏或弹独立播放器
    if(isWeChat())video.setAttribute('x5-video-player-type','h5');else video.removeAttribute('x5-video-player-type');
    soundBtn.classList.remove('hidden');
    setPreviewSoundUI(false);
    if(item.poster)video.poster=item.poster;
    video.onerror=fallback?()=>{video.onerror=null;video.src=fallback;video.load();video.play().catch(()=>{})}:null;
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
    playPreviewMusic();
  }else{
    soundBtn.classList.add('hidden');
    img.onerror=fallback?()=>{img.onerror=null;img.src=fallback}:null;
    img.classList.remove('hidden');img.decoding='async';img.src=src;
    // 普通图片点开也播放图文作品的背景音乐。
    playPreviewMusic();
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
async function openPreview(jobId,fileId,name,previewUrl,trigger){
  if(!session)return;
  setVideoPreviewFailure(jobId,fileId,false);
  previewReturnFocus=trigger;
  const modal=$('#preview-modal'),video=$('#preview-video');
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
  if(trigger)modal.querySelector('.modal-close').focus();
  resetPreviewMedia();
  video.classList.remove('hidden');
  video.controls=true;video.muted=false;video.loop=false;video.playsInline=true;

  const epoch=sessionEpoch,request=videoPreviewRequest;
  const jobPath=`/api/v1/${adminMode?'admin/':''}jobs/${encodeURIComponent(jobId)}`;
  const key=`${jobPath}:${fileId}`;
  const active=()=>epoch===sessionEpoch&&request===videoPreviewRequest;
  let refreshedAfterError=false;

  function fail(message='视频加载失败，请稍后重试'){
    if(!active())return;
    if(!session)enter(null);else closePreview();
    if(session)setVideoPreviewFailure(jobId,fileId,true);
    toast(message,'error');
  }

  function loadSignedVideo(url,position=0,autoplay=true){
    videoPreviewURLs.set(key,url);
    const loadedAt=Date.now(),expires=previewExpiry(url);
    let loadedMetadata=false;
    video.onloadedmetadata=()=>{
      if(!active())return;
      video.onloadedmetadata=null;
      loadedMetadata=true;
      if(position>0){
        video.currentTime=Number.isFinite(video.duration)?Math.min(position,video.duration):position;
        if(autoplay)video.play().catch(()=>{});
      }
    };
    video.onerror=()=>{
      if(!active())return;
      video.onerror=null;
      videoPreviewURLs.delete(key);
      // 新签名也失败时终止重试；已成功播放的签名以后仍可能正常过期。
      if(refreshedAfterError&&!(loadedMetadata&&expires>loadedAt&&expires<=Date.now())){
        fail();
        return;
      }
      refreshedAfterError=true;
      return refreshSignedVideo(video.currentTime,!video.paused||!loadedMetadata);
    };
    video.src=url;
    video.load();
    if(autoplay&&position===0)video.play().catch(()=>{});
  }

  async function refreshSignedVideo(position,autoplay){
    try{
      if(!jobId)throw new Error('无法获取视频预览地址');
      const data=await api(jobPath,{cache:'no-store'});
      if(!active())return;
      if(!session)throw new Error('登录已过期，请重新登录');
      if(jobExpired(data.job)&&currentJob?.id===jobId){
        const job={...currentJob,mediaExpiresAt:jobMediaExpiry(data.job)};
        if(adminMode)renderAdminDetail(job);
        else{currentJob=job;renderDetail()}
      }
      const url=data.job?.result?.files?.find(file=>file.id===fileId)?.previewUrl;
      if(previewExpiry(url)===null)throw new Error('视频预览已不可用');
      loadSignedVideo(url,position,autoplay);
    }catch(err){fail(err.message)}
  }

  const latestUrl=currentJob?.id===jobId?currentJob.result?.files?.find(file=>file.id===fileId)?.previewUrl:'';
  // 优先复用已播放的签名，让浏览器沿用同一地址的缓存和分段请求。
  const url=[videoPreviewURLs.get(key),previewUrl,latestUrl].find(value=>previewExpiry(value)>Date.now());
  if(url)loadSignedVideo(url);
  else await refreshSignedVideo();
}
function setVideoPreviewFailure(jobId,fileId,failed){
  if(!failed&&(!videoPreviewFailure||videoPreviewFailure.jobId!==jobId||videoPreviewFailure.fileId!==fileId))return;
  videoPreviewFailure=failed?{jobId,fileId}:null;
  const detail=$(adminMode?'#admin-detail':'#detail').querySelector('[data-job]');
  const notice=detail?.dataset.job===jobId?detail.querySelector('[data-video-notice]'):null;
  if(notice?.dataset.videoNotice===fileId)notice.classList.toggle('hidden',!failed);
}
// 画廊点击查看：图片/动图点击即播放；「原声」按钮切换动图原声。
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
  previewSoundOn=false;
  $('#preview-sound-toggle').classList.add('hidden');
  if(previewMusicSuspended){previewMusicSuspended=false;resumeTaskMusic()}
  if(previewReturnFocus?.isConnected)previewReturnFocus.focus({preventScroll:true});
  previewReturnFocus=null;
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
(async()=>{
  const epoch=sessionEpoch;
  let initial=null;
  try{initial=await readWebSession()}catch{}
  finally{
    for(const button of document.querySelectorAll('#login-form button[type="submit"], #cdk-login-form button[type="submit"]'))button.disabled=false;
  }
  if(epoch===sessionEpoch)enter(initial);
})();
