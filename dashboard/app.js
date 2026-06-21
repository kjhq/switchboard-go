import { Auth } from './auth.js';
import { Router } from './router.js';
import { Api } from './api.js';

const api = new Api();
const auth = new Auth(api);

async function init() {
  const authed = await auth.init();
  if (!authed) return;
  const router = new Router(api);
  router.start();
}

init();
