import * as m from '$lib/paraglide/messages.js';
import type { SavedConnection, SecretMeta } from './types';

type ApiPayload = {
	success?: boolean;
	message?: string;
	[key: string]: unknown;
};

const secretCollectionPath = '/secret-manager/secrets';

async function requestJSON<T extends ApiPayload>(path: string, options: RequestInit = {}): Promise<T> {
	const response = await fetch(path, {
		credentials: 'same-origin',
		cache: 'no-store',
		...options,
		headers: {
			Accept: 'application/json',
			'Content-Type': 'application/json',
			...(options.headers || {})
		}
	});

	let payload: ApiPayload;
	try {
		payload = (await response.json()) as ApiPayload;
	} catch {
		throw new Error(m.unexpected_response({ path }));
	}
	if (!response.ok || payload.success === false) {
		throw new Error(payload.message || m.request_failed_status({ status: String(response.status) }));
	}
	return payload as T;
}

export async function listConnections(): Promise<SavedConnection[]> {
	const payload = await requestJSON<ApiPayload & { connections: SavedConnection[] }>(
		'/ssh/api/connections'
	);
	return Array.isArray(payload.connections) ? payload.connections : [];
}

export async function saveConnection(connection: SavedConnection): Promise<void> {
	await requestJSON('/ssh/api/connections', {
		method: 'POST',
		body: JSON.stringify(connection)
	});
}

export async function updateConnection(
	originalName: string,
	connection: SavedConnection
): Promise<void> {
	await requestJSON('/ssh/api/connections', {
		method: 'PUT',
		body: JSON.stringify({ original_name: originalName, connection })
	});
}

export async function deleteConnection(name: string): Promise<void> {
	await requestJSON('/ssh/api/connections', {
		method: 'DELETE',
		body: JSON.stringify({ name })
	});
}

export async function listSecrets(): Promise<SecretMeta[]> {
	const payload = await requestJSON<ApiPayload & { keys: SecretMeta[] }>(secretCollectionPath);
	return Array.isArray(payload.keys) ? payload.keys : [];
}

export async function revealSecret(name: string, passphrase: string): Promise<string> {
	const payload = await requestJSON<ApiPayload & { value: string }>(
		`${secretResourcePath(name)}/reveal`,
		{
			method: 'POST',
			body: JSON.stringify(passphrase ? { passphrase } : {})
		}
	);
	if (typeof payload.value !== 'string') throw new Error(m.selected_secret_empty());
	return payload.value;
}

function secretResourcePath(name: string): string {
	// Browsers normalize a single-dot path segment, including its encoded form.
	if (name === '.') return `${secretCollectionPath}/~.`;
	return `${secretCollectionPath}/${encodeURIComponent(name)}`;
}
