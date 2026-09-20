<script>
  import { onMount, onDestroy } from 'svelte';
  import { link } from 'svelte-spa-router';
  import { api, qs } from '../lib/api.js';
  import { session } from '../lib/session.svelte.js';
  import { toasts } from '../lib/toast.svelte.js';
  import { fmtDateTime, fmtDuration, fmtNum, ago, STORE_LABEL } from '../lib/format.js';
  import ProgressBar from '../lib/components/ProgressBar.svelte';
  import ConfirmDialog from '../lib/components/ConfirmDialog.svelte';
  import Skeleton from '../lib/components/Skeleton.svelte';
  import Banner from '../lib/components/Banner.svelte';
  import Icon from '../lib/components/Icon.svelte';

  let status = $state(null);
  let runs = $state(null);
  let settings = $state(null);
  let fullFor = $state(null);
  let resetFor = $state(null);
  let timer;

  async function load() {
    try {
      const [s, r] = await Promise.all([api.get('/sync/status'), api.get('/sync/runs')]);
      status = s; runs = r;
      if (!settings) settings = await api.get('/settings').catch(() => ({}));
    } catch {}
  }
  onMount(() => { load(); timer = setInterval(load, 3000); });
  onDestroy(() => clearInterval(timer));

  async function start(store, mode) {
    try { await api.post('/sync/runs', { store, mode }); toasts.success((mode === 'full' ? 'Full' : 'Delta') + ' sync started'); load(); }
    catch (e) { toasts.error(e.message); }
  }
  async function cancel(run) {
    try { await api.post('/sync/runs/' + run.id + '/cancel'); toasts.success('Cancelling…'); }
    catch (e) { toasts.error(e.message); }
  }
  async function reset(store) {
    await api.post('/sync/reset', { store, confirm: store });
    toasts.success('Data for ' + STORE_LABEL[store] + ' wiped'); load();
  }

  const statusCls = { succeeded: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300', failed: 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300', running: 'bg-brand-50 text-brand-700 dark:bg-brand-700/30 dark:text-brand-50', cancelled: 'bg-zinc-100 text-zinc-600 dark:bg-zinc-800', interrupted: 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300', queued: 'bg-zinc-100 text-zinc-600 dark:bg-zinc-800' };
</script>

<h1 class="mb-4 text-lg font-semibold">Sync</h1>

{#if !status}<Skeleton rows={6} />
{:else}
  {#if status.unassigned_apps > 0}
    <div class="mb-3"><Banner kind="info" message="{status.unassigned_apps} store app{status.unassigned_apps > 1 ? 's' : ''} discovered by sync need{status.unassigned_apps > 1 ? '' : 's'} a product — link or ignore them on the Products screen." /></div>
  {/if}
  <div class="mb-4 grid gap-4 lg:grid-cols-2">
    {#each ['appstore', 'googleplay'] as st}
      {@const s = status.stores[st]}
      <div class="card">
        <div class="mb-2 flex items-center justify-between">
          <h2 class="flex items-center gap-2 font-medium"><Icon name={st === 'appstore' ? 'apple' : 'android'} />{STORE_LABEL[st]}</h2>
          {#if !s.configured}<a href="/setup/{st}" use:link class="text-sm text-brand-600 hover:underline">Set up →</a>{/if}
        </div>
        {#if !s.configured}
          <p class="text-sm text-zinc-500">Not configured. Follow the guide to add credentials.</p>
        {:else}
          {#if s.running}
            <div class="mb-3 rounded-md bg-brand-50 p-3 dark:bg-brand-700/20">
              <div class="mb-1 flex items-center gap-2 text-sm"><Icon name="sync" class="h-4 w-4 animate-spin" />Running <b>{s.running.mode}</b> sync · {s.running.apps_done}/{s.running.apps_total} apps · {fmtDuration(s.running.started_at)}</div>
              <ProgressBar value={s.running.apps_done} max={s.running.apps_total || 1} />
              <div class="mt-2 flex items-center gap-2 text-xs text-zinc-600 dark:text-zinc-300"><span>{fmtNum(s.running.rows_metrics)} metric rows · {fmtNum(s.running.rows_reviews)} reviews</span><a href="/sync/runs/{s.running.id}" use:link class="underline">log</a>{#if session.isAdmin}<button class="ml-auto btn-secondary" onclick={() => cancel(s.running)}><Icon name="stop" />Cancel</button>{/if}</div>
            </div>
          {/if}
          <dl class="mb-3 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-sm">
            <dt class="text-zinc-500">Last run</dt>
            <dd>{#if s.last}<span class="badge {statusCls[s.last.status]}">{s.last.status}</span> <span class="text-zinc-500">{s.last.mode} · {ago(s.last.finished_at || s.last.started_at)} · {fmtDuration(s.last.started_at, s.last.finished_at)} · {fmtNum(s.last.rows_metrics)} rows</span>{#if s.last.error}<div class="mt-0.5 text-xs text-red-600">{s.last.error} <a href="/sync/runs/{s.last.id}" use:link class="underline whitespace-nowrap">view log</a></div>{/if}{:else}<span class="text-zinc-400">never</span>{/if}</dd>
            <dt class="text-zinc-500">Next scheduled</dt>
            <dd>{status.next_scheduled ? fmtDateTime(status.next_scheduled) : 'disabled'}{#if settings && !(settings['sync.schedule.stores'] || '').includes(st)}<span class="text-xs text-zinc-400"> (this store excluded)</span>{/if}</dd>
          </dl>
          {#if session.isAdmin}
            <div class="flex flex-wrap gap-2">
              <button class="btn-primary" disabled={!!s.running} onclick={() => start(st, 'delta')}><Icon name="play" />Sync now</button>
              <button class="btn-secondary" disabled={!!s.running} onclick={() => (fullFor = st)}>Full sync…</button>
              <button class="btn-secondary text-red-600" disabled={!!s.running} onclick={() => (resetFor = st)}><Icon name="trash" />Reset data…</button>
            </div>
          {/if}
        {/if}
      </div>
    {/each}
  </div>

  <div class="card mb-4 flex flex-wrap items-center gap-3 text-sm">
    <Icon name="sync" class="text-zinc-400" />
    {#if settings?.['sync.schedule.enabled'] === 'true'}
      <span>Daily delta sync at <b>{settings['sync.schedule.time']}</b> (server time) for {(settings['sync.schedule.stores'] || '').split(',').filter(Boolean).map((x) => STORE_LABEL[x]).join(', ') || 'no stores'}.</span>
    {:else}<span>Automatic sync is disabled.</span>{/if}
    {#if session.isAdmin}<a href="/settings" use:link class="ml-auto underline">Change schedule</a>{/if}
  </div>

  <h2 class="mb-2 font-medium">History</h2>
  <div class="card overflow-x-auto p-0">
    <table class="table">
      <thead><tr><th>Started</th><th>Store</th><th>Mode</th><th>Trigger</th><th>Status</th><th>Duration</th><th class="text-right">Apps</th><th class="text-right">Metric rows</th><th class="text-right">Reviews</th><th>Error</th></tr></thead>
      <tbody>
        {#each runs || [] as r (r.id)}
          <tr>
            <td><a href="/sync/runs/{r.id}" use:link class="hover:underline">{fmtDateTime(r.started_at || r.created_at)}</a></td>
            <td>{STORE_LABEL[r.store]}</td><td>{r.mode}</td>
            <td class="text-xs">{r.trigger === 'schedule' ? 'schedule' : r.requested_by_name || 'manual'}</td>
            <td><span class="badge {statusCls[r.status]}">{r.status}</span></td>
            <td class="text-xs">{fmtDuration(r.started_at, r.finished_at)}</td>
            <td class="text-right tabular-nums">{r.apps_done}/{r.apps_total}</td>
            <td class="text-right tabular-nums">{fmtNum(r.rows_metrics)}</td>
            <td class="text-right tabular-nums">{fmtNum(r.rows_reviews)}</td>
            <td class="max-w-xs text-xs text-red-600">{#if r.error}<a href="/sync/runs/{r.id}" use:link class="line-clamp-2 hover:underline" title={r.error}>{r.error}</a>{/if}</td>
          </tr>
        {:else}<tr><td colspan="10" class="py-8 text-center text-zinc-400">No runs yet</td></tr>{/each}
      </tbody>
    </table>
  </div>
{/if}

<ConfirmDialog open={!!fullFor} title="Start a full sync?" message="A full sync re-fetches every day since the account started and re-upserts all data. On App Store Connect the first full run waits for Apple to generate a snapshot report, which can take hours. Existing data is not deleted." confirmLabel="Start full sync" onconfirm={() => start(fullFor, 'full')} onclose={() => (fullFor = null)} />
<ConfirmDialog open={!!resetFor} title="Reset {STORE_LABEL[resetFor] || ''} data" message="This deletes all metrics, reviews and checkpoints for this store. Apps, products and links stay. You will need a full sync afterwards." confirmLabel="Wipe data" danger typeToConfirm={resetFor || ''} onconfirm={() => reset(resetFor)} onclose={() => (resetFor = null)} />
