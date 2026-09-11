<script>
  import { onMount } from 'svelte';
  import { link } from 'svelte-spa-router';
  import { api, qs } from '../lib/api.js';
  import { session } from '../lib/session.svelte.js';
  import { toasts } from '../lib/toast.svelte.js';
  import { fmtCompact, fmtDate, ago, PLATFORM_LABEL } from '../lib/format.js';
  import PlatformGlyph from '../lib/components/PlatformGlyph.svelte';
  import StoreBadge from '../lib/components/StoreBadge.svelte';
  import Skeleton from '../lib/components/Skeleton.svelte';
  import EmptyState from '../lib/components/EmptyState.svelte';
  import Modal from '../lib/components/Modal.svelte';
  import Icon from '../lib/components/Icon.svelte';
  import ProductPicker from '../lib/components/ProductPicker.svelte';
  import IgnoreDialog from '../lib/components/IgnoreDialog.svelte';

  let apps = $state(null);
  let products = $state({});
  let store = $state('');
  let platform = $state('');
  let unassignedOnly = $state(false);
  let search = $state('');
  let selected = $state({});
  let pickerApp = $state(null);
  let ignoreTarget = $state(null);
  let editApp = $state(null);
  let editForm = $state({ name: '', icon_url: '' });

  async function load() {
    const [a, p] = await Promise.all([api.get('/apps' + qs({ store, platform, unassigned: unassignedOnly ? 1 : undefined })), api.get('/products?archived=1')]);
    apps = a; products = Object.fromEntries(p.map((x) => [x.product.id, x.product])); selected = {};
  }
  onMount(load);
  $effect(() => { store; platform; unassignedOnly; load(); });

  const visible = $derived((apps || []).filter((a) => !search || a.name.toLowerCase().includes(search.toLowerCase()) || a.store_app_id.toLowerCase().includes(search.toLowerCase())));
  const selectedApps = $derived(visible.filter((a) => selected[a.id]));

  async function saveEdit(e) {
    e.preventDefault();
    try { await api.put('/apps/' + editApp.id, { name: editForm.name, icon_url: editForm.icon_url || null }); editApp = null; toasts.success('Saved'); load(); }
    catch (err) { toasts.error(err.message); }
  }
</script>

<div class="mb-4 flex items-center justify-between">
  <h1 class="text-lg font-semibold">Store apps</h1>
  <a href="/products" use:link class="text-sm text-zinc-500 hover:underline">Products →</a>
</div>

<div class="mb-3 flex flex-wrap items-center gap-2 text-sm">
  <div class="relative"><Icon name="search" class="absolute top-2 left-2 h-4 w-4 text-zinc-400" /><input class="input w-56 pl-8" placeholder="Search…" bind:value={search} /></div>
  <select class="input w-auto" bind:value={store}><option value="">All stores</option><option value="appstore">App Store</option><option value="googleplay">Google Play</option></select>
  <select class="input w-auto" bind:value={platform}><option value="">All platforms</option>{#each Object.entries(PLATFORM_LABEL) as [k, v]}<option value={k}>{v}</option>{/each}</select>
  <label class="flex items-center gap-1"><input type="checkbox" bind:checked={unassignedOnly} />Unassigned only</label>
  {#if session.isAdmin && selectedApps.length}
    <button class="btn-danger ml-auto" onclick={() => (ignoreTarget = selectedApps)}><Icon name="ban" />Ignore {selectedApps.length} selected</button>
  {/if}
</div>

{#if !apps}<Skeleton rows={5} />
{:else if !visible.length}<EmptyState icon="products" title="No store apps" message={apps.length ? 'Nothing matches the filter.' : 'Run a sync to discover the apps in your store accounts.'} />
{:else}
  <div class="card overflow-x-auto p-0">
    <table class="table">
      <thead><tr>{#if session.isAdmin}<th><input type="checkbox" aria-label="Select all" checked={selectedApps.length === visible.length} onchange={(e) => { selected = Object.fromEntries(visible.map((a) => [a.id, e.target.checked])); }} /></th>{/if}<th>App</th><th>Store</th><th>Platform</th><th>Identifier</th><th>Product</th><th>First seen</th><th>Last synced</th><th class="text-right">Downloads (30d)</th>{#if session.isAdmin}<th></th>{/if}</tr></thead>
      <tbody>
        {#each visible as a (a.id)}
          <tr>
            {#if session.isAdmin}<td><input type="checkbox" bind:checked={selected[a.id]} aria-label="Select {a.name}" /></td>{/if}
            <td class="flex items-center gap-2 font-medium">{#if a.icon_url}<img src={a.icon_url} alt="" class="h-6 w-6 rounded" />{/if}{a.name}</td>
            <td><StoreBadge store={a.store} /></td>
            <td><PlatformGlyph platform={a.platform} label /></td>
            <td class="font-mono text-xs">{a.store_app_id}</td>
            <td>{#if a.product_id}<a href="/products/{products[a.product_id]?.slug}" use:link class="hover:underline">{products[a.product_id]?.name || '#' + a.product_id}</a>{:else}<span class="text-amber-600">— unassigned —</span>{#if a.suggested_product_id && products[a.suggested_product_id]}<div class="text-xs text-zinc-400">looks like {products[a.suggested_product_id].name}</div>{/if}{/if}</td>
            <td class="text-xs text-zinc-500">{fmtDate(a.first_seen_at)}</td>
            <td class="text-xs text-zinc-500">{ago(a.last_synced_at)}</td>
            <td class="text-right tabular-nums">{fmtCompact(a.downloads)}</td>
            {#if session.isAdmin}
              <td class="text-right whitespace-nowrap">
                <button class="rounded p-1 hover:bg-zinc-200 dark:hover:bg-zinc-700" title="Edit name / icon" onclick={() => { editApp = a; editForm = { name: a.name, icon_url: a.icon_url || '' }; }}><Icon name="edit" /></button>
                {#if !a.product_id}<button class="rounded p-1 hover:bg-zinc-200 dark:hover:bg-zinc-700" title="Link to product" onclick={() => (pickerApp = a)}><Icon name="link" /></button>{/if}
                <button class="rounded p-1 text-red-600 hover:bg-red-50 dark:hover:bg-red-950" title="Ignore" onclick={() => (ignoreTarget = a)}><Icon name="ban" /></button>
              </td>
            {/if}
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
{/if}

<Modal open={!!editApp} title="Edit store app" onclose={() => (editApp = null)}>
  <form class="space-y-3" onsubmit={saveEdit}>
    <div><label class="label" for="ea-name">Display name</label><input id="ea-name" class="input" bind:value={editForm.name} required /></div>
    <div><label class="label" for="ea-icon">Icon URL</label><input id="ea-icon" class="input" bind:value={editForm.icon_url} /></div>
    <div class="flex justify-end gap-2"><button type="button" class="btn-secondary" onclick={() => (editApp = null)}>Cancel</button><button class="btn-primary">Save</button></div>
  </form>
</Modal>
<ProductPicker app={pickerApp} onclose={() => (pickerApp = null)} onlinked={() => { pickerApp = null; load(); }} />
<IgnoreDialog app={ignoreTarget} onclose={() => (ignoreTarget = null)} ondone={() => { ignoreTarget = null; load(); }} />
