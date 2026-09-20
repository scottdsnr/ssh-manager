# ssh-manager

A bubbletea TUI for your SSH hosts. View, create, edit, delete, group and
enable/disable the `Host` blocks in `~/.ssh/config`, writing straight to the
file `ssh` already reads. Anything it does not manage — `Match` blocks,
`Include` lines, your own comments — is left byte for byte as it was.

## Install

```sh
go install github.com/scotthellings/ssh-manager@latest
```

## First run

On first start you are asked for the path to your ssh config; `~/.ssh/config`
is the default, and `ctrl+n` cycles through the files found on your machine.
Settings are reachable later with `s`, or `ssh-manager -setup`, and live in
`~/.config/ssh-manager/config.json`. Settings also carries the UI accent
colour.

## Keys

| key | action |
|---|---|
| `↑`/`↓`, `k`/`j` | move |
| `enter` | fold a group / edit the host under the cursor |
| `n` | new host |
| `c` | `ssh` to the host under the cursor |
| `space` | comment the host block out, or back in |
| `d` | delete the host or group under the cursor |
| `m` | move the host to another group |
| `a` / `r` | add a group / rename the group under the cursor |
| `/` | filter by name, keyword or value |
| `s` | settings |
| `?` | help |
| `q` | quit |

## Fields

The host form has a field each for the keywords most hosts need:

| field | what it is |
|---|---|
| `Host` | the short name you type: `ssh <name>` |
| `HostName` | the real host or IP |
| `User` | login user |
| `Port` | non-standard port |
| `IdentityFile` | private key to offer |
| `ProxyJump` | bastion to hop through |
| `LocalForward` | `8000 localhost:3306` style tunnel |
| Comment | a note, kept on the `Host` line |

Everything else goes in the **Other** box as plain `Keyword value` lines,
one per line, in the order you want them written. Keyword capitalisation is
corrected for you. Common ones worth knowing:

- `RemoteForward` / `DynamicForward` — reverse tunnel, SOCKS proxy
- `ProxyCommand` — for anything `ProxyJump` cannot express
- `IdentitiesOnly yes` — stop ssh offering every key in your agent
- `ForwardAgent yes` — agent forwarding (only for hosts you trust)
- `AddKeysToAgent yes` — add the key to the agent on first use
- `ServerAliveInterval 60` / `ServerAliveCountMax 3` — keep idle sessions up
- `Compression yes` — helps on slow links
- `StrictHostKeyChecking` / `UserKnownHostsFile` — host key policy
- `ControlMaster auto` / `ControlPath` / `ControlPersist 10m` — reuse one
  connection for many sessions
- `RequestTTY` / `RemoteCommand` — run something on connect
- `SetEnv` / `SendEnv` — pass environment through
- `PreferredAuthentications publickey` — skip password prompts
- `LogLevel` — turn up ssh's own noise while debugging

## How it stores things

Groups are plain comments, so the file stays readable and portable:

```
# ===== Tunnels =====
Host forge-tunnel # work db
    HostName 68.183.254.60
    User forge
    LocalForward 8000 localhost:3306
```

Disabled hosts are commented out with `#!`, which is how ssh-manager tells
its own disabled blocks from your hand-written comments. Every save writes
atomically and keeps the previous file as `config.bak`.
