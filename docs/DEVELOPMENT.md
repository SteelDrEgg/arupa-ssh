# Arupa SSH development notes

This document describes implementation contracts and build details for
maintainers. User-facing setup and endpoint information is in [README.md](README.md).

## Architecture

- `core/` implements the Arupa gRPC service, HTTP handlers, WebSocket transport,
  SSH application, SSH transport operations, and saved connection persistence.
- The service serves HTTP through the Kernel's inherited `proxy` listener and
  registers the `ssh-proxy` transport and `/ssh/` routes.
- `ui/` is a SvelteKit static application. Shared components are in
  `ui/src/lib/components`, API and transport helpers in `ui/src/lib/utils`,
  and the terminal route in `ui/src/routes/index.html`.
- `ui/locale/` contains Paraglide language catalogs. Locale selection uses
  `localStorage["arupa.language"]`, then the browser language, then English.
- `ui/static/` contains plugin icons and the vendored xterm browser runtime.

The Kernel owns service Params. At startup, the service reads its initial Params
snapshot; profile changes use the SDK Params patch API, and the Kernel persists
those changes. The SSH service keeps one in-memory profile map for its single
process instance and updates it after a successful Params write.

## HTTP and WebSocket contracts

The static terminal is served from `/ssh/pages/`. It uses `/ssh/ws` for the SSH
terminal session and the HTTP APIs listed below. The Kernel's access policies
remain the outer access-control boundary; the route registration for `/ssh/`
does not itself require authentication.

### Saved connections

`/ssh/api/connections` accepts JSON and returns JSON:

- `GET` returns `{ "success": true, "connections": [...] }`.
- `POST` accepts a saved connection object and creates a profile.
- `PUT` accepts `{ "original_name": "...", "connection": { ... } }` and
  updates or renames a profile.
- `DELETE` accepts `{ "name": "..." }` and removes a profile.

A connection object has `name`, `target_type`, `host`, `port`, `username`, and
`auth_type`; key authentication may include `private_key`, while password
authentication may include `secret_name`. `target_type` is `address` or
`ssh_config`. The API never persists a password, Secret Manager value, or
passphrase. Profile writes are persisted through Kernel Params before the
in-memory profile map is changed.

In Params, a profile named `host1` uses these entries:

```toml
"connection.host1.host" = "example.com"
"connection.host1.target_type" = "address"
"connection.host1.port" = "22"
"connection.host1.username" = "alice"
"connection.host1.auth" = "{password, production-password}"
```

The `auth` value is `{password}` for a password entered at connection time,
`{password, secret-name}` for a Secret Manager reference, `{key}` for a key
entered at connection time, or `{key, /path/to/private-key}` for a key path.
The second value is always a reference or path, never secret material.

### Secret Manager

`GET /ssh/api/secrets` asks the `secret-manager` service to list available
entries. When connecting with a selected entry, the SSH service reveals the
value through Arupa service messaging and supplies it to the SSH client
server-side. The browser sends the Secret Manager name and an optional
passphrase; the revealed value is not sent to the browser. Secret Manager must
allow this service to list and reveal the selected entry.

### SSH WebSocket

The browser connects to `/ssh/ws`. Each text frame is a JSON envelope with
`event` and `data` fields. Client events are:

```json
{"event":"connect_ssh","data":{"target_type":"address","host":"example.com","port":"22","username":"alice","auth":{"method":"password","password":"secret"}}}
```

- `connect_ssh`: `{ target_type, host, port?, username?, auth }`. `target_type`
  is `address` or `ssh_config`; in config mode, `host` is an explicit alias
  from `ssh_config_path` and username or port can come from that alias. Auth is
  exactly one of `{ method: "password", password }`,
  `{ method: "secret", secret_name, secret_passphrase? }`, or
  `{ method: "key", private_key?, passphrase? }`.
- `terminal_input`: terminal input as a string.
- `resize`: `{ cols, rows }`.
- `disconnect`: closes the current SSH session.

Server events use the same envelope: `ssh_connected`, `terminal_output`,
`ssh_error`, and `ssh_disconnected`. One WebSocket owns at most one SSH session.
Closing it cancels an in-progress connection and closes an established
session.

## SSH configuration and host keys

`ssh_config_path` enables alias-based connections. The current supported
OpenSSH directives are `User`, `HostName`, `Port`, `IdentityFile`,
`HostKeyAlias`, and `ConnectTimeout`.

Host key checking is opt-in. If `ssh_known_hosts_path` is unset, the SSH client
accepts any server host key. If it is set, the service loads that file and
strictly checks the server key; a missing file, unknown host, or changed key
fails the connection. When using `HostKeyAlias`, that alias is used for the
known-host lookup. Configure a valid `known_hosts` file to prevent connecting
to an impersonated server.

## Build and package

Install the UI dependencies and build both components:

```sh
npm --prefix ui install
make build
```

`make build` creates the `ssh` executable and static files under `ui/build`.
The package target creates `plugins/ssh.plg`, containing `info.yaml`, the
service executable at `Content/ssh`, and the static UI at `Content/ui/build`:

```sh
make package
```

The repository also defines `make check` for frontend checks and Go tests.
