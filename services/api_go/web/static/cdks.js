function bindLoginForms(){
  const methods=[...document.querySelectorAll('[data-login-method]')];
  let submitting=false;
  for(const button of methods){
    button.addEventListener('click',()=>{
      if(submitting||button.getAttribute('aria-pressed')==='true')return;
      const viaCDK=button.dataset.loginMethod==='cdk';
      for(const method of methods)method.setAttribute('aria-pressed',String(method===button));
      show('#login-form',!viaCDK);
      show('#cdk-login-form',viaCDK);
      $(viaCDK?'#password':'#login-cdk').value='';
      $(viaCDK?'#login-cdk':'#username').focus();
    });
  }
  $('#login-form').addEventListener('submit',e=>submit(e,false));
  $('#cdk-login-form').addEventListener('submit',e=>submit(e,true));

  async function submit(event,viaCDK){
    event.preventDefault();
    const form=event.currentTarget,button=form.querySelector('button[type="submit"]');
    if(submitting||button.disabled)return;
    const secret=$(viaCDK?'#login-cdk':'#password');
    const credentials=viaCDK?{code:secret.value.trim()}:{username:$('#username').value.trim(),password:secret.value};
    if(viaCDK&&!credentials.code){secret.focus();return}
    submitting=true;
    for(const method of methods)method.disabled=true;
    button.disabled=true;
    form.setAttribute('aria-busy','true');
    try{
      const endpoint=viaCDK?'/api/v1/auth/cdk/login':'/api/v1/auth/login';
      const data=await api(endpoint,{method:'POST',body:JSON.stringify({...credentials,device:loginDevice()})},false);
      secret.value='';
      enter(data);
    }catch(err){toast(err.message,'error')}
    finally{
      submitting=false;
      for(const method of methods)method.disabled=false;
      button.disabled=false;
      form.setAttribute('aria-busy','false');
    }
  }
}

const adminCDKPath='/api/v1/admin/cdks',adminCDKPageSize=25;
const adminCDKStates={unused:['待使用','muted'],redeemed:['已登录','success'],revoked:['已吊销','danger']};
const adminCDKConfirmations={
  reset:{
    title:'重置已用次数',confirmText:'重置',success:'已用次数已重置',
    message:item=>`将 ${item.prefix}… 的累计已用次数清零，恢复 ${item.taskLimit} 次可用额度。`,
  },
  rotate:{
    title:'轮换密钥',confirmText:'轮换',success:'密钥已轮换',
    message:item=>`轮换 ${item.prefix}… 会立即使原密钥和登录会话失效。新密钥沿用当前额度，仅在本次操作后显示。`,
  },
  revoke:{
    title:'吊销密钥',confirmText:'吊销',success:'密钥已吊销',
    message:item=>`吊销 ${item.prefix}… 后，密钥及其登录会话将立即失效。`,
  },
};
let adminCDKPage=1,adminCDKListID=0,adminCDKMutating=false,adminCDKItems=[];
let generatedCDKCodes=[];

function syncAdminCDKActions(){
  const hasCodes=generatedCDKCodes.length>0;
  $('#cdk-generate').disabled=adminCDKMutating||hasCodes;
  for(const button of $('#admin-cdk-list').querySelectorAll('[data-cdk-action]')){
    const waitingForCopy=hasCodes&&button.dataset.cdkAction==='rotate';
    button.disabled=adminCDKMutating||waitingForCopy;
    button.title=waitingForCopy?'请先保存并关闭当前显示的密钥':'';
  }
}

function clearGeneratedCDKs(){
  generatedCDKCodes=[];
  $('#admin-cdk-codes').replaceChildren();
  show('#admin-cdk-generated',false);
  syncAdminCDKActions();
}

