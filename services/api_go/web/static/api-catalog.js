const apiCatalogAuthLabels={
  public:'无需登录',
  bearer:'Bearer 访问令牌',
  admin:'管理员 Bearer 访问令牌',
  refresh:'刷新令牌或会话 Cookie',
  signed:'短期签名',
};
const apiCatalogMethods=new Set(['GET','HEAD','POST','PUT','PATCH','DELETE','OPTIONS']);
let adminApiCatalogRequest=null,adminApiOperations=[];

function adminApiCatalogIsVisible(){
  return adminOpen&&adminCurrentView==='api-catalog'&&session?.user?.role==='admin';
}

function resetAdminApiCatalog(){
  adminApiCatalogRequest?.abort();
  adminApiCatalogRequest=null;
  adminApiOperations=[];
  const search=$('#api-catalog-search'),list=$('#api-catalog-list'),status=$('#api-catalog-status');
  search.value='';
  search.disabled=true;
  list.replaceChildren();
  list.setAttribute('aria-busy','false');
  status.textContent='';
  status.classList.remove('failed');
  show('#api-catalog-retry',false);
}

async function loadAdminApiCatalog(){
  if(!adminApiCatalogIsVisible()||adminApiCatalogRequest)return;
  const request=new AbortController(),epoch=sessionEpoch;
  adminApiCatalogRequest=request;
  const search=$('#api-catalog-search'),list=$('#api-catalog-list'),status=$('#api-catalog-status');
  const current=()=>adminApiCatalogRequest===request&&epoch===sessionEpoch&&adminApiCatalogIsVisible();
  $('#api-catalog-base').textContent=location.origin;
  show('#api-catalog-retry',false);
  search.disabled=true;
  list.setAttribute('aria-busy','true');
  list.innerHTML=skeletonRows(3);
  status.classList.remove('failed');
  status.textContent='正在加载接口文档…';
  try{
    const {operations}=await api('/api/v1/admin/api-catalog',{cache:'no-store',signal:request.signal});
    if(!current())return;
    if(!Array.isArray(operations))throw new Error('接口目录格式不正确');
    adminApiOperations=operations.filter(operation=>
      operation&&apiCatalogMethods.has(operation.method)&&
      typeof operation.path==='string'&&/^\/(?!\/)[^\s\\]*$/.test(operation.path)&&
      typeof operation.group==='string'&&typeof operation.summary==='string');
    search.disabled=adminApiOperations.length===0;
    renderAdminApiCatalog();
  }catch{
    if(!current())return;
    list.replaceChildren();
    status.textContent='接口文档加载失败，请重试。';
    status.classList.add('failed');
    show('#api-catalog-retry',true);
  }finally{
    if(adminApiCatalogRequest===request){
      adminApiCatalogRequest=null;
      list.setAttribute('aria-busy','false');
    }
  }
}

function renderAdminApiCatalog(){
  const terms=$('#api-catalog-search').value.toLowerCase().trim().split(/\s+/).filter(Boolean);
  const groups=new Map();
  let count=0;
  adminApiOperations.forEach((operation,index)=>{
    const auth=Object.hasOwn(apiCatalogAuthLabels,operation.auth)?apiCatalogAuthLabels[operation.auth]:'未标注';
    const {method,path,group,summary,details}=operation;
    const searchable=[method,path,group,summary,auth,details].join(' ').toLowerCase();
    if(!terms.every(term=>searchable.includes(term)))return;
    if(!groups.has(group))groups.set(group,[]);
    groups.get(group).push(`<details class="api-catalog-operation">
      <summary>
        <span class="api-catalog-method method-${method.toLowerCase()}">${method}</span>
        <code class="api-catalog-path">${esc(path)}</code>
        <span class="api-catalog-summary">${esc(summary)}</span>
        <span class="api-catalog-chevron">${icon('chevron-right',16)}</span>
      </summary>
      <div class="api-catalog-description">
        <dl><dt>鉴权方式</dt><dd>${esc(auth)}</dd></dl>
        ${details?`<p>${esc(details)}</p>`:''}
        <button type="button" class="ghost" data-api-catalog-copy="${index}" aria-label="复制 ${method} ${esc(path)} 的地址">${icon('copy',14)} 复制地址</button>
      </div>
    </details>`);
    count++;
  });
  $('#api-catalog-list').innerHTML=[...groups].map(([group,items])=>
    `<section class="api-catalog-group"><h3>${esc(group)}</h3>${items.join('')}</section>`).join('');
  $('#api-catalog-status').textContent=count
    ?`${terms.length?'匹配到':'共'} ${count} 个接口`
    :terms.length?'没有匹配的接口。':'暂无接口。';
}

function bindAdminApiCatalog(){
  $('#api-catalog-search').addEventListener('input',renderAdminApiCatalog);
  $('#api-catalog-retry').addEventListener('click',loadAdminApiCatalog);
  $('#api-catalog-list').addEventListener('click',async event=>{
    const button=event.target.closest('button[data-api-catalog-copy]');
    if(!button||!adminApiCatalogIsVisible())return;
    const operation=adminApiOperations[Number(button.dataset.apiCatalogCopy)];
    if(!operation)return;
    const epoch=sessionEpoch;
    const current=()=>epoch===sessionEpoch&&adminApiCatalogIsVisible();
    try{
      await writeClipboardText(location.origin+operation.path);
      if(current())toast('已复制接口地址','success');
    }catch{
      if(current())toast('复制失败，请检查浏览器的剪贴板权限','error');
    }
  });
}
