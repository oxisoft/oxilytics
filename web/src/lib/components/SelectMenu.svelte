<script>
  // Generic button + listbox dropdown.
  //
  // A native <select> cannot render an icon or an image inside its options, and
  // the filter bar needs both, so keyboard and screen-reader behaviour is
  // implemented explicitly here instead of being inherited from <select>.
  //
  // Options: { id, name, icon?, glyph? } — `icon` is an image URL, `glyph` is a
  // platform name rendered via PlatformGlyph. Selection is single-value by
  // design: the platform filter used to be a multi-toggle, which let you build
  // combinations nobody asked for and made the bar wide.
  import Icon from './Icon.svelte';
  import PlatformGlyph from './PlatformGlyph.svelte';

  let {
    options = [],
    value = '',
    onchange = () => {},
    label = '',
    fallbackIcon = '',
    minWidth = 'min-w-36',
    listWidth = 'w-56',
  } = $props();

  let open = $state(false);
  let btn = $state(null);
  let listEl = $state(null);
  let active = $state(-1);

  const selected = $derived(options.find((o) => String(o.id) === String(value)) || options[0] || { id: '', name: '' });

  function choose(id) {
    open = false;
    active = -1;
    btn?.focus();
    if (String(id) !== String(value)) onchange(id);
  }

  function openList() {
    open = true;
    active = Math.max(0, options.findIndex((o) => String(o.id) === String(value)));
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
    class="input flex w-auto {minWidth} items-center gap-2 text-left"
    aria-haspopup="listbox"
    aria-expanded={open}
    aria-label={label}
    onclick={() => (open ? (open = false) : openList())}
    onkeydown={onKey}
  >
    {#if selected.icon}
      <img src={selected.icon} alt="" class="h-5 w-5 shrink-0 rounded" />
    {:else if selected.glyph}
      <PlatformGlyph platform={selected.glyph} />
    {:else if fallbackIcon}
      <span class="grid h-5 w-5 shrink-0 place-items-center rounded bg-zinc-100 text-zinc-400 dark:bg-zinc-800"><Icon name={fallbackIcon} class="h-3 w-3" /></span>
    {/if}
    <span class="flex-1 truncate">{selected.name}</span>
    <Icon name="chevron" class="h-3 w-3 shrink-0 text-zinc-400" />
  </button>

  {#if open}
    <ul
      bind:this={listEl}
      role="listbox"
      tabindex="-1"
      aria-label={label}
      class="absolute z-30 mt-1 max-h-72 {listWidth} overflow-auto rounded-md border border-zinc-200 bg-white py-1 shadow-lg dark:border-zinc-700 dark:bg-zinc-900"
      onkeydown={onKey}
    >
      {#each options as o, i (o.id)}
        <li>
          <button
            type="button"
            role="option"
            aria-selected={String(o.id) === String(value)}
            class="flex w-full items-center gap-2 px-2 py-1.5 text-left text-sm {i === active ? 'bg-zinc-100 dark:bg-zinc-800' : ''}"
            onmouseenter={() => (active = i)}
            onclick={() => choose(o.id)}
          >
            {#if o.icon}
              <img src={o.icon} alt="" class="h-5 w-5 shrink-0 rounded" />
            {:else if o.glyph}
              <PlatformGlyph platform={o.glyph} />
            {:else if fallbackIcon}
              <span class="grid h-5 w-5 shrink-0 place-items-center rounded bg-zinc-100 text-zinc-400 dark:bg-zinc-800"><Icon name={fallbackIcon} class="h-3 w-3" /></span>
            {/if}
            <span class="flex-1 truncate">{o.name}</span>
            {#if String(o.id) === String(value)}<Icon name="check" class="h-3 w-3 shrink-0 text-brand-600" />{/if}
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</div>
