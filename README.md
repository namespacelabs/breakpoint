[![](https://dcbadge.vercel.app/api/server/DqMzDFR6Hc?style=flat-square&compact=true)](https://discord.gg/DqMzDFR6Hc)
[![Twitter Follow](https://img.shields.io/badge/Follow-Namespace_Labs-blue?logo=twitter&style=flat-square)](https://twitter.com/intent/follow?screen_name=namespacelabs)
[![GitHub Actions](https://img.shields.io/badge/GitHub-Action-blue?logo=githubactions&style=flat-square)](https://github.com/namespacelabs/breakpoint-action)

# Breakpoint

Insert breakpoints in CI: pause workflows, SSH access to live environments, and resume executions.

## What is Breakpoint

Have you ever needed to pause a CI run (e.g. GitHub Actions) and SSH inside the environment to debug why it fails? Breakpoint exactly solves this problem, and it's 100% open-source.

Breakpoint pauses the execution of CI workflows and waits for SSH connections. When you are done debugging the CI environment, you can resume the workflow to continue its run. In case you need more time in the SSH session, you can extend the time the workflow remains paused.

## Use Breakpoint

Breakpoint loves GitHub Actions. You add Breakpoints to your GitHub Actions CI with the [Breakpoint Action](https://github.com/namespacelabs/breakpoint-action).

The example below triggers the Breakpoint only if the previous step (i.e. `go test`) failed. When that happens, the Breakpoint pauses the workflow for 30 minutes and allows SSH from GitHub users "jack123" and "alice321".

```yaml
jobs:
  go-tests:
    runs-on: ubuntu-latest

    permissions:
      id-token: write
      contents: read

    steps:
      - name: Checkout
        uses: actions/checkout@v3

      - name: Run Go tests
        runs: |
          go test ./...

      - name: Breakpoint if tests failed
        if: failure()
        uses: namespacelabs/breakpoint-action@v0
        with:
          duration: 30m
          authorized-users: jack123, alice321
```

When the Breakpoint activates, you will see the following output in the GitHub Action logs. It tells you where you can connect SSH to:

```bash
┌───────────────────────────────────────────────────────────────────────────┐
│                                                                           │
│ Breakpoint running until 2023-05-24T16:06:48+02:00 (29 minutes from now). │
│                                                                           │
│ Connect with: ssh -p 40812 runner@breakpoint.namespace.so                 │
│                                                                           │
└───────────────────────────────────────────────────────────────────────────┘
```

You can now SSH inside the target system and explore the live environment. If you need more time, you can run `breakpoint extend` to get 30 more minutes of SSH session (for custom value see `--for` flag). When you are done, you can end the breakpoint session with `breakpoint resume`.

By default, the Breakpoint Action uses the `rendezvous` server hosted by Namespace Labs. See the [Breakpoint Action](https://github.com/namespacelabs/breakpoint-action) for more details on the arguments.

### Use Breakpoint CLI

To activate the breakpoint, you can run:

```bash
$ breakpoint wait --config config.json
```

The config file can look like as follows:

```json
{
  "endpoint": "breakpoint.namespace.so:5000",
  "login_shell": ["/bin/bash"],
  "allowed_ssh_users": ["example"],
  "authorized_keys": [],
  "github_usernames": ["<your-github-username>"],
  "initial_duration": "30m"
}
```

The `wait` command will block the caller and print the SSH endpoint where you can connect to:

```bash
┌───────────────────────────────────────────────────────────────────────────┐
│                                                                           │
│ Breakpoint running until 2023-05-24T16:06:48+02:00 (29 minutes from now). │
│                                                                           │
│ Connect with: ssh -p 40812 runner@breakpoint.namespace.so                 │
│                                                                           │
└───────────────────────────────────────────────────────────────────────────┘
```

Once you are logged into the SSH session, you can use breakpoint CLI to extend or close the Breakpoint session:

- `breakpoint extend --for 30m`: extend the wait period for 30m more minutes
- `breakpoint resume`: stops Breakpoint process and release the control flow to the caller of the `wait` command

## Architecture at 20,000 feet

Breakpoint consists of two main components: `rendezvous` server and `breakpoint` CLI.

The CLI pauses the execution of CI workflows and opens a QUIC connection to the server, signaling the intent to open a new breakpoint session. The server, in turn, exposes an SSH socket with a public port it randomly assigns for the breakpoint session.

![architecture](docs/imgs/Breakpoint%20high-level%20view.png)

The CLI implements pausing by blocking the caller process. The command `breakpoint wait` blocks until either the user runs `breakpoint resume` or the wait-timer expires. The communication between the `wait` process and the CLI is implemented with gRPC.

The `wait` command also runs `sshd` service, which terminates SSH connections and opens the shell sessions.

The Rendezvous Server proxies the TCP connections through the QUIC connections the `breakpoint` CLI established earlier. This means the SSH connection remains end-to-end encrypted between the SSH client and the target CI environment.

## Authentication

The SSH service accepts only the users or SSH keys listed in the `config.json` file when the `breakpoint wait` was called.

You can specify GitHub usernames in the `github_usernames` config field. Breakpoint automatically fetches the SSH public keys from GitHub for these users. You can also specify the SSH keys directly with the `authorized_keys` field.

The set of allowed users can be listed in the `allowed_ssh_users` field.

For example, the following `config.json` allows access to "jack123" and "alice321" GitHub users with SSH user called "runner".

```json
{
  "allowed_ssh_users": ["runner"],
  "github_usernames": ["jack123", "alice321"]
}
```

### GitHub OIDC

The Breakpoint client can optionally share the GitHub's OIDC token with the Rendezvous Server. The server uses the token to verify the GitHub Action's details - such as the organization, repository name and owner username - and enforce ACLs.

This helps with protecting the Rendezvous Server from being used by unexpected users.

## Use Namespace Rendezvous Server

Namespace Labs run a public Rendezvous Server free to use. You only need to configure the endpoint in the `config.json` file.

```json
{
  "endpoint": "breakpoint.namespace.so:5000"
}
```

## Run the Rendezvous Server

The server can be deployed to any cloud provider, it just needs to be accessible from your laptop and from the CI environment. See [documentation](doc/server-setup.md) to run your own instance of the Rendezvous Server.

## Roadmap

Here's a list of features that we'd to tackle but haven't gotten to it yet.

1. Traffic rate limiting: neither the Rendezvous Server nor the Breakpoint client restrict network traffic that is proxied. So far this hasn't been an issue because GitHub runners themselves are network capped.
2. The Rendezvous Server does not implement a control and monitoring Web UI.
3. Neither the Rendezvous Server nor the Breakpoint client expose metrics.
4. The Breakpoint session does not automatically extend itself if an SSH connection is active. You need to explicitly extend the session with `breakpoint extend`.
5. Configurable ACLs on the Rendezvous Server to specify the list of repositories and organizations allowed to connect to the server.
6. Support for more authentication schemes between `breakpoint` and `rendezvous`. Breakpoint client and Rendezvous Server only support GitHub's OIDC-based authentication today.
7. Team and Organization authorization of users in Breakpoint client's SSH service (i.e. specifying a team or org rather than individual usernames).

## Contributions

Breakpoint needs your help! We appreciate your time and effort.

If you find an issue in Breakpoint or you see a missing feature, please free to open an [Issue](https://github.com/namespacelabs/breakpoint/issues) on GitHub.

Check out the [contributing guidelines](docs/CONTRIBUTING.md) for more details on how to develop Breakpoint.

## Join the Community

If you questions, ideas or feedback, you are welcome to join our [Discord server](https://community.namespace.so/discord).
