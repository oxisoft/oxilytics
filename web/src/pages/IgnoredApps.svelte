<script>
  // The ignored list now lives as a Products tab. This route is kept so old
  // links and bookmarks still land somewhere correct instead of 404ing.
  import { onMount } from 'svelte';
  import { replace } from 'svelte-spa-router';
  onMount(() => replace('/products?tab=ignored'));
</script>
