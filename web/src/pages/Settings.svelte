<script>
  import { onMount } from 'svelte';
  import { api, ApiError } from '../lib/api.js';
  import { session } from '../lib/session.svelte.js';
  import { toasts } from '../lib/toast.svelte.js';
  import { theme } from '../lib/theme.js';
  import Skeleton from '../lib/components/Skeleton.svelte';
  import SettingsNav from '../lib/components/SettingsNav.svelte';
  import { STORE_LABEL } from '../lib/format.js';

  let s = $state(null);
  let fields = $state({});
  let busy = $state(false);
  let themeMode = $state(theme.mode);

  const stores = $derived(Object.entries(session.setup?.stores || {}).filter(([, v]) => v.configured).map(([k]) => k));

  onMount(async () => {
    const [settings, setup] = await Promise.all([api.get('/settings'), api.get('/setup/status')]);
    session.setup = setup;
    s = { ...settings, _stores: (settings['sync.schedule.stores'] || '').split(',').filter(Boolean) };
  });

  async function save(e) {
    e.preventDefault();
    busy = true; fields = {};
    const body = {
      'sync.schedule.enabled': s['sync.schedule.enabled'],
      'sync.schedule.time': s['sync.schedule.time'],
      'sync.schedule.stores': s._stores.join(','),
      'sync.delta.overlap_days': s['sync.delta.overlap_days'],
      'metrics.retention_days': s['metrics.retention_days'],
      'ui.default_range_days': s['ui.default_range_days'],
      'products.suggest': s['products.suggest'],
    };
    try { await api.put('/settings', body); toasts.success('Settings saved'); }
    catch (err) { if (err instanceof ApiError && Object.keys(err.fields).length) fields = err.fields; else toasts.error(err.message); }
    finally { busy = false; }
  }

  function toggleStore(st) {
    s._stores = s._stores.includes(st) ? s._stores.filter((x) => x !== st) : [...s._stores, st];
  }
</script>

<SettingsNav />
<h1 class="mb-4 text-lg font-semibold">General</h1>

{#if !s}<Skeleton rows={6} />
{:else}
  <form class="grid gap-4 lg:grid-cols-2" onsubmit={save}>
    <section class="card space-y-3">
      <h2 class="font-medium">Automatic sync</h2>
      <label class="flex items-center gap-2 text-sm"><input type="checkbox" checked={s['sync.schedule.enabled'] === 'true'} onchange={(e) => (s['sync.schedule.enabled'] = e.target.checked ? 'true' : 'false')} />Run a delta sync every day</label>
      <div>
        <label class="label" for="time">Time of day</label>
        <input id="time" class="input max-w-[8rem]" type="time" bind:value={s['sync.schedule.time']} />
        {#if fields['sync.schedule.time']}<p class="text-xs text-red-600">{fields['sync.schedule.time']}</p>{/if}
        <p class="mt-1 text-xs text-zinc-500">Server timezone is set by <code>OXI_TZ</code>.</p>
      </div>
      <div>
        <span class="label">Stores</span>
        {#each ['appstore', 'googleplay'] as st}
          <label class="flex items-center gap-2 text-sm {stores.includes(st) ? '' : 'opacity-50'}"><input type="checkbox" checked={s._stores.includes(st)} disabled={!stores.includes(st)} onchange={() => toggleStore(st)} />{STORE_LABEL[st]}{#if !stores.includes(st)}<span class="text-xs text-zinc-400">(not configured)</span>{/if}</label>
        {/each}
        {#if fields['sync.schedule.stores']}<p class="text-xs text-red-600">{fields['sync.schedule.stores']}</p>{/if}
      </div>
    </section>

    <section class="card space-y-3">
      <h2 class="font-medium">Data</h2>
      <div><label class="label" for="ov">Delta overlap (days re-fetched on every delta run, 1–14)</label><input id="ov" class="input max-w-[6rem]" type="number" min="1" max="14" bind:value={s['sync.delta.overlap_days']} />{#if fields['sync.delta.overlap_days']}<p class="text-xs text-red-600">{fields['sync.delta.overlap_days']}</p>{/if}</div>
      <div><label class="label" for="ret">Metrics retention in days (0 = keep forever)</label><input id="ret" class="input max-w-[6rem]" type="number" min="0" bind:value={s['metrics.retention_days']} />{#if fields['metrics.retention_days']}<p class="text-xs text-red-600">{fields['metrics.retention_days']}</p>{/if}</div>
      <div><label class="label" for="rng">Default dashboard range (days)</label><input id="rng" class="input max-w-[6rem]" type="number" min="1" max="3650" bind:value={s['ui.default_range_days']} />{#if fields['ui.default_range_days']}<p class="text-xs text-red-600">{fields['ui.default_range_days']}</p>{/if}</div>
      <label class="flex items-center gap-2 text-sm"><input type="checkbox" checked={s['products.suggest'] === 'true'} onchange={(e) => (s['products.suggest'] = e.target.checked ? 'true' : 'false')} />Suggest a product for newly discovered store apps (linking stays manual)</label>
    </section>

    <section class="card space-y-3">
      <h2 class="font-medium">Appearance <span class="text-xs font-normal text-zinc-400">(this browser only)</span></h2>
      <select class="input max-w-[10rem]" bind:value={themeMode} onchange={() => theme.set(themeMode)}><option value="system">System</option><option value="light">Light</option><option value="dark">Dark</option></select>
    </section>

    <div class="lg:col-span-2"><button class="btn-primary" disabled={busy}>Save settings</button></div>
  </form>
{/if}
