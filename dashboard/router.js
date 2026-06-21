export class Router {
  constructor(api) {
    this.api = api;
  }

  start() {
    window.addEventListener('hashchange', () => this.route());
    if (!location.hash) location.hash = '#/';
    this.route();
  }

  async route() {
    const hash = location.hash.slice(1) || '/';
    const app = document.getElementById('app');
    const page = hash.split('/').filter(Boolean)[0] || '';

    app.innerHTML = `
      <div class="layout">
        <aside class="sidebar">
          <h1>Switchboard Go</h1>
          <nav>
            <a href="#/" class="${page === '' || page === 'dashboard' ? 'active' : ''}">📊 Dashboard</a>
            <a href="#/keys" class="${page === 'keys' ? 'active' : ''}">🔑 Keys</a>
            <a href="#/requests" class="${page === 'requests' ? 'active' : ''}">📋 Requests</a>
            <a href="#/settings" class="${page === 'settings' ? 'active' : ''}">⚙️ Settings</a>
          </nav>
          <div class="disconnected" id="disconnected">Disconnected</div>
        </aside>
        <main class="content" id="content"></main>
      </div>
    `;
    this.startPolling();
    const content = document.getElementById('content');

    switch (page) {
      case '':
      case 'dashboard':
        const { renderDashboard } = await import('./pages/dashboard.js');
        await renderDashboard(content, this.api);
        break;
      case 'keys':
        const { renderKeys } = await import('./pages/keys.js');
        await renderKeys(content, this.api);
        break;
      case 'requests':
        const { renderRequests } = await import('./pages/requests.js');
        await renderRequests(content, this.api);
        break;
      case 'settings':
        const { renderSettings } = await import('./pages/settings.js');
        await renderSettings(content, this.api);
        break;
    }
  }

  startPolling() {
    if (this._pollInterval) clearInterval(this._pollInterval);
    let fails = 0;
    this._pollInterval = setInterval(async () => {
      const r = await this.api.ping();
      const el = document.getElementById('disconnected');
      if (!r) {
        fails++;
        if (fails >= 3 && el) el.classList.add('visible');
      } else {
        fails = 0;
        if (el) el.classList.remove('visible');
      }
    }, 5000);
  }
}
