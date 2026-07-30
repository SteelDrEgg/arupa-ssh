<script lang="ts">
	import ConnectionForm from '$lib/components/ConnectionForm.svelte';
	import * as m from '$lib/paraglide/messages.js';
	import { listConnections, listSecrets, revealSecret } from '$lib/utils/api';
	import { locale } from '$lib/utils/locale';
	import type {
		ConnectionFormModel,
		ConnectionSubmission,
		SavedConnection,
		SecretMeta
	} from '$lib/utils/types';

	let {
		onconnect,
		onmanage
	}: {
		onconnect: (submission: ConnectionSubmission) => void;
		onmanage: () => void;
	} = $props();

	function emptyForm(): ConnectionFormModel {
		return {
			host: 'localhost',
			port: '22',
			username: '',
			authMethod: 'password',
			passwordSource: 'manual',
			password: '',
			privateKey: '',
			passphrase: '',
			secretName: '',
			secretPassphrase: ''
		};
	}

	let dialog: HTMLDialogElement;
	let savedConnections: SavedConnection[] = $state([]);
	let secrets: SecretMeta[] = $state([]);
	let savedName = $state('');
	let form: ConnectionFormModel = $state(emptyForm());
	let shouldSave = $state(false);
	let connectionName = $state('');
	let loadingConnections = $state(false);
	let loadingSecrets = $state(false);
	let preparing = $state(false);
	let errorMessage = $state('');

	let selectedSecret = $derived(secrets.find((secret) => secret.name === form.secretName));
	let localeOptions = $derived({ locale: $locale });

	export function show() {
		errorMessage = '';
		if (!dialog.open) dialog.showModal();
		void refreshConnections();
	}

	export function close() {
		if (dialog.open) dialog.close();
	}

	async function refreshConnections() {
		loadingConnections = true;
		try {
			savedConnections = await listConnections();
		} catch (error) {
			errorMessage =
				error instanceof Error ? error.message : m.unable_load_connections({}, localeOptions);
			savedConnections = [];
		} finally {
			loadingConnections = false;
		}
	}

	async function refreshSecrets(selected = form.secretName) {
		loadingSecrets = true;
		try {
			secrets = await listSecrets();
			form.secretName = secrets.some((secret) => secret.name === selected) ? selected : '';
		} catch (error) {
			errorMessage = error instanceof Error ? error.message : m.unable_load_secrets({}, localeOptions);
			secrets = [];
			form.secretName = '';
		} finally {
			loadingSecrets = false;
		}
	}

	function applySavedConnection() {
		const connection = savedConnections.find((item) => item.name === savedName);
		if (!connection) return;
		form.host = connection.host;
		form.port = connection.port || '22';
		form.username = connection.username;
		form.privateKey = connection.private_key || '';
		form.password = '';
		form.passphrase = '';
		form.secretPassphrase = '';
		connectionName = connection.name;
		form.authMethod = connection.auth_type;
		form.passwordSource = connection.auth_type === 'password' && connection.secret_name ? 'secret' : 'manual';
		if (form.passwordSource === 'secret') void refreshSecrets(connection.secret_name || '');
	}

	function toggleSave() {
		if (shouldSave && !connectionName.trim()) connectionName = savedName || form.host.trim();
	}

	function settingsFromForm(): SavedConnection | null {
		if (!shouldSave) return null;
		const name = connectionName.trim();
		if (!name) throw new Error(m.connection_name_required({}, localeOptions));
		return {
			name,
			host: form.host.trim(),
			port: form.port.trim(),
			username: form.username.trim(),
			auth_type: form.authMethod,
			private_key: form.authMethod === 'key' ? form.privateKey.trim() : '',
			secret_name:
				form.authMethod === 'password' && form.passwordSource === 'secret' ? form.secretName : ''
		};
	}

	async function submit() {
		errorMessage = '';
		preparing = true;
		try {
			if (!form.host.trim() || !form.port.trim() || !form.username.trim()) {
				throw new Error(m.host_port_username_required({}, localeOptions));
			}
			const numericPort = Number(form.port);
			if (!Number.isInteger(numericPort) || numericPort < 1 || numericPort > 65535) {
				throw new Error(m.port_range({}, localeOptions));
			}

			const request: ConnectionSubmission['request'] = {
				host: form.host.trim(),
				port: form.port.trim(),
				username: form.username.trim()
			};
			if (form.authMethod === 'password') {
				if (form.passwordSource === 'manual') {
					request.password = form.password;
				} else {
					if (!selectedSecret) throw new Error(m.select_secret_error({}, localeOptions));
					if (selectedSecret.encryption === 'scrypt' && !form.secretPassphrase) {
						throw new Error(m.secret_requires_passphrase({}, localeOptions));
					}
					request.password = await revealSecret(selectedSecret.name, form.secretPassphrase);
				}
			} else {
				request.privateKey = form.privateKey.trim();
				request.passphrase = form.passphrase;
			}
			onconnect({ request, settings: settingsFromForm() });
		} catch (error) {
			errorMessage =
				error instanceof Error ? error.message : m.unable_prepare_connection({}, localeOptions);
		} finally {
			form.password = '';
			form.passphrase = '';
			form.secretPassphrase = '';
			preparing = false;
		}
	}

	function manageConnections() {
		close();
		onmanage();
	}
