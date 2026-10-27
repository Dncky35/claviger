# Claviger — Secure Your Network, Own the Pipeline

> **Claviger** is a self-hosted Zero-Trust VPN gateway with an integrated app marketplace and automated security hardening. Build a private, segmented network across any number of devices — no cloud dependency, no vendor lock-in, no exposure.

---

## Why Claviger?

Most organizations still connect workloads and users the way they did a decade ago: open ports, trust everything on the internal network, and hope nothing gets scanned. That model is broken.

Claviger flips the script. It gives you a **zero-trust network** that runs entirely on your own infrastructure. You decide what is exposed, to whom, and for how long. Devices are enrolled explicitly, traffic is encrypted end-to-end, and every component sits behind identity verification. No public endpoints. No open SSH ports. No flat internal network.

Everything runs on commodity hardware. One server can act as the control plane for a handful of devices or a backbone for an entire fleet.

---

## What You Get

### 🔐 A Zero-Trust VPN Gateway

Think of Claviger as your own private pipe. Devices connect over an encrypted WireGuard overlay, and every connection is tied to a verified identity. There is no implicit trust. There is no open firewall facing the internet by default.

Once enrolled, a device can:

- **Reach internal services** on your chosen subnet without exposing them to the internet
- **Route traffic selectively** — through or around the tunnel, depending on what you need
- **Be revoked instantly** with a single command, cutting off access before it becomes a problem
- **Be assigned roles** that enforce least-privilege access to specific ports and services

Your gateway is your network boundary. Only the things you explicitly allow are reachable.

### 🛒 A Self-Hosted App Marketplace

Claviger ships with a curated **App Tab Marketplace** of services that belong on a private network:

- **AdGuard Home** — local DNS filtering and ad blocking, self-managed
- **Vaultwarden** — your own password vault, not a public SaaS
- **Nginx Proxy Manager** — reverse proxy, SSL, and access control for internal services
- **Plus eight additional curated services** you can deploy with a single action

Install, configure, and update just like any app store. Your data stays on your box. No telemetry, no cloud dependency, no external dependency on third-party infrastructure.

### 🛡️ Automated Security Hardening

A zero-trust network is useless if the gateway itself is exposed. Claviger's security automation takes care of the boring, error-prone parts so you don't have to:

- **Firewall-as-API** — inspect and control access to every port with a clear policy model
- **Public exposure detection** — flag services that are inadvertently reachable from the internet
- **Authenticated origin pulls** — protect web-facing nodes with Cloudflare, or isolate them entirely
- **SSH jail management** — automated fail2ban-style protection for administrative access
- **Crash recovery** — if the gateway becomes unreachable, the tunnel is force-disconnected to prevent unauthorized access

You still own the policy. The automation just removes the gaps that come from manual configuration.

### 🛡️ Disaster Recovery

Every deployment generates a **Disaster Recovery Key** (AES-256) during setup. Keep it somewhere safe.

If your server fails, the key is the only way to decrypt and restore your network configuration and application data. It is never stored in plaintext and cannot be recovered if lost.

---

## How It Works

Claviger is built around a clean separation between **control** and **data**:

1. **Control Plane** — your server runs the identity engine, enrollment portal, app marketplace, and security policy. This is the only interface you manage.
2. **Data Plane** — each enrolled device establishes an encrypted WireGuard tunnel over the overlay network. Nothing passes through the control plane. Nothing touches your public network unless you explicitly allow it.

End users interact with one of two interfaces:

- **Desktop GUI** — download the Claviger app, enroll a device in minutes, choose what it connects to
- **CLI** — same capabilities, scriptable and headless for automation

---

## Quick Start

### On Your Server (Linux)

```bash
# Install the server
curl -sSL https://api.cloudrocean.com/v1/installers/claviger-server/latest | sudo bash

# Run interactive setup
sudo claviger-server setup
```

The setup wizard walks you through three steps:

1. **Generate an identity** for your node
2. **Validate network ports** for the VPN and management hub
3. **Enroll yourself** as the first admin

When setup is complete, start the daemon:

```bash
sudo systemctl start claviger-server
```

### Enroll a Client

There are two ways to do this.

**Desktop GUI (recommended for first-time enrollment):**

1. Open the Claviger Desktop App
2. Generate a connection request from the client
3. Paste the token into the server setup terminal
4. Copy the approval token back into the client

**CLI (for scripted or headless enrollment):**

```bash
# Client
sudo claviger-client generate

# Server
sudo claviger-server register

# Client
sudo claviger-client approve <server-approval-token>

# Client
sudo claviger-client connect
```

That's it. Your device is now on the private network with verified access.

### Verify

```bash
# List connected clients
./claviger-server list
```

You should see your enrolled clients with status `active`.

### Revoke

```bash
# Revoke a single client
./claviger-server revoke <client-id>
```

Done. Access cut immediately.

---

## Supported Platforms

| Platform | Role | Notes |
|---|---|---|
| Linux (x86_64, ARM64) | Server, Client | CLI |
| Windows (x86_64) | Client | GUI + CLI + Windows service |
| macOS | NA | NA | 

---

## For Developers

If you want to dig into the code, the source tree is organized as two independent modules.

- `claviger-server` — Go daemon, WireGuard control, app orchestration, security policy
- `claviger-client` — Go UI, VPN management, CLI, cross-platform packaging

Each module can be built and tested independently. Refer to the detailed [ARCHITECTURAL-README.md](ARCHITECTURAL-README.md) in the repository root for full technical documentation.

---

## License

[GNU AGPL v3 license](LICENSE)

---

## Support

- **Issues**: [GitHub Issues](https://github.com/<your-org>/claviger/issues)
- **Documentation**: [`docs/`](docs/)
- **Community**: *[your preferred channel]*
