<script lang="ts">
	import { onMount } from 'svelte';
	import type { Theme } from '$lib/utils/theme';

	let {
		oninput,
		onresize,
		welcomeText
	}: {
		oninput: (data: string) => void;
		onresize: (cols: number, rows: number) => void;
		welcomeText: string;
	} = $props();

	let container: HTMLDivElement;
	let terminal: XTermInstance | null = null;
	let fitAddon: FitAddonInstance | null = null;

	function cssColor(variable: string, fallback: string): [string, number, number, number] {
		const canvas = document.createElement('canvas');
		const context = canvas.getContext('2d');
		if (!context) return [fallback, 0, 0, 0];
		canvas.width = 1;
		canvas.height = 1;
		context.fillStyle = fallback;
		context.fillStyle = getComputedStyle(document.body).getPropertyValue(variable).trim();
		context.fillRect(0, 0, 1, 1);
		const [red, green, blue] = context.getImageData(0, 0, 1, 1).data;
		return [`#${[red, green, blue].map((value) => value.toString(16).padStart(2, '0')).join('')}`, red, green, blue];
	}

	// Only terminal surfaces follow the application theme. ANSI colors are
	// intentionally omitted so xterm keeps its native terminal palette.
	function interfaceTheme(): TerminalTheme {
		const [background] = cssColor('--color-base-100', '#ffffff');
		const [foreground, red, green, blue] = cssColor('--color-base-content', '#111111');
		return {
			background,
			foreground,
			cursor: foreground,
			cursorAccent: background,
			selectionBackground: `rgba(${red}, ${green}, ${blue}, 0.28)`
		};
	}

	function notifySize() {
		if (terminal) onresize(terminal.cols, terminal.rows);
	}

	export function fit() {
		if (!terminal || !fitAddon) return;
		requestAnimationFrame(() => {
			fitAddon?.fit();
			notifySize();
		});
	}

	export function write(data: string) {
		terminal?.write(data);
	}

	export function clear() {
		terminal?.clear();
	}

	export function focus() {
		terminal?.focus();
	}

	export function setTheme(_theme: Theme) {
		if (terminal) terminal.options.theme = interfaceTheme();
	}

	onMount(() => {
		terminal = new window.Terminal({
			cursorBlink: true,
			fontSize: 14,
			fontFamily: 'Menlo, Monaco, "Courier New", monospace',
			theme: interfaceTheme()
		});
		fitAddon = new window.FitAddon.FitAddon();
		terminal.loadAddon(fitAddon);
		terminal.open(container);
		const input = terminal.onData(oninput);
		const observer = new ResizeObserver(fit);
		observer.observe(container);
		write(`${welcomeText}\r\n`);
		fit();
		focus();

		return () => {
			input.dispose();
			observer.disconnect();
			terminal?.dispose();
			terminal = null;
			fitAddon = null;
		};
	});
</script>

<main class="terminal-shell min-h-0 flex-1 overflow-hidden bg-base-100">
	<div bind:this={container} class="h-full w-full" aria-label="SSH terminal"></div>
</main>
