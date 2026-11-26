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
    batchMessage:'累计已用次数将清零，总额度保留。',
  },
  rotate:{
    title:'轮换密钥',confirmText:'轮换',success:'密钥已轮换',
    message:item=>`轮换 ${item.prefix}… 会立即使原密钥和登录会话失效。新密钥沿用当前额度，仅在本次操作后显示。`,
    batchMessage:'原密钥和登录会话将立即失效。新密钥沿用当前额度，仅在本次操作后显示。',
  },
  revoke:{
    title:'吊销密钥',confirmText:'吊销',success:'密钥已吊销',
    message:item=>`吊销 ${item.prefix}… 后，密钥及其登录会话将立即失效。`,
    batchMessage:'密钥和登录会话将立即失效。',
    method:'DELETE',suffix:'',
  },
  delete:{
    title:'删除密钥记录',confirmText:'删除',success:'密钥记录已删除',
    message:item=>`删除 ${item.prefix}… 的密钥记录后无法恢复。相关任务会保留。`,
    batchMessage:'删除密钥记录后无法恢复。相关任务会保留。',
    method:'DELETE',suffix:'/record',
  },
};
let adminCDKPage=1,adminCDKListID=0,adminCDKMutating=false,adminCDKItems=[];
const adminCDKSelected=new Set();
let generatedCDKCodes=[];

function canManageCDK(item,action){
  return action==='delete'?item.status==='revoked':item.status!=='revoked';
}

function selectedAdminCDKs(){
  return adminCDKItems.filter(item=>adminCDKSelected.has(item.id));
}

function syncAdminCDKActions(){
  const hasCodes=generatedCDKCodes.length>0;
  const list=$('#admin-cdk-list'),batch=$('#admin-cdk-batch'),selectAll=$('#admin-cdk-select-all');
  const selected=selectedAdminCDKs();
  $('#cdk-generate').disabled=adminCDKMutating||hasCodes;
  $('#admin-cdk-dismiss').disabled=adminCDKMutating;
  $('#admin-cdk-refresh').disabled=adminCDKMutating;
  show('#admin-cdk-batch',adminCDKItems.length>0);
  $('#admin-cdk-selected').textContent=`已选 ${selected.length} 个`;
  selectAll.checked=selected.length>0&&selected.length===adminCDKItems.length;
  selectAll.indeterminate=selected.length>0&&!selectAll.checked;
  selectAll.disabled=adminCDKMutating||!adminCDKItems.length;
  for(const input of list.querySelectorAll('[data-cdk-select]')){
    input.checked=adminCDKSelected.has(input.dataset.cdkSelect);
    input.disabled=adminCDKMutating;
  }
  for(const button of list.querySelectorAll('[data-cdk-action]')){
    const waitingForCopy=hasCodes&&button.dataset.cdkAction==='rotate';
    button.disabled=adminCDKMutating||waitingForCopy;
    button.title=waitingForCopy?'请先保存并关闭当前显示的密钥':'';
  }
  for(const button of batch.querySelectorAll('[data-cdk-batch-action]')){
    const action=button.dataset.cdkBatchAction,waitingForCopy=hasCodes&&action==='rotate';
    button.disabled=adminCDKMutating||waitingForCopy||!selected.some(item=>canManageCDK(item,action));
    button.title=waitingForCopy?'请先保存并关闭当前显示的密钥':'';
  }
}

function clearGeneratedCDKs(){
  generatedCDKCodes=[];
  $('#admin-cdk-codes').replaceChildren();
  show('#admin-cdk-generated',false);
  syncAdminCDKActions();
}

function appendGeneratedCDKs(items){
  const start=generatedCDKCodes.length;
  generatedCDKCodes.push(...items.map(item=>item.code));
  const list=$('#admin-cdk-codes');
  list.insertAdjacentHTML('beforeend',items.map(({code,previousPrefix},offset)=>{
    const index=start+offset,codeLabel=`<code>${esc(code)}</code>`;
    return `<li>
      ${previousPrefix?`<div><div class="meta">原密钥 ${esc(previousPrefix)}…</div>${codeLabel}</div>`:codeLabel}
      <button type="button" class="ghost" data-copy-cdk="${index}" aria-label="复制第 ${index+1} 个密钥">复制</button>
    </li>`;
  }).join(''));
  const multiple=generatedCDKCodes.length>1;
  show('#admin-cdk-copy',multiple);
  show('#admin-cdk-generated',true);
  if(!start&&adminOpen&&adminCurrentView==='cdks'){
    const firstAction=multiple?$('#admin-cdk-copy'):list.querySelector('button');
    firstAction?.focus();
  }
}

