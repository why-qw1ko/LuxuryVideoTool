const androidReleaseBase='/api/v1/admin/app/android';
const androidReleaseMaxBytes=256*1024*1024;
let adminReleaseData={draft:null,release:null},adminReleaseOperation=null,adminReleaseDirty=false;

function adminReleasesVisible(){
  return adminOpen&&adminCurrentView==='releases'&&session?.user?.role==='admin';
}

function adminReleaseStatus(message,failed=false){
  const status=$('#admin-release-status');
  status.textContent=message;
  status.classList.toggle('failed',failed);
}

function syncAdminReleaseControls(){
  const busy=!!adminReleaseOperation;
  const published=adminReleaseData.release,forceUpdate=$('#admin-release-policy').checked;
  const policyChanged=!!published&&forceUpdate!==!!published.forceUpdate;
  $('#admin-view-releases').setAttribute('aria-busy',String(busy));
  $('#admin-release-refresh').disabled=busy;
  $('#admin-release-apk').disabled=busy;
  $('#admin-release-upload').disabled=busy||!$('#admin-release-apk').files?.length;
  $('#admin-release-draft-fields').disabled=busy||!adminReleaseData.draft;
  $('#admin-release-publish').disabled=busy||!adminReleaseData.draft||adminReleaseDirty;
  $('#admin-release-policy-fields').disabled=busy||!adminReleaseData.release;
  $('#admin-release-policy-save').disabled=busy||!policyChanged;
  $('#admin-release-policy-label').textContent=!published?'尚未发布安卓版本':policyChanged?`${forceUpdate?'开启':'关闭'}强制更新（待保存）`:published.forceUpdate?'当前已开启强制更新':'当前未开启强制更新';
}

function restoreAdminReleasePolicy(){
  $('#admin-release-policy').checked=!!adminReleaseData.release?.forceUpdate;
  syncAdminReleaseControls();
}

function releaseSize(bytes){
  return `${(Number(bytes||0)/1024/1024).toFixed(1)} MB`;
}

function releaseMetadataHTML(release){
  if(!release)return '<p class="meta">暂无版本</p>';
  return `<dl class="release-metadata"><div><dt>版本</dt><dd>${esc(release.versionName)} <span class="meta">（${esc(release.versionCode)}）</span></dd></div><div><dt>包名</dt><dd>${esc(release.packageName)}</dd></div><div><dt>安装包</dt><dd>${esc(releaseSize(release.size))}</dd></div><div><dt>SHA-256</dt><dd class="release-hash">${esc(release.sha256)}</dd></div>${release.publishedAt?`<div><dt>发布时间</dt><dd>${esc(fmtDate(release.publishedAt))}</dd></div>`:''}</dl>`;
}

function renderAdminReleases(data){
  if(!data||!Object.hasOwn(data,'draft')||!Object.hasOwn(data,'release'))throw new Error('版本信息格式不正确，请重新加载');
  adminReleaseData={draft:data.draft,release:data.release};
  adminReleaseDirty=false;
  const draft=data.draft,published=data.release;
  $('#admin-release-published').innerHTML=published?releaseMetadataHTML(published):'<p class="meta">尚未发布安卓版本。</p>';
  $('#admin-release-published-notes').textContent=published?.changelog||'';
  $('#admin-release-policy').checked=!!published?.forceUpdate;
  const download=$('#admin-release-download');
  const path=published?.downloadUrl||'';
  const downloadable=/^\/api\/v1\/app\/android\/releases\/[a-zA-Z0-9_-]+\/apk$/.test(path);
  download.classList.toggle('hidden',!downloadable);
  if(downloadable)download.setAttribute('href',path);else download.removeAttribute('href');
  $('#admin-release-draft').innerHTML=draft?releaseMetadataHTML(draft):'<p class="meta">上传 APK 后生成草稿，版本号和包名自动读取。</p>';
  $('#admin-release-changelog').value=draft?.changelog||'';
  $('#admin-release-force').checked=!!draft?.forceUpdate;
  syncAdminReleaseControls();
}

function resetAdminReleases(){
  const operation=adminReleaseOperation;
  adminReleaseOperation=null;
  if(operation){clearTimeout(operation.timer);operation.controller.abort();operation.xhr?.abort()}
  adminReleaseData={draft:null,release:null};
  adminReleaseDirty=false;
  $('#admin-release-apk').value='';
  $('#admin-release-progress').value=0;
  show('#admin-release-upload-progress',false);
  renderAdminReleases(adminReleaseData);
  adminReleaseStatus('');
}

