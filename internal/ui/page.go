package ui

const pageHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>GameVision</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;500&family=IBM+Plex+Sans:wght@400;500;600&family=Syne:wght@700;800&display=swap" rel="stylesheet">
<style>
  :root {
    --void: #101426;
    --ink: #e6e1d4;
    --mute: #8b93a7;
    --panel: #181d30;
    --shell: #4a3b32;
    --shell-dark: #2c241f;
    --glass: #6fbfb0;
    --ember: #d06a3f;
    --ok: #b7d989;
    --warn: #e0b15a;
    --bad: #d07070;
  }
  * { box-sizing: border-box; }
  html, body { margin: 0; height: 100%; background: var(--void); color: var(--ink);
               font: 15px/1.45 "IBM Plex Sans", sans-serif; }
  body { display: grid; grid-template-columns: minmax(0, 1fr) 340px; min-height: 100%; }
  #stage { display: flex; align-items: center; justify-content: center; padding: 28px; min-width: 0; }
  #bezel {
    background: linear-gradient(160deg, #5a4a40, var(--shell) 40%, var(--shell-dark));
    padding: 28px 28px 36px;
    border-radius: 28px 28px 36px 36px;
    box-shadow: 0 24px 80px rgba(0,0,0,.45), inset 0 1px 0 rgba(255,255,255,.12);
  }
  #brand { font-family: Syne, sans-serif; font-weight: 800; letter-spacing: .04em;
           font-size: 13px; color: #d8c4b0; margin: 0 0 14px; }
  #screen-wrap { background: #0a0c12; padding: 10px; border-radius: 6px; }
  #f { display: none; width: min(640px, 72vw); height: auto; aspect-ratio: 160/144;
       image-rendering: pixelated; image-rendering: crisp-edges; background: #000; }
  #w { color: var(--mute); margin: 48px 24px; text-align: center; }
  #side { background: var(--panel); border-left: 1px solid #252b42; padding: 22px 20px;
          display: flex; flex-direction: column; gap: 16px; min-height: 100vh; }
  h1 { font-family: Syne, sans-serif; font-size: 28px; margin: 0; letter-spacing: -.03em; }
  .goal { color: var(--glass); white-space: pre-wrap; }
  .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px 14px; }
  .k { color: var(--mute); font-size: 12px; }
  .v { font-family: "IBM Plex Mono", monospace; font-size: 15px; }
  .v.big { font-size: 22px; color: var(--ok); }
  .chips { display: flex; flex-wrap: wrap; gap: 6px; }
  .chip { font-family: "IBM Plex Mono", monospace; font-size: 12px; padding: 3px 8px;
          background: #232944; border-radius: 999px; }
  .chip.human { background: #3a2a22; color: #f0c2a4; }
  .row { display: flex; flex-wrap: wrap; gap: 8px; }
  button {
    font: 500 13px "IBM Plex Sans", sans-serif; color: var(--ink);
    background: #232944; border: 1px solid #343c5c; border-radius: 8px;
    padding: 8px 12px; cursor: pointer;
  }
  button:hover { border-color: var(--glass); }
  button.ember { background: var(--ember); border-color: transparent; color: #fff; }
  .pads { display: grid; grid-template-columns: repeat(3, 40px); gap: 6px; justify-content: start; }
  .pad { width: 40px; height: 40px; padding: 0; border-radius: 10px; }
  .hint { color: var(--mute); font-size: 12px; margin-top: auto; }
  @media (max-width: 860px) {
    body { grid-template-columns: 1fr; }
    #side { min-height: 0; }
  }
  @media (prefers-reduced-motion: reduce) { * { transition: none !important; } }
</style>
</head>
<body>
  <main id="stage">
    <div id="bezel">
      <div id="brand">GAMEVISION</div>
      <div id="screen-wrap">
        <p id="w">waiting for the first frame</p>
        <img id="f" alt="Game framebuffer">
      </div>
    </div>
  </main>
  <aside id="side">
    <h1>Live run</h1>
    <div class="goal" id="goal"></div>
    <div>
      <div class="k">Loaded model</div>
      <select id="catalog" style="width:100%;margin:6px 0;background:#232944;color:var(--ink);border:1px solid #343c5c;border-radius:8px;padding:8px;font:13px IBM Plex Sans,sans-serif"></select>
      <div class="row">
        <button id="swap">Unload & load</button>
      </div>
      <div class="k" id="swapmsg" style="margin-top:6px"></div>
    </div>
    <div class="grid">
      <div><div class="k">Model</div><div class="v" id="model"></div></div>
      <div><div class="k">Status</div><div class="v" id="status"></div></div>
      <div><div class="k">Decision</div><div class="v big" id="decision"></div></div>
      <div><div class="k">Latency</div><div class="v" id="latency"></div></div>
      <div><div class="k">Last action</div><div class="v" id="last"></div></div>
      <div><div class="k">Runtime</div><div class="v" id="runtime"></div></div>
    </div>
    <div>
      <div class="k">Recent</div>
      <div class="chips" id="recent"></div>
    </div>
    <div class="row">
      <button id="pause">Pause</button>
      <button id="resume">Resume</button>
      <button class="ember" id="stop">Stop</button>
      <button id="savestate">Save state</button>
    </div>
    <div>
      <div class="k">Manual buttons — logged as source=human</div>
      <div class="pads" style="margin:8px 0 10px">
        <span></span><button class="pad" data-a="UP">▲</button><span></span>
        <button class="pad" data-a="LEFT">◀</button>
        <button class="pad" data-a="WAIT">·</button>
        <button class="pad" data-a="RIGHT">▶</button>
        <span></span><button class="pad" data-a="DOWN">▼</button><span></span>
      </div>
      <div class="row">
        <button data-a="A">A</button>
        <button data-a="B">B</button>
        <button data-a="START">Start</button>
        <button data-a="SELECT">Select</button>
      </div>
    </div>
    <p class="hint">Arrows move. Z = A, X = B, Enter = Start, Backspace = Select, Space = Wait. The browser does not clock the emulator.</p>
  </aside>
<script>
const img = document.getElementById('f'), wait = document.getElementById('w');
let inFlight = false;
async function tickFrame() {
  if (inFlight) return;
  inFlight = true;
  try {
    const r = await fetch('/frame.png', { cache: 'no-store' });
    if (r.ok) {
      const url = URL.createObjectURL(await r.blob());
      const old = img.src;
      img.src = url;
      if (old.startsWith('blob:')) URL.revokeObjectURL(old);
      img.style.display = 'block';
      wait.style.display = 'none';
    }
  } catch (e) {}
  finally { inFlight = false; }
}
function render(s) {
  document.getElementById('goal').textContent = s.goal || '';
  document.getElementById('model').textContent = s.model || '';
  document.getElementById('status').textContent = s.status || '';
  document.getElementById('decision').textContent = '#' + (s.decision_number || 0);
  document.getElementById('latency').textContent = (s.last_latency_ms || 0).toFixed(0) + ' ms';
  const last = (s.last_action || '—') + (s.last_source ? ' · ' + s.last_source : '');
  document.getElementById('last').textContent = last;
  document.getElementById('runtime').textContent = s.runtime || '0s';
  const rec = document.getElementById('recent');
  rec.innerHTML = '';
  (s.recent_actions || []).forEach(a => {
    const el = document.createElement('span');
    el.className = 'chip';
    el.textContent = a;
    rec.appendChild(el);
  });
}
async function tickStatus() {
  try {
    const r = await fetch('/status', { cache: 'no-store' });
    if (r.ok) render(await r.json());
  } catch (e) {}
}
async function post(path, body) {
  await fetch(path, { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(body) });
  tickStatus(); tickFrame();
}
document.getElementById('pause').onclick = () => post('/control', {cmd:'pause'});
document.getElementById('resume').onclick = () => post('/control', {cmd:'resume'});
document.getElementById('stop').onclick = () => post('/control', {cmd:'stop'});
document.getElementById('savestate').onclick = () => post('/state', {});
async function tickModels() {
  try {
    const r = await fetch('/models', { cache: 'no-store' });
    if (!r.ok) return;
    const data = await r.json();
    const sel = document.getElementById('catalog');
    const keep = sel.value;
    sel.innerHTML = '';
    (data.models || []).forEach(m => {
      const opt = document.createElement('option');
      opt.value = m.id;
      opt.textContent = (m.status ? m.status + ' · ' : '') + m.id;
      sel.appendChild(opt);
    });
    sel.value = keep || data.current || '';
    if (sel.selectedIndex < 0 && sel.options.length) sel.selectedIndex = 0;
  } catch (e) {}
}
document.getElementById('swap').onclick = async () => {
  const sel = document.getElementById('catalog');
  const msg = document.getElementById('swapmsg');
  msg.textContent = 'unloading current model, loading ' + sel.value + '…';
  const r = await fetch('/model', { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({model: sel.value}) });
  const t = await r.text();
  msg.textContent = r.ok ? 'loaded' : t;
  tickModels(); tickStatus();
};
setInterval(tickModels, 4000);
tickModels();
document.querySelectorAll('[data-a]').forEach(b => {
  b.onclick = () => post('/input', {action: b.getAttribute('data-a')});
});
const keys = { ArrowUp:'UP', ArrowDown:'DOWN', ArrowLeft:'LEFT', ArrowRight:'RIGHT',
  z:'A', Z:'A', x:'B', X:'B', Enter:'START', Backspace:'SELECT', ' ':'WAIT' };
window.addEventListener('keydown', (e) => {
  const a = keys[e.key];
  if (!a) return;
  e.preventDefault();
  post('/input', {action: a});
});
setInterval(tickFrame, 120);
setInterval(tickStatus, 250);
tickFrame(); tickStatus();
</script>
</body>
</html>
`
