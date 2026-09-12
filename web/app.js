import {materialSize,drawTransformed} from './geometry.mjs';
import {$,api,apiBlob,uploadFile,toast,clone,renderIcons,formatBytes,authHeaders} from './ui.js';
import {MaterialEditor} from './editor.js';

const MODES={
 'pdf-images':{title:'PDF 转图片',sub:'支持去空白裁剪、高质量导出、自定义大小限制，让材料更规范、更清晰。',desc:'支持批量添加 PDF，每页独立导出。',drop:'支持 PDF 文件 · 可批量添加',accept:'.pdf',kind:'pdf',scope:'每张图片',tip:'有白边的证书、成绩单，先裁剪再压缩，更容易保留清晰的正文。',kicker:'PDF → IMAGE',features:[['智能裁剪','自动去除空白边'],['多格式导出','JPG / PNG / GIF'],['自定义大小','自由设置 KB/MB'],['批量处理','高效处理多文件']]},
 'image-compress':{title:'图片压缩与编辑',sub:'裁剪、旋转、去白边，再把图片压到你指定的大小以内。',desc:'支持 JPG、JPEG、PNG、静态 GIF；可批量编辑。',drop:'支持 JPG / JPEG / PNG / GIF · 可批量添加',accept:'.jpg,.jpeg,.png,.gif',kind:'image',scope:'每张图片',tip:'图片可以先裁剪、旋转或限制最长边，再执行大小控制。',kicker:'IMAGE STUDIO',features:[['自由裁剪','多种常用比例'],['旋转翻转','快速校正方向'],['自定义大小','精确控制 KB/MB'],['批量处理','多图统一操作']]},
 'images-pdf':{title:'图片合并 PDF',sub:'按需要的顺序把多张图片合成 PDF，并控制整份文件的最终体积。',desc:'拖动排序后合并成一份图像型 PDF。',drop:'支持 JPG / JPEG / PNG / GIF · 拖动调整顺序',accept:'.jpg,.jpeg,.png,.gif',kind:'image',scope:'整份 PDF',tip:'合并 PDF 的上限作用于整份文件；图片越多，单页可用体积越少。',kicker:'IMAGE → PDF',features:[['灵活排序','拖动调整页面顺序'],['页面适配','A4 / 贴合图片'],['整份控大小','自定义最终体积'],['批量合并','一次生成 PDF']]},
 'pdf-compress':{title:'PDF 压缩',sub:'适合扫描版材料，在保持页数的同时尽量减小文件体积。',desc:'适合扫描版 PDF；不会保留文本层和数字签名。',drop:'支持扫描版 PDF · 可批量添加',accept:'.pdf',kind:'pdf',scope:'每份 PDF',tip:'需要验证数字签名、电子章或文本层的 PDF，不要使用扫描重建模式。',kicker:'PDF COMPRESS',features:[['智能压缩','按目标大小重建'],['保持页数','页面顺序不变'],['自定义上限','自由设置 KB/MB'],['本地处理','材料不上传']]}
};
const DEFAULT_TOOL={amount:'2',unit:'MB',format:'jpg',profile:'balanced',dpi:200,paper:'a4',cropMode:'auto',marginMM:2,maxEdge:0,strict:true,makeZip:false,trimImages:false};
const state={config:null,prefs:null,mode:'pdf-images',files:new Map(),queues:{'pdf-images':[],'image-compress':[],'images-pdf':[],'pdf-compress':[]},selected:{},plans:{},edits:{},results:[],job:null,lastJobId:'',jobOutput:'',lastFailed:[],dragId:null,saving:false,busy:false,uploading:false,workView:'queue',sidePreview:{id:'',page:1,total:1,meta:null,url:'',serial:0,tab:'original',image:null}};
for(const k of Object.keys(MODES))state.selected[k]=new Set();
const ids=()=>state.queues[state.mode];
const items=()=>ids().map(id=>state.files.get(id)).filter(Boolean);
const selectedIds=()=>ids().filter(id=>state.selected[state.mode].has(id));
const imageFiles=()=>items().filter(x=>x.kind==='image');

