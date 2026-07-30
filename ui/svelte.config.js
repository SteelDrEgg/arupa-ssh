import adapter from '@sveltejs/adapter-static';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	kit: {
		adapter: adapter({
			pages: 'build',
			assets: 'build',
			fallback: undefined,
			precompress: false,
			strict: true
		}),
		paths: {
			base: '/ssh/pages'
		},
		prerender: {
			handleHttpError: ({ path, message }) => {
				if (path === '/assets/css/scheme.css') return;
				throw new Error(message);
			}
		}
	}
};

export default config;