function showGeneratedCDKs(codes){
  generatedCDKCodes=codes;
  const list=$('#admin-cdk-codes');
  list.innerHTML=codes.map((code,index)=>`<li>
    <code>${esc(code)}</code>
    <button type="button" class="ghost" data-copy-cdk="${index}" aria-label="复制第 ${index+1} 个密钥">复制</button>
  </li>`).join('');
  show('#admin-cdk-copy',codes.length>1);
  show('#admin-cdk-generated',true);
  if(adminOpen&&adminCurrentView==='cdks'){
    const firstAction=codes.length>1?$('#admin-cdk-copy'):list.querySelector('button');
    firstAction?.focus();
  }
}

function resetCDKView(){
  adminCDKPage=1;
  adminCDKListID++;
  adminCDKMutating=false;
  adminCDKItems=[];
  $('#admin-cdk-form').reset();
  $('#admin-cdk-form').setAttribute('aria-busy','false');
  $('#admin-cdk-status').textContent='';
  $('#admin-cdk-list').setAttribute('aria-busy','false');
  $('#admin-cdk-list').replaceChildren();
  $('#admin-cdk-pager').replaceChildren();
  clearGeneratedCDKs();
}

async function loadAdminCDKs(page=adminCDKPage){
  if(!adminOpen||session?.user?.role!=='admin')return;
  adminCDKPage=page;
  const id=++adminCDKListID,epoch=sessionEpoch;
  const current=()=>id===adminCDKListID&&epoch===sessionEpoch;
  const status=$('#admin-cdk-status'),list=$('#admin-cdk-list'),pager=$('#admin-cdk-pager');
  adminCDKItems=[];
  status.textContent='正在加载密钥…';
  list.setAttribute('aria-busy','true');
  list.replaceChildren();
  pager.replaceChildren();
  const query=new URLSearchParams({limit:adminCDKPageSize,offset:(page-1)*adminCDKPageSize});
  try{
    const {items,total}=await api(`${adminCDKPath}?${query}`,{cache:'no-store'});
    if(!current())return;
    adminCDKItems=items||[];
    status.textContent=total?`共 ${total} 个密钥`:'暂无密钥';
    if(adminCDKItems.length)renderAdminCDKs(list,adminCDKItems);
    renderPager(pager,page,Math.ceil(total/adminCDKPageSize),loadAdminCDKs);
  }catch(err){if(current())status.textContent=err.message}
  finally{if(current())list.setAttribute('aria-busy','false')}
}

function renderAdminCDKs(list,items){
  list.innerHTML=`<ul class="admin-cdk-items">${items.map(item=>{
    const state=item.status!=='revoked'&&item.taskCount>=item.taskLimit?['次数用尽','warning']:adminCDKStates[item.status];
    const [label,tone]=state||['未知','muted'];
    const actions=item.status==='revoked'?'':`<div class="admin-card-actions">
      <button type="button" data-cdk-action="quota">修改次数</button>
      <button type="button" data-cdk-action="reset">重置次数</button>
      <button type="button" data-cdk-action="rotate">轮换密钥</button>
      <button type="button" class="danger" data-cdk-action="revoke">吊销</button>
    </div>`;
    return `<li class="admin-cdk-row" data-cdk-id="${esc(item.id)}"><dl>
      <div><dt>密钥前缀</dt><dd><code>${esc(item.prefix)}…</code></dd></div>
      <div><dt>状态</dt><dd><span class="badge badge-${tone}">${label}</span></dd></div>
      <div><dt>已用 / 总次数</dt><dd>${esc(item.taskCount)} / ${esc(item.taskLimit)}</dd></div>
      <div><dt>生成时间</dt><dd><time datetime="${esc(item.createdAt)}">${esc(fmtDate(item.createdAt))}</time></dd></div>
    </dl>${actions}</li>`;
  }).join('')}</ul>`;
  syncAdminCDKActions();
}