function toolPrefs(mode=state.mode){
 state.prefs.tools ||= {};
 if(!state.prefs.tools[mode])state.prefs.tools[mode]={...DEFAULT_TOOL,format:mode==='image-compress'?'auto':'jpg',strict:mode==='pdf-images'?false:true};
 return state.prefs.tools[mode];
}
function captureSettings(){
 const p=toolPrefs();p.amount=$('sizeAmount').value.trim();p.unit=$('sizeUnit').value;p.format=$('outputFormat').value;p.profile=$('profile').value;p.dpi=Number($('dpi').value)||200;p.paper=$('paper').value;p.cropMode=$('cropMode').value;p.marginMM=Number($('cropMargin').value);p.maxEdge=Number($('maxEdge').value)||0;p.strict=$('strict').checked;p.makeZip=$('makeZip').checked;p.trimImages=$('trimImages').checked;
 state.prefs.output=$('outputDir').value.trim();state.prefs.mode=state.mode;state.prefs.theme=document.documentElement.dataset.theme||'light';
}
let saveTimer=0;function schedulePrefs(){clearTimeout(saveTimer);saveTimer=setTimeout(savePrefs,450)}
async function savePrefs(){if(!state.prefs||state.saving)return;captureSettings();state.saving=true;try{await api('prefs','PUT',state.prefs)}catch(e){console.warn(e)}finally{state.saving=false}}
function loadSettings(){
 const p=toolPrefs();$('sizeAmount').value=p.amount||'2';$('sizeUnit').value=p.unit||'MB';$('outputFormat').value=p.format||'jpg';$('profile').value=p.profile||'balanced';$('dpi').value=String(p.dpi||200);$('paper').value=p.paper||'a4';$('cropMode').value=p.cropMode||'auto';$('cropMargin').value=String(p.marginMM??2);$('maxEdge').value=String(p.maxEdge||0);$('strict').checked=!!p.strict;$('makeZip').checked=!!p.makeZip;$('trimImages').checked=!!p.trimImages;$('outputDir').value=state.prefs.output||'';validateSize();updateCropSummary();updateStart();
}
function setMode(mode,initial=false){
 if(state.busy||state.uploading){toast('处理进行中，请先等待完成或停止。','error');return}
 if(!initial)captureSettings();clearSidePreview();state.mode=mode;const m=MODES[mode];document.documentElement.dataset.mode=mode;
 document.querySelectorAll('.nav-item').forEach(b=>b.classList.toggle('active',b.dataset.mode===mode));
 $('pageTitle').textContent=m.title;$('pageSubtitle').textContent=m.sub;$('heroKicker').textContent=m.kicker||'MATERIAL STUDIO';
 (m.features||[]).slice(0,4).forEach((f,i)=>{$('heroFeature'+(i+1)).textContent=f[0];$('heroFeatureSub'+(i+1)).textContent=f[1]});
 $('queueDescription').textContent=m.desc;$('dropDescription').textContent=m.drop;$('limitScope').textContent=m.scope;$('workspaceTip').textContent=m.tip;$('fileInput').accept=m.accept;
 const pdfImages=mode==='pdf-images', imageMode=mode==='image-compress', merge=mode==='images-pdf', pdfCompress=mode==='pdf-compress';
 $('pdfOptions').hidden=!pdfImages&&!pdfCompress;$('pagesField').hidden=!pdfImages;$('cropSettings').hidden=!pdfImages;$('imageOptions').hidden=!imageMode;$('paperOptions').hidden=!merge;$('consentBox').hidden=!pdfCompress;$('formatField').hidden=merge||pdfCompress;$('passwordField').hidden=!(pdfImages||pdfCompress);$('maxEdgeField').hidden=!(imageMode||pdfImages);
 $('strictLabel').hidden=pdfCompress;$('strictNote').hidden=pdfCompress;$('openCrop').hidden=!pdfImages;$('openImageEdit').hidden=!imageMode;
 const profileHint=$('profile').closest('.field')?.querySelector('.field-label span');if(profileHint)profileHint.textContent=merge?'影响合并页清晰度':'优先保留小字';
 state.prefs.mode=mode;loadSettings();renderQueue();renderResults(true);setWorkView('queue');updateWorkflow();schedulePrefs();
}
function sizeBytes(){
 const raw=$('sizeAmount').value.trim();if(!/^\d+(?:\.\d+)?$/.test(raw))return NaN;const n=Number(raw)*($('sizeUnit').value==='MB'?1_000_000:1000);return Number.isFinite(n)?Math.floor(n):NaN;
}
function validateSize(){
 const n=sizeBytes(),err=$('sizeError'),note=$('sizeNote');let message='';
 if(!Number.isFinite(n)||n<1000||n>100_000_000)message='请输入 1 KB～100 MB，例如 500 KB、1.5 MB。';
 err.hidden=!message;err.textContent=message;$('sizeAmount').classList.toggle('invalid',!!message);
 if(!message){const unit=n>=1_000_000?(n/1_000_000).toFixed(2).replace(/\.00$/,'')+' MB':(n/1000).toFixed(1).replace(/\.0$/,'')+' KB';const target=Math.floor(n*.95);note.textContent=`严格小于 ${n.toLocaleString()} 字节 · 安全目标 ${formatBytes(target)} · 当前上限 ${unit}`}
 updateStart();return !message;
}
function updateCropSummary(){
 const m=$('cropMode').value;$('cropMargin').disabled=!['auto','uniform'].includes(m)||state.busy;const map={none:'保留整页，不改变页面范围。',auto:'逐页识别白边，保留 '+$('cropMargin').value+' mm 边距。',uniform:'先分析所选页，再用内容边界并集统一裁剪。',manual:'按你在预览窗口保存的裁剪方案导出；未框选页保留整页。'};$('cropSummary').textContent=map[m]||map.none;
}
function formatKind(item){return item.kind==='pdf'?'PDF':(item.name.split('.').pop()||'IMG').toUpperCase()}
function currentPreviewRect(item,page){
 if(!item)return null;
 if(item.kind==='pdf'){
  const plan=state.plans[item.id];if(!plan)return null;
  return plan.pages?.[String(page)]||plan.pages?.[page]||plan.all||null;
 }
 return state.edits[item.id]?.rect||null;
}
function revokeSidePreview(){if(state.sidePreview.url){URL.revokeObjectURL(state.sidePreview.url);state.sidePreview.url=''}state.sidePreview.image=null}
function clearSidePreview(){state.sidePreview.serial++;state.sidePreview.abort?.abort();revokeSidePreview();Object.assign(state.sidePreview,{id:'',page:1,total:1,meta:null,tab:'original'});const pe=$('sidePreviewEmpty');pe.querySelector('strong').textContent='选择一个材料查看预览';pe.querySelector('span').textContent='添加文件后会自动显示第一页';pe.hidden=false;$('sidePreviewMedia').hidden=true;$('retryPreview').hidden=true;$('sidePageText').textContent='— / —';$('sidePrevPage').disabled=true;$('sideNextPage').disabled=true}
function transformedPreviewCanvas(img,edit){
 const rot=((Number(edit?.rotation)||0)%360+360)%360,swap=rot===90||rot===270,w=swap?img.naturalHeight:img.naturalWidth,h=swap?img.naturalWidth:img.naturalHeight,c=document.createElement('canvas');c.width=Math.max(1,w);c.height=Math.max(1,h);const g=c.getContext('2d');drawTransformed(g,img,edit||{},w,h);return c;
}
function sourceDimensions(item,meta,edit={}){return materialSize(item,meta,Number($('dpi').value),edit.rotation||0)}
function renderSidePreview(){
 const sp=state.sidePreview,item=state.files.get(sp.id);if(!item||!sp.meta||!sp.image)return;
 $('sidePreviewEmpty').hidden=true;$('sidePreviewMedia').hidden=false;$('sidePreviewLoading').hidden=true;$('retryPreview').hidden=true;
 $('sidePreviewName').textContent=item.name;$('sidePreviewName').title=item.name;$('sidePreviewBytes').textContent=formatBytes(item.bytes);
 $('sidePreviewPages').textContent=$('sidePageText').textContent=`${sp.page} / ${sp.total}`;
 const editable=state.mode==='pdf-images'||state.mode==='image-compress';$('sideEditButton').hidden=!editable;$('sideEditButton').disabled=state.busy;
 $('sidePrevPage').disabled=sp.page<=1;$('sideNextPage').disabled=sp.page>=sp.total;
 $('previewOriginalTab').classList.toggle('active',sp.tab==='original');$('previewCropTab').classList.toggle('active',sp.tab==='crop');
 const edit=item.kind==='image'?(state.edits[item.id]||{}):{},[ow,oh]=sourceDimensions(item,sp.meta),[sw,sh]=sourceDimensions(item,sp.meta,edit);
 $('previewDimensionLabel').textContent=item.kind==='pdf'?'按 DPI 渲染':'原始尺寸';$('sidePreviewDimensions').textContent=`${ow} × ${oh}`;
 let rect=null,note='';const cropMode=state.mode==='pdf-images'?$('cropMode').value:'none';
 if(item.kind==='pdf'){
  if(cropMode==='manual')rect=currentPreviewRect(item,sp.page);
  if(cropMode==='auto')rect=sp.autoRect;
  if(cropMode==='uniform')note='统一范围在导出时计算所选页并集；此处显示原页。';
 }else rect=edit.rect;
 if(item.kind==='image'&&!rect&&(edit.autoTrim||$('trimImages').checked))rect=sp.autoRect;
 const needsAuto=(item.kind==='pdf'&&cropMode==='auto')||(item.kind==='image'&&!edit.rect&&(edit.autoTrim||$('trimImages').checked));
 if(needsAuto&&!rect)note=sp.autoError||'正在识别白边…';
 if(sp.autoBlank&&needsAuto)note='空白页完整保留。';
 if(rect&&needsAuto&&!note)note='预览为白边识别参考，最终以导出结果为准。';
 $('sidePreviewNote').textContent=note;
 const r=rect||{left:0,top:0,right:1,bottom:1};$('sidePreviewCropSize').textContent=`${Math.ceil(r.right*sw)-Math.floor(r.left*sw)} × ${Math.ceil(r.bottom*sh)-Math.floor(r.top*sh)}`;
 const img=$('sidePreviewImage'),canvas=$('sidePreviewCanvas');
 if(sp.tab==='original'){canvas.hidden=true;img.hidden=false;img.src=sp.url;return}
 const source=item.kind==='image'?transformedPreviewCanvas(sp.image,edit):sp.image,w=source.width||source.naturalWidth,h=source.height||source.naturalHeight;
 const x=Math.floor(r.left*w),y=Math.floor(r.top*h),cw=Math.max(1,Math.ceil(r.right*w)-x),ch=Math.max(1,Math.ceil(r.bottom*h)-y),scale=Math.min(1,900/Math.max(cw,ch));
 canvas.width=Math.max(1,Math.round(cw*scale));canvas.height=Math.max(1,Math.round(ch*scale));canvas.getContext('2d').drawImage(source,x,y,cw,ch,0,0,canvas.width,canvas.height);img.hidden=true;canvas.hidden=false;
}
async function refreshAutoPreview(){
 const sp=state.sidePreview,item=state.files.get(sp.id);if(!item||!sp.meta)return;
 const edit=item.kind==='image'?(state.edits[item.id]||{}):{},needed=item.kind==='pdf'?state.mode==='pdf-images'&&$('cropMode').value==='auto':!edit.rect&&(edit.autoTrim||$('trimImages').checked);
 const key=JSON.stringify([sp.meta.key,edit,$('cropMargin').value,needed]);if(sp.autoKey===key){renderSidePreview();return}
 sp.autoKey=key;sp.autoRect=null;sp.autoError='';sp.autoBlank=false;renderSidePreview();if(!needed)return;
 try{const res=await api('autocrop','POST',{id:item.id,previewKey:sp.meta.key,edit:{...edit,rect:null,autoTrim:false,threshold:250,padding:8},marginMM:Number($('cropMargin').value)});if(sp.autoKey!==key||sp.id!==item.id)return;sp.autoRect=res.rect;sp.autoBlank=res.blank;renderSidePreview()}
 catch(e){if(sp.autoKey!==key)return;sp.autoError='白边预览暂不可用，可重试或手动编辑。';renderSidePreview()}
}
async function selectSidePreview(id,page=1,force=false){
 const item=state.files.get(id);if(!item||!ids().includes(id))return;page=Math.max(1,Math.floor(Number(page)||1));
 const sp=state.sidePreview;if(!force&&sp.id===id&&sp.page===page&&sp.meta){renderSidePreview();return}
 sp.abort?.abort();sp.abort=new AbortController();const signal=sp.abort.signal,serial=++sp.serial;revokeSidePreview();Object.assign(sp,{id,page,meta:null,total:1,autoKey:'',autoRect:null,autoError:''});
 $('sidePreviewEmpty').hidden=true;$('sidePreviewMedia').hidden=false;$('sidePreviewLoading').hidden=false;$('sidePreviewImage').hidden=true;$('sidePreviewCanvas').hidden=true;$('sidePreviewName').textContent=item.name;$('sidePrevPage').disabled=$('sideNextPage').disabled=true;
 try{const meta=await api(`preview/${id}`,'POST',{page,password:item.kind==='pdf'?$('pdfPassword').value:''},{signal});const blob=await apiBlob(`blob/${meta.key}`,{signal});if(serial!==sp.serial)return;
  const url=URL.createObjectURL(blob),image=new Image();image.src=url;try{await image.decode()}catch(e){URL.revokeObjectURL(url);throw e}if(serial!==sp.serial){URL.revokeObjectURL(url);return}
  Object.assign(sp,{meta,url,image,total:Math.max(1,meta.total||1),page:Math.min(page,meta.total||1)});item.pages=sp.total;renderSidePreview();renderQueue(false);refreshAutoPreview();
 }catch(e){if(serial!==sp.serial||e.name==='AbortError')return;$('sidePreviewLoading').hidden=true;$('sidePreviewMedia').hidden=true;$('sidePreviewEmpty').hidden=false;$('sidePreviewEmpty').querySelector('strong').textContent='暂时无法生成预览';$('sidePreviewEmpty').querySelector('span').textContent=e.message;$('retryPreview').hidden=false}
}
function setWorkView(view){state.workView=view;$('queueCard').hidden=view!=='queue';$('resultsCard').hidden=view!=='results';for(const [id,v] of [['queueViewTab','queue'],['resultsViewTab','results']]){$(id).classList.toggle('active',v===view);$(id).setAttribute('aria-selected',String(v===view))}}
function renderQueue(allowAutoPreview=true){
 const list=$('fileList'),empty=$('emptyState'),tools=$('queueTools'),footer=$('queueFooter'),head=$('queueTableHead'),arr=items();$('queueCount').textContent=$('queueTabCount').textContent=arr.length;$('addMoreFiles').hidden=!arr.length;$('loadExample').hidden=arr.length>0;list.replaceChildren();
 empty.hidden=arr.length>0;list.hidden=arr.length===0;tools.hidden=arr.length===0;footer.hidden=arr.length===0;if(head)head.hidden=arr.length===0;
 const sel=state.selected[state.mode],all=arr.length>0&&arr.every(x=>sel.has(x.id));$('selectAll').checked=all;$('selectAll').indeterminate=!all&&arr.some(x=>sel.has(x.id));$('selectionText').textContent=sel.size?`已选 ${arr.filter(x=>sel.has(x.id)).length}`:'全选';
 let total=0;arr.forEach((item,index)=>{total+=item.bytes;const row=document.createElement('div');row.className='file-row'+(state.sidePreview.id===item.id?' preview-selected':'');row.role='listitem';row.draggable=!state.busy;row.tabIndex=0;row.setAttribute('aria-label','预览 '+item.name);row.onkeydown=e=>{if(e.target===row&&(e.key==='Enter'||e.key===' ')){e.preventDefault();selectSidePreview(item.id)}};row.dataset.id=item.id;
  const check=document.createElement('input');check.type='checkbox';check.className='file-check';check.checked=sel.has(item.id);check.disabled=state.busy;check.setAttribute('aria-label','选择 '+item.name);check.onchange=()=>{check.checked?sel.add(item.id):sel.delete(item.id);renderQueue(false);updateStart()};
  const thumb=document.createElement('div');thumb.className='file-thumb';thumb.innerHTML='<i data-icon="'+(item.kind==='pdf'?'pdf':'image')+'"></i>';
  const main=document.createElement('div');main.className='file-main';const name=document.createElement('div');name.className='file-name';name.textContent=item.name;name.title=item.name;const meta=document.createElement('div');meta.className='file-meta';const type=document.createElement('span');type.className='file-type';type.textContent=formatKind(item);meta.append(type);if(item.width&&item.height)meta.append(document.createTextNode(`${item.width}×${item.height}`));main.append(name,meta);
  const pageCount=document.createElement('div');pageCount.className='file-pages';pageCount.textContent=item.kind==='image'?'1':(item.pages||'—');const size=document.createElement('div');size.className='file-size';size.textContent=formatBytes(item.bytes);const status=document.createElement('span');status.className='file-state'+((state.plans[item.id]||state.edits[item.id])?' edited':'');status.textContent=(state.plans[item.id]||state.edits[item.id])?'已编辑':'待处理';const recs=state.results.filter(r=>r.inputId===item.id);if(recs.length){const failed=recs.some(r=>r.status==='failed');status.textContent=failed?'需检查':'已完成';status.className='file-state '+(failed?'failed':'saved')}else if(state.busy){status.textContent=state.selected[state.mode].has(item.id)?'处理中':'未选中'}
  const actions=document.createElement('div');actions.className='file-actions';if((state.mode==='pdf-images'&&item.kind==='pdf')||(state.mode==='image-compress'&&item.kind==='image')){const edit=document.createElement('button');edit.className='text-button file-edit';edit.title='裁剪 / 调整';edit.innerHTML='<i data-icon="crop"></i><span>编辑</span>';edit.disabled=state.busy;edit.onclick=e=>{e.stopPropagation();selectSidePreview(item.id,1);editor.open(item)};actions.append(edit)}
  const run=document.createElement('button');run.className='icon-button compact file-run';run.title='处理此文件';run.setAttribute('aria-label','处理 '+item.name);run.innerHTML='<i data-icon="play"></i>';run.disabled=state.busy||state.uploading;run.onclick=e=>{e.stopPropagation();startJob([item.id])};if(state.mode!=='images-pdf')actions.prepend(run);const order=document.createElement('span');order.className='file-order';for(const [icon,delta,label] of [['left',-1,'上移'],['right',1,'下移']]){const b=document.createElement('button');b.className='icon-button compact';b.title=label;b.innerHTML='<i data-icon="'+icon+'"></i>';b.disabled=state.busy||(delta<0?index===0:index===arr.length-1);b.onclick=e=>{e.stopPropagation();move(item.id,delta)};order.append(b)}if(state.mode==='images-pdf')actions.append(order);
  const rm=document.createElement('button');rm.className='icon-button compact';rm.title='移除';rm.innerHTML='<i data-icon="trash"></i>';rm.disabled=state.busy;rm.onclick=e=>{e.stopPropagation();removeFromQueue(item.id)};actions.append(rm);
  row.append(check,thumb,main,pageCount,size,status,actions);row.onclick=e=>{if(!e.target.closest('button,input,label,select'))selectSidePreview(item.id,state.sidePreview.id===item.id?state.sidePreview.page:1)};row.addEventListener('dragstart',e=>{state.dragId=item.id;row.classList.add('dragging');e.dataTransfer.effectAllowed='move'});row.addEventListener('dragend',()=>{state.dragId=null;document.querySelectorAll('.file-row').forEach(r=>r.classList.remove('dragging','drop-before'))});row.addEventListener('dragover',e=>{e.preventDefault();if(state.dragId&&state.dragId!==item.id)row.classList.add('drop-before')});row.addEventListener('dragleave',()=>row.classList.remove('drop-before'));row.addEventListener('drop',e=>{e.preventDefault();row.classList.remove('drop-before');e.stopPropagation();reorder(state.dragId,item.id)});list.append(row);renderIcons(row)
 });
 $('queueTotal').textContent=`合计 ${formatBytes(total)}`;$('editSelected').disabled=state.busy||selectedIds().length===0||!['pdf-images','image-compress'].includes(state.mode);updateStart();updateWorkflow();
 $('clearQueue').disabled=$('removeSelected').disabled=$('selectAll').disabled=state.busy||state.uploading;if(!arr.length)clearSidePreview();else if(allowAutoPreview&&!arr.some(x=>x.id===state.sidePreview.id))queueMicrotask(()=>selectSidePreview(arr[0].id,1));
}
function move(id,delta){if(state.busy||state.uploading)return;const a=ids(),i=a.indexOf(id),j=i+delta;if(i<0||j<0||j>=a.length)return;[a[i],a[j]]=[a[j],a[i]];renderQueue()}
function reorder(from,to){if(state.busy||state.uploading)return;if(!from||from===to)return;const a=ids(),i=a.indexOf(from),j=a.indexOf(to);if(i<0||j<0)return;a.splice(i,1);a.splice(j,0,from);renderQueue()}
async function removeFromQueue(id){if(state.busy||state.uploading)return;const a=ids(),i=a.indexOf(id);if(i>=0)a.splice(i,1);state.selected[state.mode].delete(id);let used=false;for(const q of Object.values(state.queues))if(q.includes(id))used=true;if(!used){try{await api('files/'+id,'DELETE')}catch{}delete state.plans[id];delete state.edits[id];state.files.delete(id)}renderQueue()}
async function clearQueue(){for(const id of [...ids()])await removeFromQueue(id);renderQueue()}
function acceptable(file){const ext='.'+(file.name.split('.').pop()||'').toLowerCase();return MODES[state.mode].accept.split(',').includes(ext)}
async function addFiles(files){
 if(state.busy||state.uploading)return;const targetMode=state.mode;const valid=[...files].filter(f=>acceptable(f));if(!valid.length){toast('当前工具不支持这些文件类型。','error');return}if(valid.length+ids().length>200){toast('每个工作区最多 200 个文件，请分批处理。','error');return}
 state.uploading=true;setBusy(true);setWorkView('queue');$('statusTitle').textContent='正在添加材料';$('statusDetail').textContent=`读取 ${valid.length} 个本地文件…`;let added=0;
 for(const file of valid){try{const res=await uploadFile(file);const item=res.file;state.files.set(item.id,item);if(!ids().includes(item.id))ids().push(item.id);state.selected[state.mode].add(item.id);added++;if(res.duplicate)toast(`${item.name} 已在工作区，已直接复用。`,'info',2200)}catch(e){toast(`${file.name}：${e.message}`,'error',6500)}renderQueue()}
 $('statusTitle').textContent=added?'材料已就绪':'没有添加材料';$('statusDetail').textContent=added?`已添加 ${added} 个材料，可预览或直接导出。`:'请检查文件类型或大小。';state.uploading=false;setBusy(false);updateWorkflow();
}
function updateWorkflow(){const has=items().length>0,edited=items().some(i=>state.plans[i.id]||state.edits[i.id])||$('cropMode').value!=='none'||$('trimImages').checked;const valid=validateSizeSilently();document.querySelectorAll('.step').forEach((el,i)=>{const n=i+1;el.classList.remove('active','done');if(n===1){el.classList.add(has?'done':'active')}else if(n===2){if(has)el.classList.add(edited?'done':'active')}else if(n===3){if(has&&valid)el.classList.add(state.results.length?'done':'active')}else if(n===4&&state.results.length)el.classList.add('active')})}
function validateSizeSilently(){const n=sizeBytes();return Number.isFinite(n)&&n>=1000&&n<=100_000_000}
function updateStart(){const n=selectedIds().length,ok=n>0&&validateSizeSilently()&&!state.busy&&!state.uploading&&($('outputDir').value.trim().length>0)&&!(state.mode==='pdf-compress'&&!$('consent').checked);$('startExport').disabled=!ok;$('startText').textContent=state.busy?'处理中…':`开始处理${n>0?' ('+n+')':''}`}

