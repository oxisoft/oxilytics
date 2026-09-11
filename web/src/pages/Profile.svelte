<script>
  import { api, ApiError } from '../lib/api.js';
  import { session } from '../lib/session.svelte.js';
  import { toasts } from '../lib/toast.svelte.js';
  import Qr from '../lib/components/Qr.svelte';
  import Modal from '../lib/components/Modal.svelte';
  import CopyBlock from '../lib/components/CopyBlock.svelte';

  let name = $state(session.user?.name || '');
  let pw = $state({ current: '', new: '', confirm: '' });
  let pwErr = $state({});
  let totpSetup = $state(null);
  let totpCode = $state('');
  let totpErr = $state('');
  let recoveryCodes = $state(null);
  let disableOpen = $state(false);
  let disablePw = $state('');
  let disableErr = $state('');

  async function saveName() {
    try { const u = await api.put('/me', { name }); session.set(u); toasts.success('Name updated'); }
    catch (e) { toasts.error(e.message); }
  }

  async function changePw(e) {
    e.preventDefault();
    pwErr = {};
    if (pw.new !== pw.confirm) { pwErr = { confirm: 'does not match' }; return; }
    try {
      await api.put('/me/password', { current: pw.current, new: pw.new });
      pw = { current: '', new: '', confirm: '' };
      toasts.success('Password changed');
    } catch (err) { pwErr = err instanceof ApiError ? err.fields : { current: err.message }; }
  }

  async function startTotp() {
    totpSetup = await api.post('/me/totp/setup');
    totpCode = ''; totpErr = '';
  }

  async function enableTotp(e) {
    e.preventDefault();
    try {
      const r = await api.post('/me/totp/enable', { secret: totpSetup.secret, code: totpCode });
      recoveryCodes = r.recovery_codes;
      totpSetup = null;
      session.set({ ...session.user, totp_enabled: true });
    } catch (err) { totpErr = err.fields?.code || err.message; }
  }

  async function disableTotp() {
    try {
      await api.del('/me/totp', { password: disablePw });
      session.set({ ...session.user, totp_enabled: false });
      disableOpen = false; disablePw = '';
      toasts.success('Two-factor disabled');
    } catch (err) { disableErr = err.fields?.password || err.message; }
  }
</script>

<h1 class="mb-4 text-lg font-semibold">My profile</h1>

<div class="grid gap-4 lg:grid-cols-2">
  <section class="card space-y-3">
    <h2 class="font-medium">Account</h2>
    <div><span class="label">E-mail</span><div class="text-sm">{session.user?.email}</div></div>
    <div>
      <label class="label" for="name">Name</label>
      <div class="flex gap-2"><input id="name" class="input" bind:value={name} /><button class="btn-primary" onclick={saveName} disabled={!name.trim() || name === session.user?.name}>Save</button></div>
    </div>
  </section>

  <form class="card space-y-3" onsubmit={changePw}>
    <h2 class="font-medium">Change password</h2>
    <div><label class="label" for="cur">Current password</label><input id="cur" class="input" type="password" bind:value={pw.current} autocomplete="current-password" required />{#if pwErr.current}<p class="text-xs text-red-600">{pwErr.current}</p>{/if}</div>
    <div><label class="label" for="new">New password (min 10 chars)</label><input id="new" class="input" type="password" bind:value={pw.new} autocomplete="new-password" required minlength="10" />{#if pwErr.new}<p class="text-xs text-red-600">{pwErr.new}</p>{/if}</div>
    <div><label class="label" for="conf">Confirm new password</label><input id="conf" class="input" type="password" bind:value={pw.confirm} autocomplete="new-password" required />{#if pwErr.confirm}<p class="text-xs text-red-600">{pwErr.confirm}</p>{/if}</div>
    <button class="btn-primary">Change password</button>
  </form>

  <section class="card space-y-3 lg:col-span-2">
    <h2 class="font-medium">Two-factor authentication</h2>
    {#if session.user?.totp_enabled}
      <p class="text-sm text-emerald-600">Enabled.</p>
      <button class="btn-secondary" onclick={() => (disableOpen = true)}>Disable…</button>
    {:else if totpSetup}
      <form class="grid gap-4 md:grid-cols-[auto_1fr]" onsubmit={enableTotp}>
        <Qr text={totpSetup.otpauth_url} />
        <div class="space-y-3">
          <p class="text-sm text-zinc-600 dark:text-zinc-400">Scan the QR code with your authenticator app, or enter the secret manually:</p>
          <code class="block rounded bg-zinc-100 p-2 font-mono text-xs break-all dark:bg-zinc-800">{totpSetup.secret}</code>
          <div><label class="label" for="tc">Code from the app</label><input id="tc" class="input max-w-xs" bind:value={totpCode} inputmode="numeric" autocomplete="one-time-code" required />{#if totpErr}<p class="text-xs text-red-600">{totpErr}</p>{/if}</div>
          <div class="flex gap-2"><button class="btn-primary">Enable</button><button type="button" class="btn-secondary" onclick={() => (totpSetup = null)}>Cancel</button></div>
        </div>
      </form>
    {:else}
      <p class="text-sm text-zinc-500">Not enabled. Protect your account with a time-based one-time code.</p>
      <button class="btn-primary" onclick={startTotp}>Enable two-factor</button>
    {/if}
  </section>
</div>

<Modal open={!!recoveryCodes} title="Recovery codes" onclose={() => (recoveryCodes = null)}>
  <p class="mb-3 text-sm text-zinc-600 dark:text-zinc-400">Store these somewhere safe. Each code works once and they are shown only now.</p>
  <CopyBlock text={(recoveryCodes || []).join('\n')} />
  <div class="mt-4 flex justify-end"><button class="btn-primary" onclick={() => (recoveryCodes = null)}>I saved them</button></div>
</Modal>

<Modal bind:open={disableOpen} title="Disable two-factor">
  <label class="label" for="dpw">Confirm with your password</label>
  <input id="dpw" class="input" type="password" bind:value={disablePw} autocomplete="current-password" />
  {#if disableErr}<p class="mt-1 text-xs text-red-600">{disableErr}</p>{/if}
  <div class="mt-4 flex justify-end gap-2"><button class="btn-secondary" onclick={() => (disableOpen = false)}>Cancel</button><button class="btn-danger" onclick={disableTotp} disabled={!disablePw}>Disable</button></div>
</Modal>
