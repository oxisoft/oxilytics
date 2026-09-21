<script>
  // Inline SVG icons; `name` selects one. No icon package.
  // `class` ADDS to the default size instead of replacing it: passing only a
  // colour (class="text-zinc-400") used to drop h-4 w-4 and render the glyph at
  // full container width. Pass any h-*/w-* to override the size deliberately.
  let { name, class: cls = '' } = $props();
  const klass = $derived(/(^|\s)[hw]-/.test(cls) ? cls : ('h-4 w-4 ' + cls).trim());
  const paths = {
    dashboard: 'M3 13h8V3H3v10zm10 8h8V11h-8v10zM3 21h8v-6H3v6zm10-18v6h8V3h-8z',
    products: 'M20 7H4a2 2 0 00-2 2v10a2 2 0 002 2h16a2 2 0 002-2V9a2 2 0 00-2-2zM16 7V5a2 2 0 00-2-2h-4a2 2 0 00-2 2v2',
    reviews: 'M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z',
    sync: 'M21 12a9 9 0 11-3-6.7M21 3v6h-6',
    settings: 'M12 15a3 3 0 100-6 3 3 0 000 6zM19.4 15a1.7 1.7 0 00.3 1.8l.1.1a2 2 0 11-2.8 2.8l-.1-.1a1.7 1.7 0 00-1.8-.3 1.7 1.7 0 00-1 1.5V21a2 2 0 11-4 0v-.1a1.7 1.7 0 00-1.1-1.5 1.7 1.7 0 00-1.8.3l-.1.1a2 2 0 11-2.8-2.8l.1-.1a1.7 1.7 0 00.3-1.8 1.7 1.7 0 00-1.5-1H3a2 2 0 110-4h.1a1.7 1.7 0 001.5-1.1 1.7 1.7 0 00-.3-1.8l-.1-.1a2 2 0 112.8-2.8l.1.1a1.7 1.7 0 001.8.3H9a1.7 1.7 0 001-1.5V3a2 2 0 114 0v.1a1.7 1.7 0 001 1.5 1.7 1.7 0 001.8-.3l.1-.1a2 2 0 112.8 2.8l-.1.1a1.7 1.7 0 00-.3 1.8V9a1.7 1.7 0 001.5 1H21a2 2 0 110 4h-.1a1.7 1.7 0 00-1.5 1z',
    user: 'M20 21v-2a4 4 0 00-4-4H8a4 4 0 00-4 4v2M12 11a4 4 0 100-8 4 4 0 000 8z',
    logout: 'M9 21H5a2 2 0 01-2-2V5a2 2 0 012-2h4M16 17l5-5-5-5M21 12H9',
    sun: 'M12 17a5 5 0 100-10 5 5 0 000 10zM12 1v2M12 21v2M4.2 4.2l1.4 1.4M18.4 18.4l1.4 1.4M1 12h2M21 12h2M4.2 19.8l1.4-1.4M18.4 5.6l1.4-1.4',
    moon: 'M21 12.8A9 9 0 1111.2 3 7 7 0 0021 12.8z',
    check: 'M20 6L9 17l-5-5',
    x: 'M18 6L6 18M6 6l12 12',
    alert: 'M10.3 3.9L1.8 18a2 2 0 001.7 3h17a2 2 0 001.7-3L13.7 3.9a2 2 0 00-3.4 0zM12 9v4M12 17h0',
    info: 'M12 22a10 10 0 100-20 10 10 0 000 20zM12 16v-4M12 8h0',
    play: 'M5 3l14 9-14 9V3z',
    stop: 'M6 6h12v12H6z',
    refresh: 'M23 4v6h-6M1 20v-6h6M20.5 9A9 9 0 005.6 5.6L1 10M23 14l-4.6 4.4A9 9 0 013.5 15',
    eye: 'M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8zM12 15a3 3 0 100-6 3 3 0 000 6z',
    eyeoff: 'M17.9 17.9A10 10 0 0112 20C5 20 1 12 1 12a18 18 0 015-5.9M9.9 4.2A9 9 0 0112 4c7 0 11 8 11 8a18 18 0 01-2.2 3.2M14.1 14.1a3 3 0 11-4.2-4.2M1 1l22 22',
    link: 'M10 13a5 5 0 007.5.5l3-3a5 5 0 00-7-7l-1.7 1.7M14 11a5 5 0 00-7.5-.5l-3 3a5 5 0 007 7l1.7-1.7',
    unlink: 'M18.8 13.3a5 5 0 001.7-1.8l0 0a5 5 0 00-7-7l-1.7 1.7M5.2 10.7a5 5 0 00-1.7 1.8l0 0a5 5 0 007 7l1.7-1.7M8 2l1 3M2 8l3 1M16 22l-1-3M22 16l-3-1',
    plus: 'M12 5v14M5 12h14',
    trash: 'M3 6h18M8 6V4a2 2 0 012-2h4a2 2 0 012 2v2M19 6l-1 14a2 2 0 01-2 2H8a2 2 0 01-2-2L5 6',
    edit: 'M11 4H4a2 2 0 00-2 2v14a2 2 0 002 2h14a2 2 0 002-2v-7M18.5 2.5a2.1 2.1 0 013 3L12 15l-4 1 1-4 9.5-9.5z',
    external: 'M18 13v6a2 2 0 01-2 2H5a2 2 0 01-2-2V8a2 2 0 012-2h6M15 3h6v6M10 14L21 3',
    search: 'M21 21l-4.35-4.35M11 19a8 8 0 100-16 8 8 0 000 16z',
    star: 'M12 2l3.1 6.3 6.9 1-5 4.9 1.2 6.8L12 17.8 5.8 21l1.2-6.8-5-4.9 6.9-1z',
    download: 'M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4M7 10l5 5 5-5M12 15V3',
    chevron: 'M9 18l6-6-6-6',
    menu: 'M3 12h18M3 6h18M3 18h18',
    copy: 'M20 9h-9a2 2 0 00-2 2v9a2 2 0 002 2h9a2 2 0 002-2v-9a2 2 0 00-2-2zM5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1',
    ban: 'M12 22a10 10 0 100-20 10 10 0 000 20zM4.9 4.9l14.2 14.2',
    restore: 'M3 12a9 9 0 109-9 9.75 9.75 0 00-6.74 2.74L3 8M3 3v5h5',
    apple: 'M16.4 12.6c0-2.5 2-3.6 2.1-3.7a4.5 4.5 0 00-3.5-1.9c-1.5-.2-2.9.9-3.7.9-.8 0-1.9-.9-3.2-.8A4.7 4.7 0 004.2 9.5c-1.7 2.9-.4 7.3 1.2 9.7.8 1.2 1.8 2.5 3 2.4 1.2 0 1.7-.8 3.1-.8s1.9.8 3.2.8c1.3 0 2.2-1.2 3-2.4a10 10 0 001.3-2.8 4.3 4.3 0 01-2.6-3.8zM14 5.5A4.2 4.2 0 0015 2.4a4.3 4.3 0 00-2.8 1.4 4 4 0 00-1 3 3.6 3.6 0 002.8-1.3z',
    android: 'M6 10v7a1 1 0 001 1h1v3a1 1 0 002 0v-3h4v3a1 1 0 002 0v-3h1a1 1 0 001-1v-7H6zM4 10a1 1 0 00-1 1v5a1 1 0 002 0v-5a1 1 0 00-1-1zm16 0a1 1 0 00-1 1v5a1 1 0 002 0v-5a1 1 0 00-1-1zM15.5 3.5l1-1.5-.5-.3-1 1.6a6.3 6.3 0 00-6 0L8 1.7l-.5.3 1 1.5A5.5 5.5 0 006 8v1h12V8a5.5 5.5 0 00-2.5-4.5zM9.5 6.5a.7.7 0 110-1.5.7.7 0 010 1.5zm5 0a.7.7 0 110-1.5.7.7 0 010 1.5z',
    // macOS gets a display rather than a second apple: iOS and macOS otherwise
    // render the identical glyph, so the two platforms were told apart only by
    // colour — and in a legend or a table they sat next to each other looking
    // like a duplicate row.
    macos: 'M3 5a1 1 0 011-1h16a1 1 0 011 1v10a1 1 0 01-1 1H4a1 1 0 01-1-1V5zm6 15h6m-3-4v4',
    windows: 'M3 5.5l7.5-1v7H3v-6zM11.5 4.3L21 3v8.5h-9.5V4.3zM3 12.5h7.5v7L3 18.5v-6zM11.5 12.5H21V21l-9.5-1.3v-7.2z',
  };
  const filled = $derived(name === 'apple' || name === 'android' || name === 'windows' || name === 'star');
</script>

<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" class={klass} fill={filled ? 'currentColor' : 'none'} stroke={filled ? 'none' : 'currentColor'} stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
  <path d={paths[name] || paths.info} />
</svg>
