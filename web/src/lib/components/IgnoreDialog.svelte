<script>
  import { api } from '../api.js';
  import { toasts } from '../toast.svelte.js';
  import Modal from './Modal.svelte';
  import PlatformGlyph from './PlatformGlyph.svelte';

  // app: single app object, or an array for bulk
  let { app = null, onclose = () => {}, ondone = () => {} } = $props();
  let reason = $state('');
  let busy = $state(false);
  const list = $derived(Array.isArray(app) ? app : app ? [app] : []);
  const linked = $derived(list.filter((a) => a.product_id));

  async function go() {
    busy = true;
    try {
      for (const a of list) await api.post('/apps/' + a.id + '/ignore', { reason: reason || null });
      toasts.success(list.length > 1 ? `${list.length} apps ignored` : 'App ignored');
      reason = ''; ondone();
    } catch (err) { toasts.error(err.message); }
    finally { busy = false; }
  }
</script>

<Modal open={list.length > 0} title={list.length > 1 ? `Ignore ${list.length} store apps` : 'Ignore store app'} {onclose}>
  <ul class="mb-3 max-h-40 space-y-1 overflow-y-auto text-sm">
    {#each list as a}<li class="flex items-center gap-2"><PlatformGlyph platform={a.platform} /><span class="font-medium">{a.name}</span><span class="text-zinc-500">{a.store_app_id}</span></li>{/each}
  </ul>
  <p class="mb-3 text-xs text-zinc-500">Ignored apps are skipped by sync and hidden from every screen. Existing data is kept; you can restore them under Settings → Ignored apps.</p>
  {#if linked.length}<p class="mb-3 text-xs text-amber-700 dark:text-amber-300">This will unlink {linked.length === 1 ? 'it' : 'them'} from {linked.length === 1 ? 'its' : 'their'} product{linked.length > 1 ? 's' : ''}.</p>{/if}
  <label class="label" for="ig-reason">Reason (optional)</label>
  <input id="ig-reason" class="input" bind:value={reason} placeholder="e.g. old test build" />
  <div class="mt-4 flex justify-end gap-2"><button class="btn-secondary" onclick={onclose}>Cancel</button><button class="btn-danger" disabled={busy} onclick={go}>Ignore</button></div>
</Modal>
