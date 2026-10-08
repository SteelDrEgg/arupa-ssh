# Arupa SSH

Arupa SSH is a service for [Arupa](https://github.com/SteelDrEgg/Arupa) that
provides a browser-based terminal for connecting to SSH servers. You
can connect with a password, a private key, or a password stored in Arupa Secret
Manager, and save connection profiles for later.

## Info

```text
Name: ssh
Type: grpc
```

## HTTP endpoints

| Endpoint | Method | Description | Access control |
| --- | --- | --- | --- |
| `/ssh/` | `GET`, `HEAD` | Opens the SSH terminal. | The service route does not require authentication. Kernel access rules still apply. |
| `/ssh/pages/` | `GET` | Serves the terminal application and its static files. | The service route does not require authentication. Kernel access rules still apply. |
| `/ssh/api/connections` | `GET`, `POST`, `PUT`, `DELETE` | Lists, saves, updates, and deletes saved connection profiles. | Governed by Kernel access rules. |
| `/ssh/api/secrets` | `GET` | Lists Secret Manager entries available to this service. | Governed by Kernel access rules. |
| `/ssh/ws` | `GET` (WebSocket) | Opens an interactive SSH terminal session. | Governed by Kernel access rules. |

The Kernel's route and service access policies determine who can reach these
endpoints. Review those policies when exposing the SSH service to other users
or networks. Secret Manager must also permit the SSH service to list and reveal
the entries you want to use.

## Configuration

The SSH service accepts these optional settings in its service Params:

| Setting | Description |
| --- | --- |
| `ssh_config_path` | Path to an OpenSSH config file. Enables connections using aliases from that file. |
| `ssh_known_hosts_path` | Optional path to an existing `known_hosts` file. When configured, SSH server keys are verified against it; unknown or changed keys fail the connection. If omitted, server keys are accepted without verification. |

Connection profiles can be created and managed in the terminal. Profiles keep
connection details and references to credentials, but do not save passwords,
private-key contents, or passphrases. For password authentication, choose a
Secret Manager entry or enter a password for the current session. For key
authentication, provide a private-key path available to the SSH service.

Example service configuration:

```toml
[Services.ssh]
Restart = "always"
RunAsUser = ""

[Services.ssh.Params]
ssh_config_path = "~/.ssh/config"
ssh_known_hosts_path = "~/.ssh/known_hosts"
```

## Build and install

Build the service and its web terminal with:

```sh
make build
```

Create the installable Arupa plugin archive with:

```sh
make package
```

The package is written to `plugins/ssh.plg`.

## Development

Implementation notes, API contracts, and local development details are in
[DEVELOPMENT.md](DEVELOPMENT.md).