</script>

<dialog
	bind:this={dialog}
	class="modal modal-bottom sm:modal-middle"
	aria-labelledby="ssh-connection-title"
	onclick={(event) => event.target === dialog && close()}
>
	<div class="modal-box w-11/12 max-w-lg rounded-xl border border-base-300 p-0 shadow-2xl">
		<header class="flex items-center justify-between border-b border-base-300 px-6 py-4">
			<div>
				<p class="text-xs font-semibold uppercase tracking-[0.18em] text-base-content/50">
					{m.remote_shell({}, localeOptions)}
				</p>
				<h1 id="ssh-connection-title" class="text-lg font-semibold">
					{m.ssh_connection({}, localeOptions)}
				</h1>
			</div>
			<button
				class="btn btn-ghost btn-sm btn-square"
				type="button"
				aria-label={m.close({}, localeOptions)}
				onclick={close}
			>
				<span aria-hidden="true">×</span>
			</button>
		</header>

		<form
			class="grid max-h-[75vh] gap-4 overflow-y-auto px-6 py-5"
			onsubmit={(event) => {
				event.preventDefault();
				void submit();
			}}
		>
			{#if errorMessage}
				<div class="alert alert-error py-3 text-sm" role="alert">{errorMessage}</div>
			{/if}

			<div class="fieldset">
				<label class="fieldset-legend" for="saved-connection">
					{m.saved_connection({}, localeOptions)}
				</label>
				<div class="flex gap-2">
					<select
						id="saved-connection"
						class="select min-w-0 flex-1"
						bind:value={savedName}
						onchange={applySavedConnection}
						disabled={loadingConnections}
					>
						<option value="">
							{loadingConnections
								? m.loading({}, localeOptions)
								: m.new_connection({}, localeOptions)}
						</option>
						{#each savedConnections as connection}
							<option value={connection.name}>{connection.name}</option>
						{/each}
					</select>
					<button class="btn btn-primary" type="button" onclick={manageConnections}>
						{m.edit({}, localeOptions)}
					</button>
				</div>
			</div>

			<ConnectionForm bind:model={form} {secrets} {loadingSecrets} mode="connect" onrefreshsecrets={refreshSecrets} />

			<label class="flex cursor-pointer items-center gap-3 py-1">
				<input class="checkbox checkbox-sm" type="checkbox" bind:checked={shouldSave} onchange={toggleSave} />
				<span class="text-sm font-medium">{m.save_connection_settings({}, localeOptions)}</span>
			</label>
			{#if shouldSave}
				<label class="fieldset">
					<span class="fieldset-legend">{m.connection_name({}, localeOptions)}</span>
					<input class="input w-full" bind:value={connectionName} maxlength="80" required />
					<p class="fieldset-label">{m.credentials_never_saved({}, localeOptions)}</p>
				</label>
			{/if}

			<button class="btn btn-primary mt-1 w-full" type="submit" disabled={preparing}>
				{preparing ? m.preparing({}, localeOptions) : m.connect({}, localeOptions)}
			</button>
		</form>
	</div>
</dialog>
