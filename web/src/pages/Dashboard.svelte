<script>
  import EmptyState from '../lib/components/EmptyState.svelte';
  let { params = {} } = $props();
</script>

<h1 class="mb-4 text-lg font-semibold">Dashboard</h1>
<EmptyState icon="dashboard" title="Coming in a later milestone" message="Dashboard is not implemented yet." />
