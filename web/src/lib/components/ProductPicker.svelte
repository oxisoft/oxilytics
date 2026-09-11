<script>
  import { api } from '../api.js';
  import { toasts } from '../toast.svelte.js';
  import Modal from './Modal.svelte';
  import PlatformGlyph from './PlatformGlyph.svelte';

  let { app = null, onclose = () => {}, onlinked = () => {} } = $props();
  let products = $state([]);
  let chosen = $state('');
  let newName = $state('');
  let busy = $state(false);
  let error = $state('');

  $effect(() => {
    if (app) {
      chosen = ''; newName = app.name; error = '';
      api.get('/products?archived=1').then((p) => (products = p.map((x) => x.product)));
    }
  });

  async function link() {
    busy = true; error = '';
    try {
      await api.post('/products-suggestions/accept', { app_id: app.id, product_id: chosen ? Number(chosen) : null, name: chosen ? undefined : newName });
      toasts.success('Linked'); onlinked();
    } catch (err) { error = err.fields?.name || err.message; }
    finally { busy = false; }
  }
</script>

<Modal open={!!app} title="Link to product" {onclose}>
  {#if app}
    <p class="mb-3 flex items-center gap-2 text-sm"><PlatformGlyph platform={app.platform} /><span class="font-medium">{app.name}</span><span class="text-zinc-500">{app.store_app_id}</span></p>
    <label class="label" for="pp-sel">Existing product</label>
    <select id="pp-sel" class="input mb-3" bind:value={chosen}>
      <option value="">— create a new product —</option>
      {#each products as p}<option value={p.id}>{p.name}{p.archived ? ' (archived)' : ''}</option>{/each}
    </select>
    {#if !chosen}
      <label class="label" for="pp-name">New product name</label>
      <input id="pp-name" class="input" bind:value={newName} />
    {/if}
    {#if error}<p class="mt-2 text-xs text-red-600">{error}</p>{/if}
    <div class="mt-4 flex justify-end gap-2"><button class="btn-secondary" onclick={onclose}>Cancel</button><button class="btn-primary" disabled={busy || (!chosen && !newName.trim())} onclick={link}>Link</button></div>
  {/if}
</Modal>
