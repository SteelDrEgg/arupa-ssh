export type AuthMethod = 'password' | 'key';
export type PasswordSource = 'manual' | 'secret';
export type ConnectionState = 'idle' | 'connecting' | 'connected' | 'error';

export type SavedConnection = {
	name: string;
	host: string;
	port: string;
	username: string;
	auth_type: AuthMethod;
	private_key?: string;
	secret_name?: string;
};

export type ConnectionFormModel = {
	host: string;
	port: string;
	username: string;
	authMethod: AuthMethod;
	passwordSource: PasswordSource;
	password: string;
	privateKey: string;
	passphrase: string;
	secretName: string;
	secretPassphrase: string;
};

export type SecretMeta = {
	name: string;
	description?: string;
	encryption?: string;
};

export type ConnectRequest = {
	host: string;
	port: string;
	username: string;
	password?: string;
	privateKey?: string;
	passphrase?: string;
};

export type ConnectionSubmission = {
	request: ConnectRequest;
	settings: SavedConnection | null;
};

export type SocketMessage = {
	event: 'ssh_connected' | 'ssh_disconnected' | 'ssh_error' | 'terminal_output' | string;
	data?: unknown;
};
