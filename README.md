# traefik-cloudflare-client-ip

A Traefik middleware plugin that gives your backends the visitor's real IP when Traefik
sits behind Cloudflare, without letting visitors forge it.

## Why

- Cloudflare **appends** the visitor's address to any `X-Forwarded-For` the visitor sent.
  Backends that read the first entry (Authentik, Django apps, many others) log and rate-limit
  whatever the visitor chose.
- `CF-Connecting-IP` is reliable *from Cloudflare*. But if your origin is reachable directly,
  anyone can send that header too.

## What it does

It looks at the connection's own address, never a header:

| Request comes from | Result |
|---|---|
| Cloudflare's ranges, with a valid `CF-Connecting-IP` | `X-Forwarded-For` and `X-Real-Ip` are replaced with that address |
| Cloudflare's ranges, header missing or invalid | `CF-Connecting-IP` is removed, nothing else changes |
| Anywhere else | `CF-Connecting-IP` is removed; `X-Forwarded-For` / `X-Real-Ip` are left to Traefik |

Cloudflare's published ranges (IPv4 and IPv6) are built in, current as of the release date.

## Use

Apply it on the entrypoint, so every router gets it. Don't list Cloudflare in the
entrypoint's `forwardedHeaders.trustedIPs`: Traefik then replaces forwarded headers from every
source it doesn't trust, and this middleware is the one place that trusts Cloudflare.

Static configuration (CLI flags shown):

```
--experimental.plugins.cfclientip.moduleName=github.com/nkcx/traefik-cloudflare-client-ip
--experimental.plugins.cfclientip.version=v0.1.0
--entrypoints.websecure.http.middlewares=cf-client-ip@docker
```

Dynamic configuration (here as labels on the Traefik container):

```
traefik.http.middlewares.cf-client-ip.plugin.cfclientip.header=CF-Connecting-IP
```

### Options

| Option | Default | Meaning |
|---|---|---|
| `header` | `CF-Connecting-IP` | Header holding the visitor's address |
| `trustedCIDRs` | Cloudflare's ranges | Replaces the built-in list (e.g. if Cloudflare adds a range before a new release) |

## License

MIT