function renderResults(clear=false){
 if(clear){state.results=[];state.jobOutput='';state.lastFailed=[]}
 const list=$('resultList');list.replaceChildren();$('resultCount').textContent=$('resultsTabCount').textContent=state.results.length;$('resultsEmpty').hidden=state.results.length>0;$('resultList').hidden=state.results.length===0;$('resultSummary').hidden=state.results.length===0;$('resultActions').hidden=state.results.length===0;$('openFolder').disabled=!state.jobOutput;if($('openOutputDir'))$('openOutputDir').disabled=!state.jobOutput;
 let saved=0,kept=0,failed=0;state.lastFailed=[];
 for(const rec of state.results){if(rec.status==='saved')saved++;else if(rec.status==='unchanged')kept++;else{failed++;if(rec.inputId)state.lastFailed.push(rec.inputId)}const row=document.createElement('div');row.className='result-row';const st=document.createElement('div');st.className='result-status '+(rec.status||'failed');st.innerHTML='<i data-icon="'+(rec.status==='failed'?'warning':rec.status==='unchanged'?'file':'check')+'"></i>';
  const main=document.createElement('div');const name=document.createElement('div');name.className='result-name';name.textContent=rec.output?rec.output.split(/[\\/]/).pop():(rec.page?`第 ${rec.page} 页`:'处理结果');const measure=document.createElement('div');measure.className='result-measure';if(rec.after){const b=document.createElement('b');b.textContent=formatBytes(rec.after);measure.append(b)}if(rec.width&&rec.height)measure.append(document.createTextNode(`${rec.width}×${rec.height}`));if(rec.before&&rec.after&&rec.status==='saved'&&rec.after<rec.before)measure.append(document.createTextNode(`↓ ${Math.round((1-rec.after/rec.before)*100)}%`));const note=document.createElement('div');note.className='result-note';note.textContent=rec.message||'';main.append(name,measure,note);
  const act=document.createElement('button');act.className='icon-button compact';const ext=(rec.output||'').split('.').pop().toLowerCase();const canPreview=['jpg','jpeg','png','gif'].includes(ext);act.title=canPreview?'预览结果':'用系统程序打开';act.innerHTML='<i data-icon="'+(canPreview?'image':'external')+'"></i>';act.onclick=()=>canPreview?previewResult(rec):openRecord(rec);if(!rec.output)act.disabled=true;row.append(st,main,act);list.append(row);renderIcons(row)}
 list.hidden=state.results.length===0;const sum=$('resultSummary');sum.replaceChildren();for(const [n,label,cls] of [[saved,'成功',''],[kept,'保留原件','kept'],[failed,'失败','failed']])if(n){const s=document.createElement('span');s.className=cls;s.textContent=`${label} ${n}`;sum.append(s)}
 $('retryFailed').hidden=state.lastFailed.length===0;updateWorkflow();
}
let previewURL='',previewRecord=null;async function previewResult(rec){try{const b=await apiBlob(`result/${state.lastJobId}/${rec.id}`);if(previewURL)URL.revokeObjectURL(previewURL);previewURL=URL.createObjectURL(b);previewRecord=rec;$('outputPreviewImage').src=previewURL;$('outputPreviewImage').classList.remove('native');$('previewInfo').textContent=`${formatBytes(rec.after)} · ${rec.width||'—'}×${rec.height||'—'} px`;$('previewDialog').showModal()}catch(e){toast(e.message,'error')}}
async function openRecord(rec){try{await api('open','POST',{job:state.lastJobId,index:String(rec.id),folder:false})}catch(e){toast(e.message,'error')}}
async function openFolder(){if(!state.jobOutput||!state.lastJobId)return;try{await api('open','POST',{job:state.lastJobId,index:'',folder:true})}catch(e){toast(e.message,'error')}}

