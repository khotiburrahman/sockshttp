package main

const uiHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>SocksHTTP Dashboard</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&family=JetBrains+Mono:wght@400;500&display=swap" rel="stylesheet">
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
  --t:all 0.18s cubic-bezier(0.4,0,0.2,1);
}
*{box-sizing:border-box;margin:0;padding:0}
html{scroll-behavior:smooth}
body{
  font-family:'Inter',-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;
  background:var(--bg);color:var(--text);font-size:14px;
  -webkit-font-smoothing:antialiased;-moz-osx-font-smoothing:grayscale;
  min-height:100vh;
}
body::before{
  content:'';position:fixed;inset:0;pointer-events:none;z-index:0;
  background:
    radial-gradient(ellipse 800px 400px at 20% -10%,rgba(88,166,255,0.06),transparent 60%),
    radial-gradient(ellipse 600px 300px at 80% 0%,rgba(163,113,247,0.04),transparent 60%);
}
::-webkit-scrollbar{width:6px;height:6px}
::-webkit-scrollbar-track{background:transparent}
::-webkit-scrollbar-thumb{background:#2d3641;border-radius:3px}
::-webkit-scrollbar-thumb:hover{background:#3d4754}
code,pre,.mono{font-family:'JetBrains Mono',ui-monospace,monospace}

/* ---------- HEADER ---------- */
header{
  position:sticky;top:0;z-index:50;
  background:rgba(13,17,23,0.85);backdrop-filter:blur(16px);-webkit-backdrop-filter:blur(16px);
  border-bottom:1px solid var(--border);
  padding:10px 24px;display:flex;align-items:center;gap:16px;flex-wrap:wrap;
}
.logo{display:flex;align-items:center;gap:10px;font-size:16px;font-weight:700;letter-spacing:-0.3px}
.logo-icon{
  width:28px;height:28px;border-radius:8px;
  background:linear-gradient(135deg,#58a6ff,#a371f7);
  display:grid;place-items:center;font-size:14px;
  box-shadow:0 4px 12px rgba(88,166,255,0.3);
}
.logo-text{
  background:linear-gradient(135deg,#e6edf3,#58a6ff);
  -webkit-background-clip:text;-webkit-text-fill-color:transparent;background-clip:text;
}
.state-badge{
  display:inline-flex;align-items:center;gap:6px;
  padding:4px 10px;border-radius:20px;font-size:11px;font-weight:600;
  text-transform:uppercase;letter-spacing:0.4px;
  transition:var(--t);
}
.state-dot{width:6px;height:6px;border-radius:50%;background:currentColor;box-shadow:0 0 8px currentColor}
.state-connected{background:var(--green-bg);color:var(--green);border:1px solid rgba(63,185,80,0.3)}
.state-connecting{background:var(--yellow-bg);color:var(--yellow);border:1px solid rgba(210,153,34,0.3)}
.state-connecting .state-dot{animation:pulse 1.4s ease-in-out infinite}
.state-disconnected{background:var(--red-bg);color:var(--red);border:1px solid rgba(248,81,73,0.3)}
@keyframes pulse{0%,100%{opacity:1;transform:scale(1)}50%{opacity:0.4;transform:scale(0.85)}}
.header-info{color:var(--text-muted);font-size:12px;font-family:'JetBrains Mono',monospace}
.header-info .hl{color:var(--text);font-weight:500}
.version-tag{
  margin-left:auto;font-size:11px;color:var(--text-subtle);
  font-family:'JetBrains Mono',monospace;
  padding:3px 8px;background:var(--surface);border:1px solid var(--border);border-radius:5px;
}

/* ---------- NAV ---------- */
nav{
  position:sticky;top:53px;z-index:49;
  background:rgba(13,17,23,0.85);backdrop-filter:blur(16px);-webkit-backdrop-filter:blur(16px);
  border-bottom:1px solid var(--border);
  padding:6px 24px;display:flex;gap:4px;
  overflow-x:auto;
}
nav button{
  background:transparent;border:none;color:var(--text-muted);
  padding:6px 14px;cursor:pointer;font-size:12px;font-weight:500;
  border-radius:var(--radius-sm);transition:var(--t);
  display:flex;align-items:center;gap:6px;white-space:nowrap;
  font-family:inherit;
}
nav button:hover{color:var(--text);background:var(--surface-hover)}
nav button.active{color:var(--text);background:var(--surface-elev);box-shadow:var(--shadow-sm)}
nav button .nav-icon{font-size:13px;opacity:0.8}
nav button.active .nav-icon{opacity:1}

/* ---------- MAIN ---------- */
main{padding:16px 24px;max-width:1600px;margin:0 auto;position:relative;z-index:1}
.tab{display:none;animation:fadeIn 0.25s ease-out}
.tab.active{display:block}
@keyframes fadeIn{from{opacity:0;transform:translateY(4px)}to{opacity:1;transform:translateY(0)}}

/* ---------- DASHBOARD LAYOUT ---------- */
.dash-grid{display:grid;grid-template-columns:1fr 340px;gap:16px;margin-bottom:16px}
@media(max-width:1024px){.dash-grid{grid-template-columns:1fr}}

/* ---------- STAT CARDS ---------- */
.stat{display:grid;grid-template-columns:repeat(auto-fit,minmax(140px,1fr));gap:10px;margin-bottom:16px}
.stat-card{
  background:linear-gradient(145deg,var(--surface) 0%,var(--bg-elev) 100%);
  border:1px solid var(--border);border-radius:var(--radius-lg);
  padding:12px 14px;position:relative;overflow:hidden;
  transition:var(--t);
}
.stat-card::before{
  content:'';position:absolute;top:0;left:0;right:0;height:2px;
  background:linear-gradient(90deg,transparent,var(--card-accent,var(--blue)),transparent);
  opacity:0.5;transition:var(--t);
}
.stat-card:hover{border-color:var(--border-hover);transform:translateY(-1px);box-shadow:var(--shadow)}
.stat-head{display:flex;align-items:center;gap:6px;margin-bottom:6px}
.stat-icon{
  width:24px;height:24px;border-radius:6px;
  display:grid;place-items:center;font-size:12px;
  background:var(--card-icon-bg,var(--blue-bg));color:var(--card-accent,var(--blue));
}
.stat-label{color:var(--text-muted);font-size:10px;text-transform:uppercase;letter-spacing:0.5px;font-weight:600}
.stat-value{font-size:18px;font-weight:700;color:var(--text);line-height:1.1;letter-spacing:-0.3px}
.stat-value.accent{color:var(--card-accent,var(--blue))}
.stat-value.small{font-size:14px;font-weight:600}
.stat-sub{color:var(--text-subtle);font-size:10px;margin-top:4px;font-family:'JetBrains Mono',monospace}

/* ---------- CHART ---------- */
.chart-wrap{
  background:linear-gradient(145deg,var(--surface) 0%,var(--bg-elev) 100%);
  border:1px solid var(--border);border-radius:var(--radius-lg);
  padding:12px;margin-bottom:0;
}
.chart-header{display:flex;align-items:center;justify-content:space-between;margin-bottom:8px}
.chart-title{font-size:12px;font-weight:600;color:var(--text)}
.chart-legend{display:flex;gap:12px;font-size:10px;color:var(--text-muted);font-weight:500}
.chart-legend span{display:flex;align-items:center;gap:4px}
.chart-legend .dot{width:6px;height:6px;border-radius:50%}
#chart{width:100%;height:100px;display:block}
.chart-range{color:var(--text-subtle);font-size:10px;font-family:'JetBrains Mono',monospace;margin-top:4px;text-align:right}

/* ---------- PROXY GROUPS (RIGHT SIDE) ---------- */
.groups-wrap{
  background:linear-gradient(145deg,var(--surface) 0%,var(--bg-elev) 100%);
  border:1px solid var(--border);border-radius:var(--radius-lg);
  padding:12px;display:flex;flex-direction:column;max-height:400px;overflow:hidden;
}
.groups-header{font-size:12px;font-weight:600;color:var(--text);margin-bottom:8px;display:flex;align-items:center;gap:6px}
.groups-header::before{content:'';width:3px;height:12px;background:var(--blue);border-radius:2px}
.groups-container{overflow-y:auto;flex:1;display:flex;flex-direction:column;gap:8px;padding-right:4px}
.group-card{
  background:var(--bg-elev);border:1px solid var(--border);
  border-radius:var(--radius-sm);padding:8px 10px;transition:var(--t);
}
.group-card:hover{border-color:var(--border-hover)}
.group-name{font-weight:600;color:var(--text-muted);font-size:11px;margin-bottom:6px;text-transform:uppercase;letter-spacing:0.5px}
.group-buttons{display:flex;gap:4px;flex-wrap:wrap}
.group-btn{
  background:var(--surface);border:1px solid var(--border);
  color:var(--text-muted);padding:4px 10px;border-radius:4px;
  cursor:pointer;font-size:11px;font-weight:600;transition:var(--t);font-family:inherit;
}
.group-btn:hover{background:var(--surface-hover);border-color:var(--border-hover);color:var(--text)}
.group-btn.active{background:var(--blue);border-color:var(--blue);color:#fff;box-shadow:0 2px 6px rgba(88,166,255,0.2)}
.group-btn.reject.active{background:var(--red);border-color:var(--red);box-shadow:0 2px 6px rgba(248,81,73,0.2)}
.group-btn.direct.active{background:var(--green);border-color:var(--green);box-shadow:0 2px 6px rgba(63,185,80,0.2)}

/* ---------- TABLE WRAP & STICKY ---------- */
.tbl-wrap{
  border:1px solid var(--border);border-radius:var(--radius-lg);
  background:var(--surface);overflow-x:auto;position:relative;
}
table{width:100%;border-collapse:separate;border-spacing:0;min-width:800px}
th{
  background:var(--bg-elev);padding:10px 12px;text-align:left;
  font-weight:600;color:var(--text-muted);font-size:11px;
  text-transform:uppercase;letter-spacing:0.6px;
  border-bottom:1px solid var(--border);white-space:nowrap;
  position:sticky;top:0;z-index:10;
}
td{
  padding:10px 12px;border-bottom:1px solid var(--border);
  font-size:12px;vertical-align:middle;color:var(--text);
  background:var(--surface); /* Wajib untuk sticky col */
}
tbody tr{transition:background 0.1s}
tbody tr:hover td{background:var(--surface-hover)}
tbody tr.selected td{background:rgba(88,166,255,0.06)}
tr.closed-row td{opacity:0.55}

/* Sticky First Column */
.sticky-col{position:sticky;left:0;z-index:5;border-right:1px solid var(--border)}
th.sticky-col{z-index:15}

/* ---------- TAGS / BADGES ---------- */
.tag{
  display:inline-flex;align-items:center;gap:4px;
  padding:2px 8px;border-radius:4px;font-size:10px;font-weight:600;
  text-transform:uppercase;letter-spacing:0.3px;
}
.tag-TUNNEL{background:var(--blue-bg);color:var(--blue)}
.tag-DIRECT{background:var(--green-bg);color:var(--green)}
.tag-REJECT{background:var(--red-bg);color:var(--red)}

/* ---------- FILTER BAR ---------- */
.filter-bar{
  display:flex;gap:8px;align-items:center;margin-bottom:12px;
  background:var(--surface);border:1px solid var(--border);
  border-radius:var(--radius-lg);padding:8px 12px;flex-wrap:wrap;
}
.filter-bar input[type=text],.filter-bar input[type=number],.filter-bar select{
  background:var(--bg-elev);border:1px solid var(--border);
  border-radius:var(--radius-sm);color:var(--text);
  padding:6px 10px;font-size:12px;min-width:140px;
  transition:var(--t);font-family:inherit;
}
.filter-bar input:focus,.filter-bar select:focus{
  outline:none;border-color:var(--blue);
  box-shadow:0 0 0 2px rgba(88,166,255,0.15);
}
.filter-bar label{
  display:flex;align-items:center;gap:4px;color:var(--text-muted);
  font-size:12px;cursor:pointer;user-select:none;
}
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
  color:var(--text);padding:6px 12px;border-radius:var(--radius-sm);
  cursor:pointer;font-size:12px;font-weight:500;
  transition:var(--t);white-space:nowrap;
  display:inline-flex;align-items:center;gap:4px;font-family:inherit;
}
.btn:hover:not(:disabled){background:var(--surface-hover);border-color:var(--border-hover)}
.btn:disabled{opacity:0.4;cursor:not-allowed}
.btn-primary{background:var(--blue);border-color:var(--blue);color:#fff}
.btn-primary:hover:not(:disabled){background:var(--blue-hover);border-color:var(--blue-hover)}
.btn-danger{background:var(--red-bg);border-color:rgba(248,81,73,0.4);color:var(--red)}
.btn-danger:hover:not(:disabled){background:rgba(248,81,73,0.2);border-color:var(--red)}
.btn-icon{
  background:transparent;border:none;color:var(--text-muted);
  cursor:pointer;padding:4px 6px;border-radius:4px;font-size:13px;
  transition:var(--t);
}
.btn-icon:hover{background:var(--surface-hover);color:var(--text)}
.btn-icon.danger:hover{background:var(--red-bg);color:var(--red)}
.btn-icon.edit:hover{background:var(--blue-bg);color:var(--blue)}

/* ---------- SUB TABS ---------- */
.sub-tabs{
  display:inline-flex;gap:2px;margin-bottom:12px;
  background:var(--surface);border:1px solid var(--border);
  border-radius:var(--radius);padding:2px;
}
.sub-tabs button{
  background:transparent;border:none;color:var(--text-muted);
  padding:6px 14px;cursor:pointer;font-size:12px;font-weight:500;
  border-radius:6px;transition:var(--t);font-family:inherit;
}
.sub-tabs button:hover{color:var(--text)}
.sub-tabs button.active{color:var(--text);background:var(--surface-elev);box-shadow:var(--shadow-sm)}

/* ---------- LOGS & CONFIG ---------- */
#logbox{
  background:#06090d;border:1px solid var(--border);
  border-radius:var(--radius-lg);padding:12px 14px;
  height:calc(100vh - 220px);overflow-y:auto;
  font-family:'JetBrains Mono',ui-monospace,monospace;
  font-size:11.5px;line-height:1.6;
  white-space:pre-wrap;word-break:break-all;color:#c9d1d9;
}
#configBox{
  background:var(--surface);border:1px solid var(--border);
  border-radius:var(--radius-lg);padding:16px;
  overflow-x:auto;font-family:'JetBrains Mono',monospace;
  font-size:12px;line-height:1.7;color:var(--text);
}

/* ---------- EMPTY / HINT ---------- */
.empty{text-align:center;color:var(--text-subtle);padding:32px 16px;font-style:italic;font-size:12px}
.hint{
  color:var(--text-muted);font-size:12px;
  background:var(--surface);border:1px solid var(--border);
  border-left:3px solid var(--blue);
  padding:10px 14px;border-radius:var(--radius);margin-bottom:12px;
  line-height:1.5;
}
.hint code{
  background:var(--surface-elev);padding:1px 5px;border-radius:3px;
  font-family:'JetBrains Mono',monospace;font-size:11px;color:var(--blue);
}
.hint b{color:var(--text)}

/* ---------- BADGE ---------- */
.badge-on{padding:2px 8px;border-radius:4px;font-size:10px;font-weight:600;background:var(--green-bg);color:var(--green);text-transform:uppercase}
.badge-off{padding:2px 8px;border-radius:4px;font-size:10px;font-weight:600;background:var(--red-bg);color:var(--red);text-transform:uppercase}

/* ---------- TOAST ---------- */
#toastContainer{
  position:fixed;bottom:20px;right:20px;z-index:9999;
  display:flex;flex-direction:column;gap:8px;pointer-events:none;
}
.toast{
  background:var(--surface-elev);border:1px solid var(--border-hover);
  color:var(--text);padding:10px 16px;border-radius:var(--radius);
  font-size:12px;font-weight:500;box-shadow:var(--shadow-lg);
  transform:translateX(120%);opacity:0;transition:all 0.3s cubic-bezier(0.4,0,0.2,1);
  max-width:320px;pointer-events:auto;
}
.toast.show{transform:translateX(0);opacity:1}
.toast.success{border-left:3px solid var(--green)}
.toast.error{border-left:3px solid var(--red)}
.toast.info{border-left:3px solid var(--blue)}
.toast.warn{border-left:3px solid var(--yellow)}

.spacer{flex:1}
.src-link{color:var(--blue);cursor:pointer;text-decoration:none;font-family:'JetBrains Mono',monospace;font-size:11.5px}
.src-link:hover{text-decoration:underline}
.speed{color:var(--green);font-size:11px;font-family:'JetBrains Mono',monospace}
.speed-down{color:var(--blue);font-size:11px;font-family:'JetBrains Mono',monospace}
.reason{color:var(--text-muted);font-size:11px;font-family:'JetBrains Mono',monospace}
.bytes{color:var(--text-muted);font-size:11px;font-family:'JetBrains Mono',monospace}
.section-h{
  font-size:11px;color:var(--text-muted);
  text-transform:uppercase;letter-spacing:0.8px;
  margin:16px 0 8px;font-weight:700;
  display:flex;align-items:center;gap:8px;
}
.section-h::after{content:'';flex:1;height:1px;background:linear-gradient(90deg,var(--border),transparent)}

/* Quick Add Row di Limiter */
.quick-add-row td { background: rgba(88,166,255,0.03); padding: 6px 10px; }
.quick-add-row input, .quick-add-row select { background: var(--bg); border: 1px solid var(--border); color: var(--text); padding: 4px 8px; border-radius: 4px; font-size: 11px; width: 100%; }
.quick-add-row input:focus, .quick-add-row select:focus { outline: none; border-color: var(--blue); }

@media(max-width:768px){
  header{padding:8px 12px}
  .header-info{display:none}
  nav{padding:4px 12px;top:49px}
  main{padding:12px}
  .stat{grid-template-columns:repeat(2,1fr)}
  .filter-bar{padding:6px}
  .filter-bar input,.filter-bar select{min-width:0;flex:1;font-size:11px;padding:4px 8px}
  .btn{padding:5px 10px;font-size:11px}
  #logbox{height:calc(100vh - 180px)}
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
  <span id="versionInfo" class="version-tag">v?</span>
</header>

<nav>
  <button class="active" onclick="showTab('dashboard',this)"><span class="nav-icon">◉</span> Dashboard</button>
  <button onclick="showTab('connections',this)"><span class="nav-icon">⇄</span> Connections</button>
  <button onclick="showTab('limiter',this)"><span class="nav-icon">⏱</span> Limiter</button>
  <button onclick="showTab('logs',this)"><span class="nav-icon">≡</span> Logs</button>
  <button onclick="showTab('config',this)"><span class="nav-icon">⚙</span> Config</button>
</nav>

<main>

<!-- DASHBOARD -->
<div id="tab-dashboard" class="tab active">
  <div class="stat" id="statsRow"></div>

  <div class="dash-grid">
    <!-- KIRI: Chart & Devices -->
    <div>
      <div class="chart-wrap">
        <div class="chart-header">
          <span class="chart-title">Live Traffic</span>
          <div class="chart-legend">
            <span><span class="dot" style="background:var(--green)"></span>Upload</span>
            <span><span class="dot" style="background:var(--blue)"></span>Download</span>
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

      <div class="section-h">Devices</div>
      <div class="tbl-wrap" style="max-height:250px;overflow-y:auto">
        <table>
          <thead><tr><th>Src IP</th><th>Active</th><th>Total</th><th>Up</th><th>Down</th></tr></thead>
          <tbody id="devicesBody"><tr><td colspan="5" class="empty">Loading...</td></tr></tbody>
        </table>
      </div>
    </div>

    <!-- KANAN: Proxy Groups -->
    <div class="groups-wrap">
      <div class="groups-header">Proxy Groups</div>
      <div class="groups-container" id="groupsContainer">
        <div class="empty">Loading...</div>
      </div>
    </div>
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
    <span class="spacer"></span>
    <button class="btn btn-danger" id="killSelBtn" onclick="killSelected()" disabled>Kill Selected (0)</button>
    <button class="btn btn-danger" id="killFilteredBtn" onclick="killFiltered()">Kill Filtered</button>
    <button class="btn btn-danger" id="killAllBtn" onclick="killAllConns()">Kill All</button>
  </div>

  <div class="tbl-wrap">
    <table>
      <thead><tr>
        <th class="sticky-col" style="width:40px;text-align:center"><input type="checkbox" id="selAll" onchange="toggleAllConns(this)"></th>
        <th>Time</th>
        <th>Src IP</th>
        <th>Destination</th>
        <th>Rule</th>
        <th>Proxy</th>
        <th>Up</th>
        <th>Down</th>
        <th id="thSpeed">Speed</th>
        <th id="thEnd" style="display:none">End</th>
        <th style="width:40px"></th>
      </tr></thead>
      <tbody id="connBody"><tr><td colspan="11" class="empty">Loading...</td></tr></tbody>
    </table>
  </div>
</div>

<!-- LIMITER -->
<div id="tab-limiter" class="tab">
  <div class="hint">
    Atur limit kecepatan <b>per IP</b>. Nilai dalam <b>kilobit/detik</b> (0 = unlimited). 
    Pilih IP dari daftar (diambil dari Rules/Devices) atau pilih <code>-- custom --</code> untuk ketik manual.
  </div>

  <div id="limiterInfo" class="hint"></div>

  <div class="filter-bar">
    <!-- Dropdown IP Utama -->
    <select id="limIPSelect" onchange="handleIPSelect(this)" style="min-width:160px; font-weight:600; color:var(--blue)">
      <option value="">-- Pilih IP --</option>
    </select>
    <!-- Input Custom (Tersembunyi sampai pilih -- custom --) -->
    <input type="text" id="limIPCustom" placeholder="Ketik IP manual..." style="display:none; min-width:160px">
    
    <input type="number" id="limUp" placeholder="⬆ Up kbps (0=off)" min="0" style="min-width:130px">
    <input type="number" id="limDown" placeholder="⬇ Down kbps (0=off)" min="0" style="min-width:130px">
    <label><input type="checkbox" id="limEnabled" checked> Enabled</label>
    <button class="btn btn-primary" onclick="saveLimiterRule()">Save / Update</button>
    <button class="btn" onclick="resetLimiterForm()">Reset</button>
    <span class="spacer"></span>
    <button class="btn" onclick="loadLimiter()">↻ Refresh</button>
  </div>

  <div class="tbl-wrap">
    <table>
      <thead><tr>
        <th>IP Address</th>
        <th>⬆ Up (kbps)</th>
        <th>⬇ Down (kbps)</th>
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
    <label><input type="checkbox" id="pauseLog" onchange="updatePauseLog()"> Pause autoscroll</label>
    <input type="text" id="logSearch" placeholder="Search log (case-insensitive)..." oninput="filterLog()" style="flex:1;min-width:200px">
    <button class="btn" onclick="document.getElementById('logbox').textContent=''">Clear View</button>
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
function fmtUptime(sec){
  if(!sec || sec < 0) return '0m';
  if(sec < 60) return sec + 's';
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  if (h > 0) return h + 'h ' + m + 'm';
  return m + 'm';
}
function toast(msg,type){
  type=type||'info';
  const c=document.getElementById('toastContainer');
  const el=document.createElement('div');
  el.className='toast '+type;
  el.textContent=msg;
  c.appendChild(el);
  requestAnimationFrame(()=>el.classList.add('show'));
  setTimeout(()=>{
    el.classList.remove('show');
    setTimeout(()=>el.remove(),350);
  },3500);
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
    document.getElementById('serverInfo').innerHTML='<span class="hl">'+esc(d.server||'')+'</span>';
    document.getElementById('versionInfo').textContent='v'+(d.version||'?');

    const stateColor=state==='connected'?'var(--green)':(state==='connecting'?'var(--yellow)':'var(--red)');
    
    document.getElementById('statsRow').innerHTML=
      card('◉','State',esc(state),'accent',stateColor)+
      card('⏱','Uptime',fmtUptime(d.uptime_sec),'')+
      card('⬆','Upload',fmtSpeed(d.speed_up),'accent','var(--green)','Total: '+fmtBytes(d.total_up))+
      card('⬇','Download',fmtSpeed(d.speed_down),'accent','var(--blue)','Total: '+fmtBytes(d.total_down))+
      card('⇄','Active',d.active_conns||0,'')+
      card('✓','Closed',d.closed_conns||0,'')+
      card('↻','Reconnects',d.reconnects||0,'small','var(--purple)')+
      card('◇','DNS',d.dns_queries||0,'small','var(--purple)','pool '+(d.dns_pool?d.dns_pool.size:0));
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
      const x=i*step;
      const y=H-((s[key]/maxVal)*(H-pad*2))-pad;
      return x.toFixed(1)+','+y.toFixed(1);
    }).join(' ');
  }
  function area(key){
    const p=pts(key);
    if(!p)return '';
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
        '<td class="bytes">'+dev.total_conns+'</td>'+
        '<td class="bytes">'+fmtBytes(dev.up)+'</td>'+
        '<td class="bytes">'+fmtBytes(dev.down)+'</td>'+
      '</tr>'
    ).join('');
    
    // Update filter dropdown
    const sel=document.getElementById('filterSrc');
    const cur=sel.value;
    let opts='<option value="">All sources</option>';
    d.devices.forEach(dev=>{opts+='<option value="'+esc(dev.srcIP)+'">'+esc(dev.srcIP)+'</option>';});
    sel.innerHTML=opts;
    sel.value=cur;

    // Update Limiter Dropdowns
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
      method:'PUT',
      headers:{'Content-Type':'application/json'},
      body:JSON.stringify({name:name})
    });
    if(!r.ok)throw new Error(await r.text());
    toast(group+' → '+name,'success');
    loadProxies();
  }catch(e){toast('Error: '+e.message,'error')}
}

