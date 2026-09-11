<script>
  import EmptyState from '../lib/components/EmptyState.svelte';
  let { params = {} } = $props();
</script>

<h1 class="mb-4 text-lg font-semibold">Store apps</h1>
<EmptyState icon="products" title="Coming in a later milestone" message="Store apps is not implemented yet." />
