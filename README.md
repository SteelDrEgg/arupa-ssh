# Arupa SSH

This is a SSH service built for [Arupa](https://github.com/SteelDrEgg/Arupa).

## Layout

- `core/` contains the gRPC service, inherited HTTP server, WebSocket transport,
  SSH connection handling, saved profiles, and tests.
- `ui/` is a SvelteKit static application. Reusable components live under
  `ui/src/lib/components`, transport and API helpers under `ui/src/lib/utils`,
  and the terminal entry route under `ui/src/routes/index.html`.
- `ui/locale/` contains the Paraglide language catalogs. Locale
  resolution uses `localStorage["arupa.language"]`, then the browser language,
  then English.
- `ui/static/` contains the plugin icons and the vendored xterm browser runtime.

## Frontend Contract

The terminal page opens `/ssh/ws`. Every text frame is a JSON envelope:

```json
{"event":"connect_ssh","data":{"host":"example.com","port":"22","username":"alice"}}
```

Client events are:

- `connect_ssh`: `{ host, port, username, password?, privateKey?, passphrase? }`
- `terminal_input`: raw terminal input string
- `resize`: `{ cols, rows }`
- `disconnect`: cleanup signal

Server events use the same envelope and are `ssh_connected`, `terminal_output`,
`ssh_error`, and `ssh_disconnected`. One WebSocket owns at most one SSH session;
closing the WebSocket cancels an in-progress connection and closes the session.

The terminal page reads Secret Manager metadata from `GET /keys` and reveals a
selected password through `POST /keys/reveal`. Secret Manager is a password
source, not a separate SSH authentication method. The authenticated browser
performs these same-origin HTTP calls, and only the revealed value is sent as
the SSH password.

## Saved connections

Authenticated clients can list or upsert connection profiles at:

```text
GET  /ssh/api/connections
POST /ssh/api/connections
```

Profiles are persisted as one readable Param group per profile.
For a profile named `host1`, the entries are:

```text
connection.host1.host = "localhost"
connection.host1.port = "22"
connection.host1.username = "root"
connection.host1.auth = "{password, local-password}"
```

`auth` uses `{password}` or `{password, secret-name}` for password
authentication, and `{key}` or `{key, /path/to/private-key}` for key
authentication. The second value is a Secret Manager reference or a
key path, not a password, private-key value, or passphrase.

## Build

```sh
npm --prefix ui install
make build
```

The result is `ssh` plus the static UI under `ui/build`. Run all
checks with:

```sh
make check
```

Create an installable plugin archive with:

```sh
make package
```

The archive is written to `plugins/ssh.plg`. It contains `info.yaml`,
`Content/ssh`, and the compiled UI at `Content/ui/build`.

## Example config

```toml
[Services.ssh]
Restart = "always"
RunAsUser = ""

[Services.ssh.Params]
ssh_config_path = "~/.ssh/config"
"connection.host1.host" = "localhost"
"connection.host1.port" = "22"
"connection.host1.username" = "root"
"connection.host1.auth" = "{key, ~/.ssh/id_ed25519}"
```
