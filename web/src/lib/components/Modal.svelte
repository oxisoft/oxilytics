<script>
  import Icon from './Icon.svelte';
  let { open = $bindable(false), title = '', onclose = () => {}, wide = false, children } = $props();

  function close() { open = false; onclose(); }
  function key(e) { if (e.key === 'Escape') close(); }
</script>

<svelte:window onkeydown={open ? key : undefined} />

{#if open}
  <div class="fixed inset-0 z-40 flex items-start justify-center overflow-y-auto bg-black/50 p-4 pt-16" role="presentation" onclick={(e) => e.target === e.currentTarget && close()}>
    <div class="card w-full {wide ? 'max-w-3xl' : 'max-w-lg'} shadow-xl" role="dialog" aria-modal="true" aria-label={title}>
      <div class="mb-3 flex items-center justify-between">
        <h2 class="text-base font-semibold">{title}</h2>
        <button class="rounded p-1 hover:bg-zinc-100 dark:hover:bg-zinc-800" onclick={close} aria-label="Close"><Icon name="x" /></button>
      </div>
      {@render children?.()}
    </div>
  </div>
{/if}
