<script lang="ts">
	import ConnectionForm from '$lib/components/ConnectionForm.svelte';
	import * as m from '$lib/paraglide/messages.js';
	import {
		deleteConnection,
		listConnections,
		listSecrets,
		updateConnection
	} from '$lib/utils/api';
	import { locale } from '$lib/utils/locale';
	import type { ConnectionFormModel, SavedConnection, SecretMeta } from '$lib/utils/types';

	function emptyForm(): ConnectionFormModel {
		return {
			host: '',
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

	function formFromConnection(connection: SavedConnection): ConnectionFormModel {
		return {
			...emptyForm(),
			host: connection.host,
			port: connection.port || '22',
			username: connection.username,
			authMethod: connection.auth_type,
			passwordSource: connection.secret_name ? 'secret' : 'manual',
			privateKey: connection.private_key || '',
			secretName: connection.secret_name || ''
		};
	}

	let dialog: HTMLDialogElement;
	let connections: SavedConnection[] = $state([]);
	let secrets: SecretMeta[] = $state([]);
	let editingOriginalName = $state<string | null>(null);
	let connectionName = $state('');
	let form: ConnectionFormModel = $state(emptyForm());
	let loadingConnections = $state(false);
	let loadingSecrets = $state(false);
	let saving = $state(false);
	let deletingName = $state('');
	let errorMessage = $state('');
	let localeOptions = $derived({ locale: $locale });

	export function show() {
		errorMessage = '';
		editingOriginalName = null;
		if (!dialog.open) dialog.showModal();
		void refreshConnections();
	}

	export function close() {
		if (dialog.open) dialog.close();
	}

	async function refreshConnections() {
		loadingConnections = true;
		try {
			connections = await listConnections();
		} catch (error) {
			errorMessage =
				error instanceof Error ? error.message : m.unable_load_connections({}, localeOptions);
			connections = [];
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

	function edit(connection: SavedConnection) {
		errorMessage = '';
		editingOriginalName = connection.name;
		connectionName = connection.name;
		form = formFromConnection(connection);
		if (form.authMethod === 'password') void refreshSecrets(form.secretName);
	}

	function backToList() {
		errorMessage = '';
		editingOriginalName = null;
		connectionName = '';
		form = emptyForm();
	}

	function editedConnection(): SavedConnection {
		const name = connectionName.trim();
		if (!name) throw new Error(m.connection_name_required({}, localeOptions));
		if (!form.host.trim() || !form.port.trim() || !form.username.trim()) {
			throw new Error(m.host_port_username_required({}, localeOptions));
		}
		const numericPort = Number(form.port);
		if (!Number.isInteger(numericPort) || numericPort < 1 || numericPort > 65535) {
			throw new Error(m.port_range({}, localeOptions));
		}
		return {
			name,
			host: form.host.trim(),
			port: form.port.trim(),
			username: form.username.trim(),
			auth_type: form.authMethod,
			private_key: form.authMethod === 'key' ? form.privateKey.trim() : '',
			secret_name: form.authMethod === 'password' ? form.secretName : ''
		};
	}

	async function save() {
		if (!editingOriginalName) return;
		errorMessage = '';
		saving = true;
		try {
			await updateConnection(editingOriginalName, editedConnection());
			await refreshConnections();
			backToList();
		} catch (error) {
			errorMessage = error instanceof Error ? error.message : m.unable_save_connection({}, localeOptions);
		} finally {
			saving = false;
		}
	}

	async function remove(connection: SavedConnection) {
		errorMessage = '';
		deletingName = connection.name;
		try {
			await deleteConnection(connection.name);
			connections = connections.filter((item) => item.name !== connection.name);
		} catch (error) {
			errorMessage = error instanceof Error ? error.message : m.unable_delete_connection({}, localeOptions);
		} finally {
			deletingName = '';
		}
	}
</script>

<dialog
	bind:this={dialog}
	class="modal modal-bottom sm:modal-middle"
	aria-labelledby="saved-connections-title"
	onclick={(event) => event.target === dialog && close()}
>
	<div class="modal-box w-11/12 max-w-xl rounded-xl border border-base-300 p-0 shadow-2xl">
		<header class="flex items-center justify-between border-b border-base-300 px-6 py-4">
			<div>
				<p class="text-xs font-semibold uppercase tracking-[0.18em] text-base-content/50">
					{m.remote_shell({}, localeOptions)}
				</p>
				<h1 id="saved-connections-title" class="text-lg font-semibold">
					{editingOriginalName
						? m.edit_connection({}, localeOptions)
						: m.manage_saved_connections({}, localeOptions)}
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

		<div class="max-h-[75vh] overflow-y-auto px-6 py-5">
			{#if errorMessage}
				<div class="alert alert-error mb-4 py-3 text-sm" role="alert">{errorMessage}</div>
			{/if}

			{#if editingOriginalName}
				<form
					class="grid gap-4"
					onsubmit={(event) => {
						event.preventDefault();
						void save();
					}}
				>
					<button class="btn btn-ghost btn-sm justify-start" type="button" onclick={backToList}>
						← {m.back_to_connections({}, localeOptions)}
					</button>

					<label class="fieldset">
						<span class="fieldset-legend">{m.connection_name({}, localeOptions)}</span>
						<input class="input w-full" bind:value={connectionName} maxlength="80" required />
						<p class="fieldset-label">{m.credentials_never_saved({}, localeOptions)}</p>
					</label>

					<ConnectionForm bind:model={form} {secrets} {loadingSecrets} mode="saved" onrefreshsecrets={refreshSecrets} />

					<button class="btn btn-primary w-full" type="submit" disabled={saving}>
						{saving ? m.preparing({}, localeOptions) : m.save_changes({}, localeOptions)}
					</button>
				</form>
			{:else if loadingConnections}
				<div class="flex justify-center py-10"><span class="loading loading-spinner" aria-label={m.loading({}, localeOptions)}></span></div>
			{:else if connections.length === 0}
				<p class="py-8 text-center text-sm text-base-content/60">{m.no_saved_connections({}, localeOptions)}</p>
			{:else}
				<div class="grid gap-3">
					<h2 class="text-sm font-semibold">{m.saved_connections({}, localeOptions)}</h2>
					{#each connections as connection (connection.name)}
						<div class="flex items-center gap-3 rounded-lg border border-base-300 p-3">
							<div class="min-w-0 flex-1">
								<p class="truncate font-medium">{connection.name}</p>
								<p class="truncate text-sm text-base-content/60">
									{m.connection_summary(
										{ username: connection.username, host: connection.host, port: connection.port },
										localeOptions
									)}
								</p>
								<p class="text-xs text-base-content/50">
									{connection.auth_type === 'key'
										? m.private_key_authentication({}, localeOptions)
										: m.password_authentication({}, localeOptions)}
								</p>
							</div>
							<div class="flex shrink-0 gap-1">
								<button class="btn btn-ghost btn-sm" type="button" onclick={() => edit(connection)}>
									{m.edit({}, localeOptions)}
								</button>
								<button
									class="btn btn-ghost btn-sm text-error"
									type="button"
									disabled={deletingName === connection.name}
									onclick={() => void remove(connection)}
								>
									{m.delete({}, localeOptions)}
								</button>
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	</div>
</dialog>
