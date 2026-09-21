<script>
  // Product selector with icons.
  //
  // A native <select> cannot render an <img> inside its options, so this is a
  // button plus a listbox. Keyboard and screen-reader behaviour is implemented
  // explicitly rather than inherited, because dropping to a div would otherwise
  // make the filter unusable without a mouse.
  import { productIcon } from '../format.js';
  import Icon from './Icon.svelte';

  let { products = [], value = '', onchange = () => {} } = $props();

  let open = $state(false);
  let btn = $state(null);
  let listEl = $state(null);
  let active = $state(-1);

  const options = $derived([
    { id: '', name: 'All products', icon: null },
    ...products.map((p) => ({
      id: String(p.product.id),
      name: p.product.name,
      icon: productIcon(p),
    })),
  ]);
  const selected = $derived(options.find((o) => o.id === String(value)) || options[0]);

  function choose(id) {
    open = false;
    active = -1;
    btn?.focus();
    if (String(id) !== String(value)) onchange(id);
  }

  function openList() {
    open = true;
    active = Math.max(0, options.findIndex((o) => o.id === String(value)));
  }

  function onKey(e) {
    if (!open) {
      if (e.key === 'ArrowDown' || e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        openList();
      }
      return;
    }
    if (e.key === 'Escape') { e.preventDefault(); open = false; btn?.focus(); return; }
    if (e.key === 'ArrowDown') { e.preventDefault(); active = Math.min(active + 1, options.length - 1); }
    if (e.key === 'ArrowUp') { e.preventDefault(); active = Math.max(active - 1, 0); }
    if (e.key === 'Home') { e.preventDefault(); active = 0; }
    if (e.key === 'End') { e.preventDefault(); active = options.length - 1; }
    if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); choose(options[active]?.id ?? ''); }
  }

  // Close when focus or a click leaves the widget entirely.
  function onWindowClick(e) {
    if (!open) return;
    if (btn?.contains(e.target) || listEl?.contains(e.target)) return;
    open = false;
  }
</script>

<svelte:window onclick={onWindowClick} />

<div class="relative">
  <button
    bind:this={btn}
    type="button"
    class="input flex w-auto min-w-44 items-center gap-2 text-left"
    aria-haspopup="listbox"
    aria-expanded={open}
    aria-label="Product"
    onclick={() => (open ? (open = false) : openList())}
    onkeydown={onKey}
  >
    {#if selected.icon}
      <img src={selected.icon} alt="" class="h-5 w-5 shrink-0 rounded" />
    {:else}
      <span class="grid h-5 w-5 shrink-0 place-items-center rounded bg-zinc-100 text-zinc-400 dark:bg-zinc-800"><Icon name="products" class="h-3 w-3" /></span>
    {/if}
    <span class="flex-1 truncate">{selected.name}</span>
    <Icon name="chevron" class="h-3 w-3 shrink-0 text-zinc-400" />
  </button>

  {#if open}
    <ul
      bind:this={listEl}
      role="listbox"
      tabindex="-1"
      aria-label="Product"
      class="absolute z-30 mt-1 max-h-72 w-64 overflow-auto rounded-md border border-zinc-200 bg-white py-1 shadow-lg dark:border-zinc-700 dark:bg-zinc-900"
      onkeydown={onKey}
    >
      {#each options as o, i (o.id)}
        <li>
          <button
            type="button"
            role="option"
            aria-selected={o.id === String(value)}
            class="flex w-full items-center gap-2 px-2 py-1.5 text-left text-sm {i === active ? 'bg-zinc-100 dark:bg-zinc-800' : ''}"
            onmouseenter={() => (active = i)}
            onclick={() => choose(o.id)}
          >
            {#if o.icon}
              <img src={o.icon} alt="" class="h-5 w-5 shrink-0 rounded" />
            {:else}
              <span class="grid h-5 w-5 shrink-0 place-items-center rounded bg-zinc-100 text-zinc-400 dark:bg-zinc-800"><Icon name="products" class="h-3 w-3" /></span>
            {/if}
            <span class="flex-1 truncate">{o.name}</span>
            {#if o.id === String(value)}<Icon name="check" class="h-3 w-3 shrink-0 text-brand-600" />{/if}
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</div>