// ============ connections ============
let connState='active';
let rawConns=[];
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
      endCell='<td class="reason">'+(et?et.toTimeString().slice(0,8):'-')+' ('+esc(c.reason||'')+')<br><span style="color:var(--text-subtle)">in '+(c.remain_sec||0)+'s</span></td>';
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
      speedCell+
      endCell+
      '<td style="text-align:center">'+(isActive?'<button class="btn-icon danger" onclick="killConn(\''+esc(c.id)+'\')" title="Kill">✕</button>':'')+'</td>'+
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
    toast('Connection killed','success');
    loadConnections();
  }catch(e){toast('Error: '+e.message,'error')}
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
    selectedConns.clear();
    toast('Killed '+ids.length+' connection(s)','success');
    loadConnections();
  }catch(e){toast('Error: '+e.message,'error')}
}
async function killFiltered(){
  const ids=filteredConns.map(c=>c.id);
  if(ids.length===0){toast('No filtered connections','warn');return;}
  if(!confirm('Kill '+ids.length+' filtered connection(s)?'))return;
  try{
    const r=await fetch('/api/connections/kill',{
      method:'POST',headers:{'Content-Type':'application/json'},
      body:JSON.stringify({ids:ids})
    });
    if(!r.ok)throw new Error(await r.text());
    selectedConns.clear();
    toast('Killed '+ids.length+' connection(s)','success');
    loadConnections();
  }catch(e){toast('Error: '+e.message,'error')}
}
async function killAllConns(){
  if(!confirm('Kill ALL active connections?'))return;
  try{
    const r=await fetch('/api/connections/kill',{
      method:'POST',headers:{'Content-Type':'application/json'},
      body:JSON.stringify({all:true})
    });
    if(!r.ok)throw new Error(await r.text());
    selectedConns.clear();
    toast('All connections killed','success');
    loadConnections();
  }catch(e){toast('Error: '+e.message,'error')}
}

