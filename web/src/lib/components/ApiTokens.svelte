<script>
  // API tokens: create, list, revoke. Read-only by construction on the server;
  // the copy here says so plainly so nobody expects write access.
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { toasts } from '../toast.svelte.js';
  import { fmtDateTime, ago } from '../format.js';
  import CopyBlock from './CopyBlock.svelte';
  import Modal from './Modal.svelte';
  import Icon from './Icon.svelte';

  let tokens = $state(null);
  let name = $state('');
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
      const t = await api.post('/me/tokens', { name: name.trim() });
      fresh = t;
      name = '';
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
    <strong>read-only</strong> — they can never change anything, even though
    yours is an admin account. They stop working if your account is disabled or deleted.
  </p>

  <form class="mb-4 flex flex-wrap gap-2" onsubmit={create}>
    <input class="input flex-1" placeholder="What is this token for? e.g. Grafana dashboard" bind:value={name} maxlength="100" />
    <button class="btn-primary" disabled={creating || !name.trim()}><Icon name="plus" />{creating ? 'Creating…' : 'Create token'}</button>
  </form>

  {#if tokens === null}
    <p class="text-sm text-zinc-500">Loading…</p>
  {:else if !tokens.length}
    <p class="text-sm text-zinc-500">No tokens yet.</p>
  {:else}
    <div class="overflow-x-auto">
      <table class="table">
        <thead><tr><th>Name</th><th>Token</th><th>Created</th><th>Last used</th><th></th></tr></thead>
        <tbody>
          {#each tokens as t (t.id)}
            <tr class={t.revoked_at ? 'opacity-50' : ''}>
              <td class="font-medium">{t.name}</td>
              <td class="font-mono text-xs">{t.prefix}…</td>
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
    <p class="mt-3 text-xs text-zinc-500">Use it as a bearer token:</p>
    <CopyBlock text={'curl -H "Authorization: Bearer ' + fresh.token + '" ' + location.origin + '/api/products'} lang="bash" />
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
