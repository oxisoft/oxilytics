<script>
  import { encodeQR } from '../qr.js';
  let { text, size = 176 } = $props();
  const modules = $derived.by(() => { try { return encodeQR(text); } catch { return null; } });
  const n = $derived(modules ? modules.length : 0);
  const path = $derived.by(() => {
    if (!modules) return '';
    let d = '';
    for (let y = 0; y < n; y++) for (let x = 0; x < n; x++) if (modules[y][x]) d += `M${x + 4} ${y + 4}h1v1h-1z`;
    return d;
  });
</script>

{#if modules}
  <svg viewBox="0 0 {n + 8} {n + 8}" width={size} height={size} class="rounded bg-white" role="img" aria-label="QR code" shape-rendering="crispEdges">
    <rect width="100%" height="100%" fill="#fff" />
    <path d={path} fill="#000" />
  </svg>
{:else}
  <div class="text-xs text-red-600">QR unavailable</div>
{/if}
