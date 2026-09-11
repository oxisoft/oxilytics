<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';
  import { toasts } from '../lib/toast.svelte.js';
  import { fmtDateTime, fmtNum } from '../lib/format.js';
  import SettingsNav from '../lib/components/SettingsNav.svelte';
  import PlatformGlyph from '../lib/components/PlatformGlyph.svelte';
  import StoreBadge from '../lib/components/StoreBadge.svelte';
  import Skeleton from '../lib/components/Skeleton.svelte';
  import EmptyState from '../lib/components/EmptyState.svelte';
  import Icon from '../lib/components/Icon.svelte';

  let rows = $state(null);
  let editing = $state(null);
  let reason = $state('');

  async function load() { rows = await api.get('/apps-ignored'); }
  onMount(load);

  async function restore(a) {
    await api.post('/apps/' + a.id + '/restore');
    toasts.success(a.name + ' restored — it is unassigned again'); load();
  }
  async function saveReason(a) {
    await api.put('/apps/' + a.id + '/ignore-reason', { reason: reason || null });
    editing = null; load();
  }
</script>

<SettingsNav />
<h1 class="mb-1 text-lg font-semibold">Ignored apps</h1>
<p class="mb-4 text-sm text-zinc-500">Listings hidden from every screen and skipped by sync. Data is kept; restore returns them as unassigned.</p>

{#if !rows}<Skeleton rows={4} />
{:else if !rows.length}<EmptyState icon="ban" title="Nothing ignored" message="Use “Ignore” on a store app to hide listings you don't care about." />
{:else}
  <div class="card overflow-x-auto p-0">
    <table class="table">
      <thead><tr><th>App</th><th>Store</th><th>Platform</th><th>Identifier</th><th>Reason</th><th>Ignored by</th><th>When</th><th>Last data</th><th class="text-right">Rows kept</th><th></th></tr></thead>
      <tbody>
        {#each rows as a (a.id)}
          <tr>
            <td class="font-medium">{a.name}</td>
            <td><StoreBadge store={a.store} /></td>
            <td><PlatformGlyph platform={a.platform} label /></td>
            <td class="font-mono text-xs">{a.store_app_id}</td>
            <td>
              {#if editing === a.id}
                <form class="flex gap-1" onsubmit={(e) => { e.preventDefault(); saveReason(a); }}><input class="input w-40" bind:value={reason} /><button class="btn-primary">Save</button></form>
              {:else}
                <span class="text-sm">{a.ignored_reason || '—'}</span> <button class="text-xs text-zinc-400 hover:underline" onclick={() => { editing = a.id; reason = a.ignored_reason || ''; }}>edit</button>
              {/if}
            </td>
            <td class="text-xs">{a.ignored_by_name || '—'}</td>
            <td class="text-xs text-zinc-500">{fmtDateTime(a.ignored_at)}</td>
            <td class="text-xs font-mono">{a.last_data_day || '—'}</td>
            <td class="text-right text-xs tabular-nums">{fmtNum(a.metric_rows)} / {fmtNum(a.review_rows)}</td>
            <td class="text-right"><button class="btn-secondary" onclick={() => restore(a)}><Icon name="restore" />Restore</button></td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
{/if}
