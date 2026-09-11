<script>
  import EmptyState from '../lib/components/EmptyState.svelte';
  let { params = {} } = $props();
</script>

<h1 class="mb-4 text-lg font-semibold">Products</h1>
<EmptyState icon="products" title="Coming in a later milestone" message="Products is not implemented yet." />
