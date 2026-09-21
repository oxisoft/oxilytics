<script>
  import { onDestroy } from 'svelte';
  import { Chart, LineController, LineElement, PointElement, LinearScale, CategoryScale, BarController, BarElement, DoughnutController, ArcElement, Tooltip, Legend, Filler } from 'chart.js';

  Chart.register(LineController, LineElement, PointElement, LinearScale, CategoryScale, BarController, BarElement, DoughnutController, ArcElement, Tooltip, Legend, Filler);

  // type: line | bar | doughnut; series: [{label, values, color?}]; labels: []
  // xUnit: day | week | month — how to abbreviate ISO date labels on the x axis.
  let { type = 'line', labels = [], series = [], stacked = false, height = 240, legend = true, yFormat = null, xUnit = 'day' } = $props();
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

  // Axis dates: keep them short and let the axis decide how many fit.
  //
  // "2026-01-01" is 10 characters; a fixed maxTicksLimit of 12 with rotation
  // disabled printed twelve of them edge to edge, so they ran together into an
  // unreadable band. Shorter text plus autoSkip driven by measured width means
  // the axis drops labels instead of colliding them, at any container size.
  //
  // Every bucket arrives as a full ISO date — month buckets are the first of
  // the month ("2026-03-01"), not "2026-03" — so the unit has to be passed in
  // rather than inferred from the string. Month buckets keep the year, since a
  // 12-month window spans two; day and week buckets drop it, because the
  // resolved range is printed above the chart. Non-date labels (country codes,
  // platform names, "3★") are passed through untouched.
  const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
  function shortLabel(raw) {
    if (typeof raw !== 'string') return raw;
    const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(raw);
    if (!m) return raw;
    const [, year, month, day] = m;
    if (xUnit === 'month') return `${MONTHS[Number(month) - 1]} ${year}`;
    return `${Number(day)} ${MONTHS[Number(month) - 1]}`;
  }

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
        // The axis skips labels; the tooltip title still shows the full raw
        // label ("2026-03-30"), which is where the skipped detail lives.
        tooltip: { callbacks: yFormat ? { label: (c) => `${c.dataset.label}: ${yFormat(c.parsed.y ?? c.parsed)}` } : {} },
      },
      scales: type === 'doughnut' ? {} : {
        // autoSkip with a minimum spacing lets Chart.js drop labels that do not
        // fit rather than cramming a fixed count together. maxTicksLimit is
        // deliberately absent: it forces a count regardless of available width,
        // which is what made the labels collide. The tooltip still shows the
        // exact date for every point, so skipped ticks lose nothing.
        x: {
          stacked,
          grid: { display: false },
          ticks: {
            color: tick,
            autoSkip: true,
            autoSkipPadding: 16,
            maxRotation: 0,
            minRotation: 0,
            callback(value) {
              // In a category scale the callback receives the index.
              return shortLabel(this.getLabelForValue(value));
            },
          },
        },
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
