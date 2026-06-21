export class Auth {
  constructor(api) {
    this.api = api;
  }

  async init() {
    const key = sessionStorage.getItem('proxy_api_key');
    if (key) {
      this.api.setKey(key);
      const ok = await this.api.ping();
      if (ok) return true;
      sessionStorage.removeItem('proxy_api_key');
    }
    return this.showLogin();
  }

  showLogin() {
    const app = document.getElementById('app');
    app.innerHTML = `
      <div class="login-page">
        <div class="login-card">
          <h1>Switchboard Go</h1>
          <p>Enter your proxy API key to continue</p>
          <div class="error" id="login-error">Invalid key</div>
          <input type="password" id="login-key" placeholder="Proxy API Key" autofocus>
          <button class="primary" id="login-btn" style="width:100%">Connect</button>
        </div>
      </div>
    `;
    return new Promise(resolve => {
      const btn = document.getElementById('login-btn');
      const input = document.getElementById('login-key');
      const error = document.getElementById('login-error');
      const attempt = async () => {
        const key = input.value.trim();
        if (!key) return;
        btn.disabled = true;
        btn.textContent = 'Validating...';
        this.api.setKey(key);
        const ok = await this.api.ping();
        if (!ok) {
          error.style.display = 'block';
          btn.disabled = false;
          btn.textContent = 'Connect';
          return;
        }
        sessionStorage.setItem('proxy_api_key', key);
        resolve(true);
      };
      btn.addEventListener('click', attempt);
      input.addEventListener('keydown', e => { if (e.key === 'Enter') attempt(); });
    });
  }
}
