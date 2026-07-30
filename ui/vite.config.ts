import tailwindcss from '@tailwindcss/vite';
import { paraglideVitePlugin } from '@inlang/paraglide-js';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';
import { paraglideOptions } from './paraglide.config.js';

export default defineConfig({
	plugins: [paraglideVitePlugin(paraglideOptions), tailwindcss(), sveltekit()]
});
