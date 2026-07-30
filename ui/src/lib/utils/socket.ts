import * as m from '$lib/paraglide/messages.js';
import type { SocketMessage } from './types';

export function terminalWebSocketURL(): string {
	const url = new URL('/ssh/ws', window.location.href);
	url.protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
	return url.toString();
}

export function sendSocket(socket: WebSocket | null, event: string, data: unknown = null): boolean {
	if (!socket || socket.readyState !== WebSocket.OPEN) return false;
	socket.send(JSON.stringify({ event, data }));
	return true;
}

export function parseSocketMessage(raw: unknown): SocketMessage {
	if (typeof raw !== 'string') throw new Error(m.invalid_ssh_response());
	const message = JSON.parse(raw) as Partial<SocketMessage>;
	if (!message || typeof message.event !== 'string') {
		throw new Error(m.invalid_ssh_response());
	}
	return message as SocketMessage;
}
