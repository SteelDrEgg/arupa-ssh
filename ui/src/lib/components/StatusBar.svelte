<script lang="ts">
	import * as m from '$lib/paraglide/messages.js';
	import { locale } from '$lib/utils/locale';
	import type { ConnectionState } from '$lib/utils/types';

	let {
		state,
		message,
		onaction
	}: {
		state: ConnectionState;
		message: string;
		onaction: () => void;
	} = $props();

	let connected = $derived(state === 'connected');
	let connecting = $derived(state === 'connecting');
	let localeOptions = $derived({ locale: $locale });
	let label = $derived(
		connected
			? m.disconnect({}, localeOptions)
			: connecting
				? m.connecting({}, localeOptions)
				: m.connect({}, localeOptions)
	);
</script>

<header class="navbar min-h-16 border-b border-base-300 bg-base-100 px-4 shadow-sm">
	<div class="min-w-0 flex-1">
		<div class="badge badge-lg badge-neutral text-neutral-content min-w-0 max-w-full gap-2 px-3">
			<span
				class:error={state === 'error'}
				class:success={connected}
				class:warning={connecting}
				class="status status-sm"
				aria-hidden="true"
			></span>
			<span class="truncate" role="status" aria-live="polite">{message}</span>
		</div>
	</div>
	<button
		class:btn-error={connected}
		class:btn-primary={!connected}
		class="btn min-w-28"
		type="button"
		disabled={connecting}
		onclick={onaction}
	>
		{label}
	</button>
</header>
