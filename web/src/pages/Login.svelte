<script>
  import { push, querystring } from 'svelte-spa-router';
  import { api, ApiError } from '../lib/api.js';
  import { session } from '../lib/session.svelte.js';
  import Icon from '../lib/components/Icon.svelte';

  let email = $state('');
  let password = $state('');
  let error = $state('');
  let busy = $state(false);

  function next() {
    const q = new URLSearchParams($querystring || '');
    return q.get('next') || '/';
  }

  async function submit(e) {
    e.preventDefault();
    error = '';
    busy = true;
    try {
      const res = await api.post('/auth/login', { email, password }, { redirectOn401: false });
      if (res?.totp_required) { push('/login/totp?next=' + encodeURIComponent(next())); return; }
      session.set(res.user);
      const h = await api.get('/health');
      session.setupRequired = !!h.setup_required;
      push(session.setupRequired ? '/setup' : next());
    } catch (err) {
      if (err instanceof ApiError) {
        error = err.code === 'rate_limited' ? 'Too many attempts. Wait a minute and try again.' : err.code === 'disabled' ? 'This account is disabled.' : err.status === 401 ? 'Invalid e-mail or password.' : `Server error (${err.status}). Try again later.`;
      } else error = 'Network error.';
    } finally { busy = false; }
  }
</script>

<div class="mx-auto mt-16 max-w-sm">
  <div class="mb-6 flex items-center justify-center gap-2 text-xl font-semibold">
    <span class="grid h-9 w-9 place-items-center rounded-md bg-brand-600 text-white"><Icon name="dashboard" class="h-5 w-5" /></span>OxiLytics
  </div>
  <form class="card space-y-4" onsubmit={submit}>
    <div>
      <label class="label" for="email">E-mail</label>
      <input id="email" class="input" type="email" bind:value={email} autocomplete="username" required autofocus />
    </div>
    <div>
      <label class="label" for="password">Password</label>
      <input id="password" class="input" type="password" bind:value={password} autocomplete="current-password" required />
    </div>
    {#if error}<p class="text-sm text-red-600" role="alert">{error}</p>{/if}
    <button class="btn-primary w-full justify-center" disabled={busy}>Sign in</button>
  </form>
  <p class="mt-4 text-center text-xs text-zinc-400">
    OxiLytics {session.version?.version || ''}{#if session.version?.short_commit && session.version.commit !== 'none' && !session.version.version?.includes(session.version.short_commit.replace('-dirty', ''))}{' · ' + session.version.short_commit}{/if}
  </p>
</div>
