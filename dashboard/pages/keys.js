function esc(s) { const d = document.createElement('div'); d.textContent = s; return d.innerHTML; }

export async function renderKeys(container, api) {
  const render = async () => {
    const res = await api.getKeys();
    const status = res.data;
    container.innerHTML = `
      <h2 style="margin-bottom:20px">API Keys</h2>
      <div class="card">
        <div style="display:flex;gap:8px;margin-bottom:16px;flex-wrap:wrap">
          <input type="text" id="new-key" placeholder="Paste API key here..." style="flex:1;min-width:200px">
          <input type="text" id="new-key-name" placeholder="Optional name..." style="flex:0.4;min-width:120px">
          <button class="primary" id="validate-btn">Validate & Add</button>
        </div>
        <div id="validate-result"></div>
      </div>
      <div id="key-list">
        ${status.keys.map((k, i) => `
          <div class="key-card">
            <span class="dot dot-${k.state}"></span>
            <div class="key-info">
              <strong>${esc(k.name) || `Key ${k.index}`}</strong>
              <span class="badge badge-${k.state}">${k.state}</span>
              ${k.current ? '<span class="badge badge-available" style="margin-left:4px">current</span>' : ''}
              <div style="font-size:12px;color:var(--text-secondary);margin-top:2px;font-family:monospace">
                ${k.key_prefix || ''}${k.key_suffix ? '...' + k.key_suffix : ''}
              </div>
              ${k.last_429_time ? `<div style="font-size:11px;color:var(--error);margin-top:2px">429 at ${esc(k.last_429_time)}</div>` : ''}
            </div>
            <div class="key-actions">
              ${i > 0 ? `<button class="secondary" data-move-up="${i}">↑</button>` : ''}
              ${i < status.keys.length - 1 ? `<button class="secondary" data-move-down="${i}">↓</button>` : ''}
              <button class="danger" data-delete="${i}" ${status.keys.length <= 1 ? 'disabled' : ''}>✕</button>
            </div>
          </div>
        `).join('')}
      </div>
    `;

    document.getElementById('validate-btn')?.addEventListener('click', async () => {
      const input = document.getElementById('new-key');
      const nameInput = document.getElementById('new-key-name');
      const key = input.value.trim();
      const name = nameInput.value.trim();
      if (!key) return;
      const result = document.getElementById('validate-result');
      result.innerHTML = 'Validating...';
      const r = await api.validateKey(key);
      if (r.data?.valid) {
        await api.addKey(key, name);
        result.innerHTML = '<span style="color:var(--success)">Key added successfully</span>';
        input.value = '';
        nameInput.value = '';
        render();
      } else {
        result.innerHTML = `<span style="color:var(--error)">Invalid key: ${esc(r.data?.error || 'unknown error')}</span>`;
      }
    });

    document.querySelectorAll('[data-move-up]').forEach(btn => {
      btn.addEventListener('click', async () => {
        const i = parseInt(btn.dataset.moveUp);
        const indices = status.keys.map((_, idx) => idx);
        [indices[i - 1], indices[i]] = [indices[i], indices[i - 1]];
        await api.reorderKeys(indices);
        render();
      });
    });

    document.querySelectorAll('[data-move-down]').forEach(btn => {
      btn.addEventListener('click', async () => {
        const i = parseInt(btn.dataset.moveDown);
        const indices = status.keys.map((_, idx) => idx);
        [indices[i], indices[i + 1]] = [indices[i + 1], indices[i]];
        await api.reorderKeys(indices);
        render();
      });
    });

    document.querySelectorAll('[data-delete]').forEach(btn => {
      btn.addEventListener('click', async () => {
        const i = parseInt(btn.dataset.delete);
        if (!confirm(`Remove key ${i}?`)) return;
        const r = await api.deleteKey(i);
        if (r.ok) render();
        else alert(r.data?.error || 'Failed to delete key');
      });
    });
  };
  await render();
}

