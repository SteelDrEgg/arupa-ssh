<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import ConnectionDialog from '$lib/components/ConnectionDialog.svelte';
	import SavedConnectionsDialog from '$lib/components/SavedConnectionsDialog.svelte';
	import StatusBar from '$lib/components/StatusBar.svelte';
	import TerminalPane from '$lib/components/TerminalPane.svelte';
	import * as m from '$lib/paraglide/messages.js';
	import type { Locale } from '$lib/paraglide/runtime.js';
	import { saveConnection } from '$lib/utils/api';
	import { initializeLocale, locale } from '$lib/utils/locale';
	import { parseSocketMessage, sendSocket, terminalWebSocketURL } from '$lib/utils/socket';
	import { initializeTheme } from '$lib/utils/theme';
	import type {
		ConnectionState,
		ConnectionSubmission,
		SocketMessage
	} from '$lib/utils/types';

	type StatusDescriptor =
		| { kind: 'not-connected' }
		| { kind: 'connecting' }
		| { kind: 'connected'; destination: string; saved: boolean }
		| { kind: 'save-failed'; detail: string }
		| { kind: 'error'; detail: string }
		| { kind: 'closed' }
		| { kind: 'disconnected'; reason: string };

	let connectionState: ConnectionState = $state('idle');
	let status: StatusDescriptor = $state({ kind: 'not-connected' });
	let socket: WebSocket | null = null;
	let terminal: TerminalPane;
	let connectionDialog: ConnectionDialog;
	let savedConnectionsDialog: SavedConnectionsDialog;
	let localeOptions = $derived({ locale: $locale });
	let statusMessage = $derived(localizedStatus(status, $locale));

	function localizedStatus(current: StatusDescriptor, currentLocale: Locale): string {
		const options = { locale: currentLocale };
		switch (current.kind) {
			case 'not-connected':
				return m.not_connected({}, options);
			case 'connecting':
				return m.connecting({}, options);
			case 'connected':
				return current.saved
					? m.connected_saved({ destination: current.destination }, options)
					: m.connected_to({ destination: current.destination }, options);
			case 'save-failed':
				return m.connected_save_failed({ detail: current.detail }, options);
			case 'error':
				return m.error_with_detail({ detail: current.detail }, options);
			case 'closed':
				return m.ssh_session_closed({}, options);
			case 'disconnected':
				return current.reason
					? m.disconnected_reason({ reason: current.reason }, options)
					: m.disconnected({}, options);
		}
	}

	function setState(next: ConnectionState, nextStatus: StatusDescriptor) {
		connectionState = next;
		status = nextStatus;
	}

	function isActive(candidate: WebSocket): boolean {
		return socket === candidate;
	}

	function closeSocket(candidate = socket) {
		if (!candidate || !isActive(candidate)) return;
		sendSocket(candidate, 'disconnect');
		socket = null;
		candidate.close(1000, 'client disconnect');
	}

	function disconnect() {
		const wasConnected = connectionState === 'connected';
		setState('idle', { kind: 'not-connected' });
		closeSocket();
		if (wasConnected) {
			terminal.clear();
			terminal.write(`${m.terminal_welcome({}, localeOptions)}\r\n`);
		}
	}

	function toggleConnection() {
		if (connectionState === 'connected' || connectionState === 'connecting') {
			disconnect();
			return;
		}
		connectionDialog.show();
	}

	function manageSavedConnections() {
		savedConnectionsDialog.show();
	}

	function connectedLabel(data: unknown): string {
		const details =
			data && typeof data === 'object'
				? (data as { user?: unknown; host?: unknown; port?: unknown })
				: {};
		return `${String(details.user || '')}@${String(details.host || '')}:${String(details.port || '')}`;
	}

	function messageText(data: unknown, fallback: string): string {
		return typeof data === 'string' && data ? data : fallback;
	}

	function handleMessage(
		candidate: WebSocket,
		message: SocketMessage,
		submission: ConnectionSubmission
	) {
		switch (message.event) {
			case 'ssh_connected': {
				const destination = connectedLabel(message.data);
				setState('connected', { kind: 'connected', destination, saved: false });
				connectionDialog.close();
				terminal.clear();
				terminal.write(`${m.terminal_connected({ destination }, localeOptions)}\r\n`);
				terminal.focus();
				terminal.fit();
				if (submission.settings) {
					void saveConnection(submission.settings)
						.then(() => {
							if (isActive(candidate)) {
								setState('connected', { kind: 'connected', destination, saved: true });
							}
						})
						.catch((error) => {
							if (isActive(candidate)) {
								const detail =
									error instanceof Error
										? error.message
										: m.unknown_error({}, localeOptions);
								setState('connected', { kind: 'save-failed', detail });
							}
						});
				}
				break;
			}
			case 'ssh_error':
				setState('error', {
					kind: 'error',
					detail: messageText(message.data, m.ssh_service_error({}, localeOptions))
				});
				closeSocket(candidate);
				break;
			case 'ssh_disconnected': {
				const rawReason = messageText(message.data, '');
				const defaultReason = rawReason === '' || rawReason === 'SSH session closed';
				const reason = defaultReason
					? m.ssh_session_closed({}, localeOptions)
					: rawReason;
				setState('idle', defaultReason ? { kind: 'closed' } : { kind: 'disconnected', reason });
				terminal.write(`\r\n${reason}\r\n`);
				closeSocket(candidate);
				break;
			}
			case 'terminal_output':
				terminal.write(messageText(message.data, ''));
				break;
		}
	}

	function connect(submission: ConnectionSubmission) {
		if (connectionState === 'connecting' || connectionState === 'connected') return;
		setState('connecting', { kind: 'connecting' });
		const candidate = new WebSocket(terminalWebSocketURL());
		socket = candidate;

		candidate.addEventListener('open', () => {
			if (!isActive(candidate)) return;
			sendSocket(candidate, 'connect_ssh', submission.request);
			submission.request.password = '';
			submission.request.passphrase = '';
		});

		candidate.addEventListener('message', (event) => {
			if (!isActive(candidate)) return;
			try {
				handleMessage(candidate, parseSocketMessage(event.data), submission);
			} catch (error) {
				const detail =
					error instanceof Error
						? error.message
						: m.invalid_ssh_response({}, localeOptions);
				setState('error', { kind: 'error', detail });
				closeSocket(candidate);
			}
		});

		candidate.addEventListener('error', () => {
			if (!isActive(candidate)) return;
			setState('error', {
				kind: 'error',
				detail: m.connection_service_failed({}, localeOptions)
			});
			closeSocket(candidate);
		});

		candidate.addEventListener('close', (event) => {
			if (!isActive(candidate)) return;
			socket = null;
			if (connectionState === 'connected' || connectionState === 'connecting') {
				setState('idle', { kind: 'disconnected', reason: event.reason });
				terminal.write(`\r\n${m.ssh_session_closed({}, localeOptions)}\r\n`);
			}
		});
	}

	function terminalInput(data: string) {
		if (connectionState === 'connected') sendSocket(socket, 'terminal_input', data);
	}

	function terminalResize(cols: number, rows: number) {
		if (connectionState === 'connected') sendSocket(socket, 'resize', { cols, rows });
	}

	onMount(() => {
		const unsubscribeLocale = initializeLocale();
		const unsubscribeTheme = initializeTheme((theme) => terminal?.setTheme(theme));
		return () => {
			unsubscribeLocale();
			unsubscribeTheme();
		};
	});
	onDestroy(() => closeSocket());
</script>

<svelte:head>
	<title>{m.page_title({}, localeOptions)}</title>
	<meta name="description" content={m.page_description({}, localeOptions)} />
</svelte:head>

<div class="flex h-screen min-h-0 flex-col overflow-hidden bg-base-100">
	<StatusBar state={connectionState} message={statusMessage} onaction={toggleConnection} />
	<TerminalPane
		bind:this={terminal}
		oninput={terminalInput}
		onresize={terminalResize}
		welcomeText={m.terminal_welcome({}, localeOptions)}
	/>
</div>

<ConnectionDialog bind:this={connectionDialog} onconnect={connect} onmanage={manageSavedConnections} />
<SavedConnectionsDialog bind:this={savedConnectionsDialog} />
