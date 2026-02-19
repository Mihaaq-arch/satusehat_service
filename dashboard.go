package main

import "net/http"

func (a *App) handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(dashboardHTML))
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="id">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Satu Sehat Dashboard</title>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*{margin:0;padding:0;box-sizing:border-box}
:root{
  --bg:#0a0e1a;--surface:#111827;--card:#1e293b;--card-hover:#253349;
  --border:#334155;--text:#e2e8f0;--text-muted:#94a3b8;--text-dim:#64748b;
  --accent:#6366f1;--accent-glow:rgba(99,102,241,.15);
  --success:#22c55e;--warning:#f59e0b;--danger:#ef4444;--info:#3b82f6;
  --gradient:linear-gradient(135deg,#6366f1,#8b5cf6,#a78bfa);
  --radius:12px;--radius-sm:8px;
}
body{font-family:'Inter',sans-serif;background:var(--bg);color:var(--text);min-height:100vh}
/* Header */
.header{
  background:linear-gradient(180deg,rgba(99,102,241,.08),transparent);
  border-bottom:1px solid var(--border);padding:16px 32px;
  display:flex;align-items:center;justify-content:space-between;
  position:sticky;top:0;z-index:100;backdrop-filter:blur(20px);
}
.header h1{font-size:20px;font-weight:700;
  background:var(--gradient);-webkit-background-clip:text;-webkit-text-fill-color:transparent;}
.health-bar{display:flex;gap:16px;align-items:center;font-size:13px}
.health-dot{width:8px;height:8px;border-radius:50%;display:inline-block;margin-right:4px}
.health-dot.ok{background:var(--success);box-shadow:0 0 8px var(--success)}
.health-dot.err{background:var(--danger);box-shadow:0 0 8px var(--danger)}
/* Controls */
.controls{
  padding:20px 32px;display:flex;gap:12px;align-items:center;flex-wrap:wrap;
  background:rgba(30,41,59,.4);border-bottom:1px solid var(--border);
}
.controls label{font-size:13px;color:var(--text-muted);font-weight:500}
.controls input[type=date]{
  background:var(--card);border:1px solid var(--border);color:var(--text);
  padding:8px 12px;border-radius:var(--radius-sm);font-family:inherit;font-size:13px;
  outline:none;transition:border .2s;
}
.controls input[type=date]:focus{border-color:var(--accent)}
.btn{
  padding:8px 16px;border-radius:var(--radius-sm);border:none;cursor:pointer;
  font-family:inherit;font-size:13px;font-weight:600;transition:all .2s;
  display:inline-flex;align-items:center;gap:6px;
}
.btn-primary{background:var(--accent);color:#fff}
.btn-primary:hover{background:#4f46e5;transform:translateY(-1px);box-shadow:0 4px 12px rgba(99,102,241,.4)}
.btn-sm{padding:6px 12px;font-size:12px}
.btn-success{background:var(--success);color:#fff}
.btn-success:hover{background:#16a34a}
.btn-outline{background:transparent;border:1px solid var(--border);color:var(--text-muted)}
.btn-outline:hover{border-color:var(--accent);color:var(--text)}
.btn:disabled{opacity:.5;cursor:not-allowed;transform:none!important}
.btn-danger{background:var(--danger);color:#fff}
.btn-danger:hover{background:#dc2626}
/* Grid */
.grid{
  display:grid;grid-template-columns:repeat(auto-fill,minmax(320px,1fr));
  gap:16px;padding:24px 32px;
}
/* Cards */
.card{
  background:var(--card);border:1px solid var(--border);border-radius:var(--radius);
  padding:20px;transition:all .3s;position:relative;overflow:hidden;
}
.card::before{
  content:'';position:absolute;top:0;left:0;right:0;height:3px;
  background:var(--gradient);opacity:0;transition:opacity .3s;
}
.card:hover{border-color:rgba(99,102,241,.3);transform:translateY(-2px);
  box-shadow:0 8px 24px rgba(0,0,0,.3)}
.card:hover::before{opacity:1}
.card-title{font-size:14px;font-weight:600;margin-bottom:12px;display:flex;align-items:center;gap:8px}
.card-title .emoji{font-size:18px}
.card-stats{display:flex;gap:20px;margin-bottom:16px}
.stat{text-align:center;flex:1}
.stat-value{font-size:28px;font-weight:700;line-height:1}
.stat-value.pending{color:var(--warning)}
.stat-value.sent{color:var(--success)}
.stat-value.total{color:var(--info)}
.stat-label{font-size:11px;color:var(--text-dim);text-transform:uppercase;letter-spacing:.5px;margin-top:4px}
.card-actions{display:flex;gap:8px}
.card-actions .btn{flex:1;justify-content:center}
.card-status{font-size:12px;color:var(--text-dim);margin-top:12px;min-height:18px;
  padding:8px;background:rgba(0,0,0,.2);border-radius:var(--radius-sm);display:none}
.card-status.visible{display:block}
.card-status.error{color:var(--danger)}
/* Log section */
.log-section{padding:24px 32px}
.log-section h2{font-size:16px;font-weight:600;margin-bottom:16px;display:flex;align-items:center;gap:8px}
.log-table-wrap{
  background:var(--card);border:1px solid var(--border);border-radius:var(--radius);
  overflow:hidden;
}
table{width:100%;border-collapse:collapse;font-size:13px}
thead{background:rgba(99,102,241,.08)}
th{padding:12px 16px;text-align:left;font-weight:600;color:var(--text-muted);
  font-size:11px;text-transform:uppercase;letter-spacing:.5px;border-bottom:1px solid var(--border)}
td{padding:10px 16px;border-bottom:1px solid rgba(51,65,85,.5)}
tr:hover td{background:rgba(99,102,241,.04)}
.badge{padding:3px 8px;border-radius:99px;font-size:11px;font-weight:600}
.badge-success{background:rgba(34,197,94,.15);color:var(--success)}
.badge-failed{background:rgba(239,68,68,.15);color:var(--danger)}
.badge-skipped{background:rgba(245,158,11,.15);color:var(--warning)}
/* Spinner */
@keyframes spin{to{transform:rotate(360deg)}}
.spinner{width:14px;height:14px;border:2px solid rgba(255,255,255,.3);
  border-top-color:#fff;border-radius:50%;animation:spin .6s linear infinite;display:inline-block}
/* Toast */
.toast-container{position:fixed;top:20px;right:20px;z-index:9999;display:flex;flex-direction:column;gap:8px}
.toast{padding:12px 20px;border-radius:var(--radius-sm);font-size:13px;font-weight:500;
  animation:slideIn .3s ease;box-shadow:0 4px 16px rgba(0,0,0,.4);max-width:360px}
.toast-success{background:rgba(34,197,94,.9);color:#fff}
.toast-error{background:rgba(239,68,68,.9);color:#fff}
@keyframes slideIn{from{transform:translateX(100%);opacity:0}to{transform:translateX(0);opacity:1}}
/* Responsive */
@media(max-width:768px){
  .header,.controls,.grid,.log-section{padding-left:16px;padding-right:16px}
  .grid{grid-template-columns:1fr}
}
</style>
</head>
<body>
<div class="header">
  <h1>🏥 Satu Sehat Dashboard</h1>
  <div class="health-bar" id="healthBar">
    <span><span class="health-dot" id="dbDot"></span>Database: <span id="dbStatus">...</span></span>
    <span><span class="health-dot" id="tokenDot"></span>Token: <span id="tokenStatus">...</span></span>
  </div>
</div>

<div class="controls">
  <label>Dari</label>
  <input type="date" id="tgl1">
  <label>Sampai</label>
  <input type="date" id="tgl2">
  <button class="btn btn-primary" onclick="checkAll()">🔍 Check Semua</button>
  <button class="btn btn-outline" onclick="refreshHealth()">🔄 Refresh Status</button>
</div>

<div class="grid" id="resourceGrid"></div>

<div class="log-section">
  <h2>📋 Activity Log <button class="btn btn-outline btn-sm" onclick="loadLogs()" style="margin-left:auto">Refresh</button></h2>
  <div class="log-table-wrap">
    <table>
      <thead><tr><th>Waktu</th><th>No Rawat</th><th>Resource</th><th>FHIR ID</th><th>Status</th></tr></thead>
      <tbody id="logBody"><tr><td colspan="5" style="text-align:center;color:var(--text-dim);padding:24px">Klik refresh untuk memuat log</td></tr></tbody>
    </table>
  </div>
</div>

<div class="log-section">
  <h2>📦 Integration Jobs
    <button class="btn btn-outline btn-sm" onclick="loadJobs()" style="margin-left:auto">Refresh</button>
    <button class="btn btn-danger btn-sm" id="retryBtn" onclick="retryFailed()">🔄 Retry Failed</button>
  </h2>
  <div class="card-stats" style="margin-bottom:16px">
    <div class="stat"><div class="stat-value pending" id="jobs-pending">—</div><div class="stat-label">Pending</div></div>
    <div class="stat"><div class="stat-value sent" id="jobs-success">—</div><div class="stat-label">Success</div></div>
    <div class="stat"><div class="stat-value" style="color:var(--danger)" id="jobs-failed">—</div><div class="stat-label">Failed</div></div>
  </div>
  <div class="log-table-wrap">
    <table>
      <thead><tr><th>ID</th><th>Resource</th><th>Key</th><th>Status</th><th>FHIR ID</th><th>Retries</th><th>Error</th></tr></thead>
      <tbody id="jobsBody"><tr><td colspan="7" style="text-align:center;color:var(--text-dim);padding:24px">Klik refresh untuk memuat jobs</td></tr></tbody>
    </table>
  </div>
</div>

<div class="toast-container" id="toasts"></div>

<script>
const resources = [
  {key:'encounter', label:'Encounter Ralan', emoji:'🏨', pending:'/api/encounters/pending', send:'/api/encounters/send'},
  {key:'encounter-ranap', label:'Encounter Ranap', emoji:'🛏️', pending:'/api/encounters-ranap/pending', send:'/api/encounters-ranap/send'},
  {key:'condition', label:'Condition (ICD-10)', emoji:'🩺', pending:'/api/conditions/pending', send:'/api/conditions/send'},
  {key:'ttv-suhu', label:'TTV Suhu', emoji:'🌡️', pending:'/api/observations-ttv/suhu/pending', send:'/api/observations-ttv/suhu/send'},
  {key:'ttv-nadi', label:'TTV Nadi', emoji:'💓', pending:'/api/observations-ttv/nadi/pending', send:'/api/observations-ttv/nadi/send'},
  {key:'ttv-tensi', label:'TTV Tensi', emoji:'🩸', pending:'/api/observations-ttv/tensi/pending', send:'/api/observations-ttv/tensi/send'},
  {key:'ttv-respirasi', label:'TTV Respirasi', emoji:'🫁', pending:'/api/observations-ttv/respirasi/pending', send:'/api/observations-ttv/respirasi/send'},
  {key:'ttv-spo2', label:'TTV SpO2', emoji:'🫀', pending:'/api/observations-ttv/spo2/pending', send:'/api/observations-ttv/spo2/send'},
  {key:'ttv-gcs', label:'TTV GCS', emoji:'🧠', pending:'/api/observations-ttv/gcs/pending', send:'/api/observations-ttv/gcs/send'},
  {key:'ttv-tb', label:'TTV Tinggi Badan', emoji:'📏', pending:'/api/observations-ttv/tb/pending', send:'/api/observations-ttv/tb/send'},
  {key:'ttv-bb', label:'TTV Berat Badan', emoji:'⚖️', pending:'/api/observations-ttv/bb/pending', send:'/api/observations-ttv/bb/send'},
  {key:'ttv-lp', label:'TTV Lingkar Perut', emoji:'📐', pending:'/api/observations-ttv/lp/pending', send:'/api/observations-ttv/lp/send'},
  {key:'lab', label:'Observation Lab', emoji:'🔬', pending:'/api/observations-lab/pending', send:'/api/observations-lab/send'},
  {key:'rad', label:'Observation Radiologi', emoji:'☢️', pending:'/api/observations-rad/pending', send:'/api/observations-rad/send'},
  {key:'procedure', label:'Procedure (ICD-9)', emoji:'🔧', pending:'/api/procedures/pending', send:'/api/procedures/send'},
  {key:'medreq', label:'Medication Request', emoji:'💊', pending:'/api/medication-requests/pending', send:'/api/medication-requests/send'},
  {key:'meddisp', label:'Medication Dispense', emoji:'💉', pending:'/api/medication-dispenses/pending', send:'/api/medication-dispenses/send'},
];

// Set default dates to today
const today = new Date().toISOString().slice(0,10);
document.getElementById('tgl1').value = today;
document.getElementById('tgl2').value = today;

// Build resource cards
const grid = document.getElementById('resourceGrid');
resources.forEach(res => {
  grid.innerHTML += '<div class="card" id="card-'+res.key+'">'
    +'<div class="card-title"><span class="emoji">'+res.emoji+'</span>'+res.label+'</div>'
    +'<div class="card-stats">'
    +'<div class="stat"><div class="stat-value pending" id="'+res.key+'-pending">—</div><div class="stat-label">Pending</div></div>'
    +'<div class="stat"><div class="stat-value sent" id="'+res.key+'-sent">—</div><div class="stat-label">Sent</div></div>'
    +'<div class="stat"><div class="stat-value total" id="'+res.key+'-total">—</div><div class="stat-label">Total</div></div>'
    +'</div>'
    +'<div class="card-actions">'
    +'<button class="btn btn-outline btn-sm" onclick="checkResource(\''+res.key+'\')">🔍 Check</button>'
    +'<button class="btn btn-success btn-sm" id="send-'+res.key+'" onclick="sendResource(\''+res.key+'\')">🚀 Send</button>'
    +'</div>'
    +'<div class="card-status" id="status-'+res.key+'"></div>'
    +'</div>';
});

function getDates(){
  return {tgl1: document.getElementById('tgl1').value, tgl2: document.getElementById('tgl2').value};
}

function findRes(key){ return resources.find(r=>r.key===key); }

function setCardStatus(key, msg, isError){
  const el = document.getElementById('status-'+key);
  el.textContent = msg;
  el.classList.add('visible');
  el.classList.toggle('error', !!isError);
}

function toast(msg, type){
  const c = document.getElementById('toasts');
  const t = document.createElement('div');
  t.className = 'toast toast-'+type;
  t.textContent = msg;
  c.appendChild(t);
  setTimeout(()=>t.remove(), 4000);
}

async function refreshHealth(){
  try{
    const r = await fetch('/api/health');
    const d = await r.json();
    const dbOk = d.database === 'ok';
    const tokOk = d.token === 'ok';
    document.getElementById('dbDot').className = 'health-dot '+(dbOk?'ok':'err');
    document.getElementById('dbStatus').textContent = dbOk?'Connected':d.database;
    document.getElementById('tokenDot').className = 'health-dot '+(tokOk?'ok':'err');
    document.getElementById('tokenStatus').textContent = tokOk?'Active':d.token;
  }catch(e){
    document.getElementById('dbStatus').textContent = 'Error';
    document.getElementById('tokenStatus').textContent = 'Error';
  }
}

async function checkResource(key){
  const res = findRes(key);
  const {tgl1,tgl2} = getDates();
  setCardStatus(key, 'Checking...');
  try{
    const r = await fetch(res.pending+'?tgl1='+tgl1+'&tgl2='+tgl2);
    const d = await r.json();
    document.getElementById(key+'-pending').textContent = d.pending_count ?? d.pending?.length ?? 0;
    document.getElementById(key+'-sent').textContent = d.sent_count ?? 0;
    document.getElementById(key+'-total').textContent = d.total ?? 0;
    setCardStatus(key, 'Checked: '+tgl1+' → '+tgl2);
  }catch(e){
    setCardStatus(key, 'Error: '+e.message, true);
  }
}

async function sendResource(key){
  const res = findRes(key);
  const {tgl1,tgl2} = getDates();
  const btn = document.getElementById('send-'+key);
  btn.disabled = true;
  btn.innerHTML = '<span class="spinner"></span> Sending...';
  setCardStatus(key, 'Sending...');
  try{
    const r = await fetch(res.send,{
      method:'POST',headers:{'Content-Type':'application/json'},
      body:JSON.stringify({tgl1,tgl2})
    });
    const d = await r.json();
    const sent = d.sent??0, failed = d.failed??0;
    setCardStatus(key, '✅ Sent: '+sent+' | ❌ Failed: '+failed);
    toast(res.label+': '+sent+' sent, '+failed+' failed', sent>0?'success':'error');
    checkResource(key);
    loadLogs();
  }catch(e){
    setCardStatus(key, 'Error: '+e.message, true);
    toast(res.label+': '+e.message, 'error');
  }finally{
    btn.disabled = false;
    btn.innerHTML = '🚀 Send';
  }
}

function checkAll(){
  resources.forEach(r=>checkResource(r.key));
}

async function loadLogs(){
  try{
    const {tgl1,tgl2} = getDates();
    const r = await fetch('/api/logs?tgl1='+tgl1+'&tgl2='+tgl2+'&limit=50');
    const d = await r.json();
    const body = document.getElementById('logBody');
    if(!d.logs || d.logs.length===0){
      body.innerHTML = '<tr><td colspan="5" style="text-align:center;color:var(--text-dim);padding:24px">Tidak ada log</td></tr>';
      return;
    }
    body.innerHTML = d.logs.map(l=>{
      const badgeClass = l.status==='success'?'badge-success':l.status==='failed'?'badge-failed':'badge-skipped';
      const t = new Date(l.created_at);
      const timeStr = t.toLocaleString('id-ID',{day:'2-digit',month:'short',hour:'2-digit',minute:'2-digit',second:'2-digit'});
      const fhirShort = l.fhir_id ? l.fhir_id.substring(0,16)+'...' : '—';
      return '<tr><td>'+timeStr+'</td><td>'+l.no_rawat+'</td><td>'+l.resource_type+'</td>'
        +'<td style="font-family:monospace;font-size:12px;color:var(--text-dim)">'+fhirShort+'</td>'
        +'<td><span class="badge '+badgeClass+'">'+l.status+'</span></td></tr>';
    }).join('');
  }catch(e){
    document.getElementById('logBody').innerHTML = '<tr><td colspan="5" style="color:var(--danger)">Error: '+e.message+'</td></tr>';
  }
}

// Init
refreshHealth();

async function loadJobs(){
  try{
    const {tgl1,tgl2} = getDates();
    const r = await fetch('/api/jobs?tgl1='+tgl1+'&tgl2='+tgl2+'&limit=100');
    const d = await r.json();
    document.getElementById('jobs-pending').textContent = d.pending??0;
    document.getElementById('jobs-success').textContent = d.success??0;
    document.getElementById('jobs-failed').textContent = d.failed??0;
    const body = document.getElementById('jobsBody');
    if(!d.jobs || d.jobs.length===0){
      body.innerHTML = '<tr><td colspan="7" style="text-align:center;color:var(--text-dim);padding:24px">Tidak ada jobs</td></tr>';
      return;
    }
    body.innerHTML = d.jobs.map(j=>{
      const badgeClass = j.status==='success'?'badge-success':j.status==='failed'?'badge-failed':'badge-skipped';
      const fhirShort = j.fhir_id ? j.fhir_id.substring(0,12)+'...' : '—';
      const errShort = j.error_message ? j.error_message.substring(0,40) : '—';
      return '<tr><td>'+j.id+'</td><td>'+j.resource_type+'</td>'
        +'<td style="font-family:monospace;font-size:11px">'+j.idempotency_key.substring(0,30)+'</td>'
        +'<td><span class="badge '+badgeClass+'">'+j.status+'</span></td>'
        +'<td style="font-family:monospace;font-size:11px;color:var(--text-dim)">'+fhirShort+'</td>'
        +'<td>'+j.retry_count+'/3</td>'
        +'<td style="font-size:11px;color:var(--text-dim)">'+errShort+'</td></tr>';
    }).join('');
  }catch(e){
    document.getElementById('jobsBody').innerHTML = '<tr><td colspan="7" style="color:var(--danger)">Error: '+e.message+'</td></tr>';
  }
}

async function retryFailed(){
  const btn = document.getElementById('retryBtn');
  btn.disabled = true;
  btn.innerHTML = '<span class="spinner"></span> Retrying...';
  try{
    const r = await fetch('/api/jobs/retry',{
      method:'POST',headers:{'Content-Type':'application/json'},
      body:JSON.stringify({status:'failed'})
    });
    const d = await r.json();
    toast('Retry: '+d.succeeded+' succeeded, '+d.still_failed+' still failed', d.succeeded>0?'success':'error');
    loadJobs();
    loadLogs();
  }catch(e){
    toast('Retry error: '+e.message, 'error');
  }finally{
    btn.disabled = false;
    btn.innerHTML = '🔄 Retry Failed';
  }
}
</script>
</body>
</html>`
