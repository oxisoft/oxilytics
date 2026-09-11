<script>
  import Icon from './Icon.svelte';
  let { rating = 0, size = 'h-3.5 w-3.5' } = $props();
</script>

<span class="inline-flex text-amber-500" title="{rating} / 5">
  {#each [1, 2, 3, 4, 5] as i}
    <span class={i <= Math.round(rating) ? '' : 'opacity-25'}><Icon name="star" class={size} /></span>
  {/each}
</span>
