function esc(s) { const d = document.createElement('div'); d.textContent = s; return d.innerHTML; }

export async function renderSettings(container, api) {
  const res = await api.getSettings();
  const s = res.data || {};
  const currentTheme = localStorage.getItem('theme') || 'light';
  document.documentElement.setAttribute('data-theme', currentTheme);

  container.innerHTML = `
    <h2 style="margin-bottom:20px">Settings</h2>
    <div class="card">
      <h2>Server</h2>
      <div class="form-group"><label>Listen Address</label><input type="text" id="s-listen" value="${esc(s.listen_addr || '')}"></div>
      <div class="form-group"><label>Upstream Base URL</label><input type="text" id="s-upstream" value="${esc(s.upstream_base_url || '')}"></div>
      <div class="form-group"><label>Max Request Body Bytes</label><input type="number" id="s-maxbody" value="${s.max_request_body_bytes || ''}"></div>
      <div class="form-group"><label>Request Log Size</label><input type="number" id="s-logsize" value="${s.request_log_size || 500}"></div>
    </div>
    <div class="card">
      <h2>SMTP Notifications</h2>
      <div class="grid-2">
        <div class="form-group"><label>Host</label><input type="text" id="s-smtp-host" value="${esc(s.smtp_host || '')}"></div>
        <div class="form-group"><label>Port</label><input type="number" id="s-smtp-port" value="${s.smtp_port || 25}"></div>
        <div class="form-group"><label>Username</label><input type="text" id="s-smtp-user" value="${esc(s.smtp_username || '')}"></div>
        <div class="form-group"><label>Password</label><div style="display:flex;gap:4px"><input type="password" id="s-smtp-pass" placeholder="${esc(s.smtp_password === '******' ? '(unchanged)' : '(not set)')}" style="flex:1"><button class="secondary" id="s-smtp-pass-clear" style="font-size:11px">Clear</button></div></div>
        <div class="form-group"><label>From</label><input type="text" id="s-smtp-from" value="${esc(s.smtp_from || '')}"></div>
        <div class="form-group"><label>To</label><input type="text" id="s-smtp-to" value="${esc(s.smtp_to || '')}"></div>
      </div>
      <div style="display:flex;gap:16px">
        <label style="font-size:13px;display:flex;align-items:center;gap:4px">
          <input type="checkbox" id="s-smtp-tls" ${s.smtp_tls ? 'checked' : ''}> TLS
        </label>
        <label style="font-size:13px;display:flex;align-items:center;gap:4px">
          <input type="checkbox" id="s-smtp-starttls" ${s.smtp_starttls ? 'checked' : ''}> STARTTLS
        </label>
      </div>
    </div>
    <div class="card">
      <h2>Appearance</h2>
      <div style="display:flex;gap:8px;align-items:center">
        <span style="font-size:13px">Theme:</span>
        <select id="s-theme">
          <option value="light" ${currentTheme === 'light' ? 'selected' : ''}>Light</option>
          <option value="dark" ${currentTheme === 'dark' ? 'selected' : ''}>Dark</option>
        </select>
      </div>
    </div>
    <button class="primary" id="save-settings" style="margin-top:8px">Save Settings</button>
    <div id="save-toast"></div>
  `;

  let smtpPasswordCleared = false;

  document.getElementById('s-theme')?.addEventListener('change', () => {
    const theme = document.getElementById('s-theme').value;
    localStorage.setItem('theme', theme);
    document.documentElement.setAttribute('data-theme', theme);
  });

  document.getElementById('s-smtp-pass-clear')?.addEventListener('click', () => {
    document.getElementById('s-smtp-pass').value = '';
    smtpPasswordCleared = true;
  });

  document.getElementById('s-smtp-pass')?.addEventListener('input', () => {
    smtpPasswordCleared = false;
  });

  document.getElementById('save-settings')?.addEventListener('click', async () => {
    const body = {
      listen_addr: document.getElementById('s-listen').value,
      upstream_base_url: document.getElementById('s-upstream').value,
      max_request_body_bytes: parseInt(document.getElementById('s-maxbody').value) || 0,
      request_log_size: parseInt(document.getElementById('s-logsize').value) || 500,
      smtp_host: document.getElementById('s-smtp-host').value,
      smtp_port: parseInt(document.getElementById('s-smtp-port').value) || 25,
      smtp_username: document.getElementById('s-smtp-user').value,
      smtp_from: document.getElementById('s-smtp-from').value,
      smtp_to: document.getElementById('s-smtp-to').value,
      smtp_tls: document.getElementById('s-smtp-tls').checked,
      smtp_starttls: document.getElementById('s-smtp-starttls').checked,
    };
    const passField = document.getElementById('s-smtp-pass');
    if (smtpPasswordCleared) {
      body.smtp_password = '';
    } else if (passField.value) {
      body.smtp_password = passField.value;
    }

    const r = await api.updateSettings(body);
    const toast = document.getElementById('save-toast');
    toast.innerHTML = `<div class="toast ${r.ok ? 'success' : 'error'}">${r.ok ? 'Settings saved' : 'Save failed'}</div>`;
    setTimeout(() => toast.innerHTML = '', 3000);
  });
}