function resetCDKView(){
  adminCDKPage=1;
  adminCDKListID++;
  adminCDKMutating=false;
  adminCDKItems=[];
  adminCDKSelected.clear();
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
  if(page!==adminCDKPage)adminCDKSelected.clear();
  adminCDKPage=page;
  const id=++adminCDKListID,epoch=sessionEpoch;
  const current=()=>id===adminCDKListID&&epoch===sessionEpoch;
  const status=$('#admin-cdk-status'),list=$('#admin-cdk-list'),pager=$('#admin-cdk-pager');
  adminCDKItems=[];
  status.textContent='正在加载密钥…';
  list.setAttribute('aria-busy','true');
  list.replaceChildren();
  pager.replaceChildren();
  syncAdminCDKActions();
  const query=new URLSearchParams({limit:adminCDKPageSize,offset:(page-1)*adminCDKPageSize});
  try{
    const {items,total}=await api(`${adminCDKPath}?${query}`,{cache:'no-store'});
    if(!current())return;
    const totalPages=Math.ceil(total/adminCDKPageSize);
    if(page>1&&page>totalPages)return loadAdminCDKs(Math.max(1,totalPages));
    adminCDKItems=items||[];
    const visibleIDs=new Set(adminCDKItems.map(item=>item.id));
    for(const selected of adminCDKSelected)if(!visibleIDs.has(selected))adminCDKSelected.delete(selected);
    status.textContent=total?`共 ${total} 个密钥`:'暂无密钥';
    if(adminCDKItems.length)renderAdminCDKs(list,adminCDKItems);
    renderPager(pager,page,totalPages,next=>{if(!adminCDKMutating)void loadAdminCDKs(next)});
  }catch(err){if(current())status.textContent=err.message}
  finally{if(current())list.setAttribute('aria-busy','false')}
}

function renderAdminCDKs(list,items){
  list.innerHTML=`<ul class="admin-cdk-items">${items.map(item=>{
    const state=item.status!=='revoked'&&item.taskCount>=item.taskLimit?['次数用尽','warning']:adminCDKStates[item.status];
    const [label,tone]=state||['未知','muted'];
    const actions=item.status==='revoked'?`
      <button type="button" class="danger" data-cdk-action="delete">删除</button>
    `:`
      <button type="button" data-cdk-action="quota">修改次数</button>
      <button type="button" data-cdk-action="reset">重置次数</button>
      <button type="button" data-cdk-action="rotate">轮换密钥</button>
      <button type="button" class="danger" data-cdk-action="revoke">吊销</button>
    `;
    return `<li class="admin-cdk-row" data-cdk-id="${esc(item.id)}">
    <input type="checkbox" class="admin-selection-checkbox admin-cdk-select" data-cdk-select="${esc(item.id)}" aria-label="选择密钥 ${esc(item.prefix)}">
    <dl>
      <div><dt>密钥前缀</dt><dd><code>${esc(item.prefix)}…</code></dd></div>
      <div><dt>状态</dt><dd><span class="badge badge-${tone}">${label}</span></dd></div>
      <div><dt>已用 / 总次数</dt><dd>${esc(item.taskCount)} / ${esc(item.taskLimit)}</dd></div>
      <div><dt>生成时间</dt><dd><time datetime="${esc(item.createdAt)}">${esc(fmtDate(item.createdAt))}</time></dd></div>
    </dl><div class="admin-card-actions">${actions}</div></li>`;
  }).join('')}</ul>`;
  syncAdminCDKActions();
}