// ============ limiter ============
let cachedConfig = null;

async function loadConfigData() {
    if (cachedConfig) return cachedConfig;
    try {
        const r = await fetch('/api/config');
        cachedConfig = await r.json();
        return cachedConfig;
    } catch(e) { return null; }
}

// Mengisi dropdown IP (baik form atas maupun Quick Add) dari Rules + Devices + Existing Limits
async function populateIPDropdowns(activeDevices) {
    const selectTop = document.getElementById('limIPSelect');
    const selectBottom = document.getElementById('qaIPSelect');
    let ips = new Set();
    
    // 1. Ambil dari Active Devices
    if (activeDevices) {
        activeDevices.forEach(d => ips.add(d.srcIP));
    }
    
    // 2. Ambil dari Config Rules (SRC-IP-CIDR)
    const cfg = await loadConfigData();
    if (cfg && cfg.rules) {
        cfg.rules.forEach(rule => {
            const parts = rule.split(',');
            if (parts[0].trim() === 'SRC-IP-CIDR' && parts[1]) {
                const ip = parts[1].split('/')[0].trim(); // Hapus /32
                if (ip) ips.add(ip);
            }
        });
    }

    // 3. Ambil dari Limiter Rules yang ada
    try {
        const r = await fetch('/api/limiter');
        const d = await r.json();
        if (d.rules) d.rules.forEach(x => ips.add(x.ip));
    } catch(e) {}

    const sortedIPs = Array.from(ips).sort();
    const curTopVal = selectTop.value;
    const curBottomVal = selectBottom ? selectBottom.value : '';

    let opts = '<option value="">-- Pilih IP --</option>';
    sortedIPs.forEach(ip => {
        opts += '<option value="'+ip+'">'+ip+'</option>';
    });
    opts += '<option value="custom" style="color:var(--yellow);font-weight:bold">-- custom --</option>';
    
    if (selectTop) {
        selectTop.innerHTML = opts;
        if (sortedIPs.includes(curTopVal)) selectTop.value = curTopVal;
    }
    if (selectBottom) {
        selectBottom.innerHTML = opts;
        if (sortedIPs.includes(curBottomVal)) selectBottom.value = curBottomVal;
    }
}

