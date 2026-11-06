package main

const uiHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>SocksHTTP Dashboard</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:#0d1117;color:#c9d1d9;font-size:14px}
header{background:#161b22;border-bottom:1px solid #30363d;padding:14px 20px;display:flex;align-items:center;gap:20px;flex-wrap:wrap}
header h1{font-size:18px;font-weight:600;color:#58a6ff}
.state-badge{padding:3px 10px;border-radius:12px;font-size:11px;font-weight:600;text-transform:uppercase}
.state-connected{background:#1f6feb;color:#fff}
.state-connecting{background:#bf8700;color:#fff}
.state-disconnected{background:#da3633;color:#fff}
nav{background:#161b22;border-bottom:1px solid #30363d;padding:0 20px;display:flex;gap:0}
nav button{background:transparent;border:none;color:#8b949e;padding:12px 20px;cursor:pointer;font-size:14px;border-bottom:2px solid transparent;transition:all .2s}
nav button:hover{color:#c9d1d9}
nav button.active{color:#58a6ff;border-bottom-color:#58a6ff}
main{padding:20px;max-width:1400px;margin:0 auto}
.tab{display:none}
.tab.active{display:block}
.group-card{background:#161b22;border:1px solid #30363d;border-radius:8px;padding:16px;margin-bottom:12px}
.group-name{font-weight:600;color:#58a6ff;margin-bottom:10px;font-size:15px}
.group-buttons{display:flex;gap:8px;flex-wrap:wrap}
.group-btn{background:#21262d;border:1px solid #30363d;color:#c9d1d9;padding:8px 16px;border-radius:6px;cursor:pointer;font-size:13px;font-weight:500;transition:all .15s}
.group-btn:hover{background:#30363d}
.group-btn.active{background:#1f6feb;border-color:#1f6feb;color:#fff}
.group-btn.reject.active{background:#da3633;border-color:#da3633}
.group-btn.direct.active{background:#238636;border-color:#238636}
table{width:100%;border-collapse:collapse;background:#161b22;border:1px solid #30363d;border-radius:8px;overflow:hidden}
th{background:#21262d;padding:10px 12px;text-align:left;font-weight:600;color:#8b949e;font-size:12px;text-transform:uppercase;letter-spacing:.5px}
td{padding:10px 12px;border-top:1px solid #21262d;font-family:ui-monospace,monospace;font-size:12px;vertical-align:middle}
.tag{padding:2px 8px;border-radius:4px;font-size:11px;font-weight:600}
.tag-TUNNEL{background:#1f6feb22;color:#58a6ff}
.tag-DIRECT{background:#23863622;color:#3fb950}
.tag-REJECT{background:#da363322;color:#f85149}
#logbox{background:#0d1117;border:1px solid #30363d;border-radius:8px;padding:12px;height:600px;overflow-y:auto;font-family:ui-monospace,monospace;font-size:12px;line-height:1.5;white-space:pre-wrap;word-break:break-all}
.stat{display:grid;grid-template-columns:repeat(auto-fit,minmax(160px,1fr));gap:12px;margin-bottom:16px}
.stat-card{background:#161b22;border:1px solid #30363d;border-radius:8px;padding:14px}
.stat-label{color:#8b949e;font-size:11px;text-transform:uppercase;letter-spacing:.5px;margin-bottom:6px}
.stat-value{font-size:22px;font-weight:600;color:#58a6ff;line-height:1.1}
.bytes{color:#8b949e;font-size:11px;margin-top:4px;font-family:ui-monospace,monospace}
.empty{text-align:center;color:#8b949e;padding:40px;font-style:italic}
.filter-bar{display:flex;gap:8px;align-items:center;margin-bottom:12px;background:#161b22;border:1px solid #30363d;border-radius:8px;padding:10px;flex-wrap:wrap}
.filter-bar input,.filter-bar select{background:#0d1117;border:1px solid #30363d;border-radius:6px;color:#c9d1d9;padding:7px 10px;font-size:13px;min-width:160px}
.filter-bar input:focus,.filter-bar select:focus{outline:none;border-color:#58a6ff}
.filter-bar label{display:flex;align-items:center;gap:6px;color:#8b949e;font-size:13px;cursor:pointer}
.spacer{flex:1}
.btn{background:#21262d;border:1px solid #30363d;color:#c9d1d9;padding:7px 14px;border-radius:6px;cursor:pointer;font-size:13px;font-weight:500;transition:all .15s;white-space:nowrap}
.btn:hover{background:#30363d}
.btn:disabled{opacity:.4;cursor:not-allowed}
.btn-danger{background:#da363322;border-color:#da3633;color:#f85149}
.btn-danger:hover:not(:disabled){background:#da363344}
.btn-icon{background:transparent;border:none;color:#f85149;cursor:pointer;padding:4px 8px;border-radius:4px;font-size:14px}
.btn-icon:hover{background:#da363322}
tr.selected{background:#1f6feb22}
tbody tr:hover{background:#21262d}
tr.closed-row{opacity:.6}
.th-sel{width:32px;text-align:center}
.sub-tabs{display:flex;gap:0;margin-bottom:12px;border-bottom:1px solid #30363d}
.sub-tabs button{background:transparent;border:none;color:#8b949e;padding:10px 18px;cursor:pointer;font-size:13px;font-weight:500;border-bottom:2px solid transparent}
.sub-tabs button.active{color:#58a6ff;border-bottom-color:#58a6ff}
.chart-wrap{background:#161b22;border:1px solid #30363d;border-radius:8px;padding:10px;margin-bottom:16px}
.chart-wrap svg{width:100%;height:100px;display:block}
.chart-legend{display:flex;gap:16px;font-size:11px;color:#8b949e;margin-top:6px;padding-left:4px}
.section-h{font-size:13px;color:#8b949e;text-transform:uppercase;letter-spacing:.5px;margin:16px 0 8px;font-weight:600}
.src-link{color:#58a6ff;cursor:pointer;text-decoration:none}
.src-link:hover{text-decoration:underline}
.speed{color:#3fb950;font-size:11px}
.speed-down{color:#58a6ff;font-size:11px}
.reason{color:#8b949e;font-size:11px}
</style>
</head>
<body>

<header>
  <h1>⚡ SocksHTTP</h1>
  <span id="stateBadge" class="state-badge state-disconnected">disconnected</span>
  <span id="serverInfo" style="color:#8b949e;font-size:13px"></span>
  <span id="versionInfo" style="color:#8b949e;font-size:11px;margin-left:auto"></span>
</header>

<nav>
  <button class="active" onclick="showTab('dashboard',this)">Dashboard</button>
  <button onclick="showTab('connections',this)">Connections</button>
  <button onclick="showTab('logs',this)">Logs</button>
  <button onclick="showTab('config',this)">Config</button>
</nav>

<main>

<div id="tab-dashboard" class="tab active">
  <div class="stat" id="statsRow"></div>

  <div class="chart-wrap">
    <svg id="chart" viewBox="0 0 600 100" preserveAspectRatio="none">
      <polyline id="chartUp" fill="none" stroke="#3fb950" stroke-width="1.5" points=""/>
      <polyline id="chartDown" fill="none" stroke="#58a6ff" stroke-width="1.5" points=""/>
    </svg>
    <div class="chart-legend">
      <span style="color:#3fb950">■ Upload</span>
      <span style="color:#58a6ff">■ Download</span>
      <span style="margin-left:auto" id="chartRange"></span>
    </div>
  </div>

  <div class="section-h">Devices</div>
  <table>
    <thead><tr><th>Src IP</th><th>Active</th><th>Total</th><th>Up</th><th>Down</th></tr></thead>
    <tbody id="devicesBody"></tbody>
  </table>

  <div class="section-h">Proxy Groups</div>
  <div id="groupsContainer"></div>
</div>

<div id="tab-connections" class="tab">
  <div class="sub-tabs">
    <button class="active" onclick="setConnState('active',this)">Active (<span id="activeCount">0</span>)</button>
    <button onclick="setConnState('closed',this)">Closed (<span id="closedCount">0</span>)</button>
  </div>

  <div class="filter-bar">
    <select id="filterSrc" onchange="applyConnFilter()"><option value="">All sources</option></select>
    <input type="text" id="filterDst" placeholder="Filter destination..." oninput="applyConnFilter()">
    <select id="filterProxy" onchange="applyConnFilter()">
      <option value="">All proxies</option>
      <option value="TUNNEL">TUNNEL</option>
      <option value="DIRECT">DIRECT</option>
      <option value="REJECT">REJECT</option>
    </select>
    <button class="btn" onclick="resetConnFilter()">Reset</button>
    <span class="spacer"></span>
    <button class="btn btn-danger" id="killSelBtn" onclick="killSelected()" disabled>Kill Selected (0)</button>
    <button class="btn btn-danger" id="killFilteredBtn" onclick="killFiltered()">Kill Filtered</button>
    <button class="btn btn-danger" id="killAllBtn" onclick="killAllConns()">Kill All</button>
  </div>

  <table>
    <thead><tr>
      <th class="th-sel"><input type="checkbox" id="selAll" onchange="toggleAllConns(this)"></th>
      <th>Time</th>
      <th>Src IP</th>
      <th>Destination</th>
      <th>Rule</th>
      <th>Proxy</th>
      <th>Up</th>
      <th>Down</th>
      <th id="thSpeed">Speed</th>
      <th id="thEnd" style="display:none">End</th>
      <th></th>
    </tr></thead>
    <tbody id="connBody"><tr><td colspan="11" class="empty">Loading...</td></tr></tbody>
  </table>
</div>

<div id="tab-logs" class="tab">
  <div class="filter-bar">
    <label><input type="checkbox" id="pauseLog" onchange="updatePauseLog()"> Pause autoscroll</label>
    <input type="text" id="logSearch" placeholder="Search log (case-insensitive)..." oninput="filterLog()">
  </div>
  <div id="logbox">Connecting to log stream...</div>
</div>

<div id="tab-config" class="tab">
  <pre id="configBox" style="background:#161b22;border:1px solid #30363d;border-radius:8px;padding:16px;overflow-x:auto">Loading...</pre>
</div>

</main>

<script>
// ============ util ============
function esc(s){
  if(s==null)return '';
  return String(s).replace(/[&<>"']/g,function(c){
    return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c];
  });
}
function fmtBytes(n){
  if(!n||n<0)return "0 B";
  if(n<1024)return n+" B";
  if(n<1048576)return (n/1024).toFixed(1)+" KB";
  if(n<1073741824)return (n/1048576).toFixed(2)+" MB";
  if(n<1099511627776)return (n/1073741824).toFixed(2)+" GB";
  return (n/1099511627776).toFixed(2)+" TB";
}
function fmtSpeed(bps){
  if(!bps||bps<0)return "0 B/s";
  if(bps<1024)return bps.toFixed(0)+" B/s";
  if(bps<1048576)return (bps/1024).toFixed(1)+" KB/s";
  if(bps<1073741824)return (bps/1048576).toFixed(2)+" MB/s";
  return (bps/1073741824).toFixed(2)+" GB/s";
}

// ============ tab switching ============
function showTab(name,btn){
  document.querySelectorAll('.tab').forEach(t=>t.classList.remove('active'));
  document.querySelectorAll('nav button').forEach(b=>b.classList.remove('active'));
  document.getElementById('tab-'+name).classList.add('active');
  btn.classList.add('active');
  if(name==='logs')connectLogs();
  if(name==='config')loadConfig();
  if(name==='connections')loadConnections();
}

// ============ status ============
let lastStatus = {};
async function loadStatus(){
  try{
    const r=await fetch('/api/status');
    const d=await r.json();
    lastStatus=d;
    const badge=document.getElementById('stateBadge');
    badge.textContent=d.state;
    badge.className='state-badge state-'+d.state;
    document.getElementById('serverInfo').textContent=d.server||'';
    document.getElementById('versionInfo').textContent='v'+(d.version||'?');
    const stateColor=d.state==='connected'?'#3fb950':'#f85149';
    document.getElementById('statsRow').innerHTML=
      '<div class="stat-card"><div class="stat-label">State</div><div class="stat-value" style="color:'+stateColor+';font-size:16px">'+esc(d.state)+'</div></div>'+
      '<div class="stat-card"><div class="stat-label">Uptime</div><div class="stat-value">'+Math.floor(d.uptime_sec/60)+'m</div></div>'+
      '<div class="stat-card"><div class="stat-label">⬆ Upload</div><div class="stat-value" style="color:#3fb950;font-size:16px">'+fmtSpeed(d.speed_up)+'</div><div class="bytes">Total: '+fmtBytes(d.total_up)+'</div></div>'+
      '<div class="stat-card"><div class="stat-label">⬇ Download</div><div class="stat-value" style="color:#58a6ff;font-size:16px">'+fmtSpeed(d.speed_down)+'</div><div class="bytes">Total: '+fmtBytes(d.total_down)+'</div></div>'+
      '<div class="stat-card"><div class="stat-label">Active</div><div class="stat-value">'+d.active_conns+'</div></div>'+
      '<div class="stat-card"><div class="stat-label">Closed</div><div class="stat-value">'+d.closed_conns+'</div></div>'+
      '<div class="stat-card"><div class="stat-label">Reconnects</div><div class="stat-value">'+d.reconnects+'</div></div>'+
      '<div class="stat-card"><div class="stat-label">DNS</div><div class="stat-value" style="font-size:15px">'+d.dns_queries+'</div><div class="bytes">pool '+(d.dns_pool?d.dns_pool.size:0)+', hit '+(d.dns_pool?d.dns_pool.hits:0)+'</div></div>';
  }catch(e){console.error(e)}
}

// ============ traffic chart ============
async function loadTraffic(){
  try{
    const r=await fetch('/api/traffic');
    const d=await r.json();
    drawChart(d.samples||[]);
  }catch(e){}
}
function drawChart(samples){
  const W=600,H=100,pad=4;
  if(samples.length===0)return;
  let maxVal=1;
  samples.forEach(s=>{if(s.up>maxVal)maxVal=s.up;if(s.down>maxVal)maxVal=s.down;});
  const step=W/Math.max(1,samples.length-1);
  function pts(key){
    return samples.map((s,i)=>{
      const x=i*step;
      const y=H-((s[key]/maxVal)*(H-pad*2))-pad;
      return x.toFixed(1)+','+y.toFixed(1);
    }).join(' ');
  }
  document.getElementById('chartUp').setAttribute('points',pts('up'));
  document.getElementById('chartDown').setAttribute('points',pts('down'));
  const span=samples.length>1 ? (samples.length-1)+'s window' : '';
  document.getElementById('chartRange').textContent=span+' · max '+fmtSpeed(maxVal);
}

// ============ devices ============
async function loadDevices(){
  try{
    const r=await fetch('/api/devices');
    const d=await r.json();
    const body=document.getElementById('devicesBody');
    if(!d.devices||d.devices.length===0){
      body.innerHTML='<tr><td colspan="5" class="empty">No devices</td></tr>';
      return;
    }
    body.innerHTML=d.devices.map(dev=>
      '<tr>'+
        '<td><a class="src-link" onclick="filterBySrc(\''+esc(dev.srcIP)+'\')">'+esc(dev.srcIP)+'</a></td>'+
        '<td>'+dev.active_conns+'</td>'+
        '<td style="color:#8b949e">'+dev.total_conns+'</td>'+
        '<td class="bytes">'+fmtBytes(dev.up)+'</td>'+
        '<td class="bytes">'+fmtBytes(dev.down)+'</td>'+
      '</tr>'
    ).join('');
    // update src dropdown
    const sel=document.getElementById('filterSrc');
    const cur=sel.value;
    let opts='<option value="">All sources</option>';
    d.devices.forEach(dev=>{opts+='<option value="'+esc(dev.srcIP)+'">'+esc(dev.srcIP)+'</option>';});
    sel.innerHTML=opts;
    sel.value=cur;
  }catch(e){}
}
function filterBySrc(ip){
  showTab('connections', document.querySelectorAll('nav button')[1]);
  document.getElementById('filterSrc').value=ip;
  applyConnFilter();
}

// ============ proxies ============
async function loadProxies(){
  try{
    const r=await fetch('/api/proxies');
    const d=await r.json();
    const c=document.getElementById('groupsContainer');
    if(!d.proxies||d.proxies.length===0){
      c.innerHTML='<div class="empty">No proxy groups</div>';
      return;
    }
    c.innerHTML=d.proxies.map(g=>{
      const btns=g.all.map(p=>{
        const cls='group-btn '+p.toLowerCase().replace(/_/g,'-')+(g.now===p?' active':'');
        return '<button class="'+cls+'" onclick="switchProxy(\''+esc(g.name)+'\',\''+esc(p)+'\')">'+esc(p)+'</button>';
      }).join('');
      return '<div class="group-card"><div class="group-name">'+esc(g.name)+'</div><div class="group-buttons">'+btns+'</div></div>';
    }).join('');
  }catch(e){}
}
async function switchProxy(group,name){
  try{
    const r=await fetch('/api/proxies/'+encodeURIComponent(group),{
      method:'PUT',
      headers:{'Content-Type':'application/json'},
      body:JSON.stringify({name:name})
    });
    if(!r.ok)throw new Error(await r.text());
    loadProxies();
  }catch(e){alert('Error: '+e.message)}
}

// ============ connections ============
let connState='active';
let rawConns=[];      // dari API (state-specific)
let filteredConns=[];
let selectedConns=new Set();
let prevSnaps={};

function setConnState(s,btn){
  connState=s;
  document.querySelectorAll('.sub-tabs button').forEach(b=>b.classList.remove('active'));
  btn.classList.add('active');
  document.getElementById('thSpeed').style.display = (s==='active')?'':'none';
  document.getElementById('thEnd').style.display = (s==='closed')?'':'none';
  document.getElementById('killSelBtn').style.display = (s==='active')?'':'none';
  document.getElementById('killFilteredBtn').style.display = (s==='active')?'':'none';
  document.getElementById('killAllBtn').style.display = (s==='active')?'':'none';
  rawConns=[];
  prevSnaps={};
  loadConnections();
}

async function loadConnections(){
  try{
    const r=await fetch('/api/connections?state='+connState);
    const d=await r.json();
    rawConns=d.connections||[];
    applyConnFilter();
    if(lastStatus.active_conns!=null){
      document.getElementById('activeCount').textContent=lastStatus.active_conns;
      document.getElementById('closedCount').textContent=lastStatus.closed_conns||0;
    }
  }catch(e){}
}

function applyConnFilter(){
  const src=(document.getElementById('filterSrc').value||'').toLowerCase();
  const dst=(document.getElementById('filterDst').value||'').toLowerCase().trim();
  const proxy=document.getElementById('filterProxy').value;
  filteredConns=rawConns.filter(c=>{
    if(src && (c.srcIP||'').toLowerCase()!==src)return false;
    if(dst){
      const d=((c.domain||'')+' '+(c.dstHost||'')).toLowerCase();
      if(!d.includes(dst))return false;
    }
    if(proxy && c.proxy!==proxy)return false;
    return true;
  });
  renderConnections();
}

function renderConnections(){
  const body=document.getElementById('connBody');
  const isActive=connState==='active';

  if(filteredConns.length===0){
    body.innerHTML='<tr><td colspan="11" class="empty">No '+(isActive?'active':'closed')+' connections</td></tr>';
    updateKillBtn();
    return;
  }

  const now=performance.now()/1000;
  const newSnaps={};
  const rows=filteredConns.map(c=>{
    let upSpd=0,downSpd=0;
    if(isActive){
      const prev=prevSnaps[c.id];
      if(prev){
        const dt=Math.max(0.05,now-prev.t);
        upSpd=Math.max(0,(c.up-prev.up)/dt);
        downSpd=Math.max(0,(c.down-prev.down)/dt);
      }
      newSnaps[c.id]={up:c.up,down:c.down,t:now};
    }
    const t=new Date(c.start);
    const ts=t.toTimeString().slice(0,8);
    const dst=c.domain||c.dstHost;
    const checked=selectedConns.has(c.id)?'checked':'';
    const selCls=selectedConns.has(c.id)?'selected':'';
    let endCell='';
    if(!isActive){
      const et=c.end?new Date(c.end):null;
      endCell='<td class="reason">'+(et?et.toTimeString().slice(0,8):'-')+' ('+esc(c.reason||'')+')<br><span style="color:#8b949e">in '+(c.remain_sec||0)+'s</span></td>';
    }
    let speedCell='';
    if(isActive){
      speedCell='<td><span class="speed">'+fmtSpeed(upSpd)+'</span><br><span class="speed-down">'+fmtSpeed(downSpd)+'</span></td>';
    }
    return '<tr class="'+selCls+(isActive?'':' closed-row')+'">'+
      '<td class="th-sel">'+(isActive?'<input type="checkbox" '+checked+' onchange="toggleConn(\''+esc(c.id)+'\',this)">':'')+'</td>'+
      '<td>'+ts+'</td>'+
      '<td>'+esc(c.srcIP)+'</td>'+
      '<td title="'+esc(dst)+':'+c.dstPort+'">'+esc(dst)+':'+c.dstPort+'</td>'+
      '<td style="color:#8b949e">'+esc(c.rule)+'</td>'+
      '<td><span class="tag tag-'+esc(c.proxy)+'">'+esc(c.proxy)+'</span></td>'+
      '<td class="bytes">'+fmtBytes(c.up)+'</td>'+
      '<td class="bytes">'+fmtBytes(c.down)+'</td>'+
      speedCell+
      endCell+
      '<td>'+(isActive?'<button class="btn-icon" onclick="killConn(\''+esc(c.id)+'\')" title="Kill">✕</button>':'')+'</td>'+
    '</tr>';
  }).join('');
  prevSnaps=newSnaps;
  body.innerHTML=rows;

  const selAll=document.getElementById('selAll');
  if(selAll && isActive){
    selAll.checked=filteredConns.length>0 && filteredConns.every(c=>selectedConns.has(c.id));
  }
  updateKillBtn();
}

function updateKillBtn(){
  const n=selectedConns.size;
  const b=document.getElementById('killSelBtn');
  b.textContent='Kill Selected ('+n+')';
  b.disabled=n===0;
}

function toggleConn(id,el){
  if(el.checked)selectedConns.add(id); else selectedConns.delete(id);
  renderConnections();
}
function toggleAllConns(el){
  if(el.checked)filteredConns.forEach(c=>selectedConns.add(c.id));
  else filteredConns.forEach(c=>selectedConns.delete(c.id));
  renderConnections();
}
function resetConnFilter(){
  document.getElementById('filterSrc').value='';
  document.getElementById('filterDst').value='';
  document.getElementById('filterProxy').value='';
  applyConnFilter();
}

async function killConn(id){
  if(!confirm('Kill connection?'))return;
  try{
    const r=await fetch('/api/connections/'+encodeURIComponent(id),{method:'DELETE'});
    if(!r.ok)throw new Error(await r.text());
    selectedConns.delete(id);
    loadConnections();
  }catch(e){alert('Error: '+e.message)}
}
async function killSelected(){
  const ids=Array.from(selectedConns);
  if(ids.length===0)return;
  if(!confirm('Kill '+ids.length+' connection(s)?'))return;
  try{
    const r=await fetch('/api/connections/kill',{
      method:'POST',headers:{'Content-Type':'application/json'},
      body:JSON.stringify({ids:ids})
    });
    if(!r.ok)throw new Error(await r.text());
    selectedConns.clear();loadConnections();
  }catch(e){alert('Error: '+e.message)}
}
async function killFiltered(){
  const ids=filteredConns.map(c=>c.id);
  if(ids.length===0){alert('No filtered connections');return;}
  if(!confirm('Kill '+ids.length+' filtered connection(s)?'))return;
  try{
    const r=await fetch('/api/connections/kill',{
      method:'POST',headers:{'Content-Type':'application/json'},
      body:JSON.stringify({ids:ids})
    });
    if(!r.ok)throw new Error(await r.text());
    selectedConns.clear();loadConnections();
  }catch(e){alert('Error: '+e.message)}
}
async function killAllConns(){
  if(!confirm('Kill ALL active connections?'))return;
  try{
    const r=await fetch('/api/connections/kill',{
      method:'POST',headers:{'Content-Type':'application/json'},
      body:JSON.stringify({all:true})
    });
    if(!r.ok)throw new Error(await r.text());
    selectedConns.clear();loadConnections();
  }catch(e){alert('Error: '+e.message)}
}

// ============ logs ============
let logSource=null;
let logPaused=false;
let logBuffer='';
function connectLogs(){
  if(logSource){logSource.close()}
  const box=document.getElementById('logbox');
  logBuffer='';
  box.textContent='';
  fetch('/api/logs').then(r=>{
    const reader=r.body.getReader();
    const dec=new TextDecoder();
    function read(){
      reader.read().then(res=>{
        if(res.done){setTimeout(connectLogs,2000);return}
        logBuffer+=dec.decode(res.value);
        if(logBuffer.length>500000)logBuffer=logBuffer.slice(-400000);
        if(!logPaused)renderLog();
        read();
      });
    }
    read();
  });
}
function updatePauseLog(){
  logPaused=document.getElementById('pauseLog').checked;
  if(!logPaused)renderLog();
}
function filterLog(){
  renderLog();
}
function renderLog(){
  const box=document.getElementById('logbox');
  const q=(document.getElementById('logSearch').value||'').trim();
  let text=logBuffer;
  if(q){
    const lines=text.split('\n');
    text=lines.filter(l=>l.toLowerCase().includes(q.toLowerCase())).join('\n');
  }
  box.textContent=text;
  if(!logPaused)box.scrollTop=box.scrollHeight;
}

// ============ config ============
async function loadConfig(){
  try{
    const r=await fetch('/api/config');
    const d=await r.json();
    document.getElementById('configBox').textContent=JSON.stringify(d,null,2);
  }catch(e){}
}

// ============ bootstrap ============
setInterval(loadStatus,1000);
setInterval(loadTraffic,2000);
setInterval(loadDevices,3000);
setInterval(loadProxies,5000);
setInterval(()=>{if(connState==='active' || document.getElementById('tab-connections').classList.contains('active'))loadConnections();},1000);
loadStatus();
loadTraffic();
loadDevices();
loadProxies();
</script>
</body>
</html>`