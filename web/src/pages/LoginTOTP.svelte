<script>
  import { push, querystring } from 'svelte-spa-router';
  import { api } from '../lib/api.js';
  import { session } from '../lib/session.svelte.js';

  let code = $state('');
  let error = $state('');
  let busy = $state(false);
  let recovery = $state(false);

  function next() {
    const q = new URLSearchParams($querystring || '');
    return q.get('next') || '/';
  }

  async function submit(e) {
    e?.preventDefault();
    if (busy) return;
    error = '';
    busy = true;
    try {
      const res = await api.post('/auth/totp', { code }, { redirectOn401: false });
      session.set(res.user);
      const h = await api.get('/health');
      session.setupRequired = !!h.setup_required;
      push(session.setupRequired ? '/setup' : next());
    } catch (err) {
      error = err.code === 'rate_limited' ? 'Too many attempts.' : 'Invalid code.';
      code = '';
    } finally { busy = false; }
  }

  $effect(() => { if (!recovery && code.replace(/\s/g, '').length === 6) submit(); });
</script>

<div class="mx-auto mt-16 max-w-sm">
  <form class="card space-y-4" onsubmit={submit}>
    <h1 class="text-base font-semibold">Two-factor authentication</h1>
    <p class="text-sm text-zinc-500">{recovery ? 'Enter one of your recovery codes.' : 'Enter the 6-digit code from your authenticator app.'}</p>
    <input class="input text-center text-lg tracking-widest" bind:value={code} inputmode={recovery ? 'text' : 'numeric'} autocomplete="one-time-code" placeholder={recovery ? 'xxxxx-xxxxx-xxxxx' : '000000'} autofocus />
    {#if error}<p class="text-sm text-red-600" role="alert">{error}</p>{/if}
    <button class="btn-primary w-full justify-center" disabled={busy || !code}>Verify</button>
    <button type="button" class="w-full text-center text-xs text-zinc-500 underline" onclick={() => { recovery = !recovery; code = ''; }}>
      {recovery ? 'Use authenticator code' : 'Use a recovery code'}
    </button>
  </form>
</div>
