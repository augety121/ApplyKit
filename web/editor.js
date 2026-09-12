import {drawTransformed} from './geometry.mjs';
import {$,api,apiBlob,toast,clone} from './ui.js';
const full=()=>({left:0,top:0,right:1,bottom:1});
const clamp=(v,min=0,max=1)=>Math.max(min,Math.min(max,v));
const cleanDraft=()=>({rotation:0,flipH:false,flipV:false,rect:full(),autoTrim:false,threshold:250,padding:8});

// Single editor for photos, scans and PDF pages. Its geometry is independent
// of CSS pixels, display scaling, preview resolution and final export DPI.
export class MaterialEditor {
 constructor(hooks){
  this.hooks=hooks;this.dialog=$('editorDialog');this.frame=$('editFrame');this.canvas=$('editCanvas');this.stage=$('editorStage');this.box=$('cropBox');this.previewCanvas=$('cropPreviewCanvas');
  this.item=null;this.abort=null;this.page=1;this.history=[];this.zoom=1;this.drag=null;this.ready=false;this.serial=0;this.imageURL='';this.pending={};this.plan={pages:{}};
  for(const id of ['editorClose','editorCancel'])$(id).onclick=()=>this.close();
  this.dialog.addEventListener('cancel',e=>{e.preventDefault();this.close()});
  $('editorSave').onclick=()=>this.save();$('editUndo').onclick=()=>this.undo();$('editReset').onclick=()=>this.reset();
  $('rotateLeft').onclick=()=>this.rotate(-90);$('rotateRight').onclick=()=>this.rotate(90);
  $('flipH').onclick=()=>this.flip('flipH');$('flipV').onclick=()=>this.flip('flipV');
  $('editorAutoCrop').onclick=()=>this.autoCrop();
  $('prevPage').onclick=()=>this.changePage(this.page-1);$('nextPage').onclick=()=>this.changePage(this.page+1);
  $('editorPageNumber').onchange=()=>this.changePage(Number($('editorPageNumber').value));
  $('aspectRatio').onchange=()=>this.changeRatio();
  $('zoomFit').onclick=()=>{this.zoom=1;this.resize()};$('zoomIn').onclick=()=>{this.zoom=Math.min(3,this.zoom+.25);this.resize()};$('zoomOut').onclick=()=>{this.zoom=Math.max(.5,this.zoom-.25);this.resize()};
  this.frame.addEventListener('pointerdown',e=>this.pointerDown(e));this.frame.addEventListener('pointermove',e=>this.pointerMove(e));
  this.frame.addEventListener('pointerup',e=>this.pointerUp(e));this.frame.addEventListener('pointercancel',e=>this.pointerUp(e));
  this.dialog.addEventListener('keydown',e=>this.keyDown(e));
  document.querySelectorAll('[data-coordinate]').forEach(el=>el.addEventListener('change',()=>this.coordinateChange()));
  this.observer=new ResizeObserver(()=>{if(this.ready)this.resize()});this.observer.observe(this.stage);
 }
 async open(item){
  if(this.dialog.open)return;this.item=item;this.page=1;this.serial++;this.pending={};this.history=[];this.zoom=1;this.ready=false;this.drag=null;
  this.plan=clone(this.hooks.getPDFPlan(item.id)||{sha256:item.sha256,pages:{}});this.plan.pages ||= {};
  this.draft=clone(this.hooks.getImageEdit(item.id)||cleanDraft());this.draft.rect ||= full();
  $('editorTitle').textContent=item.kind==='pdf'?'PDF 页面裁剪':'图片裁剪与调整';$('editorFilename').textContent=item.name;
  $('editorPages').hidden=item.kind!=='pdf';$('imageTransformTools').hidden=item.kind==='pdf';$('applyAll').checked=false;
  $('applyAllText').textContent=item.kind==='pdf'?'将当前框应用到本 PDF 全部页':'将此编辑应用到当前工具全部图片（'+this.hooks.imageFiles().length+' 张）';
  $('applyAllLabel').hidden=item.kind!=='pdf'&&this.hooks.imageFiles().length<2;
  $('aspectRatio').value='free';this.dialog.showModal();this.hooks.onOpen?.();
  await this.loadPage(1);
 }
 stash(){if(this.item?.kind==='pdf'&&this.ready){this.pending[String(this.page)]=clone(this.draft.rect)}}
 async loadPage(page){
  this.stash();this.abort?.abort();this.abort=new AbortController();const signal=this.abort.signal;const serial=++this.serial;
  this.ready=false;this.page=page;this.frame.hidden=true;$('editorLoading').hidden=false;$('editorLoading').innerHTML='<span class="spinner"></span><strong>正在准备第 '+page+' 页预览</strong><p>按需读取当前页，不预先渲染整份 PDF。取消即可停止等待。</p>';
  this.setEnabled(false);$('editorPageNumber').value=page;$('editUndo').disabled=true;
  try{
   const p=await api('preview/'+this.item.id,'POST',{page,password:this.item.kind==='pdf'?this.hooks.password():''},{signal});
   const blob=await apiBlob('blob/'+p.key,{signal});if(signal.aborted||serial!==this.serial)return;
   if(this.imageURL)URL.revokeObjectURL(this.imageURL);this.imageURL=URL.createObjectURL(blob);
   const image=new Image();image.src=this.imageURL;await image.decode();if(signal.aborted||serial!==this.serial)return;
   this.source=image;this.meta=p;this.history=[];this.zoom=1;
   if(this.item.kind==='pdf'){
    this.draft=cleanDraft();this.draft.rect=clone(this.pending[String(page)]||this.plan.pages[String(page)]||this.plan.all||full());
    $('editorPageTotal').textContent='/ '+p.total;$('editorPageNumber').max=p.total;
   }
   this.frame.hidden=false;$('editorLoading').hidden=true;this.ready=true;this.setEnabled(true);this.drawTransform();this.render();this.resize();
   $('editorSafety').textContent=this.item.kind==='pdf'?'绿色框仅控制保留范围。最终按 '+this.hooks.settings().dpi+' DPI 重新渲染，不会用预览图充当成品。':'只保存编辑方案，不改写原件。文件体积以实际导出后的字节数为准。';
   const mode=this.hooks.settings().cropMode;
   if(this.item.kind==='pdf'&&(mode==='auto'||mode==='uniform')&&!this.pending[String(page)]&&!this.plan.pages[String(page)]&&!this.plan.all){await this.autoCrop(false);if(mode==='uniform')toast('这里显示当前页的建议框。保存将切换为手动；统一自动模式在正式导出时计算所选页并集。','info',8000)}
  }catch(e){if(e.name==='AbortError'||serial!==this.serial)return;$('editorLoading').innerHTML='';const strong=document.createElement('strong');strong.textContent='预览暂时无法读取';const p=document.createElement('p');p.textContent=e.message;const button=document.createElement('button');button.className='button secondary small';button.textContent='重新加载当前页';button.onclick=()=>this.loadPage(this.page);$('editorLoading').append(strong,p,button);toast(e.message,'error')}
 }
 setEnabled(enabled){for(const id of ['editorSave','editReset','editorAutoCrop','rotateLeft','rotateRight','flipH','flipV','aspectRatio','zoomIn','zoomOut','zoomFit'])$(id).disabled=!enabled;document.querySelectorAll('[data-coordinate]').forEach(el=>el.disabled=!enabled);$('prevPage').disabled=!enabled||this.page<=1;$('nextPage').disabled=!enabled||this.page>=(this.meta?.total||1)}
 async changePage(page){if(!this.ready)return;page=clamp(Math.floor(page)||1,1,this.meta.total);if(page!==this.page)await this.loadPage(page);else $('editorPageNumber').value=page}
 close(){this.abort?.abort();this.serial++;this.ready=false;this.drag=null;if(this.imageURL){URL.revokeObjectURL(this.imageURL);this.imageURL=''};this.canvas.width=this.canvas.height=1;this.previewCanvas.width=this.previewCanvas.height=1;this.source=null;this.dialog.close();this.hooks.onClose?.()}
 push(){if(!this.ready)return;const snap=clone(this.draft);if(!this.history.length||JSON.stringify(snap)!==JSON.stringify(this.history.at(-1)))this.history.push(snap);if(this.history.length>30)this.history.shift();$('editUndo').disabled=!this.history.length}
 undo(){if(!this.ready||!this.history.length)return;this.draft=this.history.pop();$('editUndo').disabled=!this.history.length;this.drawTransform();this.render();this.resize()}
 reset(){if(!this.ready)return;this.push();this.draft=cleanDraft();$('aspectRatio').value='free';this.drawTransform();this.render();this.resize()}
 rotate(delta){if(!this.ready)return;this.push();this.draft.rotation=(this.draft.rotation+delta+360)%360;this.draft.rect=full();this.draft.autoTrim=false;$('aspectRatio').value='free';this.drawTransform();this.render();this.resize();toast('已旋转。裁剪框重置为整图，可重新框选。')}
 flip(key){if(!this.ready)return;this.push();this.draft[key]=!this.draft[key];const r=this.draft.rect;if(key==='flipH')this.draft.rect={...r,left:1-r.right,right:1-r.left};else this.draft.rect={...r,top:1-r.bottom,bottom:1-r.top};this.drawTransform();this.render()}
 drawTransform(){if(!this.source)return;const {width:w,height:h}=this.source;const turn=this.draft.rotation%180!==0;this.canvas.width=turn?h:w;this.canvas.height=turn?w:h;const ctx=this.canvas.getContext('2d');drawTransformed(ctx,this.source,this.draft,this.canvas.width,this.canvas.height)}
 resize(){if(!this.ready)return;const aw=Math.max(60,this.stage.clientWidth-48),ah=Math.max(60,this.stage.clientHeight-48);const scale=Math.min(aw/this.canvas.width,ah/this.canvas.height,1)*this.zoom;this.frame.style.width=Math.max(20,this.canvas.width*scale)+'px';this.frame.style.height=Math.max(20,this.canvas.height*scale)+'px';this.stage.classList.toggle('zoomed',this.zoom>1);$('zoomFit').textContent=this.zoom===1?'适应窗口':Math.round(this.zoom*100)+'%';this.render(false)}
 render(updateFields=true){if(!this.ready)return;const r=this.draft.rect;this.box.style.left=r.left*100+'%';this.box.style.top=r.top*100+'%';this.box.style.width=(r.right-r.left)*100+'%';this.box.style.height=(r.bottom-r.top)*100+'%';
  if(updateFields)document.querySelectorAll('[data-coordinate]').forEach(el=>{if(document.activeElement!==el)el.value=(r[el.dataset.coordinate]*100).toFixed(2).replace(/\.?0+$/,'')||'0'});
  const cw=this.canvas.width,ch=this.canvas.height;const x=Math.floor(r.left*cw),y=Math.floor(r.top*ch),w=Math.max(1,Math.ceil(r.right*cw)-x),h=Math.max(1,Math.ceil(r.bottom*ch)-y);const sc=Math.min(200/w,125/h,1);this.previewCanvas.width=Math.max(1,Math.round(w*sc));this.previewCanvas.height=Math.max(1,Math.round(h*sc));this.previewCanvas.getContext('2d').drawImage(this.canvas,x,y,w,h,0,0,this.previewCanvas.width,this.previewCanvas.height);
  let sw=this.meta.sourceWidth,sh=this.meta.sourceHeight;if(this.draft.rotation%180!==0)[sw,sh]=[sh,sw];const ow=Math.ceil(r.right*sw)-Math.floor(r.left*sw),oh=Math.ceil(r.bottom*sh)-Math.floor(r.top*sh);
  $('dimensionLabel').textContent=this.item.kind==='pdf'?'当前预览范围':'裁剪后原始像素';$('cropDimensions').textContent=ow.toLocaleString()+' × '+oh.toLocaleString();$('sourceDimensions').textContent=(this.item.kind==='pdf'?'预览':'原图')+' '+sw.toLocaleString()+' × '+sh.toLocaleString()+' px';
 }
 async autoCrop(withHistory=true){if(!this.ready)return;const serial=this.serial;const prev=this.draft.rect;const edit={...this.draft,rect:null,autoTrim:false,threshold:250,padding:8};$('editorAutoCrop').disabled=true;
  try{const res=await api('autocrop','POST',{id:this.item.id,previewKey:this.meta.key,edit,marginMM:this.hooks.settings().marginMM??2},{signal:this.abort.signal});if(serial!==this.serial)return;if(withHistory)this.push();this.draft.rect=res.rect;this.draft.autoTrim=false;$('aspectRatio').value='free';this.render();if(withHistory)toast(res.note)}catch(e){if(e.name!=='AbortError'){this.draft.rect=prev;toast(e.message,'error')}}finally{if(serial===this.serial)$('editorAutoCrop').disabled=false}}
 aspect(){const value=$('aspectRatio').value;return value==='free'?null:Number(value)*this.canvas.height/this.canvas.width}
 changeRatio(){if(!this.ready)return;this.push();const ratio=this.aspect();if(!ratio)return;const r=this.draft.rect;const cx=(r.left+r.right)/2,cy=(r.top+r.bottom)/2;let w=r.right-r.left,h=r.bottom-r.top;if(w/h>ratio)w=h*ratio;else h=w/ratio;this.draft.rect={left:cx-w/2,right:cx+w/2,top:cy-h/2,bottom:cy+h/2};this.render()}
 coordinateChange(){if(!this.ready)return;const rect={};for(const el of document.querySelectorAll('[data-coordinate]'))rect[el.dataset.coordinate]=Number(el.value)/100;
  if(Object.values(rect).some(v=>!Number.isFinite(v)||v<0||v>1)||rect.right<=rect.left||rect.bottom<=rect.top){toast('坐标应在 0～100%；右大于左，下大于上。','error');this.render(true);return}
  this.push();this.draft.rect=rect;$('aspectRatio').value='free';this.render(false)
 }
 point(e){const b=this.frame.getBoundingClientRect();return{x:clamp((e.clientX-b.left)/b.width),y:clamp((e.clientY-b.top)/b.height)}}
 pointerDown(e){if(!this.ready||e.button!==0)return;e.preventDefault();this.frame.setPointerCapture(e.pointerId);const p=this.point(e),r=clone(this.draft.rect);const inBox=e.target===this.box||this.box.contains(e.target);const handle=e.target.closest('[data-handle]')?.dataset.handle;this.push();this.drag={id:e.pointerId,start:p,rect:r,mode:handle||(inBox?'move':'new')};if(!inBox){this.draft.rect={left:p.x,top:p.y,right:clamp(p.x+.005),bottom:clamp(p.y+.005)};this.render()}this.box.focus({preventScroll:true})}
 pointerMove(e){if(!this.drag||e.pointerId!==this.drag.id)return;e.preventDefault();const p=this.point(e),d=this.drag;const dx=p.x-d.start.x,dy=p.y-d.start.y;const minW=2/this.canvas.width,minH=2/this.canvas.height;let r={...d.rect};
  if(d.mode==='move'){const w=r.right-r.left,h=r.bottom-r.top;r.left=clamp(r.left+dx,0,1-w);r.top=clamp(r.top+dy,0,1-h);r.right=r.left+w;r.bottom=r.top+h}
  else if(d.mode==='new'){r={left:Math.min(d.start.x,p.x),right:Math.max(d.start.x,p.x),top:Math.min(d.start.y,p.y),bottom:Math.max(d.start.y,p.y)}}
  else {if(d.mode.includes('w'))r.left=clamp(r.left+dx,0,r.right-minW);if(d.mode.includes('e'))r.right=clamp(r.right+dx,r.left+minW,1);if(d.mode.includes('n'))r.top=clamp(r.top+dy,0,r.bottom-minH);if(d.mode.includes('s'))r.bottom=clamp(r.bottom+dy,r.top+minH,1)}
  const ratio=this.aspect();if(ratio&&d.mode!=='move'){
   let w=Math.max(minW,r.right-r.left),h=w/ratio;
   if(d.mode==='n'||d.mode==='s'){h=Math.max(minH,r.bottom-r.top);w=h*ratio}
   if(w>1){w=1;h=w/ratio}if(h>1){h=1;w=h*ratio}
   const west=d.mode.includes('w'),north=d.mode.includes('n');if(west)r.left=r.right-w;else r.right=r.left+w;if(north)r.top=r.bottom-h;else r.bottom=r.top+h;
   if(r.left<0){r.right-=r.left;r.left=0}if(r.right>1){r.left-=r.right-1;r.right=1}if(r.top<0){r.bottom-=r.top;r.top=0}if(r.bottom>1){r.top-=r.bottom-1;r.bottom=1}
  }
  if(r.right-r.left>=minW&&r.bottom-r.top>=minH){this.draft.rect=r;this.draft.autoTrim=false;this.render()}
 }
 pointerUp(e){if(!this.drag||e.pointerId!==this.drag.id)return;if(this.frame.hasPointerCapture(e.pointerId))this.frame.releasePointerCapture(e.pointerId);this.drag=null;this.render()}
 keyDown(e){if(!this.ready)return;const inField=/^(INPUT|SELECT|TEXTAREA)$/.test(document.activeElement?.tagName);if((e.ctrlKey||e.metaKey)&&e.key.toLowerCase()==='z'&&!inField){e.preventDefault();this.undo();return}if(inField||!['ArrowLeft','ArrowRight','ArrowUp','ArrowDown'].includes(e.key))return;e.preventDefault();this.push();const r=this.draft.rect,sw=this.canvas.width,sh=this.canvas.height;const step=e.shiftKey?10:1;let dx=0,dy=0;if(e.key==='ArrowLeft')dx=-step/sw;if(e.key==='ArrowRight')dx=step/sw;if(e.key==='ArrowUp')dy=-step/sh;if(e.key==='ArrowDown')dy=step/sh;dx=clamp(dx,-r.left,1-r.right);dy=clamp(dy,-r.top,1-r.bottom);this.draft.rect={left:r.left+dx,right:r.right+dx,top:r.top+dy,bottom:r.bottom+dy};this.render()}
 save(){if(!this.ready)return;const r=this.draft.rect;if(r.right<=r.left||r.bottom<=r.top){toast('请保留有效的裁剪区域。','error');return}
  const all=$('applyAll').checked;
  if(this.item.kind==='pdf'){
   this.stash();const plan=clone(this.plan);plan.sha256=this.item.sha256;
   if(all){plan.all=clone(r);plan.pages={}}else plan.pages={...plan.pages,...this.pending};
   this.hooks.commitPDF(this.item.id,plan);toast(all?'已将当前框应用到本 PDF 的全部页面。':'已保存页面裁剪方案；未框选的页保留整页。');
  }else{
   const targets=all?this.hooks.imageFiles():[this.item];const edit={...clone(this.draft),autoTrim:false};
   for(const item of targets)this.hooks.commitImage(item.id,{...clone(edit),sha256:item.sha256});
   toast(all?'已将编辑方案应用到 '+targets.length+' 张图片；不同尺寸图片按相同比例裁剪，请逐张检查。':'编辑方案已保存，正式导出时应用到原始像素。');
  }
  this.hooks.onSaved?.();this.close();
 }
}
