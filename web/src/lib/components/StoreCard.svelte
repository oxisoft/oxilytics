<script>
  import { link } from 'svelte-spa-router';
  import { api } from '../api.js';
  import { session } from '../session.svelte.js';
  import { toasts } from '../toast.svelte.js';
  import { STORE_LABEL } from '../format.js';
  import Icon from './Icon.svelte';

  let { store, status } = $props();
  let testing = $state(false);
  let result = $state(null);

  async function test() {
    testing = true; result = null;
    try { result = await api.post('/setup/test/' + store); }
    catch (e) { toasts.error(e.message); }
    finally { testing = false; }
  }
</script>

<div class="card">
  <div class="mb-2 flex items-center justify-between">
    <h3 class="flex items-center gap-2 font-medium"><Icon name={store === 'appstore' ? 'apple' : 'android'} />{STORE_LABEL[store]}</h3>
    {#if status?.configured}
      <span class="badge bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300">Configured</span>
    {:else}
      <span class="badge bg-zinc-100 text-zinc-600 dark:bg-zinc-800 dark:text-zinc-300">Not configured</span>
    {/if}
  </div>

  <ul class="mb-3 space-y-1 text-sm">
    {#each status?.checks || [] as c}
      <li class="flex items-start gap-2 {c.ok ? 'text-zinc-700 dark:text-zinc-300' : 'text-zinc-500'}">
        <span class={c.ok ? 'text-emerald-600' : 'text-red-500'}><Icon name={c.ok ? 'check' : 'x'} /></span>
        <span>{c.name}{#if c.detail}<span class="ml-1 text-xs text-zinc-400">— {c.detail}</span>{/if}</span>
      </li>
    {/each}
  </ul>

  {#if status?.info && Object.values(status.info).some(Boolean)}
    <dl class="mb-3 grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-xs text-zinc-500">
      {#each Object.entries(status.info) as [k, v]}{#if v}<dt class="font-mono">{k}</dt><dd class="truncate">{v}</dd>{/if}{/each}
    </dl>
  {/if}

  <div class="flex flex-wrap gap-2">
    <a href="/setup/{store}" use:link class="btn-secondary">Open guide</a>
    {#if status?.configured && session.isAdmin}
      <button class="btn-primary" onclick={test} disabled={testing}><Icon name="refresh" />{testing ? 'Testing…' : 'Test connection'}</button>
    {/if}
  </div>

  {#if result}
    <div class="mt-3 rounded-md border p-3 text-sm {result.ok ? 'border-emerald-300 bg-emerald-50 dark:border-emerald-900 dark:bg-emerald-950/30' : 'border-red-300 bg-red-50 dark:border-red-900 dark:bg-red-950/30'}">
      <div class="mb-1 font-medium">{result.ok ? 'Connection OK' : 'Connection failed'}</div>
      <ul class="space-y-0.5">
        {#each result.steps as s}
          <li class="flex items-start gap-2"><span class={s.info ? 'text-zinc-400' : s.ok ? 'text-emerald-600' : 'text-red-600'}><Icon name={s.info ? 'info' : s.ok ? 'check' : 'x'} /></span><span>{s.name}{#if s.detail}<span class="ml-1 text-xs text-zinc-500">— {s.detail}</span>{/if}</span></li>
        {/each}
      </ul>
    </div>
  {/if}
</div>
