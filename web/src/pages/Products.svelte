<script>
  import { onMount } from 'svelte';
  import { link, querystring, replace } from 'svelte-spa-router';
  import { api, ApiError, qs } from '../lib/api.js';
  import { filter } from '../lib/filter.svelte.js';
  import { session } from '../lib/session.svelte.js';
  import { toasts } from '../lib/toast.svelte.js';
  import { fmtCompact, fmtRating, ago, PLATFORM_LABEL, productIcon } from '../lib/format.js';
  import PlatformGlyph from '../lib/components/PlatformGlyph.svelte';
  import StoreBadge from '../lib/components/StoreBadge.svelte';
  import Modal from '../lib/components/Modal.svelte';
  import Skeleton from '../lib/components/Skeleton.svelte';
  import EmptyState from '../lib/components/EmptyState.svelte';
  import FilterBar from '../lib/components/FilterBar.svelte';
  import Icon from '../lib/components/Icon.svelte';
  import ProductPicker from '../lib/components/ProductPicker.svelte';
  import IgnoreDialog from '../lib/components/IgnoreDialog.svelte';
  import IgnoredAppsTable from '../lib/components/IgnoredAppsTable.svelte';

  let products = $state(null);
  let suggestions = $state([]);
  let ignored = $state(null);
  let editOpen = $state(false);
  let editing = $state(null);
  let form = $state({ name: '', icon_url: '', description: '' });
  let fields = $state({});
  let pickerFor = $state(null);
  let ignoreFor = $state(null);

  const TABS = ['products', 'unassigned', 'ignored'];
  // The tab lives in the URL so a reload, a bookmark or a link from elsewhere
  // (Settings used to own the ignored list) all land on the right one.
  const tab = $derived.by(() => {
    const t = new URLSearchParams($querystring || '').get('tab');
    return TABS.includes(t) ? t : 'products';
  });
  function setTab(t) { replace('/products' + (t === 'products' ? '' : '?tab=' + t)); }

  async function load() {
    const [p, s, ig] = await Promise.all([
      // The totals on this page are range-scoped like everywhere else; without
      // passing the filter they silently used the API default (last 30 days)
      // while the page showed no date control at all, so the numbers looked
      // like lifetime figures and disagreed with every other screen.
      api.get('/products' + qs(filter.params)),
      session.isAdmin ? api.get('/products-suggestions') : Promise.resolve([]),
      session.isAdmin ? api.get('/apps-ignored') : Promise.resolve([]),
    ]);
    products = p; suggestions = s; ignored = ig;
  }
  onMount(load);

  // Reload when the shared filter changes.
  //
  // This must NOT read `products`: load() reassigns it, so touching it here
  // makes the effect re-trigger itself forever — a request storm that starves
  // every other page in the SPA. Track only the filter fields, and use a plain
  // (non-reactive) flag to skip the initial run that onMount already covers.
  let filterKey = null;
  $effect(() => {
    const key = [filter.from, filter.to, filter.platform, filter.product].join('|');
    if (filterKey === null) { filterKey = key; return; }  // onMount already loaded
    if (key === filterKey) return;                         // nothing actually changed
    filterKey = key;
    load();
  });

  function openCreate() { editing = null; form = { name: '', icon_url: '', description: '' }; fields = {}; editOpen = true; }
  function openEdit(p) { editing = p; form = { name: p.name, icon_url: p.icon_url || '', description: p.description || '' }; fields = {}; editOpen = true; }

  async function save(e) {
    e.preventDefault(); fields = {};
    const body = { name: form.name, icon_url: form.icon_url || null, description: form.description || null };
    try {
      if (editing) await api.put('/products/' + editing.id, body); else await api.post('/products', body);
      editOpen = false; toasts.success('Saved'); load();
    } catch (err) { if (err instanceof ApiError && Object.keys(err.fields).length) fields = err.fields; else toasts.error(err.message); }
  }

  async function archive(p, archived) {
    await api.put('/products/' + p.id, { archived });
    toasts.success(archived ? 'Archived' : 'Restored'); load();
  }

  async function accept(s, productId) {
    try { await api.post('/products-suggestions/accept', { app_id: s.app.id, product_id: productId ?? null }); toasts.success('Linked'); load(); }
    catch (err) { toasts.error(err.message); }
  }