function jobPayload(overrideIds=null){const p=toolPrefs(),cropMode=state.mode==='pdf-images'?$('cropMode').value:'none';const manual={};for(const id of (overrideIds||selectedIds()))if(state.plans[id])manual[id]=state.plans[id];const edits={};for(const id of (overrideIds||selectedIds()))if(state.edits[id])edits[id]=state.edits[id];return {mode:state.mode,inputs:overrideIds||selectedIds(),output:$('outputDir').value.trim(),format:$('outputFormat').value,profile:$('profile').value,dpi:Number($('dpi').value)||200,pages:$('pages').value.trim(),paper:$('paper').value,strict:$('strict').checked,makeZip:$('makeZip').checked,consent:$('consent').checked,maxEdge:Number($('maxEdge').value)||0,trimImages:$('trimImages').checked,edits,crop:{mode:cropMode,threshold:250,marginMM:Number($('cropMargin').value),manual},amount:$('sizeAmount').value.trim(),unit:$('sizeUnit').value,password:$('pdfPassword').value};}
async function startJob(overrideIds=null){
 if(!validateSize()||state.busy||state.uploading)return;const chosen=overrideIds||selectedIds();if(!chosen.length)return;if(state.mode==='pdf-compress'&&!$('consent').checked){toast('请先确认扫描重建说明。','error');return}
 const payload=jobPayload(chosen);setBusy(true);renderResults(true);setWorkView('results');await savePrefs();$('statusTitle').textContent='正在处理材料';$('statusDetail').textContent='原始文件不会被覆盖；可以随时停止。';$('jobMeter').hidden=false;$('meterText').textContent='正在准备';$('meterPercent').textContent='0%';$('progressFill').style.width='2%';
 try{const j=await api('jobs','POST',payload);state.job={id:j.id,next:0,fatal:'',canceled:false};state.lastJobId=j.id;$('cancelJob').hidden=false;await pollJob()}catch(e){state.job=null;toast(e.message,'error',7000);$('statusTitle').textContent='没有开始处理';$('statusDetail').textContent=e.message;setBusy(false)}
}
async function pollJob(){let retries=0;while(state.job){try{const data=await api(`jobs/${state.job.id}?after=${state.job.next}`);retries=0;state.job.next=data.next;state.jobOutput=data.output||state.jobOutput;for(const x of data.events){const ev=x.Event||x;handleEvent(ev)}if(data.done){const finishedId=state.job.id;state.lastJobId=finishedId;const fatal=state.job.fatal,canceled=state.job.canceled;state.job=null;if(!fatal){$('statusTitle').textContent=canceled?'已停止处理':state.results.some(r=>r.status==='failed')?'处理完成，部分材料需检查':'处理完成';$('statusDetail').textContent=state.results.length?`已生成 ${state.results.filter(r=>r.output).length} 个结果，请逐个预览。`:'没有生成可提交文件，请查看失败原因。'}setBusy(false);renderResults();renderQueue(false);if(!canceled&&!fatal&&$('openAfterExport')?.checked&&state.jobOutput&&state.results.some(r=>r.output))setTimeout(openFolder,220);break}await new Promise(r=>setTimeout(r,data.more?60:300))}catch(e){retries++;$('statusTitle').textContent='连接中断，正在重试';$('statusDetail').textContent='保留当前任务，可点击停止；正在重新连接本地引擎。';if(retries===1)toast(e.message,'error');await new Promise(r=>setTimeout(r,Math.min(5000,retries*1000)))}}}
function handleEvent(ev){if(ev.output)state.jobOutput=ev.output;if(ev.kind==='status'&&ev.message){$('meterText').textContent=ev.message;$('statusDetail').textContent=ev.message}if(ev.kind==='progress'){const total=Math.max(1,ev.total||1),pct=Math.min(96,Math.round(((ev.current||0)/total)*100));$('meterText').textContent=ev.message||'处理中';$('meterPercent').textContent=pct+'%';$('progressFill').style.width=Math.max(3,pct)+'%'}if(ev.kind==='result'&&ev.record){state.results.push(ev.record);renderResults()}if(ev.kind==='fatal'){if(state.job)state.job.fatal=ev.message||'处理失败';toast(ev.message||'处理失败','error',8000);$('statusTitle').textContent='处理已中止';$('statusDetail').textContent=ev.message||''}if(ev.kind==='complete'){if(state.job)state.job.canceled=!!ev.canceled;$('progressFill').style.width='100%';$('meterPercent').textContent='100%';$('meterText').textContent=ev.canceled?'已停止':'完成';$('elapsedText').textContent=`用时 ${(ev.elapsed||0).toFixed(1)} 秒`}}
function setBusy(busy){state.busy=busy;$('outputDir').disabled=$('browseFolder').disabled=$('settingsReset').disabled=$('emptyState').disabled=$('loadExample').disabled=$('addMoreFiles').disabled=busy;$('clearQueue').disabled=$('removeSelected').disabled=$('selectAll').disabled=busy;$('sideEditButton').disabled=busy;renderQueue(false);$('exportSettings').disabled=busy;document.querySelectorAll('#toolNav .nav-item').forEach(b=>b.disabled=busy);$('addFiles').disabled=busy;$('addFolder').disabled=busy;$('fileInput').disabled=busy;$('folderInput').disabled=busy;$('cancelJob').hidden=!busy;$('startExport').disabled=busy;if(!busy){$('jobMeter').hidden=true;$('cancelJob').hidden=true;renderResults();updateStart();renderQueue()} }
async function cancelJob(){if(!state.job)return;try{await api('cancel/'+state.job.id,'POST',{})}catch(e){toast(e.message,'error')}finally{$('meterText').textContent='正在停止…'}}

