<img src="https://raw.githubusercontent.com/namespacelabs/breakpoint/main/docs/imgs/breakpoint-banner.png" alt="Breakpoint. Debug with SSH. Resume." width="400" height="200">

[![Discord](https://img.shields.io/badge/Join-Namespace-blue?color=blue&label=Discord&logo=discord&logoColor=3eb0ff&style=flat-square)](https://discord.gg/DqMzDFR6Hc)
[![Twitter Follow](https://img.shields.io/badge/Follow-Namespace_Labs-blue?logo=twitter&style=flat-square)](https://twitter.com/intent/follow?screen_name=namespacelabs)
[![GitHub Actions](https://img.shields.io/badge/GitHub-Action-blue?logo=githubactions&style=flat-square)](https://github.com/namespacelabs/breakpoint-action)
![GitHub](https://img.shields.io/github/license/namespacelabs/breakpoint?color=blue&label=License&style=flat-square)
![Build](https://img.shields.io/github/actions/workflow/status/namespacelabs/breakpoint/build.yml?label=Build&style=flat-square)
![Checks](https://img.shields.io/github/actions/workflow/status/namespacelabs/breakpoint/checks.yml?label=Checks&style=flat-square)

# Breakpoint

Add breakpoints to CI (e.g. GitHub Action workflows): pause workflows, access the workflow with SSH, debug and resume executions.

## What is Breakpoint

Have you ever wished you could have debugged an issue in CI (e.g. GitHub Actions), by SSHing to where your build or tests are running?

Breakpoint helps you create breakpoints in CI: stop the execution of the workflow, and jump in to live debug as needed with SSH (without compromising end-to-end encryption).

You can make changes, re-run commands, and resume the workflow as needed. Need more time? Just run `breakpoint extend` to extend your breakpoint duration.

And it's 100% open-source (both client and server).

> ℹ️ Workflows that have active breakpoints are still "running" and continue to count towards your total CI usage.

## Using Breakpoint

Breakpoint loves GitHub Actions. You can use the [Breakpoint Action](https://github.com/namespacelabs/breakpoint-action) to add a breakpoint to a GitHub workflow; but most importantly, you can add breakpoints that only trigger when there's a failure in the workflow.

The example below triggers the Breakpoint only if the previous step (i.e. `go test`) failed. When that happens, Breakpoint pauses the workflow for 30 minutes and allows SSH from GitHub users "jack123" and "alice321".

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

When Breakpoint activates, it will output on a regular basis how much time left
there is in the breakpoint, and which address to SSH to get to the workflow.

```bash
┌───────────────────────────────────────────────────────────────────────────┐
│                                                                           │
│ Breakpoint running until 2023-05-24T16:06:48+02:00 (29 minutes from now). │
│                                                                           │
│ Connect with: ssh -p 40812 runner@rendezvous.namespace.so                 │
│                                                                           │
└───────────────────────────────────────────────────────────────────────────┘
```

You can now SSH the runner, re-run builds or tests, and even do changes.

If you need more time, run `breakpoint extend` to extend the breakpoint duration
by 30 more minutes (or extend by more with the `--for` flag).

When you are done, you can end the breakpoint session with `breakpoint resume`.

By default, the Breakpoint Action uses a shared `rendezvous` server provided by
Namespace Labs for free. Even though a shared server is used, your SSH traffic is always _encrypted end-to-end_ (see Architecture).

Check out the [Breakpoint Action](https://github.com/namespacelabs/breakpoint-action) for more details on
what arguments you can set.

### Using the Breakpoint CLI to create a breakpoint

To activate a breakpoint, you can run:

```bash
$ breakpoint wait --config config.json
```

The config file can look like as follows:

```json
{
  "endpoint": "rendezvous.namespace.so:5000",
  "shell": ["/bin/bash"],
  "allowed_ssh_users": ["runner"],
  "authorized_keys": [],
  "authorized_github_users": ["<your-github-username>"],
  "duration": "30m"
}
```

The `wait` command will block the caller and print an SSH endpoint that you can connect to:

```bash
┌───────────────────────────────────────────────────────────────────────────┐
│                                                                           │
│ Breakpoint running until 2023-05-24T16:06:48+02:00 (29 minutes from now). │
│                                                                           │
│ Connect with: ssh -p 40812 runner@rendezvous.namespace.so                 │
│                                                                           │
└───────────────────────────────────────────────────────────────────────────┘
```

Once you are logged into the SSH session, you can use breakpoint CLI to extend the breakpoint duration, or resume the workflow (i.e. exit the `wait`):

- `breakpoint extend --for 60m`: extend the wait period for 30m more minutes
- `breakpoint resume`: stops Breakpoint process and release the control flow to the caller of the `wait` command

## Architecture

Breakpoint consists of two main components: `rendezvous` (where public connections are terminated) and `breakpoint`.

When a breakpoint is created, the CLI blocks until an expiration time has passed.

Meanwhile, it establishes a QUIC connection to `rendezvous`, which allocates a
public endpoint (with a random port) that will be reverse proxied back to the
running `breakpoint`; each connection then serves a SSH session (from a ssh
service embedded in `breakpoint`). SSH sessions do not start new user sessions,
and always run commands using the same uid as the parent `breakpoint wait` as
well.

The first QUIC stream `breakpoint -> rendezvous` is used for gRPC; `rendezvous`
expects a `Register` stream in order to allocate an endpoint, and will serve
that endpoint while the corresponding gRPC stream is active.

Because the SSH session is established end-to-end, `rendezvous` is not capable of performing a man-in-the-middle attack.

![architecture](docs/imgs/Breakpoint%20high-level%20view.png)

The CLI implements pausing by blocking the caller process. The command
`breakpoint wait` blocks until either the user runs `breakpoint resume` or the
wait-timer expires. The communication between the `wait` process and the CLI is
implemented with gRPC.

On receive a connection, `rendezvous` establishes a new QUIC stream over the
same connection that was registered previously, in the direction `rendezvous -> breakpoint` and performs dumb TCP proxying over it, without the need of additional framing.

The lack of additional framing in addition to QUIC's streams having independent
control flow (i.e. no shared head of the line blocking), make QUIC a perfect
solution for this type of reverse proxying (in fact, cloudflare uses similar
techniques in Cloudflare Tunnel).

## Authentication

The SSH service in `breakpoint` only accepts sessions from pre-referenced keys or public SSH keys configured by GitHub users. These are specified in the configuration file when the breakpoint is created (or as arguments to the GitHub action).

You can specify GitHub usernames in the `github_usernames` config field. Breakpoint automatically fetches the SSH public keys from GitHub for these users. You can also specify the SSH keys directly via the `authorized_keys` field.

The SSH service always spawns processes with the same uid as `breakpoint wait`, and by default accepts any requested username. This can be limited by setting the `allowed_ssh_users` configuration field.

For example, the following `config.json` allows access to "jack123" and "alice321" GitHub users with a SSH user called "runner".

```json
{
  "allowed_ssh_users": ["runner"],
  "authorized_github_users": ["jack123", "alice321"]
}
```

### GitHub-based authentication (via OIDC)

`breakpoint` is able to request a fresh GitHub-emitted workflow identifying token, that it sends to `rendezvous`.

`rendezvous` has the ability to verify these, and performs access control based on the repository where the invocation was originated.

Even if no access control is enforced, repository information is logged by `rendezvous` if available.

## Using Namespace's shared Rendezvous

Namespace Labs runs a public `rendezvous` server that is open to everyone. But you can also run your own (see below).

Although `rendezvous` facilitates pushing bytes to workloads running in workers (which would otherwise not be able to offer services), the bytes it proxies are not cleartext. Breakpoint establishes end-to-end ssh sessions.

To use the shared `rendezvous`, use the following endpoint:

```json
{
  "endpoint": "rendezvous.namespace.so:5000"
}
```

## Running Rendezvous yourself

See our [documentation](docs/server-setup.md) on how to run your own instance of `rendezvous`.

## Roadmap

Here's a list of features that we'd to tackle but haven't gotten to yet.

1. Traffic rate limiting: neither the Rendezvous Server nor the Breakpoint client restrict network traffic that is proxied. So far this hasn't been an issue because GitHub runners themselves are network capped.
2. The Rendezvous Server does not implement a control and monitoring Web UI.
3. Neither the Rendezvous Server nor the Breakpoint client expose metrics.
4. The Breakpoint session does not automatically extend itself if an SSH connection is active. You need to explicitly extend the session with `breakpoint extend`.
5. Configurable ACLs on the Rendezvous Server to specify the list of repositories and organizations allowed to connect to the server.
6. Support for more authentication schemes between `breakpoint` and `rendezvous`. Breakpoint client and Rendezvous Server only support GitHub's OIDC-based authentication today.
7. Team and Organization authorization of users in Breakpoint client's SSH service (i.e. specifying a team or org rather than individual usernames).

## Contributions

Breakpoint welcomes your help! We appreciate your time and effort.

If you find an issue in Breakpoint or you see a missing feature, feel free to open an [Issue](https://github.com/namespacelabs/breakpoint/issues) on GitHub.

Check out our [contribution guidelines](docs/CONTRIBUTING.md) for more details on how to develop Breakpoint.

## Join the Community

If you have questions, ideas or feedback, chat with the team on our [Discord server](https://community.namespace.so/discord).
