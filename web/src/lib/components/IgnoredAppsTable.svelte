<script>
  // Ignored listings with restore + reason editing. Lives in its own component
  // because it is shown both as a Products tab and (historically) under
  // Settings; one copy keeps the two from drifting.
  import { api } from '../api.js';
  import { toasts } from '../toast.svelte.js';
  import { fmtDateTime, fmtNum } from '../format.js';
  import PlatformGlyph from './PlatformGlyph.svelte';
  import StoreBadge from './StoreBadge.svelte';
  import Skeleton from './Skeleton.svelte';
  import EmptyState from './EmptyState.svelte';
  import Icon from './Icon.svelte';

  let { rows = null, onchange = () => {} } = $props();

  let editing = $state(null);
  let reason = $state('');
  let busy = $state(null);

  async function restore(a) {
    busy = a.id;
    try {
      await api.post('/apps/' + a.id + '/restore');
      toasts.success(a.name + ' restored — find it under Unassigned');
      onchange();
    } catch (e) {
      toasts.error(e.message);
    } finally {
      busy = null;
    }
  }

  async function saveReason(a) {
    try {
      await api.put('/apps/' + a.id + '/ignore-reason', { reason: reason || null });
      editing = null;
      onchange();
    } catch (e) {
      toasts.error(e.message);
    }
  }
</script>

{#if !rows}<Skeleton rows={4} />
{:else if !rows.length}
  <EmptyState icon="ban" title="Nothing ignored" message="Use “Ignore” on an unassigned app to hide listings you don't care about. They stay here and can be restored at any time." />
{:else}
  <p class="mb-3 text-xs text-zinc-500">Hidden from every screen and skipped by sync. Existing data is kept, so restoring brings the history back with it.</p>
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
            <td class="font-mono text-xs">{a.last_data_day || '—'}</td>
            <td class="text-right text-xs tabular-nums">{fmtNum(a.metric_rows)} / {fmtNum(a.review_rows)}</td>
            <td class="text-right"><button class="btn-secondary" disabled={busy === a.id} onclick={() => restore(a)}><Icon name="restore" />{busy === a.id ? 'Restoring…' : 'Restore'}</button></td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
{/if}