async function runAdminReleaseOperation(message,work,upload=false){
  if(!adminReleasesVisible()||adminReleaseOperation)return;
  const operation={epoch:sessionEpoch,controller:new AbortController(),xhr:null,timedOut:false};
  const current=()=>adminReleaseOperation===operation&&operation.epoch===sessionEpoch&&adminReleasesVisible();
  operation.current=current;
  operation.timer=setTimeout(()=>{
    operation.timedOut=true;
    operation.controller.abort();operation.xhr?.abort();
  },upload?600000:30000);
  adminReleaseOperation=operation;
  syncAdminReleaseControls();
  adminReleaseStatus(message);
  try{await work(operation)}catch(error){
    if(current())adminReleaseStatus(operation.timedOut?'请求超时，请重试或重新加载。':error.message||'操作失败，请重试。',true);
  }finally{
    clearTimeout(operation.timer);
    if(adminReleaseOperation===operation){
      adminReleaseOperation=null;
      if(adminReleasesVisible())syncAdminReleaseControls();
    }
  }
}

async function loadAdminReleases(){
  return runAdminReleaseOperation('正在加载版本信息…',async operation=>{
    const data=await api(`${androidReleaseBase}/release`,{cache:'no-store',signal:operation.controller.signal});
    if(!operation.current())return;
    renderAdminReleases(data);
    adminReleaseStatus('版本信息已更新。');
  });
}

function uploadAndroidAPKRequest(file,operation){
  return new Promise((resolve,reject)=>{
    if(!operation.current()){reject(new Error('操作已取消'));return}
    const xhr=new XMLHttpRequest();
    operation.xhr=xhr;
    xhr.open('POST',`${androidReleaseBase}/apk`);
    xhr.timeout=600000;
    if(session?.accessToken)xhr.setRequestHeader('Authorization',`Bearer ${session.accessToken}`);
    xhr.upload.onprogress=event=>{
      if(!operation.current())return;
      if(event.lengthComputable){
        const percent=Math.min(100,Math.round(event.loaded/event.total*100));
        $('#admin-release-progress').value=percent;
        $('#admin-release-progress-text').textContent=percent===100?'上传完成，正在校验 APK…':`正在上传 ${percent}%`;
      }
    };
    xhr.onload=()=>{
      let body;
      try{body=JSON.parse(xhr.responseText)}catch{reject(new Error('服务器未返回有效版本信息'));return}
      resolve({status:xhr.status,body});
    };
    xhr.onerror=()=>reject(new Error('上传失败，请检查网络后重试。'));
    xhr.ontimeout=()=>reject(new Error('上传超时，请重试。'));
    xhr.onabort=()=>reject(new Error('上传已取消。'));
    const form=new FormData();
    form.append('apk',file);
    xhr.send(form);
  });
}

async function uploadAdminRelease(){
  const file=$('#admin-release-apk').files?.[0];
  if(!file||!adminReleasesVisible()||adminReleaseOperation)return;
  if(!/\.apk$/i.test(file.name)||file.size<=0||file.size>androidReleaseMaxBytes){
    adminReleaseStatus('请选择不超过 256 MB 的 APK 文件。',true);return;
  }
  return runAdminReleaseOperation('正在上传 APK…',async operation=>{
    if(adminReleaseData.draft){
      const accepted=await confirmDialog({title:'替换待发布草稿',message:'上传新 APK 将替换当前草稿及其更新说明。确认继续？',confirmText:'上传并替换',danger:false});
      if(!accepted||!operation.current()){if(operation.current())adminReleaseStatus('已取消上传。');return}
    }
    show('#admin-release-upload-progress',true);
    $('#admin-release-progress').value=0;
    $('#admin-release-progress-text').textContent='正在上传 0%';
    let response=await uploadAndroidAPKRequest(file,operation);
    if(!operation.current())return;
    if(response.status===401){
      if(!await refreshSession()||!operation.current())return;
      response=await uploadAndroidAPKRequest(file,operation);
    }
    if(!operation.current())return;
    if(response.status<200||response.status>=300)throw new Error(response.body.error?.message||`上传失败 (${response.status})`);
    renderAdminReleases(response.body);
    $('#admin-release-apk').value='';
    $('#admin-release-progress').value=100;
    $('#admin-release-progress-text').textContent='APK 校验完成';
    adminReleaseStatus('APK 已生成草稿。编辑更新说明并保存，然后发布。');
  },true);
}

