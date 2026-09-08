# Railway-style Open Source PaaS on AWS — Architecture Plan

> Historical design proposal. Some choices below (canvas, ECR, EBS/S3, SSE) are not implemented in this alpha. Use [current architecture](architecture.md), [user guide](user-guide.md) and [roadmap](ROADMAP.md) for actual behavior.

## Goal

Coolify ကို architecture/reference အနေနဲ့ယူပြီး၊ Railway လို developer UX ရှိတဲ့ lightweight self-hosted PaaS တစ်ခုကို AWS-first အနေနဲ့ဆောက်မယ်။

အဓိက direction က:

> **Coolify-style self-hosted control plane + Railway-style developer UX + AWS EC2-first runtime**

Coolify ကို fork လုပ်တာထက် architecture/reference အနေနဲ့ယူပြီး အသစ်ရေးတာပိုသင့်တယ်။

---

## Recommended Stack

**Backend + Agent = Go**

Laravel အစား Go သုံးတာကို recommend လုပ်မယ်။

| Component | Tech |
|---|---|
| Web UI | React + TypeScript + Vite |
| UI Styling | Tailwind + shadcn-style components |
| Railway Canvas | React node/flow canvas |
| API | **Go** |
| Node Agent | **Go** |
| Database | PostgreSQL |
| Job Queue | PostgreSQL-backed queue initially |
| Real-time Logs | WebSocket / SSE |
| Builder | **Railpack + BuildKit** |
| Runtime | Docker initially |
| Reverse Proxy | Traefik |
| Registry | AWS ECR |
| Storage / Backups | EBS + S3 |
| Secrets | AES envelope encryption / AWS KMS |
| AWS SDK | AWS SDK for Go |

### Why Go?

Rust က memory/runtime efficiency ကောင်းပေမယ့် development velocity က Go ထက်နှေးနိုင်တယ်။

ဒီ project မှာ:

- Docker API
- SSH / networking
- AWS API
- streaming logs
- concurrent jobs
- long-running agents
- health checks
- process orchestration

စတာတွေများလို့ Go က sweet spot ဖြစ်တယ်။

Node/Bun backend နဲ့လည်းရပေမယ့် platform daemon + agent လို infrastructure software အတွက် Go ကိုပိုသင့်တယ်။

Frontend ကို React သီးသန့်ထား။ Railway UX လို interaction-heavy UI လုပ်ဖို့ React ပိုကောင်းတယ်။

---

# Core Architecture

```text
                       ┌──────────────────────┐
                       │      React UI        │
                       │   Railway-like UX    │
                       └──────────┬───────────┘
                                  │ HTTPS / WS
                                  ▼
┌────────────────────────────────────────────────────┐
│                  CONTROL PLANE                     │
│                                                    │
│  ┌─────────────┐      ┌────────────────────────┐  │
│  │   Go API    │─────▶│      PostgreSQL        │  │
│  └──────┬──────┘      │ projects/services/etc │  │
│         │             └────────────────────────┘  │
│         │                                          │
│  ┌──────▼──────┐       ┌───────────────────────┐ │
│  │ Job Manager │       │ GitHub App/Webhooks   │ │
│  └──────┬──────┘       └───────────────────────┘ │
└─────────┼──────────────────────────────────────────┘
          │
          │ gRPC / WebSocket + mTLS
          │
     ┌────▼──────────────────────────────────────────┐
     │              AWS EC2 WORKER NODE             │
     │                                               │
     │  ┌───────────────┐                            │
     │  │   Go Agent    │                            │
     │  └───────┬───────┘                            │
     │          │                                    │
     │   ┌──────▼───────┐       ┌────────────────┐  │
     │   │ Railpack     │──────▶│    BuildKit    │  │
     │   └──────────────┘       └───────┬────────┘  │
     │                                  │           │
     │                                  ▼           │
     │                           ┌───────────────┐   │
     │                           │ Docker Images │───┼──▶ ECR
     │                           └───────────────┘   │
     │                                               │
     │ ┌────────┐ ┌────────┐ ┌────────┐             │
     │ │ App A  │ │ App B  │ │Postgres│             │
     │ └───┬────┘ └───┬────┘ └────────┘             │
     │     └──────┬────┘                             │
     │            ▼                                  │
     │         Traefik                               │
     └────────────┬──────────────────────────────────┘
                  │
                  ▼
               Internet
```

