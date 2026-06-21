function esc(s) { const d = document.createElement('div'); d.textContent = s; return d.innerHTML; }

export async function renderRequests(container, api) {
  const res = await api.getRequests();
  const entries = res.data?.entries || [];
  const total = entries.length;

  let page = 0;
  const perPage = 50;
  let sortCol = 'timestamp';
  let sortDir = 'desc';

  const doSort = (a, b) => {
    let va = a[sortCol], vb = b[sortCol];
    if (sortCol === 'duration_ms') { va = va || 0; vb = vb || 0; }
    if (sortCol === 'timestamp') { va = new Date(va).getTime(); vb = new Date(vb).getTime(); }
    if (typeof va === 'string') return sortDir === 'asc' ? va.localeCompare(vb) : vb.localeCompare(va);
    return sortDir === 'asc' ? va - vb : vb - va;
  };

  const renderTable = () => {
    const sorted = [...entries].sort(doSort);
    const paged = sorted.slice(page * perPage, (page + 1) * perPage);
    const totalPages = Math.ceil(sorted.length / perPage) || 1;

    container.innerHTML = `
      <h2 style="margin-bottom:20px">Requests (${total})</h2>
      <div class="card">
        <table class="table">
          <thead><tr>
            <th data-sort="timestamp">Time</th>
            <th data-sort="method">Method</th>
            <th data-sort="path">Path</th>
            <th data-sort="key_index">Key</th>
            <th data-sort="status">Status</th>
            <th data-sort="duration_ms">Duration</th>
          </tr></thead>
          <tbody>
            ${paged.map(e => `
              <tr>
                <td>${new Date(e.timestamp).toLocaleTimeString()}</td>
                <td>${esc(e.method)}</td>
                <td style="font-family:monospace;font-size:12px">${esc(e.path)}</td>
                <td>#${e.key_index}</td>
                <td><span class="badge ${e.status >= 400 ? 'badge-error' : 'badge-available'}">${e.status}</span></td>
                <td>${e.duration_ms}ms</td>
              </tr>
            `).join('')}
          </tbody>
        </table>
        <div class="pagination">
          <button class="secondary" ${page <= 0 ? 'disabled' : ''} id="prev-page">← Prev</button>
          <span>Page ${page + 1} of ${totalPages}</span>
          <button class="secondary" ${page >= totalPages - 1 ? 'disabled' : ''} id="next-page">Next →</button>
        </div>
      </div>
    `;

    document.querySelectorAll('[data-sort]').forEach(th => {
      th.addEventListener('click', () => {
        const col = th.dataset.sort;
        if (sortCol === col) sortDir = sortDir === 'asc' ? 'desc' : 'asc';
        else { sortCol = col; sortDir = 'desc'; }
        renderTable();
      });
    });

    document.getElementById('prev-page')?.addEventListener('click', () => {
      if (page > 0) { page--; renderTable(); }
    });
    document.getElementById('next-page')?.addEventListener('click', () => {
      if (page < totalPages - 1) { page++; renderTable(); }
    });
  };
  renderTable();
}
