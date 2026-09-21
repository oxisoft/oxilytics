<script>
  import { onDestroy } from 'svelte';
  import { Chart, LineController, LineElement, PointElement, LinearScale, CategoryScale, BarController, BarElement, DoughnutController, ArcElement, Tooltip, Legend, Filler } from 'chart.js';

  Chart.register(LineController, LineElement, PointElement, LinearScale, CategoryScale, BarController, BarElement, DoughnutController, ArcElement, Tooltip, Legend, Filler);

  // type: line | bar | doughnut; series: [{label, values, color?}]; labels: []
  let { type = 'line', labels = [], series = [], stacked = false, height = 240, legend = true, yFormat = null } = $props();
  let canvas = $state(null);
  let chart;

  const palette = ['#6366f1', '#10b981', '#f59e0b', '#ef4444', '#0ea5e9', '#a855f7', '#f97316', '#14b8a6', '#84cc16', '#ec4899'];
  // Platform colours are separated by HUE, not by shade. iOS indigo (#6366f1)
  // and macOS violet (#8b5cf6) were neighbours on the wheel: in a doughnut
  // slice or a 2px line they read as the same colour, so a chart with both was
  // unreadable. Each platform now sits in its own hue family, and the pairs
  // stay distinct in greyscale and for red-green colour blindness because their
  // lightness differs too.
  const platformColor = {
    ios: '#6366f1',     // indigo
    macos: '#f59e0b',   // amber
    android: '#10b981', // emerald
    windows: '#ec4899', // pink
  };

  function isDark() { return document.documentElement.classList.contains('dark'); }

  function build(labels, series) {
    const grid = isDark() ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.06)';
    const tick = isDark() ? '#a1a1aa' : '#71717a';
    const datasets = series.map((s, i) => {
      const color = s.color || platformColor[s.key] || palette[i % palette.length];
      if (type === 'doughnut') return { label: s.label, data: s.values, backgroundColor: labels.map((l, j) => platformColor[s.keys?.[j]] || palette[j % palette.length]), borderWidth: 0 };
      return {
        label: s.label, data: s.values, borderColor: color, backgroundColor: type === 'bar' ? color : color + '22',
        fill: type === 'line' && series.length === 1, tension: 0.25, pointRadius: labels.length > 60 ? 0 : 2, borderWidth: 2,
      };
    });
    const opts = {
      responsive: true, maintainAspectRatio: false, animation: false,
      interaction: { mode: 'index', intersect: false },
      plugins: {
        legend: { display: legend && (series.length > 1 || type === 'doughnut'), labels: { color: tick, boxWidth: 10 } },
        tooltip: { callbacks: yFormat ? { label: (c) => `${c.dataset.label}: ${yFormat(c.parsed.y ?? c.parsed)}` } : {} },
      },
      scales: type === 'doughnut' ? {} : {
        x: { stacked, grid: { display: false }, ticks: { color: tick, maxTicksLimit: 12, maxRotation: 0 } },
        y: { stacked, beginAtZero: true, grid: { color: grid }, ticks: { color: tick, callback: (v) => yFormat ? yFormat(v) : v } },
      },
    };
    if (type === 'doughnut') { opts.cutout = '65%'; }
    if (chart) chart.destroy();
    chart = new Chart(canvas, { type, data: { labels, datasets }, options: opts });
    canvas.__chart = chart;
  }

  onDestroy(() => chart?.destroy());
  $effect(() => {
    // snapshot props so Chart.js gets plain arrays, not reactive proxies
    const l = $state.snapshot(labels), s = $state.snapshot(series);
    void stacked; void type;
    if (canvas) build(l, s);
  });
</script>

<div style="height:{height}px" class="relative"><canvas bind:this={canvas}></canvas></div>