---

# Control Plane vs Node Agent

Coolify-style SSH orchestration ကို core mechanism မလုပ်ချင်ဘူး။

Server တစ်ခုစီပေါ်မှာ **tiny Go agent** တင်မယ်။

```text
agent ──outbound──> control.example.com
```

Agent က control plane ဆီ outbound connection ချိတ်မယ်။

ဒီလိုလုပ်ရင် control-plane မှာ root SSH private keys တွေစုထားစရာမလိုတော့ဘူး။

Recommended communication:

- gRPC
- WebSocket
- mTLS
- Agent heartbeat
- signed deployment instructions

---

# AWS Runtime Strategy

## v1 — EC2 + Docker

AWS-first ဆိုပေမယ့် v1 မှာ ECS/EKS မစသင့်သေးဘူး။

```text
AWS
 └── EC2
      └── Docker
```

အကြောင်းရင်း:

- deployment logic ထိန်းရလွယ်
- local dev လုပ်ရလွယ်
- debug လုပ်ရလွယ်
- cost predict လုပ်ရလွယ်
- Railway-like workflow ဆောက်ဖို့အမြန်ဆုံး

Runtime abstraction ကို interface နဲ့သန့်သန့်ထား။

```go
type RuntimeProvider interface {
    Deploy(...)
    Stop(...)
    Restart(...)
    Logs(...)
    Metrics(...)
    Scale(...)
}
```

နောက်ပိုင်း:

```text
DockerProvider
ECSProvider
KubernetesProvider
```

ထပ်ထည့်နိုင်မယ်။

### Future

```text
AWS
 ├── EC2 + Docker
 ├── ECS EC2
 └── ECS Fargate
```

---

# Railway UX Mental Model

Railway ရဲ့ design/colors ကိုကူးတာမဟုတ်ဘဲ mental model ကိုယူမယ်။

```text
Project
  ↓
Environment
  ↓
Services
  ↓
Deployments
```

Service detail:

```text
Source
Variables
Deployments
Metrics
Settings
Networking
Volumes
```

Main project screen:

```text
┌───────────────────────────────────────────────┐
│ my-project                    production ▼    │
├───────────────────────────────────────────────┤
│                                               │
│       ┌───────────────┐                       │
│       │   backend     │                       │
│       │ ● Running     │                       │
│       └───────┬───────┘                       │
│               │                               │
│       ┌───────▼───────┐                       │
│       │   postgres    │                       │
│       │ ● Running     │                       │
│       └───────────────┘                       │
│                                               │
│                             ＋ New Service    │
└───────────────────────────────────────────────┘
```

Service side panel:

```text
backend

Deployments
Variables
Metrics
Settings
```

### Important

Railway ရဲ့ actual:

- logo
- branding
- exact CSS
- exact icons
- copywriting

တွေကိုမကူးသင့်ဘူး။

“Inspired by Railway” ဖြစ်အောင်လုပ်၊ pixel-perfect clone မလုပ်နဲ့။

---

# Build System — Railpack + BuildKit

GitHub repo တစ်ခုလာတာနဲ့:

```text
Node?
Python?
Laravel?
Go?
Ruby?
PHP?

What build command?
What start command?
```

ဒါတွေကိုကိုယ်တိုင် detection logic ပြန်မရေးဘဲ **Railpack** ကိုသုံး။

Flow:

