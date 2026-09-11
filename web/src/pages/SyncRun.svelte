<script>
  import { onMount, onDestroy } from 'svelte';
  import { link } from 'svelte-spa-router';
  import { api } from '../lib/api.js';
  import { session } from '../lib/session.svelte.js';
  import { toasts } from '../lib/toast.svelte.js';
  import { fmtDateTime, fmtDuration, fmtNum, STORE_LABEL } from '../lib/format.js';
  import ProgressBar from '../lib/components/ProgressBar.svelte';
  import Skeleton from '../lib/components/Skeleton.svelte';
  import Icon from '../lib/components/Icon.svelte';

  let { params = {} } = $props();
  let run = $state(null);
  let logs = $state([]);
  let level = $state('');
  let autoScroll = $state(true);
  let box = $state(null);
  let timer;

  async function poll() {
    try {
      run = await api.get('/sync/runs/' + params.id);
      const after = logs.length ? logs[logs.length - 1].id : 0;
      const more = await api.get('/sync/runs/' + params.id + '/logs?after=' + after);
      if (more.length) { logs = [...logs, ...more]; if (autoScroll && box) requestAnimationFrame(() => (box.scrollTop = box.scrollHeight)); }
      if (['succeeded', 'failed', 'cancelled', 'interrupted'].includes(run.status)) clearInterval(timer);
    } catch {}
  }
  onMount(() => { poll(); timer = setInterval(poll, 3000); });
  onDestroy(() => clearInterval(timer));

  const stats = $derived.by(() => { try { return run?.stats ? JSON.parse(run.stats) : null; } catch { return null; } });
  const shown = $derived(level ? logs.filter((l) => l.level === level) : logs);
  const lvlCls = { error: 'text-red-600', warn: 'text-amber-600', info: 'text-zinc-700 dark:text-zinc-300', debug: 'text-zinc-400' };

  async function cancel() { try { await api.post('/sync/runs/' + run.id + '/cancel'); toasts.success('Cancelling…'); } catch (e) { toasts.error(e.message); } }
</script>

<a href="/sync" use:link class="mb-2 inline-flex items-center gap-1 text-sm text-zinc-500 hover:underline"><Icon name="chevron" class="h-3.5 w-3.5 rotate-180" />Sync</a>

{#if !run}<Skeleton rows={6} />
{:else}
  <div class="card mb-4">
    <div class="flex flex-wrap items-center gap-3">
      <h1 class="text-lg font-semibold">Run #{run.id} · {STORE_LABEL[run.store]} · {run.mode}</h1>
      <span class="badge {run.status === 'succeeded' ? 'bg-emerald-100 text-emerald-700' : run.status === 'failed' ? 'bg-red-100 text-red-700' : run.status === 'running' ? 'bg-brand-50 text-brand-700' : 'bg-zinc-100 text-zinc-600'}">{run.status}</span>
      {#if run.status === 'running' && session.isAdmin}<button class="btn-secondary ml-auto" onclick={cancel}><Icon name="stop" />Cancel</button>{/if}
    </div>
    <dl class="mt-3 grid grid-cols-2 gap-x-6 gap-y-1 text-sm md:grid-cols-4">
      <dt class="text-zinc-500">Trigger</dt><dd>{run.trigger === 'schedule' ? 'schedule' : run.requested_by_name || 'manual'}</dd>
      <dt class="text-zinc-500">Range</dt><dd class="font-mono text-xs">{run.range_from || '?'} → {run.range_to || '?'}</dd>
      <dt class="text-zinc-500">Started</dt><dd>{fmtDateTime(run.started_at)}</dd>
      <dt class="text-zinc-500">Duration</dt><dd>{fmtDuration(run.started_at, run.finished_at)}</dd>
      <dt class="text-zinc-500">Apps</dt><dd>{run.apps_done}/{run.apps_total}</dd>
      <dt class="text-zinc-500">Metric rows</dt><dd>{fmtNum(run.rows_metrics)}</dd>
      <dt class="text-zinc-500">Reviews</dt><dd>{fmtNum(run.rows_reviews)}</dd>
      {#if stats}<dt class="text-zinc-500">API calls / bytes</dt><dd>{fmtNum(stats.api_calls)} / {fmtNum(Math.round((stats.bytes || 0) / 1024))} KB</dd>{/if}
    </dl>
    {#if run.status === 'running'}<div class="mt-3"><ProgressBar value={run.apps_done} max={run.apps_total || 1} /></div>{/if}
    {#if run.error}<div class="mt-3 rounded-md bg-red-50 p-3 text-sm text-red-700 dark:bg-red-950/40 dark:text-red-300">{run.error}</div>{/if}
  </div>

  <div class="grid gap-4 lg:grid-cols-3">
    <div class="card lg:col-span-2">
      <div class="mb-2 flex items-center gap-2">
        <h2 class="font-medium">Log</h2>
        <select class="input ml-auto w-auto text-xs" bind:value={level}><option value="">all levels</option><option value="info">info</option><option value="warn">warn</option><option value="error">error</option></select>
        <label class="flex items-center gap-1 text-xs"><input type="checkbox" bind:checked={autoScroll} />auto-scroll</label>
      </div>
      <div bind:this={box} class="h-[480px] overflow-auto rounded-md bg-zinc-50 p-2 font-mono text-xs dark:bg-black/40" aria-live="polite">
        {#each shown as l (l.id)}
          <div class="flex gap-2 {lvlCls[l.level] || ''}"><span class="shrink-0 text-zinc-400">{new Date(l.ts).toLocaleTimeString()}</span><span class="w-10 shrink-0 uppercase">{l.level}</span><span class="whitespace-pre-wrap">{l.message}</span></div>
        {:else}<div class="text-zinc-400">No log lines yet.</div>{/each}
      </div>
    </div>
    <div class="card">
      <h2 class="mb-2 font-medium">Per app</h2>
      {#if stats?.apps && Object.keys(stats.apps).length}
        <table class="table"><thead><tr><th>App</th><th class="text-right">Rows</th><th class="text-right">Err</th><th class="text-right">Time</th></tr></thead>
          <tbody>{#each Object.entries(stats.apps) as [id, a]}<tr class={a.errors ? 'text-red-600' : ''}><td class="truncate">{a.name}</td><td class="text-right tabular-nums">{fmtNum((a.metrics || 0) + (a.reviews || 0))}</td><td class="text-right">{a.errors || ''}</td><td class="text-right text-xs">{Math.round((a.ms || 0) / 1000)}s</td></tr>{/each}</tbody></table>
        {#if stats.step_ms && Object.keys(stats.step_ms).length}<h3 class="mt-3 mb-1 text-xs font-medium text-zinc-500">Rows per source</h3><ul class="text-xs">{#each Object.entries(stats.step_ms) as [k, v]}<li class="flex justify-between"><span>{k}</span><span class="tabular-nums">{fmtNum(v)}</span></li>{/each}</ul>{/if}
      {:else}<p class="text-sm text-zinc-400">Available when the run finishes.</p>{/if}
    </div>
  </div>
{/if}
