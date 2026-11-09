package main

const uiHTML = `<!DOCTYPE html>
<html lang="en" data-theme="dark">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>SocksHTTP Dashboard</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&family=JetBrains+Mono:wght@400;500&display=swap" rel="stylesheet">
<script>
  // Set theme ASAP untuk mencegah flash
  (function(){
    var t = localStorage.getItem('sockshttp_theme') || 'dark';
    document.documentElement.setAttribute('data-theme', t);
  })();
</script>
<style>
:root{
  --bg:#080b10;
  --bg-elev:#0d1117;
  --surface:#11161d;
  --surface-hover:#161c25;
  --surface-elev:#1a2129;
  --border:#1f2630;
  --border-hover:#2d3641;
  --text:#e6edf3;
  --text-muted:#7d8590;
  --text-subtle:#4a5260;
  --blue:#58a6ff;
  --blue-hover:#79c0ff;
  --blue-bg:rgba(88,166,255,0.12);
  --green:#3fb950;
  --green-bg:rgba(63,185,80,0.12);
  --red:#f85149;
  --red-bg:rgba(248,81,73,0.12);
  --yellow:#d29922;
  --yellow-bg:rgba(210,153,34,0.12);
  --purple:#a371f7;
  --purple-bg:rgba(163,113,247,0.12);
  --radius:10px;
  --radius-sm:6px;
  --radius-lg:14px;
  --shadow-sm:0 1px 3px rgba(0,0,0,0.4);
  --shadow:0 4px 16px rgba(0,0,0,0.35);
  --shadow-lg:0 12px 32px rgba(0,0,0,0.5);
  --header-bg:rgba(13,17,23,0.85);
  --log-bg:#06090d;
  --t:all 0.18s cubic-bezier(0.4,0,0.2,1);
}
/* ---------- LIGHT THEME (Susu/Cream) ---------- */
:root[data-theme="light"]{
  --bg:#fbf7ee;
  --bg-elev:#f5efe1;
  --surface:#ffffff;
  --surface-hover:#faf6ec;
  --surface-elev:#f1ead9;
  --border:#e3dccb;
  --border-hover:#cdc2aa;
  --text:#2a2418;
  --text-muted:#6b634f;
  --text-subtle:#948b75;
  --blue:#0969da;
  --blue-hover:#0550ae;
  --blue-bg:rgba(9,105,218,0.10);
  --green:#1a7f37;
  --green-bg:rgba(26,127,55,0.10);
  --red:#cf222e;
  --red-bg:rgba(207,34,46,0.10);
  --yellow:#9a6700;
  --yellow-bg:rgba(154,103,0,0.10);
  --purple:#8250df;
  --purple-bg:rgba(130,80,223,0.10);
  --shadow-sm:0 1px 3px rgba(120,100,60,0.08);
  --shadow:0 4px 16px rgba(120,100,60,0.12);
  --shadow-lg:0 12px 32px rgba(120,100,60,0.18);
  --header-bg:rgba(251,247,238,0.85);
  --log-bg:#f7f2e6;
}
*{box-sizing:border-box;margin:0;padding:0}
html{scroll-behavior:smooth}
body{
  font-family:'Inter',-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;
  background:var(--bg);color:var(--text);font-size:14px;
  -webkit-font-smoothing:antialiased;-moz-osx-font-smoothing:grayscale;
  min-height:100vh; overflow-x: hidden;
  transition: background 0.2s ease, color 0.2s ease;
}
body::before{
  content:'';position:fixed;inset:0;pointer-events:none;z-index:0;
  background:
    radial-gradient(ellipse 800px 400px at 20% -10%,rgba(88,166,255,0.06),transparent 60%),
    radial-gradient(ellipse 600px 300px at 80% 0%,rgba(163,113,247,0.04),transparent 60%);
}
:root[data-theme="light"] body::before{
  background:
    radial-gradient(ellipse 800px 400px at 20% -10%,rgba(9,105,218,0.04),transparent 60%),
    radial-gradient(ellipse 600px 300px at 80% 0%,rgba(130,80,223,0.03),transparent 60%);
}
::-webkit-scrollbar{width:6px;height:6px}
::-webkit-scrollbar-track{background:transparent}
::-webkit-scrollbar-thumb{background:var(--border-hover);border-radius:3px}
code,pre,.mono{font-family:'JetBrains Mono',ui-monospace,monospace}

/* ---------- HEADER ---------- */
header{
  position:sticky;top:0;z-index:50;
  background:var(--header-bg);backdrop-filter:blur(16px);-webkit-backdrop-filter:blur(16px);
  border-bottom:1px solid var(--border);
  padding:10px 16px;display:flex;align-items:center;gap:12px;flex-wrap:wrap;
}
.logo{display:flex;align-items:center;gap:8px;font-size:15px;font-weight:700;letter-spacing:-0.3px}
.logo-icon{
  width:26px;height:26px;border-radius:6px;
  background:linear-gradient(135deg,#58a6ff,#a371f7);
  display:grid;place-items:center;font-size:13px;
  box-shadow:0 4px 12px rgba(88,166,255,0.3);
}
.logo-text{
  background:linear-gradient(135deg,var(--text),var(--blue));
  -webkit-background-clip:text;-webkit-text-fill-color:transparent;background-clip:text;
}
.state-badge{
  display:inline-flex;align-items:center;gap:4px;
  padding:3px 8px;border-radius:20px;font-size:10px;font-weight:600;
  text-transform:uppercase;letter-spacing:0.4px;transition:var(--t);
}
.state-dot{width:5px;height:5px;border-radius:50%;background:currentColor;box-shadow:0 0 6px currentColor}
.state-connected{background:var(--green-bg);color:var(--green);border:1px solid var(--green)}
.state-connecting{background:var(--yellow-bg);color:var(--yellow);border:1px solid var(--yellow)}
.state-disconnected{background:var(--red-bg);color:var(--red);border:1px solid var(--red)}
.header-info{color:var(--text-muted);font-size:11px;font-family:'JetBrains Mono',monospace; display:none;}
.header-right{margin-left:auto;display:flex;align-items:center;gap:8px}
.version-tag{
  font-size:10px;color:var(--text-subtle);
  font-family:'JetBrains Mono',monospace;
  padding:2px 6px;background:var(--surface);border:1px solid var(--border);border-radius:4px;
}
.theme-btn{
  background:var(--surface);border:1px solid var(--border);
  color:var(--text);padding:4px 10px;border-radius:6px;
  cursor:pointer;font-size:11px;font-weight:600;font-family:inherit;
  transition:var(--t);display:flex;align-items:center;gap:4px;
}
.theme-btn:hover{background:var(--surface-hover);border-color:var(--border-hover)}

/* ---------- NAV ---------- */
nav{
  position:sticky;top:48px;z-index:49;
  background:var(--header-bg);backdrop-filter:blur(16px);-webkit-backdrop-filter:blur(16px);
  border-bottom:1px solid var(--border);
  padding:4px 16px;display:flex;gap:4px;overflow-x:auto;
}
nav button{
  background:transparent;border:none;color:var(--text-muted);
  padding:6px 12px;cursor:pointer;font-size:12px;font-weight:500;
  border-radius:var(--radius-sm);transition:var(--t);
  display:flex;align-items:center;gap:4px;white-space:nowrap;font-family:inherit;
}
nav button:hover{color:var(--text);background:var(--surface-hover)}
nav button.active{color:var(--text);background:var(--surface-elev);box-shadow:var(--shadow-sm)}

/* ---------- MAIN ---------- */
main{padding:12px;max-width:1600px;margin:0 auto;position:relative;z-index:1}
.tab{display:none;animation:fadeIn 0.2s ease-out}
.tab.active{display:block}
@keyframes fadeIn{from{opacity:0;transform:translateY(2px)}to{opacity:1;transform:translateY(0)}}

/* ---------- STAT CARDS ---------- */
.stat{display:grid;grid-template-columns:repeat(2,1fr);gap:6px;margin-bottom:12px}
.stat-card{
  background:linear-gradient(145deg,var(--surface) 0%,var(--bg-elev) 100%);
  border:1px solid var(--border);border-radius:var(--radius);
  padding:8px 10px;position:relative;overflow:hidden;transition:var(--t);
}
.stat-card::before{
  content:'';position:absolute;top:0;left:0;right:0;height:2px;
  background:linear-gradient(90deg,transparent,var(--card-accent,var(--blue)),transparent);opacity:0.5;
}
.stat-head{display:flex;align-items:center;gap:4px;margin-bottom:2px}
.stat-icon{
  width:20px;height:20px;border-radius:4px;display:grid;place-items:center;font-size:10px;
  background:var(--card-icon-bg,var(--blue-bg));color:var(--card-accent,var(--blue));
}
.stat-label{color:var(--text-muted);font-size:9px;text-transform:uppercase;letter-spacing:0.3px;font-weight:600}
.stat-value{font-size:15px;font-weight:700;color:var(--text);line-height:1.1;letter-spacing:-0.3px}
.stat-value.accent{color:var(--card-accent,var(--blue))}
.stat-value.small{font-size:13px;font-weight:600}
/* SUB lebih jelas (permintaan #1) */
.stat-sub{
  color:var(--card-accent,var(--text-muted));
  opacity:0.85;
  font-size:10.5px;
  margin-top:4px;
  font-family:'JetBrains Mono',monospace;
  font-weight:500;
  letter-spacing:-0.1px;
  line-height:1.3;
}

/* ---------- PROXY GROUPS ---------- */
.groups-section{margin-bottom:12px}
.groups-title{font-size:11px;font-weight:600;color:var(--text-muted);margin-bottom:6px;text-transform:uppercase;letter-spacing:0.5px}
.groups-container{display:flex;flex-direction:row;gap:8px;overflow-x:auto;padding-bottom:4px;scrollbar-width:none}
.groups-container::-webkit-scrollbar{display:none}
.group-card{
  background:var(--bg-elev);border:1px solid var(--border);
  border-radius:var(--radius-sm);padding:6px 8px;min-width:130px;flex-shrink:0;
}
.group-name{font-weight:600;color:var(--text-muted);font-size:10px;margin-bottom:4px;text-transform:uppercase;letter-spacing:0.5px;text-align:center}
.group-buttons{display:flex;gap:2px;justify-content:center;background:var(--surface);padding:2px;border-radius:4px}
.group-btn{
  background:transparent;border:none;color:var(--text-muted);
  padding:3px 6px;border-radius:3px;cursor:pointer;font-size:9px;font-weight:600;font-family:inherit;
}
.group-btn.active{background:var(--blue);color:#fff}
.group-btn.reject.active{background:var(--red);color:#fff}
.group-btn.direct.active{background:var(--green);color:#fff}

/* ---------- CHART ---------- */
.chart-wrap{
  background:linear-gradient(145deg,var(--surface) 0%,var(--bg-elev) 100%);
  border:1px solid var(--border);border-radius:var(--radius-lg);
  padding:10px;margin-bottom:12px;
}
.chart-header{display:flex;align-items:center;justify-content:space-between;margin-bottom:4px}
.chart-title{font-size:11px;font-weight:600;color:var(--text)}
.chart-legend{display:flex;gap:8px;font-size:9px;color:var(--text-muted);font-weight:500}
.chart-legend span{display:flex;align-items:center;gap:2px}
.chart-legend .dot{width:4px;height:4px;border-radius:50%}
#chart{width:100%;height:80px;display:block}
.chart-range{color:var(--text-subtle);font-size:9px;font-family:'JetBrains Mono',monospace;margin-top:2px;text-align:right}

/* ---------- TABLE ---------- */
.tbl-wrap{border:1px solid var(--border);border-radius:var(--radius-lg);background:var(--surface);overflow-x:auto;position:relative}
table{width:100%;border-collapse:separate;border-spacing:0}
th{
  background:var(--bg-elev);padding:8px 10px;text-align:left;
  font-weight:600;color:var(--text-muted);font-size:10px;
  text-transform:uppercase;letter-spacing:0.6px;
  border-bottom:1px solid var(--border);white-space:nowrap;
}
td{
  padding:8px 10px;border-bottom:1px solid var(--border);
  font-size:11px;vertical-align:middle;color:var(--text);background:var(--surface);
}
tbody tr:hover td{background:var(--surface-hover)}
tbody tr.selected td{background:var(--blue-bg)}
tr.closed-row td{opacity:0.55}
.conn-table th.sticky-col, .conn-table td.sticky-col{position:sticky;left:0;z-index:5;border-right:1px solid var(--border)}
.conn-table th.sticky-col{z-index:15}

@media(max-width:768px){
  .hide-mobile{display:none !important}
  table{min-width:100% !important}
  th,td{padding:6px 8px;font-size:10px}
}

/* ---------- TAGS / BADGES ---------- */
.tag{
  display:inline-flex;align-items:center;gap:2px;
  padding:1px 6px;border-radius:3px;font-size:9px;font-weight:600;
  text-transform:uppercase;letter-spacing:0.3px;
}
.tag-TUNNEL{background:var(--blue-bg);color:var(--blue)}
.tag-DIRECT{background:var(--green-bg);color:var(--green)}
.tag-REJECT{background:var(--red-bg);color:var(--red)}

/* ---------- FILTER BAR ---------- */
.filter-bar{
  display:flex;gap:6px;align-items:center;margin-bottom:10px;
  background:var(--surface);border:1px solid var(--border);
  border-radius:var(--radius);padding:6px 8px;flex-wrap:wrap;
}
.filter-bar input[type=text],.filter-bar input[type=number],.filter-bar select{
  background:var(--bg-elev);border:1px solid var(--border);
  border-radius:var(--radius-sm);color:var(--text);
  padding:5px 8px;font-size:11px;min-width:110px;transition:var(--t);font-family:inherit;
}
.filter-bar input:focus,.filter-bar select:focus{
  outline:none;border-color:var(--blue);box-shadow:0 0 0 2px var(--blue-bg);
}
.filter-bar label{display:flex;align-items:center;gap:4px;color:var(--text-muted);font-size:11px;cursor:pointer;user-select:none}
input[type=checkbox]{
  appearance:none;width:14px;height:14px;border-radius:3px;
  border:1.5px solid var(--border-hover);background:var(--bg-elev);
  cursor:pointer;position:relative;transition:var(--t);vertical-align:middle;
}
input[type=checkbox]:checked{background:var(--blue);border-color:var(--blue)}
input[type=checkbox]:checked::after{
  content:'✓';position:absolute;top:50%;left:50%;
  transform:translate(-50%,-50%);color:#fff;font-size:9px;font-weight:700;
}

/* ---------- BUTTONS ---------- */
.btn{
  background:var(--surface-elev);border:1px solid var(--border);
  color:var(--text);padding:5px 10px;border-radius:var(--radius-sm);
  cursor:pointer;font-size:11px;font-weight:500;
  transition:var(--t);white-space:nowrap;display:inline-flex;align-items:center;gap:4px;font-family:inherit;
}
.btn:hover:not(:disabled){background:var(--surface-hover);border-color:var(--border-hover)}
.btn:disabled{opacity:0.4;cursor:not-allowed}
.btn-primary{background:var(--blue);border-color:var(--blue);color:#fff}
.btn-primary:hover:not(:disabled){background:var(--blue-hover);border-color:var(--blue-hover)}
.btn-danger{background:var(--red-bg);border-color:var(--red);color:var(--red)}
.btn-icon{background:transparent;border:none;color:var(--text-muted);cursor:pointer;padding:2px 4px;border-radius:3px;font-size:12px;transition:var(--t)}
.btn-icon:hover{background:var(--surface-hover);color:var(--text)}
.btn-icon.danger:hover{background:var(--red-bg);color:var(--red)}
.btn-icon.edit:hover{background:var(--blue-bg);color:var(--blue)}

/* ---------- SUB TABS ---------- */
.sub-tabs{
  display:inline-flex;gap:2px;margin-bottom:8px;
  background:var(--surface);border:1px solid var(--border);
  border-radius:var(--radius-sm);padding:2px;
}
.sub-tabs button{
  background:transparent;border:none;color:var(--text-muted);
  padding:5px 10px;cursor:pointer;font-size:11px;font-weight:500;
  border-radius:4px;transition:var(--t);font-family:inherit;
}
.sub-tabs button.active{color:var(--text);background:var(--surface-elev);box-shadow:var(--shadow-sm)}

/* ---------- LOGS & CONFIG ---------- */
#logbox{
  background:var(--log-bg);border:1px solid var(--border);
  border-radius:var(--radius-lg);padding:8px 10px;
  height:calc(100vh - 240px);overflow-y:auto;
  font-family:'JetBrains Mono',ui-monospace,monospace;
  font-size:10px;line-height:1.5;white-space:pre-wrap;word-break:break-all;color:var(--text);
}
#configBox{
  background:var(--surface);border:1px solid var(--border);
  border-radius:var(--radius-lg);padding:12px;
  overflow-x:auto;font-family:'JetBrains Mono',monospace;
  font-size:10px;line-height:1.5;color:var(--text);
}

/* ---------- EMPTY / HINT ---------- */
.empty{text-align:center;color:var(--text-subtle);padding:20px 12px;font-style:italic;font-size:11px}
.hint{
  color:var(--text-muted);font-size:11px;
  background:var(--surface);border:1px solid var(--border);
  border-left:3px solid var(--blue);padding:8px 12px;
  border-radius:var(--radius-sm);margin-bottom:10px;line-height:1.5;
}
.hint code{background:var(--surface-elev);padding:1px 4px;border-radius:3px;font-family:'JetBrains Mono',monospace;font-size:10px;color:var(--blue)}

/* ---------- BADGE ---------- */
.badge-on{padding:1px 6px;border-radius:3px;font-size:9px;font-weight:600;background:var(--green-bg);color:var(--green);text-transform:uppercase}
.badge-off{padding:1px 6px;border-radius:3px;font-size:9px;font-weight:600;background:var(--red-bg);color:var(--red);text-transform:uppercase}

/* ---------- TOAST ---------- */
#toastContainer{position:fixed;bottom:16px;right:16px;z-index:9999;display:flex;flex-direction:column;gap:6px;pointer-events:none}
.toast{
  background:var(--surface-elev);border:1px solid var(--border-hover);color:var(--text);
  padding:8px 14px;border-radius:var(--radius-sm);font-size:11px;font-weight:500;
  box-shadow:var(--shadow-lg);transform:translateX(120%);opacity:0;transition:all 0.3s ease;
  max-width:280px;pointer-events:auto;
}
.toast.show{transform:translateX(0);opacity:1}
.toast.success{border-left:3px solid var(--green)}
.toast.error{border-left:3px solid var(--red)}
.toast.warn{border-left:3px solid var(--yellow)}

.spacer{flex:1}
.src-link{color:var(--blue);cursor:pointer;text-decoration:none;font-family:'JetBrains Mono',monospace;font-size:11px}
.speed{color:var(--green);font-size:10px;font-family:'JetBrains Mono',monospace}
.speed-down{color:var(--blue);font-size:10px;font-family:'JetBrains Mono',monospace}
.reason{color:var(--text-muted);font-size:10px;font-family:'JetBrains Mono',monospace}
.bytes{color:var(--text-muted);font-size:10px;font-family:'JetBrains Mono',monospace}

/* Quick Add Row di Limiter */
.quick-add-row td { background: var(--blue-bg); padding: 4px 8px; }
.quick-add-row input, .quick-add-row select { background: var(--bg); border: 1px solid var(--border); color: var(--text); padding: 3px 6px; border-radius: 3px; font-size: 10px; width: 100%; font-family:inherit; }
.quick-add-row input:focus, .quick-add-row select:focus { outline: none; border-color: var(--blue); }

/* Inline edit row */
.edit-row input[type=number]{
  background:var(--bg-elev);border:1px solid var(--border);
  border-radius:3px;color:var(--text);padding:3px 6px;font-size:10px;width:70px;font-family:inherit;
}
.edit-row input[type=number]:focus{outline:none;border-color:var(--blue)}

@media(min-width:769px){
  .stat{grid-template-columns:repeat(4,1fr)}
  .header-info{display:inline-block}
  main{padding:20px}
  .group-card{min-width:160px}
}
</style>
</head>
<body>

<header>
  <div class="logo">
    <div class="logo-icon">⚡</div>
    <span class="logo-text">SocksHTTP</span>
  </div>
  <span id="stateBadge" class="state-badge state-disconnected">
    <span class="state-dot"></span><span id="stateText">disconnected</span>
  </span>
  <span id="serverInfo" class="header-info"></span>
  <div class="header-right">
    <button id="themeBtn" class="theme-btn" onclick="toggleTheme()">☀ Terang</button>
    <span id="versionInfo" class="version-tag">v?</span>
  </div>
</header>

<nav>
  <button class="active" onclick="showTab('dashboard',this)">◉ Dashboard</button>
  <button onclick="showTab('connections',this)">⇄ Connections</button>
  <button onclick="showTab('limiter',this)">⏱ Limiter</button>
  <button onclick="showTab('logs',this)">≡ Logs</button>
  <button onclick="showTab('config',this)">⚙ Config</button>
</nav>

<main>

<!-- DASHBOARD -->
<div id="tab-dashboard" class="tab active">
  <div class="stat" id="statsRow"></div>

  <div class="groups-section">
    <div class="groups-title">Proxy Groups</div>
    <div class="groups-container" id="groupsContainer">
      <div class="empty">Loading...</div>
    </div>
  </div>

  <div class="chart-wrap">
    <div class="chart-header">
      <span class="chart-title">Live Traffic</span>
      <div class="chart-legend">
        <span><span class="dot" style="background:#3fb950"></span>Up</span>
        <span><span class="dot" style="background:#58a6ff"></span>Down</span>
      </div>
    </div>
    <svg id="chart" viewBox="0 0 600 100" preserveAspectRatio="none">
      <defs>
        <linearGradient id="gradUp" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stop-color="#3fb950" stop-opacity="0.3"/>
          <stop offset="100%" stop-color="#3fb950" stop-opacity="0"/>
        </linearGradient>
        <linearGradient id="gradDown" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stop-color="#58a6ff" stop-opacity="0.3"/>
          <stop offset="100%" stop-color="#58a6ff" stop-opacity="0"/>
        </linearGradient>
      </defs>
      <polygon id="areaUp" fill="url(#gradUp)" points=""/>
      <polygon id="areaDown" fill="url(#gradDown)" points=""/>
      <polyline id="chartUp" fill="none" stroke="#3fb950" stroke-width="1.5" points=""/>
      <polyline id="chartDown" fill="none" stroke="#58a6ff" stroke-width="1.5" points=""/>
    </svg>
    <div class="chart-range" id="chartRange"></div>
  </div>

  <div class="groups-title" style="margin-top:12px">Devices</div>
  <div class="tbl-wrap">
    <table>
      <thead><tr>
        <th>Src IP</th>
        <th>Active</th>
        <th class="hide-mobile">Total</th>
        <th class="hide-mobile">Up</th>
        <th class="hide-mobile">Down</th>
      </tr></thead>
      <tbody id="devicesBody"><tr><td colspan="5" class="empty">Loading...</td></tr></tbody>
    </table>
  </div>
</div>

<!-- CONNECTIONS -->
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
  </div>

  <div class="tbl-wrap">
    <table class="conn-table" style="min-width:800px">
      <thead><tr>
        <th class="sticky-col" style="width:36px;text-align:center"><input type="checkbox" id="selAll" onchange="toggleAllConns(this)"></th>
        <th>Time</th>
        <th>Src IP</th>
        <th>Destination</th>
        <th>Rule</th>
        <th>Proxy</th>
        <th>Up</th>
        <th>Down</th>
        <th id="thSpeed">Speed</th>
        <th id="thEnd" style="display:none">End</th>
        <th style="width:36px"></th>
      </tr></thead>
      <tbody id="connBody"><tr><td colspan="11" class="empty">Loading...</td></tr></tbody>
    </table>
  </div>
  <div style="display:flex; gap:6px; margin-top:10px;">
    <button class="btn btn-danger" id="killSelBtn" onclick="killSelected()" disabled>Kill Selected (0)</button>
    <button class="btn btn-danger" id="killFilteredBtn" onclick="killFiltered()">Kill Filtered</button>
    <button class="btn btn-danger" id="killAllBtn" onclick="killAllConns()">Kill All</button>
  </div>
</div>

<!-- LIMITER (no top form anymore) -->
<div id="tab-limiter" class="tab">
  <div class="hint">
    Kelola limit bandwidth <b>per IP</b>. Gunakan baris <b>Quick-Add</b> di bawah tabel untuk menambah, atau klik <b>✎</b> untuk edit langsung di baris.
  </div>
  <div id="limiterInfo" class="hint"></div>

  <div class="tbl-wrap">
    <table>
      <thead><tr>
        <th>IP Address</th>
        <th>⬆ Up</th>
        <th>⬇ Down</th>
        <th>Status</th>
        <th style="width:100px">Action</th>
      </tr></thead>
      <tbody id="limiterBody"></tbody>
    </table>
  </div>
</div>

<!-- LOGS -->
<div id="tab-logs" class="tab">
  <div class="filter-bar">
    <label><input type="checkbox" id="pauseLog" onchange="updatePauseLog()"> Pause</label>
    <input type="text" id="logSearch" placeholder="Search log..." oninput="filterLog()" style="flex:1;min-width:100px">
    <input type="number" id="logMaxLines" placeholder="Max lines" min="100" style="width:90px" title="Batas baris maksimum di layar">
    <input type="number" id="logClearMin" placeholder="Auto-clear (mnt)" min="0" style="width:110px" title="0 = tidak pernah clear otomatis">
    <button class="btn" onclick="document.getElementById('logbox').textContent='';logBuffer=''">Clear</button>
  </div>
  <div id="logbox">Connecting to log stream...</div>
</div>

<!-- CONFIG -->
<div id="tab-config" class="tab">
  <pre id="configBox">Loading...</pre>
</div>

</main>

<div id="toastContainer"></div>

<script>
// ============ THEME ============
function toggleTheme() {
  const cur = document.documentElement.getAttribute('data-theme') || 'dark';
  const next = cur === 'dark' ? 'light' : 'dark';
  document.documentElement.setAttribute('data-theme', next);
  localStorage.setItem('sockshttp_theme', next);
  updateThemeBtn(next);
}
function updateThemeBtn(t) {
  const btn = document.getElementById('themeBtn');
  if (btn) {
    btn.textContent = t === 'dark' ? '☀ Terang' : '🌙 Gelap';
  }
}
function initTheme() {
  const t = localStorage.getItem('sockshttp_theme') || 'dark';
  document.documentElement.setAttribute('data-theme', t);
  updateThemeBtn(t);
}

// ============ utils ============
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
  if(n<1073741824)return (n/1048576).toFixed(1)+" MB";
  return (n/1073741824).toFixed(2)+" GB";
}
function fmtSpeed(bps){
  if(!bps||bps<0)return "0 B/s";
  if(bps<1024)return bps.toFixed(0)+" B/s";
  if(bps<1048576)return (bps/1024).toFixed(1)+" KB/s";
  return (bps/1048576).toFixed(1)+" MB/s";
}
function fmtUptime(sec){
  if(!sec || sec < 0) return '0m';
  if(sec < 60) return sec + 's';
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  if (h > 0) return h + 'h ' + m + 'm';
  return m + 'm';
}
function timeAgo(unixTs){
  if(!unixTs || unixTs <= 0) return 'belum pernah';
  const now = Math.floor(Date.now() / 1000);
  const diff = now - unixTs;
  if (diff < 0) return 'baru saja';
  if (diff < 60) return diff + ' detik lalu';
  const m = Math.floor(diff / 60);
  if (m < 60) return m + ' menit lalu';
  const h = Math.floor(m / 60);
  const rm = m % 60;
  if (h > 24) {
      const d = Math.floor(h / 24);
      return d + ' hari lalu';
  }
  return h + 'j ' + rm + 'm lalu';
}
function toast(msg,type){
  type=type||'info';
  const c=document.getElementById('toastContainer');
  const el=document.createElement('div');
  el.className='toast '+type;
  el.textContent=msg;
  c.appendChild(el);
  requestAnimationFrame(()=>el.classList.add('show'));
  setTimeout(()=>{ el.classList.remove('show'); setTimeout(()=>el.remove(),300); },3000);
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
  if(name==='limiter')loadLimiter();
}

// ============ status ============
let lastStatus = {};
async function loadStatus(){
  try{
    const r=await fetch('/api/status');
    const d=await r.json();
    lastStatus=d;
    const badge=document.getElementById('stateBadge');
    const state=d.state||'disconnected';
    badge.className='state-badge state-'+state;
    document.getElementById('stateText').textContent=state;
    document.getElementById('serverInfo').innerHTML='<span style="color:var(--text);font-weight:500">'+esc(d.server||'')+'</span>';
    document.getElementById('versionInfo').textContent='v'+(d.version||'?');

    const stateColor=state==='connected'?'var(--green)':(state==='connecting'?'var(--yellow)':'var(--red)');
    const reconnectSub = d.reconnects > 0 ? timeAgo(d.last_reconnect) : '';

    document.getElementById('statsRow').innerHTML=
      card('◉','State',esc(state),'accent',stateColor)+
      card('⏱','Uptime',fmtUptime(d.uptime_sec),'')+
      card('⬆','Upload',fmtSpeed(d.speed_up),'accent','var(--green)','Total: '+fmtBytes(d.total_up))+
      card('⬇','Download',fmtSpeed(d.speed_down),'accent','var(--blue)','Total: '+fmtBytes(d.total_down))+
      card('⇄','Active',d.active_conns||0,'')+
      card('✓','Closed',d.closed_conns||0,'')+
      card('↻','Reconnects',d.reconnects||0,'small','var(--purple)', reconnectSub)+
      card('◇','DNS',d.dns_queries||0,'small','var(--purple)','pool: '+(d.dns_pool?d.dns_pool.size:0));
  }catch(e){console.error(e)}
}
function card(icon,label,value,extra,accent,sub){
  let accentVar='';
  if(accent && accent.startsWith('var(--')){
    accentVar=accent.replace('var(--','').replace(')','');
  }
  const style2=accentVar?'style="--card-accent:var(--'+accentVar+');--card-icon-bg:var(--'+accentVar+'-bg)"':'';
  const cls='stat-value'+(extra==='accent'?' accent':(extra==='small'?' small':''));
  return '<div class="stat-card" '+style2+'>'+
    '<div class="stat-head"><div class="stat-icon">'+icon+'</div><div class="stat-label">'+label+'</div></div>'+
    '<div class="'+cls+'">'+value+'</div>'+
    (sub?'<div class="stat-sub">'+sub+'</div>':'')+
  '</div>';
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
      const x=i*step; const y=H-((s[key]/maxVal)*(H-pad*2))-pad;
      return x.toFixed(1)+','+y.toFixed(1);
    }).join(' ');
  }
  function area(key){
    const p=pts(key); if(!p)return '';
    return '0,'+H+' '+p+' '+W+','+H;
  }
  document.getElementById('chartUp').setAttribute('points',pts('up'));
  document.getElementById('chartDown').setAttribute('points',pts('down'));
  document.getElementById('areaUp').setAttribute('points',area('up'));
  document.getElementById('areaDown').setAttribute('points',area('down'));
  const span=samples.length>1 ? (samples.length-1)+'s window' : '';
  document.getElementById('chartRange').textContent=span+'  ·  peak '+fmtSpeed(maxVal);
}

// ============ devices ============
async function loadDevices(){
  try{
    const r=await fetch('/api/devices');
    const d=await r.json();
    const body=document.getElementById('devicesBody');
    if(!d.devices||d.devices.length===0){
      body.innerHTML='<tr><td colspan="5" class="empty">No devices connected</td></tr>';
      return;
    }
    body.innerHTML=d.devices.map(dev=>
      '<tr>'+
        '<td><a class="src-link" onclick="filterBySrc(\''+esc(dev.srcIP)+'\')">'+esc(dev.srcIP)+'</a></td>'+
        '<td>'+dev.active_conns+'</td>'+
        '<td class="bytes hide-mobile">'+dev.total_conns+'</td>'+
        '<td class="bytes hide-mobile">'+fmtBytes(dev.up)+'</td>'+
        '<td class="bytes hide-mobile">'+fmtBytes(dev.down)+'</td>'+
      '</tr>'
    ).join('');
    
    const sel=document.getElementById('filterSrc');
    const cur=sel.value;
    let opts='<option value="">All sources</option>';
    d.devices.forEach(dev=>{opts+='<option value="'+esc(dev.srcIP)+'">'+esc(dev.srcIP)+'</option>';});
    sel.innerHTML=opts;
    sel.value=cur;

    populateIPDropdowns(d.devices);
  }catch(e){}
}
function filterBySrc(ip){
  document.querySelectorAll('nav button').forEach((b,i)=>{
    if(i===1) showTab('connections',b);
  });
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
      c.innerHTML='<div class="empty">No proxy groups configured</div>';
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
      method:'PUT', headers:{'Content-Type':'application/json'}, body:JSON.stringify({name:name})
    });
    if(!r.ok)throw new Error(await r.text());
    toast(group+' → '+name,'success');
    loadProxies();
  }catch(e){toast('Error: '+e.message,'error')}
}

// ============ connections ============
let connState='active';
let rawConns=[]; let filteredConns=[]; let selectedConns=new Set(); let prevSnaps={};

function setConnState(s,btn){
  connState=s;
  document.querySelectorAll('.sub-tabs button').forEach(b=>b.classList.remove('active'));
  btn.classList.add('active');
  document.getElementById('thSpeed').style.display = (s==='active')?'':'none';
  document.getElementById('thEnd').style.display = (s==='closed')?'':'none';
  document.getElementById('killSelBtn').style.display = (s==='active')?'':'none';
  document.getElementById('killFilteredBtn').style.display = (s==='active')?'':'none';
  document.getElementById('killAllBtn').style.display = (s==='active')?'':'none';
  rawConns=[]; prevSnaps={}; loadConnections();
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
    if(dst && !((c.domain||'')+' '+(c.dstHost||'')).toLowerCase().includes(dst))return false;
    if(proxy && c.proxy!==proxy)return false;
    return true;
  });
  renderConnections();
}

function renderConnections(){
  const body=document.getElementById('connBody');
  const isActive=connState==='active';
  if(filteredConns.length===0){
    body.innerHTML='<tr><td colspan="11" class="empty">No connections</td></tr>';
    updateKillBtn(); return;
  }

  const now=performance.now()/1000; const newSnaps={};
  const rows=filteredConns.map(c=>{
    let upSpd=0,downSpd=0;
    if(isActive){
      const prev=prevSnaps[c.id];
      if(prev){
        const dt=Math.max(0.05,now-prev.t);
        upSpd=Math.max(0,(c.up-prev.up)/dt); downSpd=Math.max(0,(c.down-prev.down)/dt);
      }
      newSnaps[c.id]={up:c.up,down:c.down,t:now};
    }
    const t=new Date(c.start); const ts=t.toTimeString().slice(0,8);
    const dst=c.domain||c.dstHost;
    const checked=selectedConns.has(c.id)?'checked':''; const selCls=selectedConns.has(c.id)?'selected':'';
    
    let endCell='';
    if(!isActive){
      const et=c.end?new Date(c.end):null;
      endCell='<td class="reason">'+(et?et.toTimeString().slice(0,8):'-')+' ('+esc(c.reason||'')+')</td>';
    }
    let speedCell='';
    if(isActive){
      speedCell='<td><span class="speed">'+fmtSpeed(upSpd)+'</span><br><span class="speed-down">'+fmtSpeed(downSpd)+'</span></td>';
    }
    return '<tr class="'+selCls+(isActive?'':' closed-row')+'">'+
      '<td class="sticky-col" style="text-align:center">'+(isActive?'<input type="checkbox" '+checked+' onchange="toggleConn(\''+esc(c.id)+'\',this)">':'')+'</td>'+
      '<td class="mono">'+ts+'</td>'+
      '<td class="mono">'+esc(c.srcIP)+'</td>'+
      '<td class="mono" title="'+esc(dst)+':'+c.dstPort+'">'+esc(dst)+':'+c.dstPort+'</td>'+
      '<td class="bytes">'+esc(c.rule)+'</td>'+
      '<td><span class="tag tag-'+esc(c.proxy)+'">'+esc(c.proxy)+'</span></td>'+
      '<td class="bytes">'+fmtBytes(c.up)+'</td>'+
      '<td class="bytes">'+fmtBytes(c.down)+'</td>'+
      speedCell+ endCell+
      '<td style="text-align:center">'+(isActive?'<button class="btn-icon danger" onclick="killConn(\''+esc(c.id)+'\')">✕</button>':'')+'</td>'+
    '</tr>';
  }).join('');
  prevSnaps=newSnaps; body.innerHTML=rows;
  const selAll=document.getElementById('selAll');
  if(selAll && isActive){ selAll.checked=filteredConns.length>0 && filteredConns.every(c=>selectedConns.has(c.id)); }
  updateKillBtn();
}

function updateKillBtn(){ const n=selectedConns.size; const b=document.getElementById('killSelBtn'); b.textContent='Kill Selected ('+n+')'; b.disabled=n===0; }
function toggleConn(id,el){ if(el.checked)selectedConns.add(id); else selectedConns.delete(id); renderConnections(); }
function toggleAllConns(el){ if(el.checked)filteredConns.forEach(c=>selectedConns.add(c.id)); else filteredConns.forEach(c=>selectedConns.delete(c.id)); renderConnections(); }
function resetConnFilter(){ document.getElementById('filterSrc').value=''; document.getElementById('filterDst').value=''; document.getElementById('filterProxy').value=''; applyConnFilter(); }
async function killConn(id){
  if(!confirm('Kill connection?'))return;
  try{ await fetch('/api/connections/'+encodeURIComponent(id),{method:'DELETE'}); selectedConns.delete(id); toast('Killed','success'); loadConnections(); }
  catch(e){toast('Err: '+e.message,'error')}
}
async function killSelected(){
  const ids=Array.from(selectedConns); if(!ids.length)return; if(!confirm('Kill '+ids.length+' connection(s)?'))return;
  try{ await fetch('/api/connections/kill',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({ids:ids})}); selectedConns.clear(); toast('Killed','success'); loadConnections(); }
  catch(e){toast('Err: '+e.message,'error')}
}
async function killFiltered(){
  const ids=filteredConns.map(c=>c.id); if(!ids.length){toast('No filter','warn');return;} if(!confirm('Kill '+ids.length+' filtered?'))return;
  try{ await fetch('/api/connections/kill',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({ids:ids})}); selectedConns.clear(); toast('Killed','success'); loadConnections(); }
  catch(e){toast('Err: '+e.message,'error')}
}
async function killAllConns(){
  if(!confirm('Kill ALL active connections?'))return;
  try{ await fetch('/api/connections/kill',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({all:true})}); selectedConns.clear(); toast('Killed All','success'); loadConnections(); }
  catch(e){toast('Err: '+e.message,'error')}
}

// ============ limiter ============
let cachedConfig = null;
async function loadConfigData() {
    if (cachedConfig) return cachedConfig;
    try { const r = await fetch('/api/config'); cachedConfig = await r.json(); return cachedConfig; } catch(e) { return null; }
}

async function populateIPDropdowns(activeDevices) {
    const sel = document.getElementById('qaIPSelect');
    if (!sel) return;
    let ips = new Set();
    if (activeDevices) { activeDevices.forEach(d => ips.add(d.srcIP)); }
    const cfg = await loadConfigData();
    if (cfg && cfg.rules) {
        cfg.rules.forEach(rule => {
            const parts = rule.split(',');
            if (parts[0].trim() === 'SRC-IP-CIDR' && parts[1]) {
                const ip = parts[1].split('/')[0].trim(); if (ip) ips.add(ip);
            }
        });
    }
    try { const r = await fetch('/api/limiter'); const d = await r.json(); if (d.rules) d.rules.forEach(x => ips.add(x.ip)); } catch(e) {}
    const sortedIPs = Array.from(ips).sort();
    const cur = sel.value;
    let opts = '<option value="">-- Pilih IP --</option>';
    sortedIPs.forEach(ip => { opts += '<option value="'+ip+'">'+ip+'</option>'; });
    opts += '<option value="custom" style="color:var(--yellow);font-weight:bold">-- custom --</option>';
    sel.innerHTML = opts;
    if (sortedIPs.includes(cur)) sel.value = cur;
}

function handleQaIPSelect(sel) {
    const custom = document.getElementById('qaIPCustom');
    if (sel.value === 'custom') { custom.style.display = 'block'; custom.focus(); }
    else { custom.style.display = 'none'; custom.value = ''; }
}

async function loadLimiter(){
  try{
    const r=await fetch('/api/limiter');
    const d=await r.json();
    document.getElementById('limiterInfo').innerHTML='Status: <b style="color:'+(d.enabled?'var(--green)':'var(--red)')+'">'+(d.enabled?'✅ enabled':'⛔ disabled')+'</b> · File: <code>'+esc(d.file||'(none)')+'</code> · Rules: <b>'+(d.rules?d.rules.length:0)+'</b>';
    
    const body=document.getElementById('limiterBody');
    let html='';
    if(!d.rules||d.rules.length===0){
      html='<tr><td colspan="5" class="empty">Belum ada rule. Tambahkan lewat baris di bawah.</td></tr>';
    } else {
      html = d.rules.map(rule=>{
        const statusCls=rule.enabled?'badge-on':'badge-off'; const statusTxt=rule.enabled?'on':'off';
        return '<tr id="rule-'+esc(rule.ip)+'">'+
          '<td class="mono">'+esc(rule.ip)+'</td>'+
          '<td class="bytes">'+(rule.up_kbps||0)+'</td>'+
          '<td class="bytes">'+(rule.down_kbps||0)+'</td>'+
          '<td><span class="'+statusCls+'">'+statusTxt+'</span></td>'+
          '<td>'+
            '<button class="btn-icon edit" onclick="editRow(\''+esc(rule.ip)+'\','+(rule.up_kbps||0)+','+(rule.down_kbps||0)+','+(rule.enabled?'true':'false')+')" title="Edit">✎</button>'+
            '<button class="btn-icon danger" onclick="deleteLimiterRule(\''+esc(rule.ip)+'\')" title="Delete">✕</button>'+
          '</td>'+
        '</tr>';
      }).join('');
    }
    // Quick add row
    html += '<tr class="quick-add-row">'+
        '<td>'+
          '<select id="qaIPSelect" onchange="handleQaIPSelect(this)"><option value="">-- Pilih IP --</option></select>'+
          '<input type="text" id="qaIPCustom" placeholder="Ketik IP..." style="display:none; margin-top:2px;">'+
        '</td>'+
        '<td><input type="number" id="qaUp" placeholder="Up"></td>'+
        '<td><input type="number" id="qaDown" placeholder="Down"></td>'+
        '<td><label style="display:flex;align-items:center;gap:2px;cursor:pointer"><input type="checkbox" id="qaEnabled" checked style="margin:0"> On</label></td>'+
        '<td><button class="btn btn-primary" style="padding:3px 6px;font-size:10px" onclick="quickAddLimiter()">Add</button></td>'+
      '</tr>';

    body.innerHTML = html;
    loadDevices(); // to populate dropdown
  }catch(e){console.error(e)}
}

// Inline edit — klik ✎ mengubah baris menjadi input
function editRow(ip, up, down, enabled){
  const row = document.getElementById('rule-'+ip);
  if (!row) return;
  row.className = 'edit-row';
  row.innerHTML =
    '<td class="mono">'+esc(ip)+'</td>'+
    '<td><input type="number" id="eUp_'+ip+'" value="'+up+'"></td>'+
    '<td><input type="number" id="eDown_'+ip+'" value="'+down+'"></td>'+
    '<td><label style="display:flex;align-items:center;gap:2px;cursor:pointer"><input type="checkbox" id="eEnabled_'+ip+'" '+(enabled?'checked':'')+' style="margin:0"> On</label></td>'+
    '<td>'+
      '<button class="btn-icon" onclick="saveRow(\''+esc(ip)+'\')" title="Save" style="color:var(--green);font-weight:700">✓</button>'+
      '<button class="btn-icon" onclick="loadLimiter()" title="Cancel">✕</button>'+
    '</td>';
  document.getElementById('eUp_'+ip).focus();
}

async function saveRow(ip){
  const up = parseInt(document.getElementById('eUp_'+ip).value)||0;
  const down = parseInt(document.getElementById('eDown_'+ip).value)||0;
  const enabled = document.getElementById('eEnabled_'+ip).checked;
  try{
    const r=await fetch('/api/limiter',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({ip:ip,up_kbps:up,down_kbps:down,enabled:enabled})});
    if(!r.ok)throw new Error(await r.text());
    toast('Updated: '+ip,'success'); loadLimiter();
  }catch(e){toast('Err: '+e.message,'error')}
}

async function quickAddLimiter(){
  let ip = document.getElementById('qaIPSelect').value;
  if (ip === 'custom') { ip = document.getElementById('qaIPCustom').value.trim(); }
  if(!ip){toast('IP kosong','warn');return;}
  const up=parseInt(document.getElementById('qaUp').value)||0;
  const down=parseInt(document.getElementById('qaDown').value)||0;
  const enabled=document.getElementById('qaEnabled').checked;
  try{
    const r=await fetch('/api/limiter',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({ip:ip,up_kbps:up,down_kbps:down,enabled:enabled})});
    if(!r.ok)throw new Error(await r.text());
    toast('Added: '+ip,'success'); loadLimiter();
  }catch(e){toast('Err: '+e.message,'error')}
}

async function deleteLimiterRule(ip){
  if(!confirm('Hapus rule '+ip+'?'))return;
  try{ await fetch('/api/limiter/'+encodeURIComponent(ip),{method:'DELETE'}); toast('Deleted','success'); loadLimiter(); }
  catch(e){toast('Err: '+e.message,'error')}
}

// ============ logs ============
let logSource=null;
let logPaused=false;
let logBuffer='';
let logMaxLines = parseInt(localStorage.getItem('sockshttp_logMaxLines')) || 2000;
let logClearMin = parseInt(localStorage.getItem('sockshttp_logClearMin')) || 0;
let logClearTimer = null;

function initLogSettings() {
  const el1 = document.getElementById('logMaxLines');
  const el2 = document.getElementById('logClearMin');
  if (el1) el1.value = logMaxLines;
  if (el2) el2.value = logClearMin;
  scheduleLogClear();
}

function applyLogSettings() {
  const m = parseInt(document.getElementById('logMaxLines').value);
  const c = parseInt(document.getElementById('logClearMin').value);
  logMaxLines = (isNaN(m) || m < 100) ? 2000 : m;
  logClearMin = (isNaN(c) || c < 0) ? 0 : c;
  localStorage.setItem('sockshttp_logMaxLines', logMaxLines);
  localStorage.setItem('sockshttp_logClearMin', logClearMin);
  trimLogBuffer();
  scheduleLogClear();
  renderLog();
  toast('Log settings saved (max '+logMaxLines+' lines, clear '+logClearMin+'min)', 'success');
}

function trimLogBuffer() {
  if (!logBuffer) return;
  const lines = logBuffer.split('\n');
  if (lines.length > logMaxLines) {
    logBuffer = lines.slice(-logMaxLines).join('\n');
  }
}

function scheduleLogClear() {
  if (logClearTimer) clearTimeout(logClearTimer);
  if (logClearMin > 0) {
    logClearTimer = setTimeout(() => {
      logBuffer = '';
      renderLog();
      scheduleLogClear();
    }, logClearMin * 60000);
  }
}

function connectLogs(){
  if(logSource){logSource.close()}
  const box=document.getElementById('logbox'); logBuffer=''; box.textContent='';
  fetch('/api/logs').then(r=>{
    const reader=r.body.getReader(); const dec=new TextDecoder();
    function read(){
      reader.read().then(res=>{
        if(res.done){setTimeout(connectLogs,2000);return}
        logBuffer+=dec.decode(res.value);
        if(logMaxLines > 0){
          const lines = logBuffer.split('\n');
          if (lines.length > logMaxLines) {
            logBuffer = lines.slice(-logMaxLines).join('\n');
          }
        }
        if(!logPaused)renderLog(); read();
      });
    } read();
  });
}
function updatePauseLog(){ logPaused=document.getElementById('pauseLog').checked; if(!logPaused)renderLog(); }
function filterLog(){ renderLog(); }
function renderLog(){
  const box=document.getElementById('logbox');
  const q=(document.getElementById('logSearch').value||'').trim();
  let text=logBuffer;
  if(q){ text=text.split('\n').filter(l=>l.toLowerCase().includes(q.toLowerCase())).join('\n'); }
  box.textContent=text; if(!logPaused)box.scrollTop=box.scrollHeight;
}

// ============ config ============
async function loadConfig(){
  try{ const r=await fetch('/api/config'); const d=await r.json(); document.getElementById('configBox').textContent=JSON.stringify(d,null,2); }catch(e){}
}

// ============ bootstrap ============
initTheme();
initLogSettings();
setInterval(loadStatus,1000);
setInterval(loadTraffic,2000);
setInterval(loadDevices,3000);
setInterval(loadProxies,5000);
setInterval(()=>{if(connState==='active' || document.getElementById('tab-connections').classList.contains('active'))loadConnections();},1000);
loadStatus(); loadTraffic(); loadDevices(); loadProxies(); loadLimiter();
</script>
</body>
</html>`