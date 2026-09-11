<script>
  import Modal from './Modal.svelte';
  let { open = $bindable(false), title = 'Are you sure?', message = '', confirmLabel = 'Confirm', danger = false, typeToConfirm = '', onconfirm = () => {}, onclose = () => {} } = $props();
  let typed = $state('');
  let busy = $state(false);
  const ok = $derived(!typeToConfirm || typed === typeToConfirm);

  function close() { open = false; typed = ''; onclose(); }
  async function go() {
    busy = true;
    try { await onconfirm(); close(); } finally { busy = false; }
  }
</script>

<Modal bind:open {title} onclose={() => { typed = ''; onclose(); }}>
  <p class="text-sm text-zinc-600 dark:text-zinc-400">{message}</p>
  {#if typeToConfirm}
    <label class="label mt-3" for="confirm-input">Type <code class="font-mono">{typeToConfirm}</code> to confirm</label>
    <input id="confirm-input" class="input" bind:value={typed} autocomplete="off" />
  {/if}
  <div class="mt-4 flex justify-end gap-2">
    <button class="btn-secondary" onclick={close}>Cancel</button>
    <button class={danger ? 'btn-danger' : 'btn-primary'} disabled={!ok || busy} onclick={go}>{confirmLabel}</button>
  </div>
</Modal>
