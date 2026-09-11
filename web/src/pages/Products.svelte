<script>
  import { onMount } from 'svelte';
  import { link } from 'svelte-spa-router';
  import { api, ApiError } from '../lib/api.js';
  import { session } from '../lib/session.svelte.js';
  import { toasts } from '../lib/toast.svelte.js';
  import { fmtCompact, fmtRating, ago, PLATFORM_LABEL } from '../lib/format.js';
  import PlatformGlyph from '../lib/components/PlatformGlyph.svelte';
  import StoreBadge from '../lib/components/StoreBadge.svelte';
  import Modal from '../lib/components/Modal.svelte';
  import Skeleton from '../lib/components/Skeleton.svelte';
  import EmptyState from '../lib/components/EmptyState.svelte';
  import Icon from '../lib/components/Icon.svelte';
  import ProductPicker from '../lib/components/ProductPicker.svelte';
  import IgnoreDialog from '../lib/components/IgnoreDialog.svelte';

  let products = $state(null);
  let suggestions = $state([]);
  let editOpen = $state(false);
  let editing = $state(null);
  let form = $state({ name: '', icon_url: '', description: '' });
  let fields = $state({});
  let pickerFor = $state(null);
  let ignoreFor = $state(null);

  async function load() {
    const [p, s] = await Promise.all([api.get('/products'), session.isAdmin ? api.get('/products-suggestions') : Promise.resolve([])]);
    products = p; suggestions = s;
  }
  onMount(load);

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
  {#if session.isAdmin}<button class="btn-primary" onclick={openCreate}><Icon name="plus" />New product</button>{/if}
</div>

{#if !products}<Skeleton rows={5} />
{:else}
  {#if session.isAdmin && suggestions.length}
    <section class="card mb-4">
      <h2 class="mb-1 font-medium">Unassigned store apps <span class="badge bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-200">{suggestions.length}</span></h2>
      <p class="mb-3 text-xs text-zinc-500">Data from these apps is not counted anywhere until you link them. Nothing is linked automatically.</p>
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
    </section>
  {/if}

  {#if products.length === 0}
    <EmptyState icon="products" title="No products yet" message="Run a full sync to discover your store apps, then link them into products here." />
  {:else}
    <div class="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
      {#each products as p (p.product.id)}
        <div class="card flex flex-col">
          <div class="flex items-start gap-3">
            {#if p.product.icon_url}<img src={p.product.icon_url} alt="" class="h-10 w-10 rounded-lg" />{:else}<div class="grid h-10 w-10 place-items-center rounded-lg bg-zinc-100 text-zinc-400 dark:bg-zinc-800"><Icon name="products" /></div>{/if}
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
            <div><div class="text-lg font-semibold tabular-nums">{p.apps.some((a) => a.rating_avg) ? fmtRating(p.apps.reduce((s, a) => s + (a.rating_avg || 0), 0) / p.apps.filter((a) => a.rating_avg).length) : '—'}</div><div class="text-xs text-zinc-500">rating</div></div>
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
