export class Api {
  constructor() {
    this.key = '';
  }

  setKey(key) { this.key = key; }

  async request(method, path, body) {
    const opts = { method, headers: { 'Authorization': `Bearer ${this.key}` } };
    if (body) {
      opts.headers['Content-Type'] = 'application/json';
      opts.body = JSON.stringify(body);
    }
    try {
      const res = await fetch(path, opts);
      const data = await res.json();
      return { ok: res.ok, status: res.status, data };
    } catch {
      return { ok: false, status: 0, data: null };
    }
  }

  ping() { return this.request('GET', '/admin/status').then(r => r.ok); }

  getStatus() { return this.request('GET', '/admin/status'); }
  getKeys() { return this.request('GET', '/admin/keys'); }
  addKey(key, name) { const body = { key }; if (name) body.name = name; return this.request('POST', '/admin/keys', body); }
  deleteKey(index) { return this.request('DELETE', `/admin/keys/${index}`); }
  reorderKeys(indices) { return this.request('PUT', '/admin/keys/reorder', { indices }); }
  validateKey(key) { return this.request('POST', '/admin/validate-key', { key }); }
  getSettings() { return this.request('GET', '/admin/settings'); }
  updateSettings(data) { return this.request('PUT', '/admin/settings', data); }
  getRequests() { return this.request('GET', '/admin/requests'); }
}
