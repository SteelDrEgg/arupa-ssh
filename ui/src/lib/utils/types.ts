export type AuthMethod = 'password' | 'key';
export type TargetType = 'address' | 'ssh_config';
export type PasswordSource = 'manual' | 'secret';
export type ConnectionState = 'idle' | 'connecting' | 'connected' | 'error';

export type SavedConnection = {
	name: string;
	target_type?: TargetType;
	host: string;
	port: string;
	username: string;
	auth_type: AuthMethod;
	private_key?: string;
	secret_name?: string;
};

export type ConnectionFormModel = {
	host: string;
	targetType: TargetType;
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

export type SecretProtection = 'identity' | 'passphrase';

export type SecretMeta = {
	name: string;
	description: string;
	allowed_plugins: string[];
	protection: SecretProtection;
	updated_at: string;
};

export type ConnectRequest = {
	target_type: TargetType;
	host: string;
	port: string;
	username: string;
	auth:
		| { method: 'password'; password: string }
		| { method: 'secret'; secret_name: string; secret_passphrase?: string }
		| { method: 'key'; private_key?: string; passphrase?: string };
};

export type ConnectionSubmission = {
	request: ConnectRequest;
	settings: SavedConnection | null;
};

export type SocketMessage = {
	event: 'ssh_connected' | 'ssh_disconnected' | 'ssh_error' | 'terminal_output' | string;
	data?: unknown;
};
