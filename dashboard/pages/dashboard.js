import { renderSparkline } from '../charts.js';

function esc(s) { const d = document.createElement('div'); d.textContent = s; return d.innerHTML; }

const stateLabels = {
  available: 'Active',
  exhausted: 'Rate limited',
  unknown: 'Standby'
};

function stateLabel(state) {
  return stateLabels[state] || state;
}

export async function renderDashboard(container, api) {
  const [statusRes, reqRes, updateRes] = await Promise.all([api.getStatus(), api.getRequests(), api.checkUpdate()]);
  const status = statusRes.data;
  const requests = reqRes.data;
  const update = updateRes.data;

  const total = status.keys.length;
  const available = status.keys.filter(k => k.state === 'available').length;
  const exhausted = status.keys.filter(k => k.state === 'exhausted').length;
  const unknown = status.keys.filter(k => k.state === 'unknown').length;

  const now = Date.now();
  const buckets = new Array(30).fill(0);
  if (requests.entries) {
    for (const e of requests.entries) {
      const t = new Date(e.timestamp).getTime();
      const minAgo = Math.floor((now - t) / 60000);
      if (minAgo >= 0 && minAgo < 30) buckets[29 - minAgo]++;
    }
  }

  const currentKey = status.keys.find(k => k.current);
  const currentDot = currentKey ? `dot-${currentKey.state}` : 'dot-unknown';

  let updateBanner = '';
  if (update && update.update_available) {
    updateBanner = `
      <div class="update-banner" id="update-banner">
        <span>📦 Update available: <strong>${esc(update.latest)}</strong> (current: ${esc(update.current)})</span>
        <div class="update-banner-actions">
          <button class="primary" id="btn-update">Update & Restart</button>
          <button class="secondary" id="btn-dismiss-update">Dismiss</button>
        </div>
        <span id="update-status"></span>
      </div>`;
  }

  container.innerHTML = `
    ${updateBanner}
    <h2 style="margin-bottom:20px">Dashboard</h2>
    <div class="card">
      <h2>Active Key</h2>
      <div style="display:flex;align-items:center;gap:12px;font-size:18px">
        <span class="dot ${currentDot}"></span>
        ${currentKey ? `Key ${currentKey.index} — ${esc(stateLabel(currentKey.state))}` : 'No active key'}
      </div>
    </div>
    <div class="card">
      <h2>Request Rate</h2>
      <canvas id="sparkline" style="width:100%;height:80px"></canvas>
    </div>
    <div class="card">
      <h2>Key Health</h2>
      <div class="key-summary">
        <div class="key-summary-item"><div class="count">${total}</div><div class="label">Total</div></div>
        <div class="key-summary-item"><div class="count">${available}</div><div class="label" style="color:var(--success)">Active</div></div>
        <div class="key-summary-item"><div class="count">${exhausted}</div><div class="label" style="color:var(--warning)">Rate limited</div></div>
        <div class="key-summary-item"><div class="count">${unknown}</div><div class="label" style="color:var(--unknown)">Standby</div></div>
      </div>
    </div>
    <div class="card">
      <h2>Recent Requests</h2>
      <table class="table">
        <thead><tr><th>Time</th><th>Method</th><th>Path</th><th>Key</th><th>Status</th><th>Duration</th></tr></thead>
        <tbody>
          ${(requests.entries || []).slice(-10).reverse().map(e => `
            <tr>
              <td>${new Date(e.timestamp).toLocaleTimeString()}</td>
              <td>${e.method}</td>
              <td>${e.path}</td>
              <td>#${e.key_index}</td>
              <td><span class="badge ${e.status >= 400 ? 'badge-exhausted' : 'badge-available'}">${e.status}</span></td>
              <td>${e.duration_ms}ms</td>
            </tr>
          `).join('')}
        </tbody>
      </table>
    </div>
  `;

  document.getElementById('btn-update')?.addEventListener('click', async () => {
    const btn = document.getElementById('btn-update');
    const status = document.getElementById('update-status');
    btn.disabled = true;
    status.textContent = 'Pulling image and restarting...';
    const r = await api.runUpdate();
    if (r.ok) {
      status.textContent = 'Updated! Reloading...';
      setTimeout(() => location.reload(), 2000);
    } else {
      status.textContent = `Update failed: ${r.data?.error || 'unknown error'}`;
      btn.disabled = false;
    }
  });

  document.getElementById('btn-dismiss-update')?.addEventListener('click', () => {
    const banner = document.getElementById('update-banner');
    if (banner) banner.style.display = 'none';
  });

  const canvas = document.getElementById('sparkline');
  if (canvas) renderSparkline(canvas, buckets);
}
