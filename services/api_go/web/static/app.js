const $=s=>document.querySelector(s);
let session=null,poll=null;
let selectedId=null,currentJob=null,jobsCache=[],detailSig='';

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
  'arrow-up':'<path d="m5 12 7-7 7 7"/><path d="M12 19V5"/>'
};
function icon(name,size=18){
  return `<svg class="ic" width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${ICONS[name]||''}</svg>`;
}

function show(id,on){$(id).classList.toggle('hidden',!on)}
function save(value){session=value;value?sessionStorage.setItem('dc_session',JSON.stringify(value)):sessionStorage.removeItem('dc_session')}
function restore(){try{return JSON.parse(sessionStorage.getItem('dc_session'))}catch{return null}}
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

async function api(path,options={},retry=true){
  options.headers={...(options.headers||{}),'Content-Type':'application/json'};
  if(session?.accessToken)options.headers.Authorization=`Bearer ${session.accessToken}`;
  let response=await fetch(path,options);
  if(response.status===401&&retry&&session?.refreshToken){
    const refreshed=await fetch('/api/v1/auth/refresh',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({refreshToken:session.refreshToken})});
    if(refreshed.ok){save(await refreshed.json());return api(path,options,false)}
    save(null);throw new Error('登录已过期，请重新登录');
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
$('#theme-toggle').addEventListener('click',()=>{theme=theme==='dark'?'light':'dark';localStorage.setItem('dc_theme',theme);applyTheme()});
applyTheme();

/* ---------- 移动端收缩侧边栏 ---------- */
function setSidebar(open){
  $('.master-list').classList.toggle('open',open);
  $('#sidebar-backdrop').classList.toggle('hidden',!open);
  $('#sidebar-toggle').setAttribute('aria-expanded',open?'true':'false');
}
$('#sidebar-toggle').innerHTML=icon('menu',18);
$('#sidebar-toggle').addEventListener('click',()=>setSidebar(!$('.master-list').classList.contains('open')));
$('#sidebar-backdrop').addEventListener('click',()=>setSidebar(false));

/* ---------- 返回顶部 ---------- */
$('#back-top').innerHTML=icon('arrow-up',18);
$('#back-top').addEventListener('click',()=>window.scrollTo({top:0,behavior:'smooth'}));
window.addEventListener('scroll',()=>{$('#back-top').classList.toggle('hidden',window.scrollY<400)},{passive:true});

/* ---------- 视图 / 选中 ---------- */
function showView(name){
  ['new','detail','settings'].forEach(v=>show('#view-'+v,v===name));
  $('#new-task-btn').classList.toggle('active',name==='new');
  $('#settings-link').classList.toggle('active',name==='settings');
  if(name==='settings')loadProviders();
}
function selectJob(id){
  if(selectedId!==id){selectedId=id;detailSig='';currentJob=id?(jobsCache.find(j=>j.id===id)||null):null}
  document.querySelectorAll('.job-row').forEach(r=>r.classList.toggle('selected',r.dataset.id===id));
  showView(id?'detail':'new');
  renderDetail();
  if(matchMedia('(max-width:960px)').matches)setSidebar(false);
  if(id&&matchMedia('(max-width:960px)').matches)$('#view-detail').scrollIntoView({behavior:'smooth',block:'start'});
}
$('#new-task-btn').addEventListener('click',()=>selectJob(null));
$('#settings-link').addEventListener('click',e=>{e.preventDefault();selectJob(null);showView('settings')});

function enter(value){
  save(value);
  show('#login',!value);show('#app',!!value);show('#logout',!!value);
  document.querySelectorAll('.admin-only').forEach(el=>el.classList.toggle('hidden',value?.user?.role!=='admin'));
  if(value){
    selectJob(null);
    if(value.user?.role==='admin')loadProviders();
    loadJobs();
    poll=setInterval(loadJobs,5000);
  }else{clearInterval(poll)}
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
$('#search-form').addEventListener('submit',e=>{e.preventDefault();loadJobs()});
$('#refresh').addEventListener('click',loadJobs);
$('#status').addEventListener('change',loadJobs);
async function loadJobs(){if(!session)return;try{const q=new URLSearchParams({q:$('#query').value.trim(),status:$('#status').value,limit:'100',offset:'0'});const data=await api(`/api/v1/jobs?${q}`);renderList(data.jobs||[]);if($('#job-total'))$('#job-total').textContent=data.total!=null?`${data.total} 个任务`:''}catch(err){if(err.message.includes('登录'))enter(null);else toast(err.message,'error')}}

/* ---------- 渲染 ---------- */
function esc(value){return String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))}
function stripHashtags(title,hashtags){let t=String(title||'');(hashtags||[]).forEach(h=>{if(h)t=t.split('#'+h).join(' ')});return t.replace(/[#＃]\S*/g,'').replace(/\s{2,}/g,' ').replace(/^\s+|\s+$/g,'')}
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
  const cover=w.coverUrl?`<img class="detail-cover" src="${esc(w.coverUrl)}" alt="" loading="lazy" referrerpolicy="no-referrer" onerror="this.style.display='none'">`:'';

  const error=job.error?`<div class="error-box">${icon('alert-circle',14)}<div><strong>${esc(job.error.code)}</strong><p>${esc(job.error.message)}</p></div></div>`:'';
  const progressPct=Math.max(0,Math.min(100,Math.round(Number(job.progress)||0)));
  const progressBlock=!terminal?`<div class="progress-row"><div class="progress"><i style="width:${progressPct}%"></i></div><span>${progressPct}%</span></div>`:'';
  const statusText=job.statusMessage||(['completed'].includes(job.status)?'处理完成':'');

  const isNote=w.type==='note';
  const linkFiles=isNote?files.filter(f=>f.kind!=='image'&&f.kind!=='animated'):files;
  const fileLinks=linkFiles.map(f=>`<a class="btn download" href="/api/v1/files/${encodeURIComponent(f.id)}" data-file="${esc(f.id)}" data-name="${esc(f.name)}">${icon('download',14)} ${esc(fileLabel(f))}</a>`);
  const viewOps=[],taskOps=[];
  if(videoFile)viewOps.push(`<button type="button" class="btn preview-btn" data-op="preview" data-fid="${esc(videoFile.id)}" data-name="${esc(videoFile.name)}">${icon('play',14)} 预览视频</button>`);
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
  const galleryItems=noteImages.map((img,i)=>{
    const file=mediaFiles[i];
    const previewSrc=file&&file.previewUrl?file.previewUrl:(img.animatedUrl||img.url);
    const dl=file?`<a class="gallery-dl" href="/api/v1/files/${encodeURIComponent(file.id)}" data-file data-name="${esc(file.name||'image')}" title="下载">${icon('download',14)}</a>`:'';
    const badge=`<span class="gallery-badge">${img.animatedUrl?'动图':'图片'}</span>`;
    // 动图：video 内联自动循环（与查看器同一地址，浏览器缓存后打开不再重新加载）；普通配图：缩略图用 CDN 小图，加载失败回退服务器本地文件。
    const onerr=file&&file.previewUrl?`this.onerror=null;this.src='${esc(file.previewUrl)}'`:`this.style.display='none'`;
    const media=img.animatedUrl
      ?`<video class="gallery-anim" src="${esc(previewSrc)}" autoplay muted loop playsinline preload="metadata"></video>`
      :`<img src="${esc(img.url)}" alt="" loading="lazy" referrerpolicy="no-referrer" onerror="${onerr}">`;
    const foot=(badge||dl)?`<div class="gallery-foot">${badge}${dl}</div>`:'';
    return `<figure class="gallery-item" data-preview="${esc(previewSrc)}" data-animated="${img.animatedUrl?'1':'0'}" data-title="${esc((stripHashtags(w.title,w.hashtags)||'预览').slice(0,40))}" data-music="${esc(w.musicUrl||'')}"><div class="gallery-media">${media}</div>${foot}</figure>`;
  }).join('');
  const noteGallery=isNote&&noteImages.length?`<div class="note-gallery">${galleryItems}</div>`:'';
  const zipLink=(isNote&&mediaFiles.length)?`<a class="btn" href="/api/v1/jobs/${encodeURIComponent(job.id)}/images/archive" data-file data-name="${esc((job.work&&job.work.douyinWorkId)||job.id)}_images.zip">${icon('download',14)} 打包下载全部</a>`:'';

  const musicRow=isNote&&w.musicUrl?`<div class="music-row">${icon('music',13)} ${esc(w.musicTitle||'背景音乐')}${w.musicArtist?' · '+esc(w.musicArtist):''}</div>`:'';
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
  if(sig===detailSig)return;
  detailSig=sig;
  $('#detail').innerHTML=jobDetailHTML(currentJob);
}

$('#app').addEventListener('click',e=>{
  const row=e.target.closest('.job-row');
  if(row){selectJob(row.dataset.id);return}
  const gItem=e.target.closest('.gallery-item');
  if(gItem&&!e.target.closest('.gallery-dl')){openMediaPreview(gItem.dataset.preview,gItem.dataset.animated,gItem.dataset.title,gItem.dataset.music);return}
  const btn=e.target.closest('[data-op],a[data-file],[data-copy]');
  if(!btn)return;
  if(btn.dataset.op==='preview'){e.preventDefault();openPreview(btn.dataset.fid,btn.dataset.name);return}
  if(btn.hasAttribute('data-file')){e.preventDefault();download(btn);return}
  if(btn.hasAttribute('data-copy')){e.preventDefault();e.stopPropagation();copyText(btn);return}
  if(btn.dataset.op){const job=btn.closest('[data-job]');if(job)operate(job.dataset.job,btn.dataset.op)}
});

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

async function operate(id,op){
  if(op==='delete'&&!(await confirmDialog({title:'删除任务',message:'确认删除该任务及其文件？此操作不可撤销。',confirmText:'删除'})))return;
  try{
    await api(`/api/v1/jobs/${encodeURIComponent(id)}${op==='delete'?'':`/${op}`}`,{method:op==='delete'?'DELETE':'POST'});
    await loadJobs();
    if(op==='delete'&&selectedId===id)selectJob(null);
    if(op==='delete')toast('任务已删除','success');
    if(op==='cancel')toast('已请求取消');
    if(op==='retry')toast('已重新开始');
  }catch(err){toast(err.message,'error')}
}
async function download(link){try{const response=await fetch(link.href,{headers:{Authorization:`Bearer ${session.accessToken}`}});if(!response.ok)throw new Error('下载失败');const blob=await response.blob(),url=URL.createObjectURL(blob),a=document.createElement('a');a.href=url;a.download=link.dataset.name;a.click();setTimeout(()=>URL.revokeObjectURL(url),30000)}catch(err){toast(err.message,'error')}}
async function copyText(btn){
  const section=btn.closest('.detail-section')||btn.closest('.result-details');
  const text=(section&&section.querySelector('.result')||{}).textContent||'';
  try{await navigator.clipboard.writeText(text)}
  catch{const ta=document.createElement('textarea');ta.value=text;ta.style.position='fixed';ta.style.opacity='0';document.body.appendChild(ta);ta.select();document.execCommand('copy');ta.remove()}
  btn.innerHTML=icon('check',13)+' 已复制';btn.classList.add('copied');
  setTimeout(()=>{btn.innerHTML=icon('copy',13)+' 复制';btn.classList.remove('copied')},1400);
}

/* ---------- 视频预览 / 图文查看 ---------- */
let previewURL=null,previewBgmURL='',previewSound='audio'; // 'audio'=作品背景音乐, 'clip'=动图原声
function currentMuted(){
  const v=$('#preview-video'),a=$('#preview-audio');
  return previewSound==='audio'?a.muted:v.muted;
}
function setActiveMuted(muted){
  const v=$('#preview-video'),a=$('#preview-audio');
  if(previewSound==='audio')a.muted=muted;else v.muted=muted;
}
function updateSoundUI(){
  const sound=$('#preview-sound'),source=$('#preview-source');
  const muted=currentMuted();
  sound.innerHTML=icon(muted?'volume-x':'volume-2',15);
  sound.setAttribute('aria-label',muted?'取消静音':'静音');
  sound.title=muted?'取消静音':'静音';
  source.textContent='♪ '+(previewSound==='audio'?'背景音乐':'原声');
}
function applySound(){
  const v=$('#preview-video'),a=$('#preview-audio');
  if(previewSound==='audio'&&previewBgmURL){
    v.muted=true;
    a.src=previewBgmURL;a.muted=false;a.play().catch(()=>{});
  }else{
    a.pause();a.removeAttribute('src');a.load();
    v.muted=false;v.play().catch(()=>{});
  }
  updateSoundUI();
}
$('#preview-sound').addEventListener('click',e=>{
  e.preventDefault();e.stopPropagation();
  setActiveMuted(!currentMuted());
  updateSoundUI();
});
$('#preview-source').addEventListener('click',e=>{
  e.preventDefault();e.stopPropagation();
  previewSound=previewSound==='audio'?'clip':'audio';
  applySound();
});
async function openPreview(fileId,name){
  const modal=$('#preview-modal'),video=$('#preview-video'),img=$('#preview-image');
  $('#preview-title').textContent=name||'视频预览';
  modal.classList.remove('hidden');
  document.body.classList.add('modal-open');
  img.classList.add('hidden');img.removeAttribute('src');
  $('#preview-audio').pause();$('#preview-audio').removeAttribute('src');
  video.classList.remove('hidden');
  video.controls=true;video.muted=false;video.loop=false;video.playsInline=true;
  $('#preview-sound').classList.add('hidden');
  $('#preview-source-row').classList.add('hidden');
  video.removeAttribute('src');
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
// 画廊点击查看。动图：视频静音作画面，声音默认取作品背景音乐，可切到原声；
// 静态配图：显示大图，有背景音乐则随画面播放。
function openMediaPreview(src,animated,title,musicUrl){
  const modal=$('#preview-modal'),video=$('#preview-video'),img=$('#preview-image'),sound=$('#preview-sound');
  $('#preview-title').textContent=title||'预览';
  modal.classList.remove('hidden');
  document.body.classList.add('modal-open');
  previewBgmURL=musicUrl||'';
  sound.classList.remove('hidden');
  if(animated==='1'){
    img.classList.add('hidden');img.removeAttribute('src');
    video.classList.remove('hidden');
    // 先静音再播放，避免 applySound 前瞬间漏出动图原声（有 bgm 时画面应静音）。
    video.controls=false;video.loop=true;video.playsInline=true;video.muted=true;
    video.removeAttribute('src');video.src=src;video.load();video.play().catch(()=>{});
    if(previewBgmURL){previewSound='audio';$('#preview-source-row').classList.remove('hidden');}
    else{previewSound='clip';$('#preview-source-row').classList.add('hidden');}
    applySound();
  }else{
    video.pause();video.removeAttribute('src');video.load();video.classList.add('hidden');
    img.classList.remove('hidden');img.src=src;
    if(previewBgmURL){
      previewSound='audio';$('#preview-source-row').classList.remove('hidden');applySound();
    }else{
      // 静态图且无背景音乐：没有任何声音可控制，隐藏声音按钮与来源切换。
      $('#preview-source-row').classList.add('hidden');
      sound.classList.add('hidden');
      $('#preview-audio').pause();$('#preview-audio').removeAttribute('src');
    }
  }
}
function closePreview(){
  const modal=$('#preview-modal');modal.classList.add('hidden');document.body.classList.remove('modal-open');
  const video=$('#preview-video');video.pause();video.currentTime=0;video.removeAttribute('src');video.load();
  const img=$('#preview-image');img.classList.add('hidden');img.removeAttribute('src');
  const a=$('#preview-audio');a.pause();a.removeAttribute('src');a.load();
  if(previewURL){URL.revokeObjectURL(previewURL);previewURL=null}
}
$('#preview-modal').addEventListener('click',e=>{if(e.target.closest('[data-close]'))closePreview()});
document.addEventListener('keydown',e=>{
  if(e.key!=='Escape')return;
  if(!$('#preview-modal').classList.contains('hidden')){closePreview();return}
  if(!$('#confirm-modal').classList.contains('hidden')){$('#confirm-cancel').click()}
});

enter(restore());