// Event handler ketika dropdown top diganti
function handleIPSelect(sel) {
    const custom = document.getElementById('limIPCustom');
    if (sel.value === 'custom') {
        custom.style.display = 'inline-block';
        custom.focus();
    } else {
        custom.style.display = 'none';
        custom.value = '';
    }
}

// Event handler ketika dropdown Quick Add diganti
function handleQaIPSelect(sel) {
    const custom = document.getElementById('qaIPCustom');
    if (sel.value === 'custom') {
        custom.style.display = 'block';
        custom.focus();
    } else {
        custom.style.display = 'none';
        custom.value = '';
    }
}

async function loadLimiter(){
  try{
    const r=await fetch('/api/limiter');
    const d=await r.json();
    const info=document.getElementById('limiterInfo');
    info.innerHTML='Status: <b style="color:'+(d.enabled?'var(--green)':'var(--red)')+'">'+(d.enabled?'✅ enabled':'⛔ disabled')+'</b> · '+
      'File: <code>'+esc(d.file||'(none)')+'</code> · '+
      'Rules: <b>'+(d.rules?d.rules.length:0)+'</b>'+
      (d.enabled?'':' · <span style="color:var(--yellow)">limit tidak akan diterapkan sampai enabled=true</span>');
    
    // Trigger populate dropdowns
    loadDevices();

    const body=document.getElementById('limiterBody');
    if(!d.rules||d.rules.length===0){
      body.innerHTML='<tr><td colspan="5" class="empty">Belum ada rule. Tambah via form di atas.</td></tr>';
    } else {
      body.innerHTML=d.rules.map(rule=>{
        const statusCls=rule.enabled?'badge-on':'badge-off';
        const statusTxt=rule.enabled?'enabled':'disabled';
        return '<tr>'+
          '<td class="mono">'+esc(rule.ip)+'</td>'+
          '<td class="bytes">'+(rule.up_kbps||0)+'</td>'+
          '<td class="bytes">'+(rule.down_kbps||0)+'</td>'+
          '<td><span class="'+statusCls+'">'+statusTxt+'</span></td>'+
          '<td>'+
            '<button class="btn-icon edit" onclick="editLimiterRule(\''+esc(rule.ip)+'\','+(rule.up_kbps||0)+','+(rule.down_kbps||0)+','+(rule.enabled?'true':'false')+')" title="Edit">✎</button>'+
            '<button class="btn-icon danger" onclick="deleteLimiterRule(\''+esc(rule.ip)+'\')" title="Delete">✕</button>'+
          '</td>'+
        '</tr>';
      }).join('');
    }

    // Append Quick Add Row (Kolom Kosong Manual dengan Dropdown)
    body.innerHTML += 
      '<tr class="quick-add-row">'+
        '<td>'+
          '<select id="qaIPSelect" onchange="handleQaIPSelect(this)">'+
            '<option value="">-- Pilih IP --</option>'+
          '</select>'+
          '<input type="text" id="qaIPCustom" placeholder="Ketik IP..." style="display:none; margin-top:4px;">'+
        '</td>'+
        '<td><input type="number" id="qaUp" placeholder="Up kbps"></td>'+
        '<td><input type="number" id="qaDown" placeholder="Down kbps"></td>'+
        '<td><label style="display:flex;align-items:center;gap:4px;cursor:pointer"><input type="checkbox" id="qaEnabled" checked style="margin:0"> On</label></td>'+
        '<td><button class="btn btn-primary" style="padding:4px 8px;font-size:11px" onclick="quickAddLimiter()">Add</button></td>'+
      '</tr>';

    // Setelah menambahkan row Quick Add, panggil ulang populate agar dropdown bawah terisi
    loadDevices();

  }catch(e){console.error(e)}
}

