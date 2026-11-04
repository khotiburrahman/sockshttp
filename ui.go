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
header{background:#161b22;border-bottom:1px solid #30363d;padding:14px 20px;display:flex;align-items:center;gap:20px}
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
td{padding:10px 12px;border-top:1px solid #21262d;font-family:ui-monospace,monospace;font-size:12px}
.tag{padding:2px 8px;border-radius:4px;font-size:11px;font-weight:600}
.tag-TUNNEL{background:#1f6feb22;color:#58a6ff}
.tag-DIRECT{background:#23863622;color:#3fb950}
.tag-REJECT{background:#da363322;color:#f85149}
#logbox{background:#0d1117;border:1px solid #30363d;border-radius:8px;padding:12px;height:600px;overflow-y:auto;font-family:ui-monospace,monospace;font-size:12px;line-height:1.5;white-space:pre-wrap;word-break:break-all}
.stat{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px;margin-bottom:20px}
.stat-card{background:#161b22;border:1px solid #30363d;border-radius:8px;padding:16px}
.stat-label{color:#8b949e;font-size:12px;text-transform:uppercase;letter-spacing:.5px;margin-bottom:6px}
.stat-value{font-size:22px;font-weight:600;color:#58a6ff}
.bytes{color:#8b949e;font-size:11px}
.empty{text-align:center;color:#8b949e;padding:40px;font-style:italic}
</style>
</head>
<body>

<header>
  <h1>⚡ SocksHTTP</h1>
  <span id="stateBadge" class="state-badge state-disconnected">disconnected</span>
  <span style="color:#8b949e;font-size:13px" id="serverInfo"></span>
</header>

<nav>
  <button class="active" onclick="showTab('proxies',this)">Proxies</button>
  <button onclick="showTab('connections',this)">Connections</button>
  <button onclick="showTab('logs',this)">Logs</button>
  <button onclick="showTab('config',this)">Config</button>
</nav>

<main>

<div id="tab-proxies" class="tab active">
  <div class="stat" id="statsRow"></div>
  <div id="groupsContainer"></div>
</div>

<div id="tab-connections" class="tab">
  <table>
    <thead><tr>
      <th>Time</th><th>Src IP</th><th>Destination</th><th>Rule</th><th>Proxy</th><th>Up</th><th>Down</th>
    </tr></thead>
    <tbody id="connBody"><tr><td colspan="7" class="empty">Loading...</td></tr></tbody>
  </table>
</div>

<div id="tab-logs" class="tab">
  <div id="logbox">Connecting to log stream...</div>
</div>

<div id="tab-config" class="tab">
  <pre id="configBox" style="background:#161b22;border:1px solid #30363d;border-radius:8px;padding:16px;overflow-x:auto">Loading...</pre>
</div>

</main>

<script>
function fmtBytes(n){
  if(n<1024)return n+" B";
  if(n<1048576)return (n/1024).toFixed(1)+" KB";
  if(n<1073741824)return (n/1048576).toFixed(1)+" MB";
  return (n/1073741824).toFixed(2)+" GB";
}

function showTab(name,btn){
  document.querySelectorAll('.tab').forEach(t=>t.classList.remove('active'));
  document.querySelectorAll('nav button').forEach(b=>b.classList.remove('active'));
  document.getElementById('tab-'+name).classList.add('active');
  btn.classList.add('active');
  if(name==='logs')connectLogs();
  if(name==='config')loadConfig();
}

async function loadStatus(){
  try{
    const r=await fetch('/api/status');
    const d=await r.json();
    const badge=document.getElementById('stateBadge');
    badge.textContent=d.state;
    badge.className='state-badge state-'+d.state;
    document.getElementById('serverInfo').textContent=d.server||'';
    document.getElementById('statsRow').innerHTML=''+
      '<div class="stat-card"><div class="stat-label">State</div><div class="stat-value" style="color:'+(d.state==='connected'?'#3fb950':'#f85149')+'">'+d.state+'</div></div>'+
      '<div class="stat-card"><div class="stat-label">Uptime</div><div class="stat-value">'+Math.floor(d.uptime_sec/60)+'m</div></div>'+
      '<div class="stat-card"><div class="stat-label">Reconnects</div><div class="stat-value">'+d.reconnects+'</div></div>'+
      '<div class="stat-card"><div class="stat-label">DNS Queries</div><div class="stat-value">'+d.dns_queries+'</div></div>';
  }catch(e){console.error(e)}
}

async function loadProxies(){
  try{
    const r=await fetch('/api/proxies');
    const d=await r.json();
    const container=document.getElementById('groupsContainer');
    if(!d.proxies||d.proxies.length===0){
      container.innerHTML='<div class="empty">No proxy groups</div>';
      return;
    }
    container.innerHTML=d.proxies.map(function(g){
      var btns=g.all.map(function(p){
        var cls='group-btn '+p.toLowerCase().replace(/_/g,'-')+(g.now===p?' active':'');
        return '<button class="'+cls+'" onclick="switchProxy(\''+g.name+'\',\''+p+'\')">'+p+'</button>';
      }).join('');
      return '<div class="group-card"><div class="group-name">'+g.name+'</div><div class="group-buttons">'+btns+'</div></div>';
    }).join('');
  }catch(e){console.error(e)}
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

async function loadConnections(){
  try{
    const r=await fetch('/api/connections');
    const d=await r.json();
    const body=document.getElementById('connBody');
    if(!d.connections||d.connections.length===0){
      body.innerHTML='<tr><td colspan="7" class="empty">No active connections</td></tr>';
      return;
    }
    body.innerHTML=d.connections.map(function(c){
      var t=new Date(c.start);
      var ts=t.toTimeString().slice(0,8);
      var dst=c.domain||c.dstHost;
      return '<tr>'+
        '<td>'+ts+'</td>'+
        '<td>'+c.srcIP+'</td>'+
        '<td>'+dst+':'+c.dstPort+'</td>'+
        '<td style="color:#8b949e">'+c.rule+'</td>'+
        '<td><span class="tag tag-'+c.proxy+'">'+c.proxy+'</span></td>'+
        '<td class="bytes">'+fmtBytes(c.up)+'</td>'+
        '<td class="bytes">'+fmtBytes(c.down)+'</td>'+
        '</tr>';
    }).join('');
  }catch(e){console.error(e)}
}

let logSource=null;
function connectLogs(){
  if(logSource){logSource.close()}
  const box=document.getElementById('logbox');
  box.textContent='';
  fetch('/api/logs').then(function(r){
    const reader=r.body.getReader();
    const dec=new TextDecoder();
    function read(){
      reader.read().then(function(res){
        if(res.done){setTimeout(connectLogs,2000);return}
        box.textContent+=dec.decode(res.value);
        box.scrollTop=box.scrollHeight;
        if(box.textContent.length>200000)box.textContent=box.textContent.slice(-150000);
        read();
      });
    }
    read();
  });
}

async function loadConfig(){
  try{
    const r=await fetch('/api/config');
    const d=await r.json();
    document.getElementById('configBox').textContent=JSON.stringify(d,null,2);
  }catch(e){}
}

setInterval(loadStatus,3000);
setInterval(loadProxies,5000);
setInterval(loadConnections,2000);
loadStatus();
loadProxies();
loadConnections();
</script>
</body>
</html>`