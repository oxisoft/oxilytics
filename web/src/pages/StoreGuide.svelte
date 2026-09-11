<script>
  import { onMount } from 'svelte';
  import { link, push } from 'svelte-spa-router';
  import { json } from 'svelte-i18n';
  import { api } from '../lib/api.js';
  import { session } from '../lib/session.svelte.js';
  import CopyBlock from '../lib/components/CopyBlock.svelte';
  import StoreCard from '../lib/components/StoreCard.svelte';
  import Icon from '../lib/components/Icon.svelte';

  let { params = {} } = $props();
  const store = $derived(params.store);
  const g = $derived($json('guides.' + store));
  let meta = $state(null);
  let done = $state({});

  const key = $derived('oxi.guide.' + store);

  onMount(async () => {
    if (!g) { push('/setup'); return; }
    try { done = JSON.parse(localStorage.getItem(key) || '{}'); } catch { done = {}; }
    meta = await api.get('/setup/guide/' + store);
    if (!session.setup) session.setup = await api.get('/setup/status');
  });

  function toggle(i) { done = { ...done, [i]: !done[i] }; localStorage.setItem(key, JSON.stringify(done)); }

  const envText = $derived((meta?.env || []).map((e) => `${e.name}=${e.example}`).join('\n'));
</script>

{#if g}
  <a href="/setup" use:link class="mb-2 inline-flex items-center gap-1 text-sm text-zinc-500 hover:underline"><Icon name="chevron" class="h-3.5 w-3.5 rotate-180" />All stores</a>
  <h1 class="mb-1 text-lg font-semibold">Set up {g.title}</h1>
  <p class="mb-4 text-sm text-zinc-600 dark:text-zinc-400">{g.intro}</p>

  <div class="grid gap-4 lg:grid-cols-[2fr_1fr]">
    <div class="space-y-4">
      <section class="card">
        <h2 class="mb-1 font-medium">You need</h2>
        <p class="text-sm text-zinc-600 dark:text-zinc-400">{g.need}</p>
      </section>

      <section class="card">
        <h2 class="mb-3 font-medium">Steps</h2>
        <ol class="space-y-2">
          {#each g.steps as step, i}
            <li class="flex gap-3">
              <input type="checkbox" class="mt-1" checked={!!done[i]} onchange={() => toggle(i)} aria-label="Step {i + 1} done" />
              <div class="text-sm {done[i] ? 'text-zinc-400 line-through' : ''}"><span class="mr-1 font-mono text-xs text-zinc-400">{i + 1}.</span>{step}</div>
            </li>
          {/each}
        </ol>
      </section>

      <section class="card">
        <h2 class="mb-2 font-medium">Environment variables</h2>
        <table class="table mb-3">
          <thead><tr><th>Variable</th><th>What to put</th></tr></thead>
          <tbody>
            {#each meta?.env || [] as e}
              <tr><td class="font-mono text-xs">{e.name}</td><td class="text-xs text-zinc-600 dark:text-zinc-400">{e.help}<br /><span class="text-zinc-400">e.g. {e.example}</span></td></tr>
            {/each}
          </tbody>
        </table>
        {#if envText}<CopyBlock text={envText} />{/if}
      </section>

      <section class="card">
        <h2 class="mb-2 font-medium">What “Test connection” checks</h2>
        <ol class="list-decimal space-y-1 pl-5 text-sm text-zinc-600 dark:text-zinc-400">{#each g.checks as c}<li>{c}</li>{/each}</ol>
      </section>

      <section class="card">
        <h2 class="mb-2 font-medium">Common errors</h2>
        <table class="table">
          <thead><tr><th>Message</th><th>Cause / fix</th></tr></thead>
          <tbody>{#each g.errors as [m, f]}<tr><td class="font-mono text-xs">{m}</td><td class="text-xs">{f}</td></tr>{/each}</tbody>
        </table>
      </section>

      <section class="card">
        <h2 class="mb-2 font-medium">Notes</h2>
        <ul class="list-disc space-y-1 pl-5 text-sm text-zinc-600 dark:text-zinc-400">{#each g.notes as n}<li>{n}</li>{/each}</ul>
      </section>
    </div>

    <div>
      {#if session.setup}
        <div class="sticky top-20"><StoreCard {store} status={session.setup.stores[store]} /></div>
      {/if}
    </div>
  </div>
{/if}
