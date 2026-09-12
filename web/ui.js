export const $=id=>document.getElementById(id);
export const clone=v=>structuredClone?structuredClone(v):JSON.parse(JSON.stringify(v));
const hashToken=()=>{
  const raw=location.hash.slice(1), q=new URLSearchParams(raw);
  return q.get('token')||'';
};
let token=hashToken()||sessionStorage.getItem('applykit-token')||'';
if(token){sessionStorage.setItem('applykit-token',token);history.replaceState(null,'',location.pathname+location.search)}
export const authHeaders=()=>token?{Authorization:'Bearer '+token}:{};
async function parseError(res){
  let msg='请求失败 ('+res.status+')';
  try{const j=await res.json();if(j?.error)msg=j.error}catch{}
  const e=new Error(msg);e.status=res.status;throw e;
}
export async function api(path,method='GET',body=null,options={}){
  const headers={...authHeaders(),...(body!==null?{'Content-Type':'application/json'}:{}),...(options.headers||{})};
  const res=await fetch('/api/'+path,{method,headers,body:body===null?undefined:JSON.stringify(body),signal:options.signal,cache:'no-store'});
  if(!res.ok)return parseError(res);
  const ct=res.headers.get('content-type')||'';
  return ct.includes('application/json')?res.json():res.text();
}
export async function apiBlob(path,options={}){
  const res=await fetch('/api/'+path,{headers:{...authHeaders(),...(options.headers||{})},signal:options.signal,cache:'no-store'});
  if(!res.ok)return parseError(res);return res.blob();
}
export async function uploadFile(file,signal){
  const res=await fetch('/api/upload?name='+encodeURIComponent(file.name),{method:'POST',headers:{...authHeaders(),'Content-Type':'application/octet-stream'},body:file,signal,cache:'no-store'});
  if(!res.ok)return parseError(res);return res.json();
}
export function toast(message,type='info',duration=4200){
  const host=$('toasts'); if(!host)return;
  const el=document.createElement('div');el.className='toast '+type;
  const dot=document.createElement('span');dot.className='toast-dot';
  const p=document.createElement('p');p.textContent=message;
  const close=document.createElement('button');close.type='button';close.setAttribute('aria-label','关闭提示');close.textContent='×';close.onclick=()=>el.remove();
  el.append(dot,p,close);host.appendChild(el);requestAnimationFrame(()=>el.classList.add('show'));
  setTimeout(()=>{el.classList.remove('show');setTimeout(()=>el.remove(),220)},duration);
}
const icons={
 play:'<path d="m8 4 12 8-12 8z"/>',
 pdf:'<path d="M6 2.7h7l5 5V21H6z"/><path d="M13 2.7V8h5"/><path d="M8.7 16h1.7c1.7 0 1.7-3 0-3H8.7v5M13 18v-5h3M13 15.5h2.4"/>',
 image:'<rect x="3" y="4" width="18" height="16" rx="2"/><circle cx="8.5" cy="9" r="1.5"/><path d="m4.5 17 4.7-4.5 3.2 3 2.3-2.2 4.8 4.7"/>',
 layers:'<path d="m12 2 9 5-9 5-9-5 9-5Z"/><path d="m3 12 9 5 9-5M3 17l9 5 9-5"/>',
 compress:'<path d="M8 3v5H3M16 3v5h5M8 21v-5H3M16 21v-5h5"/><path d="m3 8 5-5M21 8l-5-5M3 16l5 5M21 16l-5 5"/>',
 shield:'<path d="M12 22s8-3.6 8-10V5l-8-3-8 3v7c0 6.4 8 10 8 10Z"/><path d="m8.5 12 2.2 2.2 4.8-5"/>',
 help:'<circle cx="12" cy="12" r="9"/><path d="M9.6 9a2.7 2.7 0 1 1 4.6 2c-1 .8-2.2 1.2-2.2 2.8M12 17.4h.01"/>',
 sliders:'<path d="M4 6h5M15 6h5M4 12h10M18 12h2M4 18h2M12 18h8"/><circle cx="12" cy="6" r="2"/><circle cx="16" cy="12" r="2"/><circle cx="9" cy="18" r="2"/>',
 moon:'<path d="M20 15.5A8.5 8.5 0 0 1 8.5 4 8.5 8.5 0 1 0 20 15.5Z"/>',
 plus:'<path d="M12 5v14M5 12h14"/>', file:'<path d="M6 2.7h7l5 5V21H6z"/><path d="M13 2.7V8h5M9 12h6M9 16h6"/>',
 check:'<path d="m5 12 4 4L19 6"/>', crop:'<path d="M6 2v14a2 2 0 0 0 2 2h14M2 6h14a2 2 0 0 1 2 2v14"/>',
 trash:'<path d="M4 7h16M9 7V4h6v3M7 7l1 14h8l1-14M10 11v6M14 11v6"/>', grip:'<path d="M9 5h.01M15 5h.01M9 12h.01M15 12h.01M9 19h.01M15 19h.01"/>',
 checklist:'<path d="m4 6 1.5 1.5L8 5M11 6h9M4 12l1.5 1.5L8 11M11 12h9M4 18l1.5 1.5L8 17M11 18h9"/>',
 folder:'<path d="M3 6h7l2 2h9v11H3z"/>', sparkle:'<path d="m12 2 1.2 4.2L17 8l-3.8 1.8L12 14l-1.2-4.2L7 8l3.8-1.8L12 2ZM5 14l.8 2.7L8.5 18l-2.7 1.3L5 22l-.8-2.7L1.5 18l2.7-1.3L5 14ZM19 12l.7 2.2L22 15l-2.3.8L19 18l-.7-2.2L16 15l2.3-.8L19 12Z"/>',
 bookmark:'<path d="M6 3h12v18l-6-4-6 4z"/>', warning:'<path d="M12 3 2.5 20h19L12 3Z"/><path d="M12 9v5M12 17h.01"/>', tune:'<path d="M4 7h8M16 7h4M4 17h4M12 17h8M12 4v6M8 14v6"/>',
 chevron:'<path d="m8 10 4 4 4-4"/>', arrow:'<path d="M5 12h14M14 7l5 5-5 5"/>', upload:'<path d="M12 16V4M7 9l5-5 5 5"/><path d="M4 15v5h16v-5"/>',
 close:'<path d="m6 6 12 12M18 6 6 18"/>', left:'<path d="m15 18-6-6 6-6"/>', right:'<path d="m9 18 6-6-6-6"/>', undo:'<path d="M9 7 4 12l5 5"/><path d="M5 12h8a6 6 0 0 1 6 6"/>',
 refresh:'<path d="M20 7v5h-5M4 17v-5h5"/><path d="M6.1 8.2A7 7 0 0 1 18.8 9M5.2 15A7 7 0 0 0 18 15.8"/>',
 rotateLeft:'<path d="M5 8V3m0 0h5M5 3a9 9 0 1 1-2 10"/>', rotateRight:'<path d="M19 8V3m0 0h-5m5 0a9 9 0 1 0 2 10"/>',
 flipH:'<path d="M12 3v18M9 6 4 12l5 6M15 6l5 6-5 6"/>', flipV:'<path d="M3 12h18M6 9l6-5 6 5M6 15l6 5 6-5"/>',
 minus:'<path d="M5 12h14"/>', external:'<path d="M14 4h6v6M20 4l-9 9"/><path d="M18 13v7H4V6h7"/>', download:'<path d="M12 3v12M7 10l5 5 5-5M5 21h14"/>'
};
export function renderIcons(root=document){
  root.querySelectorAll('[data-icon]').forEach(el=>{
    const name=el.dataset.icon, body=icons[name]||icons.file;
    el.outerHTML=`<svg class="icon" aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">${body}</svg>`;
  });
}
export function formatBytes(n){
  n=Number(n)||0;if(n>=1000000)return (n/1000000).toFixed(n>=10000000?1:2).replace(/\.00$/,'')+' MB';
  if(n>=1000)return (n/1000).toFixed(n>=100000?0:1).replace(/\.0$/,'')+' KB';return n+' B';
}
export function escapeHTML(s){return String(s).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))}
export function setToken(v){token=v||'';if(token)sessionStorage.setItem('applykit-token',token)}
