<script lang="ts">
	import * as m from '$lib/paraglide/messages.js';
	import { locale } from '$lib/utils/locale';
	import type { ConnectionFormModel, SecretMeta } from '$lib/utils/types';

	let {
		model = $bindable<ConnectionFormModel>(),
		secrets = [],
		loadingSecrets = false,
		mode = 'connect',
		onrefreshsecrets
	}: {
		model: ConnectionFormModel;
		secrets?: SecretMeta[];
		loadingSecrets?: boolean;
		mode?: 'connect' | 'saved';
		onrefreshsecrets: (selected?: string) => void;
	} = $props();

	let selectedSecret = $derived(secrets.find((secret) => secret.name === model.secretName));
	let secretNeedsPassphrase = $derived(selectedSecret?.encryption === 'scrypt');
	let localeOptions = $derived({ locale: $locale });

	function selectAuth(method: ConnectionFormModel['authMethod']) {
		model.authMethod = method;
		if (method === 'password' && mode === 'connect' && model.passwordSource === 'secret' && secrets.length === 0) {
			onrefreshsecrets(model.secretName);
		}
	}

	function changePasswordSource() {
		model.secretPassphrase = '';
		if (model.passwordSource === 'secret' && secrets.length === 0) onrefreshsecrets(model.secretName);
	}
</script>

<div class="grid gap-4">
	<div class="grid grid-cols-[minmax(0,1fr)_7rem] gap-3">
		<label class="fieldset">
			<span class="fieldset-legend">{m.host({}, localeOptions)}</span>
			<input
				class="input w-full"
				bind:value={model.host}
				placeholder={m.host_placeholder({}, localeOptions)}
				autocomplete="url"
				required
			/>
		</label>
		<label class="fieldset">
			<span class="fieldset-legend">{m.port({}, localeOptions)}</span>
			<input class="input w-full" type="number" bind:value={model.port} min="1" max="65535" required />
		</label>
	</div>

	<label class="fieldset">
		<span class="fieldset-legend">{m.username({}, localeOptions)}</span>
		<input class="input w-full" bind:value={model.username} autocomplete="username" required />
	</label>

	<div
		class="tabs tabs-box grid grid-cols-2"
		role="tablist"
		aria-label={m.authentication_method({}, localeOptions)}
	>
		<button
			class:tab-active={model.authMethod === 'password'}
			class="tab"
			type="button"
			role="tab"
			aria-selected={model.authMethod === 'password'}
			onclick={() => selectAuth('password')}
		>
			{m.password({}, localeOptions)}
		</button>
		<button
			class:tab-active={model.authMethod === 'key'}
			class="tab"
			type="button"
			role="tab"
			aria-selected={model.authMethod === 'key'}
			onclick={() => selectAuth('key')}
		>
			{m.private_key({}, localeOptions)}
		</button>
	</div>

	{#if model.authMethod === 'password'}
		{#if mode === 'connect'}
			<label class="fieldset">
				<span class="fieldset-legend">{m.password_source({}, localeOptions)}</span>
				<select class="select w-full" bind:value={model.passwordSource} onchange={changePasswordSource}>
					<option value="manual">{m.enter_manually({}, localeOptions)}</option>
					<option value="secret">{m.secret_manager({}, localeOptions)}</option>
				</select>
			</label>
		{/if}

		{#if mode === 'connect' && model.passwordSource === 'manual'}
			<label class="fieldset">
				<span class="fieldset-legend">{m.password({}, localeOptions)}</span>
				<input class="input w-full" type="password" bind:value={model.password} autocomplete="current-password" />
			</label>
		{:else}
			<label class="fieldset">
				<span class="fieldset-legend flex items-center justify-between">
					<span>{m.secret({}, localeOptions)}</span>
					<button
						class="btn btn-ghost btn-xs"
						type="button"
						disabled={loadingSecrets}
						onclick={() => onrefreshsecrets(model.secretName)}
					>
						{m.refresh({}, localeOptions)}
					</button>
				</span>
				<select class="select w-full" bind:value={model.secretName} disabled={loadingSecrets}>
					{#if mode === 'saved'}
						<option value="">{m.no_linked_secret({}, localeOptions)}</option>
					{:else}
						<option value="">
							{loadingSecrets
								? m.loading_secrets({}, localeOptions)
								: m.select_secret({}, localeOptions)}
						</option>
					{/if}
					{#each secrets as secret}
						<option value={secret.name}>
							{secret.name}{secret.encryption === 'scrypt'
								? ` ${m.passphrase_suffix({}, localeOptions)}`
								: ''}
						</option>
					{/each}
				</select>
				<p class="fieldset-label">
					{mode === 'saved'
						? m.saved_connection_secret_help({}, localeOptions)
						: selectedSecret?.description || m.selected_secret_password_help({}, localeOptions)}
				</p>
			</label>
			{#if mode === 'connect' && secretNeedsPassphrase}
				<label class="fieldset">
					<span class="fieldset-legend">{m.secret_manager_passphrase({}, localeOptions)}</span>
					<input class="input w-full" type="password" bind:value={model.secretPassphrase} autocomplete="off" />
				</label>
			{/if}
		{/if}
	{:else}
		<label class="fieldset">
			<span class="fieldset-legend">{m.private_key_path({}, localeOptions)}</span>
			<input
				class="input w-full"
				bind:value={model.privateKey}
				placeholder={m.private_key_placeholder({}, localeOptions)}
			/>
		</label>
		{#if mode === 'connect'}
			<label class="fieldset">
				<span class="fieldset-legend">
					{m.passphrase({}, localeOptions)}
					<span class="font-normal text-base-content/50">{m.optional({}, localeOptions)}</span>
				</span>
				<input class="input w-full" type="password" bind:value={model.passphrase} autocomplete="off" />
			</label>
		{/if}
	{/if}
</div>
