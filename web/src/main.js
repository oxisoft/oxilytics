import { mount } from 'svelte';
import { init, register, waitLocale } from 'svelte-i18n';
import './app.css';
import App from './App.svelte';
import { theme } from './lib/theme.js';

register('en', () => import('./locales/en.json'));
init({ fallbackLocale: 'en', initialLocale: 'en' });
theme.apply();

await waitLocale();
mount(App, { target: document.getElementById('app') });