function editLimiterRule(ip, up, down, enabled){
  const sel = document.getElementById('limIPSelect');
  const custom = document.getElementById('limIPCustom');
  
  // Coba cari IP di dropdown
  let found = false;
  for (let opt of sel.options) {
      if (opt.value === ip) { sel.value = ip; found = true; break; }
  }
  
  if (!found) {
      // Jika tidak ada di list, set ke custom
      sel.value = 'custom';
      custom.value = ip;
      custom.style.display = 'inline-block';
  } else {
      custom.style.display = 'none';
      custom.value = '';
  }
  
  document.getElementById('limUp').value=up;
  document.getElementById('limDown').value=down;
  document.getElementById('limEnabled').checked=enabled;
  if (found) sel.focus(); else custom.focus();
}

function resetLimiterForm(){
  document.getElementById('limIPSelect').value='';
  const custom = document.getElementById('limIPCustom');
  custom.value='';
  custom.style.display='none';
  
  document.getElementById('limUp').value='';
  document.getElementById('limDown').value='';
  document.getElementById('limEnabled').checked=true;
}

async function saveLimiterRule(){
  let ip = document.getElementById('limIPSelect').value;
  if (ip === 'custom') {
      ip = document.getElementById('limIPCustom').value.trim();
  }
  
  if(!ip){toast('IP wajib diisi','warn');return;}
  const up=parseInt(document.getElementById('limUp').value)||0;
  const down=parseInt(document.getElementById('limDown').value)||0;
  const enabled=document.getElementById('limEnabled').checked;
  try{
    const r=await fetch('/api/limiter',{
      method:'POST',
      headers:{'Content-Type':'application/json'},
      body:JSON.stringify({ip:ip,up_kbps:up,down_kbps:down,enabled:enabled})
    });
    if(!r.ok)throw new Error(await r.text());
    toast('Rule saved: '+ip,'success');
    resetLimiterForm();
    loadLimiter();
  }catch(e){toast('Error: '+e.message,'error')}
}