function bindAdminCDKs(){
  const form=$('#admin-cdk-form'),generate=$('#cdk-generate');
  $('#admin-cdk-refresh').addEventListener('click',()=>void loadAdminCDKs());
  $('#admin-cdk-select-all').addEventListener('change',event=>{
    if(adminCDKMutating)return;
    adminCDKSelected.clear();
    if(event.target.checked)for(const item of adminCDKItems)adminCDKSelected.add(item.id);
    syncAdminCDKActions();
  });
  $('#admin-cdk-list').addEventListener('change',event=>{
    const id=event.target.dataset.cdkSelect;
    if(adminCDKMutating||!adminCDKItems.some(item=>item.id===id))return;
    if(event.target.checked)adminCDKSelected.add(id);
    else adminCDKSelected.delete(id);
    syncAdminCDKActions();
  });
  $('#admin-cdk-dismiss').addEventListener('click',()=>{
    if(adminCDKMutating)return;
    clearGeneratedCDKs();
    generate.focus();
  });
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
      appendGeneratedCDKs(items);
      toast(`已生成 ${items.length} 个密钥`,'success');
      adminCDKSelected.clear();
      void loadAdminCDKs(1);
    }catch(err){if(epoch===sessionEpoch)toast(err.message,'error')}
    finally{
      if(epoch===sessionEpoch){
        adminCDKMutating=false;
        syncAdminCDKActions();
        form.setAttribute('aria-busy','false');
      }
    }
  });
  $('#admin-cdk-list').addEventListener('click',event=>{
    const button=event.target.closest('button[data-cdk-action]');
    if(!button)return;
    const row=button.closest('[data-cdk-id]');
    const item=adminCDKItems.find(entry=>entry.id===row?.dataset.cdkId);
    if(item&&canManageCDK(item,button.dataset.cdkAction))return manageCDKs(button,[item]);
  });
  $('#admin-cdk-batch').addEventListener('click',event=>{
    const button=event.target.closest('button[data-cdk-batch-action]');
    if(!button)return;
    const selected=selectedAdminCDKs();
    return manageCDKs(button,selected.filter(item=>canManageCDK(item,button.dataset.cdkBatchAction)),selected.length);
  });

  async function manageCDKs(button,items,selectionSize){
    if(!items.length||button.disabled||adminCDKMutating||!adminOpen||adminCurrentView!=='cdks'||session?.user?.role!=='admin')return;
    const epoch=sessionEpoch,action=button.dataset.cdkBatchAction||button.dataset.cdkAction;
    const batch=selectionSize!==undefined;
    const skipped=batch?selectionSize-items.length:0;
    const scope=`本次操作涉及 ${items.length} 个密钥。${skipped?`另有 ${skipped} 个所选密钥因状态不适用将被跳过。`:''}`;
    let succeeded=0;
    adminCDKMutating=true;
    syncAdminCDKActions();
    try{
      let options,success,suffix='';
      if(action==='quota'){
        const value=await promptDialog({
          title:batch?'批量修改任务总次数':'修改任务总次数',
          message:`${batch?scope:''}设置累计可创建的任务数（1 至 100000 次）。已用次数保留。`,
          placeholder:'任务总次数',numberInput:{min:1,max:100000,value:batch?'':items[0].taskLimit,required:true},confirmText:'保存',
        });
        if(value===null)return;
        options={method:'PATCH',body:JSON.stringify({taskLimit:Number(value)})};
        success='任务总次数已更新';
      }else{
        const {title,confirmText,message,batchMessage,success:successMessage,method='POST',suffix:actionSuffix=`/${action}`}=adminCDKConfirmations[action];
        if(!await confirmDialog({
          title:batch?`批量${title}`:title,confirmText:batch?`批量${confirmText}`:confirmText,
          message:batch?scope+batchMessage:message(items[0]),
        }))return;
        options={method};
        suffix=actionSuffix;
        success=successMessage;
      }
      if(epoch!==sessionEpoch||!adminOpen||adminCurrentView!=='cdks')return;
      const failures=[];
      for(const item of items){
        try{
          const {item:updated}=await api(`${adminCDKPath}/${encodeURIComponent(item.id)}${suffix}`,options);
          if(epoch!==sessionEpoch)return;
          if(action==='rotate'){
            if(typeof updated?.code!=='string'||!updated.code)throw new Error('响应中缺少轮换后的密钥');
            appendGeneratedCDKs([{code:updated.code,previousPrefix:item.prefix}]);
          }
          if(batch)adminCDKSelected.delete(item.id);
          succeeded++;
        }catch(err){
          if(epoch!==sessionEpoch)return;
          failures.push({prefix:item.prefix,message:err.message});
        }
      }
      if(batch){
        const firstFailure=failures[0];
        const summary=`批量操作成功 ${succeeded} 个，失败 ${failures.length} 个${skipped?`，跳过 ${skipped} 个`:''}`;
        toast(firstFailure?`${summary}。${firstFailure.prefix}…：${firstFailure.message}`:summary,firstFailure?(succeeded?'warning':'error'):'success');
      }else if(failures.length){
        toast(failures[0].message,'error');
        return;
      }else{
        toast(success,'success');
      }
      void loadAdminCDKs();
    }catch(err){if(epoch===sessionEpoch)toast(err.message,'error')}
    finally{
      if(epoch===sessionEpoch){
        adminCDKMutating=false;
        syncAdminCDKActions();
        if(adminOpen&&adminCurrentView==='cdks'){
          if(button.isConnected&&!button.disabled)button.focus();
          else if(action==='delete'&&succeeded)$('#admin-cdk-refresh').focus();
        }
      }
    }
  }
}