const editor=new MaterialEditor({
 getPDFPlan:id=>state.plans[id],getImageEdit:id=>state.edits[id],imageFiles,settings:()=>({dpi:Number($('dpi').value)||200,cropMode:$('cropMode').value,marginMM:Number($('cropMargin').value)}),password:()=>$('pdfPassword').value,
 commitPDF:(id,p)=>{state.plans[id]=p;$('cropMode').value='manual';toolPrefs().cropMode='manual';updateCropSummary();renderQueue(false);if(state.sidePreview.id===id)renderSidePreview();schedulePrefs()},
 commitImage:(id,e)=>{state.edits[id]=e;renderQueue(false);if(state.sidePreview.id===id)renderSidePreview()},onOpen:()=>document.body.classList.add('dialog-open'),onClose:()=>document.body.classList.remove('dialog-open'),onSaved:()=>{updateWorkflow();refreshAutoPreview()}
});

function editFirstSelected(){if(state.busy)return;const id=selectedIds()[0];if(!id){toast('请先勾选一个材料。');return}const item=state.files.get(id);if(item&&!state.busy)editor.open(item)}
function openPrompt(title,label,description,value=''){return new Promise(resolve=>{$('promptTitle').textContent=title;$('promptLabel').textContent=label;$('promptDescription').textContent=description;$('promptValue').value=value;const d=$('promptDialog');const finish=v=>{d.close();$('promptForm').onsubmit=null;$('promptCancel').onclick=null;resolve(v)};$('promptForm').onsubmit=e=>{e.preventDefault();finish($('promptValue').value.trim())};$('promptCancel').onclick=()=>finish(null);d.addEventListener('cancel',e=>{e.preventDefault();finish(null)},{once:true});d.showModal();$('promptValue').focus()})}
async function saveSizePreset(){if(!validateSize())return;const name=await openPrompt('保存大小预设','预设名称','例如：校招系统 500KB、证书 1MB');if(!name)return;if([...name].length>24){toast('预设名称最多 24 个字符。','error');return}state.prefs.presets ||= [];state.prefs.presets=state.prefs.presets.filter(x=>x.name!==name);state.prefs.presets.push({name,amount:$('sizeAmount').value.trim(),unit:$('sizeUnit').value});if(state.prefs.presets.length>12)state.prefs.presets.shift();renderPresets();await savePrefs();toast('大小预设已保存。')}
function renderPresets(){const s=$('savedPresets');const v=s.value;s.replaceChildren(new Option('我的大小预设',''));for(const p of state.prefs.presets||[])s.add(new Option(`${p.name} · ${p.amount} ${p.unit}`,p.name));s.value=v}
function applyPreset(name){const p=(state.prefs.presets||[]).find(x=>x.name===name);if(!p)return;$('sizeAmount').value=p.amount;$('sizeUnit').value=p.unit;validateSize();schedulePrefs()}