```text
GitHub Push
     ↓
Webhook
     ↓
Clone repo
     ↓
Dockerfile exists?
 ┌───┴────┐
Yes       No
 │         │
Docker    Railpack
file       │
 └────┬────┘
      ↓
   BuildKit
      ↓
 OCI Image
      ↓
     ECR
      ↓
 EC2 Worker
      ↓
 Container
```

ဒါက Railway-like zero-config developer experience ရဖို့ key ဖြစ်မယ်။

---

# Zero-Downtime Deployment

ဒီလိုမလုပ်:

```text
docker stop old
docker run new
```

အစား blue/green-style switch လုပ်မယ်။

Current:

```text
backend-v18
    │
    ▼
Traefik
```

New deploy:

```text
backend-v18 ← serving traffic

backend-v19
     ↓
starting
     ↓
health check
     ↓
healthy
```

healthy ဖြစ်မှ:

```text
Traefik
   ↓
backend-v19
```

ပြီးမှ:

```text
backend-v18 → stop
```

Health check fail ရင်:

```text
backend-v19 → destroy

traffic stays
      ↓
backend-v18
```

ဒီနည်းနဲ့:

- zero-downtime deploy
- fast rollback
- safe health-check based switching

ရမယ်။

---

# Database Architecture

Control plane အတွက် v1 မှာ PostgreSQL တစ်ခုနဲ့စ။

```text
Go API
PostgreSQL
Go Worker
```

Redis မလိုသေးဘူး။

Jobs table:

```text
pending
running
success
failed
```

Postgres-backed queue သုံးနိုင်တယ်။

Scale တက်လာမှ:

```text
NATS
Redis
Kafka
```

လိုတာထည့်။

---

# AWS Deployment Modes

## Mode 1 — Cheap / Personal

ကိုယ်တိုင်သုံးဖို့အတွက်:

```text
EC2
├── Control Plane
├── PostgreSQL
├── Agent
├── BuildKit
├── Traefik
├── Apps
└── Databases

EBS
└── persistent data

S3
└── backups
```

Machine တစ်လုံးထဲနဲ့စနိုင်တယ်။

ဒါက Railway bill လျှော့ချင်တဲ့ use case အတွက် အကောင်းဆုံး starting mode ဖြစ်တယ်။

## Mode 2 — Production

နောက်ပိုင်း:

```text
Control Plane EC2

RDS PostgreSQL

Builder EC2
 └── BuildKit

Worker EC2 #1
Worker EC2 #2
Worker EC2 #3

ECR

S3

Route53
```

လိုခွဲမယ်။

---

# Spot Instance Strategy

Spot ကို worker/build workload တွေအတွက်သုံးနိုင်တယ်။

Recommended:

```text
Control Plane → On Demand
Database      → On Demand

Builders      → Spot ✓
Stateless app → Spot ✓
Workers       → Spot ✓

Stateful DB   → Spot ✗
```

Spot interruption handling ပါ architecture ထဲကနေ support လုပ်ထားရမယ်။

---

# Project Structure

```text
cloudrail/
│
├── apps/
│   └── web/
│       ├── src/
│       └── package.json
│
├── cmd/
│   ├── server/
│   │   └── main.go
│   │
│   └── agent/
│       └── main.go
│
├── internal/
│   ├── api/
│   ├── auth/
│   ├── project/
│   ├── service/
│   ├── deployment/
│   ├── build/
│   ├── runtime/
│   │   ├── docker/
│   │   └── ecs/
│   ├── github/
│   ├── aws/
│   ├── proxy/
│   ├── logs/
│   ├── metrics/
│   └── secrets/
│
├── migrations/
│
├── deploy/
│   ├── docker-compose.yml
│   └── install.sh
│
├── agent/
│   └── install.sh
│
└── README.md
```

---

# MVP Scope

အစမှာ Coolify feature-for-feature မပြိုင်နဲ့။

## Milestone 1

1. Login
2. GitHub App connect
3. Create Project
4. GitHub repo select
5. Create Service
6. Environment variables
7. Deploy
8. Live build logs
9. Running / Failed status
10. Generated domain
11. Custom domain + HTTPS
12. Redeploy
13. Rollback
14. Restart / Stop