function bindAdminCDKs(){
  const form=$('#admin-cdk-form'),generate=$('#cdk-generate');
  $('#admin-cdk-refresh').addEventListener('click',()=>void loadAdminCDKs());
  $('#admin-cdk-dismiss').addEventListener('click',()=>{clearGeneratedCDKs();generate.focus()});
  $('#admin-cdk-copy').addEventListener('click',()=>copyGeneratedCDKs(generatedCDKCodes.join('\n')));
  $('#admin-cdk-codes').addEventListener('click',event=>{
    const button=event.target.closest('button[data-copy-cdk]');
    if(button)return copyGeneratedCDKs(generatedCDKCodes[Number(button.dataset.copyCdk)]);
  });

  async function copyGeneratedCDKs(text){
    if(!text||!adminOpen||adminCurrentView!=='cdks'||session?.user?.role!=='admin')return;
    const epoch=sessionEpoch;
    const isCurrent=()=>epoch===sessionEpoch&&adminOpen&&adminCurrentView==='cdks';
    try{
      await writeClipboardText(text);
      if(isCurrent())toast('已复制密钥','success');
    }catch{
      if(isCurrent())toast('复制失败，请检查浏览器的剪贴板权限','error');
    }
  }

  form.addEventListener('submit',async event=>{
    event.preventDefault();
    if(generate.disabled||adminCDKMutating||!adminOpen||adminCurrentView!=='cdks'||session?.user?.role!=='admin')return;
    const epoch=sessionEpoch;
    const count=Number($('#cdk-count').value),taskLimit=Number($('#cdk-task-limit').value);
    adminCDKMutating=true;
    syncAdminCDKActions();
    form.setAttribute('aria-busy','true');
    try{
      const {items}=await api(adminCDKPath,{method:'POST',body:JSON.stringify({count,taskLimit})});
      if(epoch!==sessionEpoch)return;
      showGeneratedCDKs(items.map(item=>item.code));
      toast(`已生成 ${items.length} 个密钥`,'success');
      adminCDKPage=1;
      void loadAdminCDKs();
    }catch(err){if(epoch===sessionEpoch)toast(err.message,'error')}
    finally{
      if(epoch===sessionEpoch){
        adminCDKMutating=false;
        syncAdminCDKActions();
        form.setAttribute('aria-busy','false');
      }
    }
  });
  $('#admin-cdk-list').addEventListener('click',async event=>{
    const button=event.target.closest('button[data-cdk-action]');
    if(!button||button.disabled||adminCDKMutating||!adminOpen||adminCurrentView!=='cdks'||session?.user?.role!=='admin')return;
    const row=button.closest('[data-cdk-id]');
    const item=adminCDKItems.find(entry=>entry.id===row?.dataset.cdkId);
    if(!item||item.status==='revoked')return;
    const epoch=sessionEpoch,action=button.dataset.cdkAction;
    adminCDKMutating=true;
    syncAdminCDKActions();
    try{
      let path=`${adminCDKPath}/${encodeURIComponent(item.id)}`,options,success;
      if(action==='quota'){
        const value=await promptDialog({title:'修改任务总次数',message:'设置该密钥累计可创建的任务数（1 至 100000 次）。',placeholder:'任务总次数',numberInput:{min:1,max:100000,value:item.taskLimit,required:true},confirmText:'保存'});
        if(value===null)return;
        options={method:'PATCH',body:JSON.stringify({taskLimit:Number(value)})};
        success='任务总次数已更新';
      }else{
        const {title,confirmText,message,success:successMessage}=adminCDKConfirmations[action];
        if(!await confirmDialog({title,confirmText,message:message(item)}))return;
        options={method:action==='revoke'?'DELETE':'POST'};
        if(action!=='revoke')path+=`/${action}`;
        success=successMessage;
      }
      if(epoch!==sessionEpoch||!adminOpen||adminCurrentView!=='cdks')return;
      const {item:updated}=await api(path,options);
      if(epoch!==sessionEpoch)return;
      if(action==='rotate')showGeneratedCDKs([updated.code]);
      toast(success,'success');
      void loadAdminCDKs();
    }catch(err){if(epoch===sessionEpoch)toast(err.message,'error')}
    finally{
      if(epoch===sessionEpoch){
        adminCDKMutating=false;
        syncAdminCDKActions();
        if(button.isConnected&&adminOpen&&adminCurrentView==='cdks')button.focus();
      }
    }
  });
}