async function browseFolder(){try{const r=await api('folder','POST',{});$('outputDir').value=r.path;state.prefs.output=r.path;schedulePrefs();updateStart()}catch(e){if(!/取消/.test(e.message))toast(e.message,'error')}}
function showHelp(){if(!$('helpDialog').open)$('helpDialog').showModal()}
function toggleTheme(){const dark=document.documentElement.dataset.theme==='dark';document.documentElement.dataset.theme=dark?'light':'dark';state.prefs.theme=dark?'light':'dark';$('themeToggle').title=dark?'切换深色':'切换浅色';schedulePrefs()}
function resetCurrentSettings(){state.prefs.tools[state.mode]={...DEFAULT_TOOL,format:state.mode==='image-compress'?'auto':'jpg',strict:state.mode==='pdf-images'?false:true};loadSettings();schedulePrefs();renderSidePreview();toast('已恢复当前工具的推荐设置。','success')}
async function diagnostics(){try{const d=await api('diagnostics');const blob=new Blob([JSON.stringify(d,null,2)],{type:'application/json'});const u=URL.createObjectURL(blob),a=document.createElement('a');a.href=u;a.download='ApplyKit-diagnostics.json';a.click();setTimeout(()=>URL.revokeObjectURL(u),1000)}catch(e){toast(e.message,'error')}}

