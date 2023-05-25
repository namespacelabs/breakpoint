# Rendezvous Server Setup

The `rendezvous` source code is 100% open-source and you can self-host it wherever you want.

## Requirements

Rendezvous Server needs two main properties in order to function:

1. Public IP
2. The process can listen to any port
3. Traffic to all ports is allowed in both directions (ingress and egress)

## Fly.io Deployment

Breakpoint provides a ready-to-deploy Fly.io configuration.

Create a Fly.io application.

```bash
$ flyctl apps create rendezvous
```

Allocate a public IPv4 address and assign it to the application. Note that this is a paid feature of Fly.io.

```bash
$ flyctl ips allocate-v4 -a rendezvous
```

Take note of the public IPv4 address created before and deploy the `rendezvous` service.

```bash
$ flyctl deploy -a rendezvous --env PROXY_PUBLIC={public_ip}
```

Done! Now your instance of Rendezvous Server is listening to `{public_ip}:5000` endpoint.
