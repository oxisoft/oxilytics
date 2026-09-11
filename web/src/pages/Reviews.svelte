<script>
  import EmptyState from '../lib/components/EmptyState.svelte';
  let { params = {} } = $props();
</script>

<h1 class="mb-4 text-lg font-semibold">Reviews</h1>
<EmptyState icon="reviews" title="Coming in a later milestone" message="Reviews is not implemented yet." />
