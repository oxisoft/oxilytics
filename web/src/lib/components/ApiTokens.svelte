<script>
  // API tokens: create, list, revoke. Read-only by default on the server; a
  // token gains extra powers only when explicitly ticked at creation, and the
  // copy here says plainly what each token can do.
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { session } from '../session.svelte.js';
  import { toasts } from '../toast.svelte.js';
  import { fmtDateTime, ago } from '../format.js';
  import CopyBlock from './CopyBlock.svelte';
  import Modal from './Modal.svelte';
  import Icon from './Icon.svelte';

  let tokens = $state(null);
  let name = $state('');
  let canRunSync = $state(false);
  let creating = $state(false);
  let fresh = $state(null);
  let confirmRevoke = $state(null);

  async function load() { tokens = await api.get('/me/tokens'); }
  onMount(load);

  async function create(e) {
    e.preventDefault();
    if (!name.trim()) return;
    creating = true;
    try {
      const t = await api.post('/me/tokens', { name: name.trim(), can_run_sync: canRunSync });
      fresh = t;
      name = '';
      canRunSync = false;
      await load();
    } catch (err) { toasts.error(err.message); }
    finally { creating = false; }
  }

  async function revoke(t) {
    try {
      await api.del('/me/tokens/' + t.id);
      confirmRevoke = null;
      toasts.success('Token revoked');
      await load();
    } catch (err) { toasts.error(err.message); }
  }

  const active = $derived((tokens || []).filter((t) => !t.revoked_at));
</script>

<section class="card">
  <h2 class="mb-1 font-medium">API tokens</h2>
  <p class="mb-3 text-sm text-zinc-500">
    For scripts and integrations that need to read your analytics. Tokens are
    <strong>read-only</strong> unless you grant a capability below — they cannot
    change settings, products or users, even though yours is an admin account.
    They stop working if your account is disabled or deleted.
  </p>

  <form class="mb-4" onsubmit={create}>
    <div class="flex flex-wrap gap-2">
      <input class="input flex-1" placeholder="What is this token for? e.g. Grafana dashboard" bind:value={name} maxlength="100" />
      <button class="btn-primary" disabled={creating || !name.trim()}><Icon name="plus" />{creating ? 'Creating…' : 'Create token'}</button>
    </div>
    {#if session.isAdmin}
      <label class="mt-2 flex items-start gap-2 text-sm">
        <input type="checkbox" class="mt-0.5" bind:checked={canRunSync} />
        <span>
          Allow this token to <strong>start a sync</strong>
          <span class="block text-xs text-zinc-500">
            Lets the token trigger a sync that is already configured here. It still
            cannot change any setting, and it cannot cancel or reset a run. Chosen
            now and fixed for the life of the token.
          </span>
        </span>
      </label>
    {/if}
  </form>

  {#if tokens === null}
    <p class="text-sm text-zinc-500">Loading…</p>
  {:else if !tokens.length}
    <p class="text-sm text-zinc-500">No tokens yet.</p>
  {:else}
    <div class="overflow-x-auto">
      <table class="table">
        <thead><tr><th>Name</th><th>Token</th><th>Can</th><th>Created</th><th>Last used</th><th></th></tr></thead>
        <tbody>
          {#each tokens as t (t.id)}
            <tr class={t.revoked_at ? 'opacity-50' : ''}>
              <td class="font-medium">{t.name}</td>
              <td class="font-mono text-xs">{t.prefix}…</td>
              <td class="text-xs">
                {#if t.can_run_sync}
                  <span class="badge bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-200">read + start sync</span>
                {:else}
                  <span class="text-zinc-500">read only</span>
                {/if}
              </td>
              <td class="text-xs text-zinc-500">{fmtDateTime(t.created_at)}</td>
              <td class="text-xs text-zinc-500">
                {#if t.revoked_at}<span class="badge bg-zinc-100 text-zinc-600 dark:bg-zinc-800 dark:text-zinc-300">revoked</span>
                {:else if t.last_used_at}{ago(t.last_used_at)}
                {:else}<span class="text-amber-600">never used</span>{/if}
              </td>
              <td class="text-right">
                {#if !t.revoked_at}<button class="btn-secondary" onclick={() => (confirmRevoke = t)}>Revoke</button>{/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</section>

<!-- Shown exactly once: the server never returns the plaintext again. -->
<Modal open={!!fresh} title="Copy your token now" onclose={() => (fresh = null)}>
  {#if fresh}
    <p class="mb-3 text-sm">This is the only time <strong>{fresh.name}</strong> will be shown. Store it somewhere safe — if you lose it, revoke it and create another.</p>
    <CopyBlock text={fresh.token} />
    <p class="mt-3 text-xs text-zinc-500">Read your data:</p>
    <CopyBlock text={'curl -H "Authorization: *** ' + fresh.token + '" ' + location.origin + '/api/products'} lang="bash" />
    {#if fresh.can_run_sync}
      <p class="mt-3 text-xs text-zinc-500">Start a sync (the only write this token can make):</p>
      <CopyBlock text={'curl -X POST -H "Authorization: *** ' + fresh.token + '" -H "Content-Type: application/json" -d \'{"store":"appstore","mode":"delta"}\' ' + location.origin + '/api/sync/runs'} lang="bash" />
    {/if}
    <div class="mt-4 text-right"><button class="btn-primary" onclick={() => (fresh = null)}>Done</button></div>
  {/if}
</Modal>

<Modal open={!!confirmRevoke} title="Revoke token?" onclose={() => (confirmRevoke = null)}>
  {#if confirmRevoke}
    <p class="text-sm">Anything using <strong>{confirmRevoke.name}</strong> will stop working immediately. This cannot be undone.</p>
    <div class="mt-4 flex justify-end gap-2">
      <button class="btn-secondary" onclick={() => (confirmRevoke = null)}>Cancel</button>
      <button class="btn-danger" onclick={() => revoke(confirmRevoke)}>Revoke</button>
    </div>
  {/if}
</Modal>
