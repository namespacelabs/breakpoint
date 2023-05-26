# Contributing to Breakpoint

## Where to Start

You can find good issues to tackle with labels [`good first issue`](https://github.com/namespacelabs/breakpoint/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22) and [`help wanted`](https://github.com/namespacelabs/breakpoint/issues?q=is%3Aissue+is%3Aopen+label%3A%22help+wanted%22).

## Issues tracking and Pull Requests

We don't enforce any rigid contributing procedure. We appreciate you spending time improving `breakpoint`!

If in doubt, please open a [new Issue](https://github.com/namespacelabs/breakpoint/issues/new) on GitHub. One of the maintainers will reach out soon, and you can discuss the next steps with them.

Please include relevant GitHub Issues in the PR message when opening a Pull Request.

## Development

Developing `breakpoint` requires `nix` and optionally `docker`. We use `nix` to ensure reproducible development flow: it guarantees the identical versions of dependencies and tools. While `docker` is required only if you plan to build the Docker image of the Rendezvous server.

Follow the instructions to install them for your operating system:

- [Install nix](https://github.com/DeterminateSystems/nix-installer)
- Docker: [Docker engine](https://docs.docker.com/engine/install/) or [OrbStack](https://docs.docker.com/engine/install/)

When `nix` is installed, you can:

- Run `nix develop` to enter a shell with every dependency pre-setup (e.g. Go, `buf`, etc.)
- Use the "nix environment selector" VSCode extension to apply a nix environment in VSCode.

### Building

Compiling the Go binaries:

```bash
$ go build -o . ./cmd/...

# Binaries available in the current working directory

$ ls breakpoint; ls rendezvous;
```

Installing the Go binaries:

```bash
$ go install ./cmd/...

# Binaries installed in $GOPATH

$ which breakpoint; which rendezvous;
```

Building the Docker image of Rendezvous server:

```bash
$ docker build . -t rendezvous:latest
```

### Protos

Breakpoint uses gRPC and protos to implement both internal and public API. Internal API is used between the `breakpoint wait` process and the rest of CLI commands. The public API is provided by the `rendezvous` server to accept incoming `breakpoint` registrations.

Whenever you change the protos definition under the [`api/`](../api) folder, then you must also regenerate the Go code:

```bash
$ buf generate
```

This will add changes to the Go files under the [`api/`](../api) folder. Include them in your commit.
