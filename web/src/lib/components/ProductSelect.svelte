<script>
  // Product picker: a thin wrapper that maps products onto the generic
  // SelectMenu. This used to be its own button+listbox implementation; the
  // platform and period filters needed exactly the same widget, so the
  // behaviour lives in SelectMenu and is shared rather than copied.
  import { productIcon } from '../format.js';
  import SelectMenu from './SelectMenu.svelte';

  let { products = [], value = '', onchange = () => {} } = $props();

  const options = $derived([
    { id: '', name: 'All products', icon: null },
    ...products.map((p) => ({
      id: String(p.product.id),
      name: p.product.name,
      icon: productIcon(p),
    })),
  ]);
</script>

<SelectMenu
  {options}
  {value}
  {onchange}
  label="Product"
  fallbackIcon="products"
  minWidth="min-w-44"
  listWidth="w-64"
/>
