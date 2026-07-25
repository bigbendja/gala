import { defineConfig } from 'vitest/config';
import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';

export default defineConfig({
	server: {
		// npm run dev sirve solo el frontend; las llamadas a /api/v1/* se
		// reenvían al binario Go real corriendo en :7420 (README: "desarrollo
		// del frontend"). En producción no hace falta: el propio binario sirve
		// ambos desde el mismo origen.
		proxy: {
			'/api': 'http://127.0.0.1:7420'
		}
	},
	plugins: [
		sveltekit({
			compilerOptions: {
				// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
				runes: ({ filename }) => filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},

			// SPA estática (CLAUDE.md: build volcada a web/dist/, embebida con
			// go:embed; fallback a index.html para el enrutado del lado
			// cliente). pages/assets apuntan a "dist" (relativo a web/) para que
			// coincida EXACTAMENTE con lo que embebe web/web.go.
			adapter: adapter({
				pages: 'dist',
				assets: 'dist',
				fallback: 'index.html',
				precompress: false,
				strict: true
			})
		})
	],
	// Sin esto, Vitest resuelve el paquete "svelte" por su entrada de
	// servidor (SSR) incluso bajo jsdom, y montar un componente revienta con
	// "mount(...) is not available on the server" (Brief M10a §7).
	resolve: {
		conditions: ['browser']
	},
	test: {
		expect: { requireAssertions: true },
		environment: 'jsdom',
		setupFiles: ['./vitest-setup.ts'],
		include: ['src/**/*.{test,spec}.{js,ts}']
	}
});