ဒီအဆင့်ရရင် Railway အစား စမ်းသုံးနိုင်တဲ့ level ရပြီ။

---

# Milestone 2

One-click managed services:

```text
PostgreSQL
Redis
MySQL
MongoDB
```

Additional:

```text
Persistent volumes
Backups → S3
CPU/RAM metrics
Cron jobs
```

---

# Milestone 3

```text
Multiple EC2 nodes
Scheduler
Resource limits
Placement
Horizontal replicas
Private networking
```

---

# Milestone 4

```text
AWS EC2 Auto Scaling
Spot
ECS provider
Teams
RBAC
Preview environments
```

---

# Agent Protocol

Agent က platform core ဖြစ်မယ်။

Control Plane → Agent deploy request:

```json
{
  "deployment_id": "...",
  "image": "...",
  "cpu": 1,
  "memory": 512,
  "env": {},
  "domain": "api.example.com"
}
```

Agent flow:

```text
pull image
    ↓
create container
    ↓
start
    ↓
healthcheck
    ↓
update proxy
    ↓
stream logs
    ↓
report status
```

Agent reports:

```text
CPU
RAM
Disk
Docker status
Containers
Build progress
Logs
```

UI မှာ:

```text
Deploying...
Building...
Creating container...
Healthcheck...
Active ✓
```

လို live progress ပြနိုင်မယ်။

---

# Open Source Strategy

Architecture:

```text
Coolify
↓
architecture + feature reference

Railway
↓
UX/product reference

Railpack
↓
actual dependency

Your Project
↓
new Go implementation
```

Coolify code ကို တိုက်ရိုက်ယူသုံးမယ်ဆိုရင် သူ့ license terms/attribution ကိုလိုက်နာရမယ်။

Long-term အတွက် clean implementation အသစ်ရေးတာပိုကောင်းတယ်။

---

# Recommended Build Order

AWS integration ကိုအစကတည်းက မလုပ်နဲ့။

## Phase 1 — Local / Single VM

```text
React
   ↓
Go API
   ↓
Postgres

Go Agent
   ↓
Docker
   ↓
Railpack + BuildKit
   ↓
Traefik
```

Target:

> GitHub repo ရွေး → Deploy နှိပ် → Domain တစ်ခုနဲ့ app တက်လာ

ဒီ core workflow အောင်အရင်လုပ်။

## Phase 2 — AWS

ပြီးမှ:

```text
AWS EC2 Provisioner
ECR
S3 Backup
Route53
Spot
```

ထည့်။

---

# Final Recommended Stack

> **Go control-plane + Go agent + React/TypeScript UI + PostgreSQL + Railpack/BuildKit + Docker + Traefik, AWS EC2 first.**

ဒီ stack က Railway-like UX ရှိတဲ့ self-hosted PaaS ကို AWS ပေါ်မှာ cost-efficient ဖြစ်အောင် ဆောက်ဖို့ သင့်တော်တဲ့ starting architecture ဖြစ်တယ်။

---

# Suggested Next Implementation Blueprint

နောက်အဆင့်မှာ အောက်ကအရာတွေကိုသတ်မှတ်ရမယ်:

- Database schema
  - `projects`
  - `environments`
  - `services`
  - `deployments`
  - `variables`
  - `domains`
  - `nodes`
  - `jobs`
  - `volumes`
- REST / RPC API routes
- Go agent protocol
- Deployment state machine
- Build pipeline
- Railway-style UI screen structure
- Authentication model
- Secrets encryption
- GitHub App integration
- AWS provisioning layer
- Initial Docker Compose dev environment

ဒီ blueprint ပြီးသွားရင် Claude Code / OpenCode ကို implementation prompts ပေးပြီး တကယ်စရေးနိုင်တဲ့အဆင့်ရောက်မယ်။