</script>

<div class="mb-4 flex items-center justify-between">
  <h1 class="text-lg font-semibold">Products</h1>
  {#if session.isAdmin && tab === 'products'}<button class="btn-primary" onclick={openCreate}><Icon name="plus" />New product</button>{/if}
</div>

<!-- Only the Products tab shows range-scoped numbers; Unassigned and Ignored
     are inventories, where a date filter would be misleading. -->
{#if tab === 'products'}
  <FilterBar showProduct={false} />
{/if}

{#if session.isAdmin}
  <nav class="mb-4 flex gap-1 border-b border-zinc-200 dark:border-zinc-800" aria-label="Products">
    {#each [['products', 'Products', products?.length ?? 0], ['unassigned', 'Unassigned', suggestions.length], ['ignored', 'Ignored', ignored?.length ?? 0]] as [id, label, count]}
      <button
        class="-mb-px flex items-center gap-2 border-b-2 px-3 py-2 text-sm {tab === id ? 'border-brand-600 font-medium' : 'border-transparent text-zinc-500 hover:text-zinc-800 dark:hover:text-zinc-200'}"
        aria-current={tab === id ? 'page' : undefined}
        onclick={() => setTab(id)}
      >{label}{#if count}<span class="badge {id === 'unassigned' ? 'bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-200' : 'bg-zinc-100 text-zinc-600 dark:bg-zinc-800 dark:text-zinc-300'}">{count}</span>{/if}</button>
    {/each}
  </nav>
{/if}

{#if !products}<Skeleton rows={5} />
{:else if tab === 'unassigned'}
  {#if !suggestions.length}
    <EmptyState icon="check" title="Nothing unassigned" message="Every store listing is either linked to a product or ignored." />
  {:else}
    <p class="mb-3 text-xs text-zinc-500">Data from these apps is not counted anywhere until you link them. Nothing is linked automatically.</p>
    <div class="card">
      <ul class="divide-y divide-zinc-100 dark:divide-zinc-800">
        {#each suggestions as s (s.app.id)}
          <li class="flex flex-wrap items-center gap-3 py-2">
            {#if s.app.icon_url}<img src={s.app.icon_url} alt="" class="h-8 w-8 rounded" />{/if}
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2 text-sm font-medium"><PlatformGlyph platform={s.app.platform} />{s.app.name}<StoreBadge store={s.app.store} /></div>
              <div class="truncate text-xs text-zinc-500">{s.app.store_app_id}</div>
            </div>
            {#if s.suggested_product}
              <button class="btn-primary" onclick={() => accept(s, s.suggested_product.id)}><Icon name="link" />Link to {s.suggested_product.name}</button>
            {:else}
              <button class="btn-primary" onclick={() => accept(s, null)}><Icon name="plus" />Create “{s.proposed_name}”</button>
            {/if}
            <button class="btn-secondary" onclick={() => (pickerFor = s.app)}>Pick product…</button>
            <button class="btn-secondary" title="Ignore this listing" onclick={() => (ignoreFor = s.app)}><Icon name="ban" /></button>
          </li>
        {/each}
      </ul>
    </div>
  {/if}
{:else if tab === 'ignored'}
  <IgnoredAppsTable rows={ignored} onchange={load} />
{:else if products.length === 0}
  <EmptyState icon="products" title="No products yet" message="Run a full sync to discover your store apps, then link them into products here." />
{:else}
  <div class="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
    {#each products as p (p.product.id)}
      <div class="card flex flex-col">
        <div class="flex items-start gap-3">
          {#if productIcon(p)}<img src={productIcon(p)} alt="" class="h-10 w-10 rounded-lg" />{:else}<div class="grid h-10 w-10 place-items-center rounded-lg bg-zinc-100 text-zinc-400 dark:bg-zinc-800"><Icon name="products" /></div>{/if}
          <div class="min-w-0 flex-1">
            <a href="/products/{p.product.slug}" use:link class="block truncate font-medium hover:underline">{p.product.name}</a>
            <div class="mt-0.5 flex gap-1">
              {#each ['ios', 'android', 'windows'] as plat}
                {@const a = p.apps.find((x) => x.platform === plat || (plat === 'ios' && x.platform === 'macos'))}
                <PlatformGlyph platform={plat} dim={!a} />
              {/each}
            </div>
          </div>
          {#if session.isAdmin}
            <button class="rounded p-1 text-zinc-400 hover:bg-zinc-100 dark:hover:bg-zinc-800" onclick={() => openEdit(p.product)} aria-label="Edit"><Icon name="edit" /></button>
          {/if}
        </div>
        <div class="mt-3 grid grid-cols-3 gap-2 text-center">
          <div><div class="text-lg font-semibold tabular-nums">{fmtCompact(p.totals.downloads)}</div><div class="text-xs text-zinc-500">downloads</div></div>
          <div><div class="text-lg font-semibold tabular-nums">{fmtCompact(p.totals.crashes)}</div><div class="text-xs text-zinc-500">crashes</div></div>
          <div><div class="text-lg font-semibold tabular-nums">{p.apps.some((a) => a.rating_avg) ? fmtRating(p.apps.reduce((s, a) => s + (a.rating_avg || 0), 0) / p.apps.filter((a) => a.rating_avg).length) : '—'}</div><div class="text-xs text-zinc-500">rating<span class="text-zinc-400"> · all time</span></div></div>
        </div>
        <div class="mt-2 space-y-0.5 text-xs text-zinc-500">
          {#each p.apps as a}
            <div class="flex items-center gap-1"><PlatformGlyph platform={a.platform} class="h-3 w-3" /><span>{PLATFORM_LABEL[a.platform]}</span><span class="ml-auto tabular-nums">{fmtCompact(p.by_platform[a.platform]?.downloads ?? 0)}</span><span class="w-16 text-right">{ago(a.last_synced_at)}</span></div>
          {/each}
        </div>
      </div>
    {/each}
  </div>
{/if}

<Modal bind:open={editOpen} title={editing ? 'Edit product' : 'New product'}>
  <form class="space-y-3" onsubmit={save}>
    <div><label class="label" for="p-name">Name</label><input id="p-name" class="input" bind:value={form.name} required />{#if fields.name}<p class="text-xs text-red-600">{fields.name}</p>{/if}</div>
    <div><label class="label" for="p-icon">Icon URL <span class="text-zinc-400">(optional, defaults to the first store app icon)</span></label><input id="p-icon" class="input" bind:value={form.icon_url} /></div>
    <div><label class="label" for="p-desc">Description</label><textarea id="p-desc" class="input" rows="3" bind:value={form.description}></textarea></div>
    <div class="flex items-center justify-between">
      {#if editing}<button type="button" class="text-xs text-zinc-500 underline" onclick={() => { archive(editing, !editing.archived); editOpen = false; }}>{editing.archived ? 'Unarchive' : 'Archive'}</button>{:else}<span></span>{/if}
      <div class="flex gap-2"><button type="button" class="btn-secondary" onclick={() => (editOpen = false)}>Cancel</button><button class="btn-primary">{editing ? 'Save' : 'Create'}</button></div>
    </div>
  </form>
</Modal>

<ProductPicker app={pickerFor} onclose={() => (pickerFor = null)} onlinked={() => { pickerFor = null; load(); }} />
<IgnoreDialog app={ignoreFor} onclose={() => (ignoreFor = null)} ondone={() => { ignoreFor = null; load(); }} />
