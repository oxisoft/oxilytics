<script>
  import FilterBar from '../lib/components/FilterBar.svelte';
  import ReviewsList from '../lib/components/ReviewsList.svelte';
  let { params = {} } = $props();
</script>

<h1 class="mb-4 text-lg font-semibold">Reviews</h1>
<FilterBar />
<ReviewsList openId={params.id ? Number(params.id) : null} />
