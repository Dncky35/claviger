# 🗝️ Claviger: Zero Trust VPN Gateway & Internal Developer Platform

Claviger is an enterprise-grade Zero Trust networking appliance and self-hosted IDP designed for seamless infrastructure orchestration. It operates as a high-performance service daemon on Linux, providing deterministic routing, automated IP Address Management (IPAM), and a state-aware application marketplace. 

By decoupling the **Control Plane** (centralized identity and enrollment) from the **Data Plane** (kernel-level WireGuard transit), Claviger ensures that sensitive cryptographic material and private network traffic never leave your sovereign infrastructure.

---

## 🏗️ Architecture Overview

The Claviger ecosystem is architected as a distributed system composed of two primary pillars:

*   **`claviger-server` (The Node Engine):** A Golang-based daemon that manages the Linux networking stack. It utilizes `wgctrl` for kernel-level WireGuard manipulation, Docker Compose for application lifecycle management, and an embedded SQLite instance for local state persistence.
*   **`claviger-client` (The Management Interface):** A cross-platform GUI built with Fyne, enabling secure administrative enrollment via cryptographic "Visa" tokens. It facilitates remote node management and secure peer provisioning without direct SSH exposure.

---

## 🚀 Core Technical Features

### 1\. Network Engineering & Zero Trust


*   **Hot-Injection Peer Management:** Claviger performs zero-reload peer injection. New devices are added to the WireGuard interface (wg0) in real-time using wgctrl.ConfigureDevice, ensuring seamless M2M connectivity without dropping existing tunnel sessions.
*   **Sequential 1808X Port Allocation:** To avoid port collisions, the IDP implements a collision-aware dynamic allocation strategy. Services are automatically assigned ports in a 100-port block starting from the user-defined hub\_port (defaulting to the 1808X range).
*   **Automated IPAM:** The system manages a virtual overlay subnet (default 10.8.0.0/24). It dynamically discovers available addresses, reserving .1 for the Hub and sequentially assigning .2 through .254 to peers.  
*   **Role-Based Access Control (RBAC):** Claviger enforces identity-aware network segmentation by assigning granular roles to connected clients, ensuring strict adherence to the principle of least privilege:
    *   **Hub Access Role:** Grants administrative privileges to interact with the gateway management plane, modify system configurations, and manage infrastructure lifecycle.
    *   **Global Routing Role:** Enables full network traversal for designated "Master" nodes, allowing them to route traffic across the entire VPN overlay and access all internal services.
    *   **Service-Bound Role:** Restricts client access to specific IP/Port mappings, effectively isolating individual developer tools (like Gitea or Vaultwarden) and preventing lateral movement within the network.


### 2\. Security Automation & OS Hardening

*   **RESTful Firewall-as-API:** A comprehensive security controller exposes UFW (Uncomplicated Firewall) management via a secure API. It features a smart scanner that detects and flags "Critical" vulnerabilities, such as public SSH (22) or Database exposure.
*   **Cloudflare Authenticated Origin Pulls (Opt-In):** For web-facing nodes, users can optionally enable an automated lockdown for ports 80/443. When activated, Claviger fetches live Cloudflare IP ranges and reconfigures UFW to drop all traffic not originating from Cloudflare’s edge, with an option to auto-update these IP ranges on a schedule.
*   **Failure Protection:** Integrated **Fail2Ban** orchestration allows admins to manage SSH jails, monitor banned IPs, and perform surgical unbans directly from the management UI.    
*   **Systemd Integration:** The daemon manages its own lifecycle as a standard Linux service, ensuring high availability and automatic recovery after host reboots.


### 3. Infrastructure Orchestration & PaaS
Claviger transforms standard Linux instances into a private PaaS through its **"App Tab" Marketplace**.
*   **State-Aware Orchestration:** The engine dynamically generates and injects environment-specific variables into Docker Compose templates for services like **Vaultwarden**, **AdGuard Home**, and **Nginx Proxy Manager**.
*   **Deterministic Dependency Gates:** System-core apps (e.g., NPM) are treated as master gateways, ensuring required network bridges (`cloudrocean-net`) and shared configuration volumes (Cloudflare IP lists) are provisioned before service instantiation.

---

## 🛠️ Tech Stack Breakdown

| Layer | Technology | Purpose |
| :--- | :--- | :--- |
| **Core Logic** | Golang 1.21+ | High-concurrency daemon execution |
| **VPN Kernel** | WireGuard / `wgctrl` | High-performance encrypted transit |
| **Orchestration** | Docker Compose | Application container lifecycle |
| **Database** | SQLite (Pure Go) | Low-overhead local state persistence |
| **Desktop UI** | Fyne (Go-GUI) | Cross-platform management client |

---

## 🚀 Installation & Bootstrapping

### 1. Server Provisioning (Linux)
Execute the following on your target Linux server to begin the Zero-Trust enrollment:

```bash
# Build the daemon
go build -o claviger-server .

# Start the interactive setup wizard (Requires Root)
sudo ./claviger-server setup
```

**The Setup Flow:**
1.  **Identity Generation:** The node generates a unique UUID and cryptographic keypair.
2.  **Network Validation:** The wizard validates UDP port availability (default `51820`) and TCP port availability for the management hub (default `18080`).
3.  **Endpoint Verification:** Performs DNS/IP verification to ensure your node is reachable by clients.
4.  **Admin Enrollment:** You will be prompted to paste a "Connection Request" token from your Claviger Desktop Client to link the node.

### 2. Service Management
Once configured, Claviger installs itself as a `systemd` unit:

```bash
sudo systemctl start claviger
sudo systemctl enable claviger
```

### 3. Client Enrollment
1.  Open the **Claviger Desktop App**.
2.  Click **"Generate Connection Request"**.
3.  Paste the resulting token into the server's setup terminal.
4.  The server will provide an **Approval Token (Visa)**. Paste this back into the client to establish the secure tunnel.

---

## 🛡️ Disaster Recovery
During setup, a **Disaster Recovery Key** (AES-256) is generated. 

> ⚠️ **IMPORTANT:** Save this key immediately. If the server is destroyed, this key is the only way to decrypt and restore your network configurations and application data from backups. It is never stored in plain text and cannot be recovered if lost.