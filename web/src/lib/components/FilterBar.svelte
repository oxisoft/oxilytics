<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { filter } from '../filter.svelte.js';
  import { session } from '../session.svelte.js';
  import PlatformGlyph from './PlatformGlyph.svelte';
  import ProductSelect from './ProductSelect.svelte';
  import { STORE_PLATFORMS } from '../format.js';

  let { showProduct = true } = $props();
  let products = $state([]);
  let customOpen = $state(false);
  let cf = $state(filter.from);
  let ct = $state(filter.to);

  const platforms = $derived.by(() => {
    const out = [];
    for (const [st, v] of Object.entries(session.setup?.stores || {})) if (v.configured) out.push(...(STORE_PLATFORMS[st] || []));
    return out.length ? [...new Set(out)] : ['ios', 'android'];
  });

  onMount(async () => {
    filter.load();
    try { products = await api.get('/products'); } catch {}
    if (!session.setup) { try { session.setup = await api.get('/setup/status'); } catch {} }
  });

  const presets = [['7d', '7 days'], ['30d', '30 days'], ['90d', '90 days'], ['12m', '12 months'], ['ytd', 'YTD']];
</script>

<div class="mb-4 flex flex-wrap items-center gap-2 rounded-lg border border-zinc-200 bg-white px-3 py-2 text-sm dark:border-zinc-800 dark:bg-zinc-900">
  {#if showProduct}
    <ProductSelect {products} value={filter.product} onchange={(id) => filter.setProduct(id)} />
  {/if}

  <div class="flex items-center gap-1 rounded-md border border-zinc-200 p-0.5 dark:border-zinc-700" role="group" aria-label="Platforms">
    <button class="rounded px-2 py-1 {filter.platforms.length === 0 ? 'bg-zinc-100 font-medium dark:bg-zinc-800' : 'text-zinc-500'}" onclick={() => { filter.platforms = []; filter.sync(); }}>All</button>
    {#each platforms as p}
      <button class="rounded px-2 py-1 {filter.platforms.includes(p) ? 'bg-zinc-100 font-medium dark:bg-zinc-800' : 'text-zinc-500'}" onclick={() => filter.togglePlatform(p)} aria-pressed={filter.platforms.includes(p)}>
        <PlatformGlyph platform={p} label />
      </button>
    {/each}
  </div>

  <div class="ml-auto flex items-center gap-1 rounded-md border border-zinc-200 p-0.5 dark:border-zinc-700" role="group" aria-label="Date range">
    {#each presets as [k, label]}
      <button class="rounded px-2 py-1 {filter.preset === k ? 'bg-zinc-100 font-medium dark:bg-zinc-800' : 'text-zinc-500'}" onclick={() => filter.setPreset(k)}>{label}</button>
    {/each}
    <button class="rounded px-2 py-1 {filter.preset === 'custom' ? 'bg-zinc-100 font-medium dark:bg-zinc-800' : 'text-zinc-500'}" onclick={() => { customOpen = !customOpen; cf = filter.from; ct = filter.to; }}>Custom</button>
  </div>
  <span class="text-xs text-zinc-400">{filter.from} → {filter.to}</span>

  {#if customOpen}
    <div class="flex w-full items-center gap-2 border-t border-zinc-100 pt-2 dark:border-zinc-800">
      <input type="date" class="input w-auto" bind:value={cf} />
      <span>→</span>
      <input type="date" class="input w-auto" bind:value={ct} />
      <button class="btn-primary" onclick={() => { if (cf && ct) { filter.setCustom(cf, ct); customOpen = false; } }}>Apply</button>
    </div>
  {/if}
</div>