async function saveAdminReleaseDraft(){
  const draft=adminReleaseData.draft;
  if(!draft||!adminReleaseDirty)return;
  const notes=$('#admin-release-changelog').value;
  if(notes.length>10000){adminReleaseStatus('更新说明不能超过 10000 个字符。',true);return}
  const body={draftId:draft.id,versionCode:draft.versionCode,versionName:draft.versionName,changelog:notes,forceUpdate:$('#admin-release-force').checked};
  return runAdminReleaseOperation('正在保存草稿…',async operation=>{
    const data=await api(`${androidReleaseBase}/release`,{method:'PUT',body:JSON.stringify(body),signal:operation.controller.signal});
    if(!operation.current())return;
    renderAdminReleases(data);
    adminReleaseStatus('草稿已保存，可以发布。');
  });
}

async function publishAdminRelease(){
  const draft=adminReleaseData.draft;
  if(!draft||adminReleaseDirty)return;
  return runAdminReleaseOperation('等待确认发布…',async operation=>{
    const accepted=await confirmDialog({title:'发布安卓版本',message:`确认发布 ${draft.versionName}（${draft.versionCode}）？${draft.forceUpdate?'此版本将要求旧版本用户更新后继续使用。':'用户进入应用时将收到可选更新提示。'}`,confirmText:'发布',danger:!!draft.forceUpdate});
    if(!accepted||!operation.current()){
      if(operation.current())adminReleaseStatus('已取消发布。');return;
    }
    adminReleaseStatus('正在发布版本…');
    const data=await api(`${androidReleaseBase}/publish`,{method:'POST',body:JSON.stringify({draftId:draft.id}),signal:operation.controller.signal});
    if(!operation.current())return;
    renderAdminReleases(data);
    adminReleaseStatus('安卓版本已发布。');
  });
}

async function saveAdminReleasePolicy(){
  const release=adminReleaseData.release,forceUpdate=$('#admin-release-policy').checked;
  if(!release||forceUpdate===!!release.forceUpdate)return;
  return runAdminReleaseOperation('正在更新发布策略…',async operation=>{
    try{
      if(forceUpdate){
        const accepted=await confirmDialog({title:'开启强制更新',message:`旧于 ${release.versionName}（${release.versionCode}）的客户端需要更新后才能继续使用。确认开启？`,confirmText:'开启',danger:true});
        if(!accepted||!operation.current()){
          if(operation.current()){restoreAdminReleasePolicy();adminReleaseStatus('已取消修改发布策略。')}return;
        }
      }
      const data=await api(`${androidReleaseBase}/policy`,{method:'PUT',body:JSON.stringify({releaseId:release.id,forceUpdate}),signal:operation.controller.signal});
      if(!operation.current())return;
      // 修改现行策略时保留尚未保存的草稿编辑。
      const dirty=adminReleaseDirty,draftId=adminReleaseData.draft?.id,notes=$('#admin-release-changelog').value,draftForce=$('#admin-release-force').checked;
      renderAdminReleases(data);
      if(dirty&&data.draft?.id===draftId){adminReleaseDirty=true;$('#admin-release-changelog').value=notes;$('#admin-release-force').checked=draftForce}
      adminReleaseStatus(data.release?.forceUpdate?'已开启当前版本的强制更新。':'已关闭当前版本的强制更新。');
    }catch(error){
      if(operation.current())restoreAdminReleasePolicy();
      throw error;
    }
  });
}

function bindAdminReleases(){
  $('#admin-release-refresh').addEventListener('click',async()=>{
    if(adminReleaseOperation||!adminReleasesVisible())return;
    const epoch=sessionEpoch;
    if(adminReleaseDirty&&!(await confirmDialog({title:'重新加载版本信息',message:'重新加载会放弃尚未保存的更新说明和草稿设置。',confirmText:'重新加载',danger:false})))return;
    if(epoch===sessionEpoch&&adminReleasesVisible())void loadAdminReleases();
  });
  $('#admin-release-apk').addEventListener('change',syncAdminReleaseControls);
  $('#admin-release-upload-form').addEventListener('submit',event=>{event.preventDefault();void uploadAdminRelease()});
  $('#admin-release-draft-form').addEventListener('submit',event=>{event.preventDefault();void saveAdminReleaseDraft()});
  $('#admin-release-publish').addEventListener('click',()=>void publishAdminRelease());
  $('#admin-release-policy-form').addEventListener('submit',event=>{event.preventDefault();void saveAdminReleasePolicy()});
  $('#admin-release-policy').addEventListener('change',syncAdminReleaseControls);
  const edit=()=>{
    if(!adminReleaseData.draft||adminReleaseOperation)return;
    adminReleaseDirty=true;
    syncAdminReleaseControls();
    adminReleaseStatus('草稿有未保存的修改，请先保存再发布。');
  };
  $('#admin-release-changelog').addEventListener('input',edit);
  $('#admin-release-force').addEventListener('change',edit);
}
