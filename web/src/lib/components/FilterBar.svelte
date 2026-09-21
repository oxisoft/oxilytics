<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { filter } from '../filter.svelte.js';
  import { session } from '../session.svelte.js';
  import ProductSelect from './ProductSelect.svelte';
  import SelectMenu from './SelectMenu.svelte';
  import { PLATFORM_LABEL, STORE_PLATFORMS } from '../format.js';

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

  // One platform or all of them — not an arbitrary combination. The old row of
  // toggle buttons allowed subsets nobody asked for and took most of the bar.
  const platformOptions = $derived([
    { id: '', name: 'All platforms' },
    ...platforms.map((p) => ({ id: p, name: PLATFORM_LABEL[p] || p, glyph: p })),
  ]);

  const presets = [
    ['7d', '7 days'],
    ['30d', '30 days'],
    ['90d', '90 days'],
    ['12m', '12 months'],
    ['ytd', 'Year to date'],
  ];
  const periodOptions = [...presets.map(([id, name]) => ({ id, name })), { id: 'custom', name: 'Custom range…' }];

  onMount(async () => {
    filter.load();
    try { products = await api.get('/products'); } catch {}
    if (!session.setup) { try { session.setup = await api.get('/setup/status'); } catch {} }
  });

  function pickPeriod(id) {
    if (id === 'custom') {
      cf = filter.from;
      ct = filter.to;
      customOpen = true;
      return;
    }
    customOpen = false;
    filter.setPreset(id);
  }
</script>

<div class="mb-4">
  <!-- The resolved dates belong above the controls: they are the result of the
       selection, not another control competing for width. -->
  <div class="mb-1 px-1 text-[11px] leading-none text-zinc-400 dark:text-zinc-500">
    {filter.from} → {filter.to}
  </div>

  <div class="flex flex-wrap items-center gap-2 rounded-lg border border-zinc-200 bg-white px-3 py-2 text-sm dark:border-zinc-800 dark:bg-zinc-900">
    {#if showProduct}
      <ProductSelect {products} value={filter.product} onchange={(id) => filter.setProduct(id)} />
    {/if}

    <SelectMenu
      options={platformOptions}
      value={filter.platform}
      onchange={(id) => filter.setPlatform(id)}
      label="Platform"
      minWidth="min-w-40"
      listWidth="w-48"
    />

    <div class="ml-auto">
      <SelectMenu
        options={periodOptions}
        value={filter.preset}
        onchange={pickPeriod}
        label="Period"
        minWidth="min-w-40"
        listWidth="w-48"
      />
    </div>

    {#if customOpen}
      <div class="flex w-full items-center gap-2 border-t border-zinc-100 pt-2 dark:border-zinc-800">
        <input type="date" class="input w-auto" bind:value={cf} />
        <span>→</span>
        <input type="date" class="input w-auto" bind:value={ct} />
        <button class="btn-primary" onclick={() => { if (cf && ct) { filter.setCustom(cf, ct); customOpen = false; } }}>Apply</button>
        <button class="text-zinc-500" onclick={() => (customOpen = false)}>Cancel</button>
      </div>
    {/if}
  </div>
</div>
