# 🗝️ Project Claviger

Claviger is a Zero Trust, Edge-Networking SaaS designed to automate and secure WireGuard deployments. It operates as a decentralized network appliance, providing secure machine-to-machine (M2M) provisioning, automated IP Address Management (IPAM), and role-based micro-segmentation.

Similar in architecture to enterprise solutions like Tailscale or Netmaker, Claviger separates the **Control Plane** (Auth & Billing) from the **Data Plane** (Network Routing).

---

## 🏗️ Architecture



Claviger is divided into three distinct components:

### 1. The Control Plane (SaaS Backend)
A centralized API that handles user authentication, license validation, and node provisioning. It generates one-time Setup Keys and permanent API tokens for the edge servers.
* **Tech:** Python, FastAPI, PostgreSQL, SQLAlchemy.
* **Security:** API route rate-limiting via Redis, cryptographic token hashing, strict Zero Trust endpoint verification.

### 2. The Edge Node (`claviger-server`)
A lightweight, self-contained daemon that runs on the user's Linux server. It handles the actual network routing, automatically configuring WireGuard interfaces and `iptables` rules based on the license state.
* **Tech:** Golang, embedded SQLite (`modernc.org/sqlite`), WireGuard.
* **Features:** Hot-reloads configs without dropping connections (`wg syncconf`), completely isolated embedded database, automated hourly heartbeat pings.

### 3. The Dashboard (Frontend)
A modern, multi-tenant web application where IT Admins can manage their Claviger networks, generate Setup Keys, and monitor node health.
* **Tech:** Next.js, React, Tailwind CSS, Framer Motion.
* **Features:** Subdomain routing (e.g., `claviger.cloudrocean.com`), Edge Middleware for strict security headers, responsive modern UI.

---

## 🚀 Features

* **Zero-Setup Provisioning:** Admins install the edge node via `apt-get` and authenticate using a single-use Setup Key. No MAC address tracking or manual configuration files required.
* **Decentralized Data Plane:** The central SaaS never sees private WireGuard keys or network traffic.
* **Fail-Silent Heartbeats:** Edge nodes ping the control plane hourly. If the SaaS goes down, nodes enter a 72-hour grace period to keep the VPN alive.
* **Role-Based Access Control:** Instantiate distinct firewall rules dynamically based on user roles (e.g., restricting standard users from accessing specific ports on the subnet).

---

## 🛠️ Tech Stack

**Frontend & API Gateway:**
* Next.js 14+ (App Router)
* React & Framer Motion
* Tailwind CSS

**Backend (Control Plane):**
* Python 3.10+
* FastAPI
* PostgreSQL & Alembic (Migrations)
* Redis (Rate Limiting)

**Edge Node (Daemon):**
* Golang 1.21+
* SQLite (Pure Go driver)
* WireGuard (`wg-tools`)

---

## 📁 Repository Structure

```text
.
├── backend/               # FastAPI Control Plane
│   ├── app/
│   │   ├── core/          # Database config, auth, rate limiters
│   │   ├── models/        # SQLAlchemy ORM models (Account, ClavigerNode, SetupKey)
│   │   ├── schemas/       # Pydantic validation models
│   │   └── api/           # Routers (/auth, /nodes, /dashboard)
│   ├── alembic/           # Database migrations
│   └── requirements.txt
├── edge-node/             # Golang Claviger Daemon
│   ├── main.go            # CLI setup and daemon router
│   ├── go.mod
│   └── (embedded SQLite)
├── frontend/              # Next.js Dashboard
│   ├── app/
│   │   ├── claviger-app/  # Claviger specific routing
│   │   └── (main app)
│   ├── components/
│   └── middleware.ts      # Edge proxy and security headers
├── .gitignore
└── .gitattributes