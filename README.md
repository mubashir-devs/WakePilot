# WakePilot

A lightweight, standalone Wake-on-LAN manager. One Go binary serves a REST API and an embedded React dashboard; data lives in a single SQLite file.

* Register computers (MAC, IP, broadcast, UDP port, interface) and wake them with one click.
* Interface- and subnet-aware broadcast resolution: nothing assumes `192.168.1.255`, and ambiguous setups are rejected instead of guessed.
* Honest results: *packet sent* ≠ *device reachable* ≠ *device status unknown*. A silent device is never reported as "off".
* Wake history, availability monitoring, local-only by default, bearer-token auth for remote access.

## Quick start

Requirements: Go ≥ 1.22 and Node ≥ 20 (build time only; the binary has no runtime dependencies).

```sh
make build            # builds the frontend, then bin/wakepilot with the UI embedded
./bin/wakepilot       # http://127.0.0.1:8080
```

Config is optional. To customise: `cp configs/config.example.yaml config.yaml` (picked up automatically) or `./bin/wakepilot -config path.yaml`.

## Using it

1. **Network** page: confirm your interface and its broadcast address.
2. **Add Device**: name + MAC are required. Choose an interface (or give an IP so it can be matched to one). Leave *Broadcast* empty to use the interface's broadcast address.
3. **Devices** → **Wake**. The toast says the packet was *sent*. If the device has an IP, WakePilot then probes it for up to `verify_timeout` and updates the history entry to *Reachable* or *Not confirmed*.

Target machines must have Wake-on-LAN enabled in BIOS/UEFI and the NIC driver, and be on the same Layer-2 network as the interface (broadcasts don't cross routers).

## Architecture

Modular monolith; dependencies point inward and the WoL engine depends on nothing else.

```
cmd/server          main: flags, logging, HTTP server, graceful shutdown
internal/app        composition root (wires everything; reused by tests with fakes)
internal/api        net/http routing, middleware (auth, host/origin checks), JSON, validation mapping
internal/service    use cases: DeviceService, WakeService (+ post-wake verifier), Monitor, Overview
internal/device     domain types, input validation, repository interfaces
internal/network    interface discovery, subnet/broadcast math, Resolve() with explicit errors
internal/wol        MAC parsing, 102-byte packet builder, Sender interface + UDP sender, Engine
internal/storage    SQLite (modernc, pure Go) repositories + migration runner
internal/config     YAML config, validation, env overrides
migrations/         embedded *.up.sql schema migrations
frontend/           React + TypeScript + Vite + Tailwind; dist/ embedded via go:embed
tests/              full-stack integration tests (HTTP → SQLite → engine) with fake networking
```

Wake flow: `POST /devices/{id}/wake` → load device → `network.Resolve` (interface / broadcast / source IP) → `wol.Engine.Wake` (validate MAC → build packet → `Sender.Send`) → record in `wake_history` → respond. If the device has an IP, a background verifier then probes it and updates `reachability`.

### Broadcast resolution (never guesses)

Precedence: explicit broadcast → device's interface → configured default interface → the single interface whose subnet contains the device IP. Multiple candidates → `ambiguous` error; none → `no matching interface`. When an interface is chosen the socket is bound to its address so the packet leaves through it.

### Availability

ICMP needs raw-socket privileges, so reachability uses TCP probes (`monitor.ports`): a completed connection **or an active refusal** proves the host is up. No answer is shown as **"No response"**, which can mean off, asleep, or firewalled.

## REST API

All responses: `{"data": …}` or `{"error": {"code", "message", "details"?}}`. Validation → `422`, unknown id → `404`, send failure → `502`.

| Method | Path | |
|---|---|---|
| GET | `/api/v1/health` | status, version, `authRequired` (always open) |
| POST / GET | `/api/v1/devices` | create / list (with availability) |
| GET / PUT / DELETE | `/api/v1/devices/{id}` | |
| POST | `/api/v1/devices/{id}/wake` | send magic packet; returns `packetSent`, `reachability: pending` |
| POST | `/api/v1/devices/{id}/check` | probe now |
| GET | `/api/v1/wake-history?limit=&offset=&deviceId=` | newest first, with `meta.total` |
| GET | `/api/v1/network/interfaces` | interfaces, subnets, broadcasts |
| GET | `/api/v1/overview`, `/api/v1/settings` | dashboard data; non-secret config |

## Security model

* Binds to `127.0.0.1` by default. Binding anywhere else **refuses to start** unless `auth.enabled` and a token ≥ 16 chars are set (prefer `WAKEPILOT_AUTH_TOKEN`).
* With auth on, every endpoint except `/health` needs `Authorization: Bearer <token>` (constant-time comparison).
* Host-header allow-list (DNS-rebinding) and same-origin check on state-changing requests.
* **No broadcast relay:** a device's broadcast address must be a local subnet's broadcast or `255.255.255.255`; anything else is rejected at save and at send time. Wake targets only come from saved devices; the API accepts no arbitrary destination.
* Strict JSON decoding (unknown fields rejected, 1 MiB cap), security headers + CSP, internal errors never leak to clients, `/settings` omits the token and DB path.
* Plain HTTP: if you expose it beyond loopback, put it behind a TLS reverse proxy (and add its hostname to `server.allowed_hosts`).

## Development

```sh
make dev-backend      # Go API on :8080 (uses configs/config.example.yaml)
make dev-frontend     # Vite on :5173, proxies /api to :8080
make test             # go vet + go test + frontend typecheck
```

Tests need no physical machines: the engine is tested with fake senders and a loopback UDP socket; the integration suite swaps in a fake sender, interface lister and availability checker via `app.Options`.

## Building standalone binaries

```sh
make build                 # current platform → bin/wakepilot
make dist                  # linux/darwin (amd64, arm64) + windows/amd64 → dist-bin/
```

`CGO_ENABLED=0` (pure-Go SQLite), so cross-compiling needs only the Go toolchain. Set the version with `VERSION=1.0.0 make dist`.

## Known limitations

* IPv4 only (interfaces, broadcast, device IPs); the `network` types and resolver are the extension point for IPv6/multicast.
* Settings are read-only in the UI: edit the YAML and restart.
* With multiple interfaces and `255.255.255.255`, the OS routing table decides the egress interface unless an interface is selected (we bind the source address, which doesn't force routing on every OS). Prefer directed broadcasts (the default when an interface is chosen).
* Reachability can't tell "off" from "firewalled"; devices that answer none of the probe ports will show *Not confirmed*.
* Single shared token, no users/roles, no rate limiting, no TLS built in.
* Windows and macOS binaries are cross-compiled but have not been run on those platforms. The race detector was not run (needs CGO/gcc, absent on the dev machine).

## Future improvements

IPv6/multicast WoL; ICMP probe option where privileged; per-device custom probe ports; Wake-on-WAN via a hardened relay agent on the target LAN; editable settings persisted in SQLite; TLS and per-user auth; scheduled wakes; import from ARP/DHCP; Secure-On password support.