// Fungsi untuk menyimpan dari baris Quick Add di tabel bawah
async function quickAddLimiter(){
  let ip = document.getElementById('qaIPSelect').value;
  if (ip === 'custom') {
      ip = document.getElementById('qaIPCustom').value.trim();
  }
  
  if(!ip){toast('IP wajib diisi','warn');return;}
  const up=parseInt(document.getElementById('qaUp').value)||0;
  const down=parseInt(document.getElementById('qaDown').value)||0;
  const enabled=document.getElementById('qaEnabled').checked;
  try{
    const r=await fetch('/api/limiter',{
      method:'POST',
      headers:{'Content-Type':'application/json'},
      body:JSON.stringify({ip:ip,up_kbps:up,down_kbps:down,enabled:enabled})
    });
    if(!r.ok)throw new Error(await r.text());
    toast('Quick add: '+ip,'success');
    loadLimiter();
  }catch(e){toast('Error: '+e.message,'error')}
}

async function deleteLimiterRule(ip){
  if(!confirm('Hapus rule untuk '+ip+'?'))return;
  try{
    const r=await fetch('/api/limiter/'+encodeURIComponent(ip),{method:'DELETE'});
    if(!r.ok)throw new Error(await r.text());
    toast('Deleted: '+ip,'success');
    loadLimiter();
  }catch(e){toast('Error: '+e.message,'error')}
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
loadLimiter();
</script>
</body>
</html>`