async function loadExample(){if(state.busy||state.uploading)return;const kind=MODES[state.mode].kind,name=kind==='pdf'?'crop-demo.pdf':'text-demo.png';try{$('loadExample').disabled=true;const blob=await apiBlob('example/'+name);await addFiles([new File([blob],kind==='pdf'?'示例-页面裁剪.pdf':'示例-图片编辑.png',{type:blob.type})])}catch(e){toast(e.message,'error')}finally{$('loadExample').disabled=false}}
function bind(){
 renderIcons();$('loadExample').onclick=loadExample;document.querySelectorAll('#toolNav .nav-item').forEach(b=>b.onclick=()=>setMode(b.dataset.mode));
 $('addMoreFiles').onclick=()=>$('fileInput').click();$('queueViewTab').onclick=()=>setWorkView('queue');$('resultsViewTab').onclick=()=>setWorkView('results');$('retryPreview').onclick=()=>selectSidePreview(state.sidePreview.id,state.sidePreview.page,true);$('addFiles').onclick=()=>$('fileInput').click();$('addFolder').onclick=()=>$('folderInput').click();$('emptyState').onclick=()=>$('fileInput').click();$('emptyState').onkeydown=e=>{if(e.key==='Enter'||e.key===' '){e.preventDefault();$('fileInput').click()}};$('fileInput').onchange=e=>{addFiles(e.target.files);e.target.value=''};$('folderInput').onchange=e=>{addFiles(e.target.files);e.target.value=''};
 $('selectAll').onchange=e=>{const s=state.selected[state.mode];if(e.target.checked)for(const id of ids())s.add(id);else s.clear();renderQueue(false)};$('editSelected').onclick=editFirstSelected;$('removeSelected').onclick=async()=>{for(const id of [...selectedIds()])await removeFromQueue(id)};$('clearQueue').onclick=clearQueue;
 for(const el of [$('sizeAmount'),$('sizeUnit')])el.addEventListener('input',()=>{validateSize();schedulePrefs()});document.querySelectorAll('#quickSizes button').forEach(b=>b.onclick=()=>{$('sizeAmount').value=b.dataset.amount;$('sizeUnit').value=b.dataset.unit;validateSize();schedulePrefs()});$('savePreset').onclick=saveSizePreset;$('savedPresets').onchange=e=>applyPreset(e.target.value);
 for(const id of ['outputFormat','profile','dpi','paper','cropMode','cropMargin','maxEdge','strict','makeZip','trimImages','pages'])$(id).addEventListener('change',()=>{if(['cropMode','cropMargin','trimImages','dpi'].includes(id)){updateCropSummary();refreshAutoPreview()}schedulePrefs();updateWorkflow();updateStart()});$('consent').onchange=updateStart;$('outputDir').addEventListener('input',()=>{schedulePrefs();updateStart()});$('browseFolder').onclick=browseFolder;$('openCrop').onclick=editFirstSelected;$('openImageEdit').onclick=editFirstSelected;$('settingsReset').onclick=resetCurrentSettings;
 $('startExport').onclick=()=>startJob();$('cancelJob').onclick=cancelJob;$('openFolder').onclick=openFolder;$('openOutputDir').onclick=openFolder;$('retryFailed').onclick=()=>{const unique=[...new Set(state.lastFailed)].filter(id=>ids().includes(id));if(unique.length)startJob(unique)};
 $('previewOriginalTab').onclick=()=>{state.sidePreview.tab='original';renderSidePreview()};$('previewCropTab').onclick=()=>{state.sidePreview.tab='crop';refreshAutoPreview()};$('sidePrevPage').onclick=()=>selectSidePreview(state.sidePreview.id,state.sidePreview.page-1,true);$('sideNextPage').onclick=()=>selectSidePreview(state.sidePreview.id,state.sidePreview.page+1,true);$('sideEditButton').onclick=()=>{const item=state.files.get(state.sidePreview.id);if(item&&!state.busy)editor.open(item)};
 $('helpTop').onclick=$('helpSide').onclick=showHelp;$('aboutSide').onclick=()=>$('aboutDialog').showModal();$('aboutClose').onclick=()=>$('aboutDialog').close();$('settingsSide').onclick=()=>{$('settingsCard').scrollIntoView({behavior:'smooth',block:'start'});$('sizeAmount').focus()};$('helpClose').onclick=()=>$('helpDialog').close();$('themeToggle').onclick=toggleTheme;$('jumpSettings').onclick=$('settingsSide').onclick;$('downloadDiagnostics').onclick=diagnostics;
 const openKey='applykit-open-after-export';try{$('openAfterExport').checked=localStorage.getItem(openKey)!=='0'}catch{};$('openAfterExport').onchange=()=>{try{localStorage.setItem(openKey,$('openAfterExport').checked?'1':'0')}catch{}};
 $('previewClose').onclick=()=>{$('previewDialog').close();if(previewURL){URL.revokeObjectURL(previewURL);previewURL=''}};$('previewDialog').addEventListener('cancel',e=>{e.preventDefault();$('previewClose').click()});$('outputPreviewImage').onclick=e=>e.currentTarget.classList.toggle('native');$('previewOpenExternal').onclick=()=>previewRecord&&openRecord(previewRecord);
 let dragDepth=0;window.addEventListener('dragenter',e=>{if([...e.dataTransfer.types].includes('Files')){dragDepth++;$('dropOverlay').hidden=false}});window.addEventListener('dragleave',()=>{dragDepth=Math.max(0,dragDepth-1);if(!dragDepth)$('dropOverlay').hidden=true});window.addEventListener('dragover',e=>{if([...e.dataTransfer.types].includes('Files'))e.preventDefault()});window.addEventListener('drop',e=>{e.preventDefault();dragDepth=0;$('dropOverlay').hidden=true;addFiles(e.dataTransfer.files)});
 document.addEventListener('keydown',e=>{if(document.querySelector('dialog[open]')||state.busy||state.uploading)return;if((e.ctrlKey||e.metaKey)&&e.key.toLowerCase()==='o'){e.preventDefault();$('fileInput').click()}if((e.ctrlKey||e.metaKey)&&e.key==='Enter'){e.preventDefault();if(!$('startExport').disabled)startJob()}if((e.ctrlKey||e.metaKey)&&e.key.toLowerCase()==='a'&&document.activeElement?.tagName!=='INPUT'){e.preventDefault();for(const id of ids())state.selected[state.mode].add(id);renderQueue(false)}});
 window.addEventListener('beforeunload',()=>{captureSettings();revokeSidePreview()});
}
async function keepLease(){for(;;){try{const res=await fetch('/api/lease',{headers:authHeaders(),cache:'no-store'});if(!res.ok)throw new Error('lease');const rd=res.body?.getReader();if(!rd){await new Promise(r=>setTimeout(r,15000));continue}for(;;){const {done}=await rd.read();if(done)break}}catch{await new Promise(r=>setTimeout(r,1500))}}}
async function init(){
 bind();try{state.config=await api('config');state.prefs=state.config.preferences||{};state.prefs.tools ||= {};state.prefs.presets ||= [];state.prefs.output ||= '';state.prefs.theme ||= 'light';document.documentElement.dataset.theme=state.prefs.theme;$('sideVersion').textContent=`ApplyKit ${state.config.version}`;$('aboutVersion').textContent=`版本 ${state.config.version} · MIT License`;$('helpVersion').textContent=`ApplyKit ${state.config.version} · ${state.config.renderer}`;$('verificationNotice').textContent=state.config.platform==='windows'?'当前运行 Windows 版本；PDF 使用系统 Windows.Data.Pdf。':'当前为开发 / 验证环境；PDF 端到端渲染需在 Windows 验收。';renderPresets();setMode(MODES[state.prefs.mode]?state.prefs.mode:'pdf-images',true);keepLease();
 }catch(e){const ov=document.createElement('div');ov.className='fatal-overlay';const box=document.createElement('div');const h=document.createElement('h2');h.textContent='ApplyKit 会话无法连接';const p=document.createElement('p');p.textContent=e.message+'。请关闭窗口后重新运行 ApplyKit.exe。';box.append(h,p);ov.append(box);document.body.append(ov)}
}
init();
