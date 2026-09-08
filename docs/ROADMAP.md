# Cloudrail — phase-by-phase master plan

Updated: 2026-09-08. ဒီဖိုင်ကို project scope၊ phase အစဉ်နဲ့ progress အတွက် အဓိကစာတမ်းအဖြစ် သုံးမယ်။ အရင် `M1/M2/...` notes တွေနဲ့ ကွဲရင် ဒီ roadmap အတိုင်းလိုက်မယ်။

## 1. ဘာဆောက်နေတာလဲ

**ကိုယ်ပိုင် Linux VPS ပေါ်မှာ application တွေကို Railway လို လွယ်လွယ် deploy/manage လုပ်နိုင်တဲ့ open-source platform။**

ပထမ usable release ကို ကိုယ်တိုင်နဲ့ ယုံကြည်ရတဲ့ team သုံးဖို့၊ server တစ်လုံးနဲ့ စမယ်။ AWS ကို ပထမဆုံး တကယ်စမ်းသုံးမယ့် hosting provider အဖြစ်ထားမယ်။ Core deployment code ကိုတော့ Linux VPS provider တစ်ခုတည်းနဲ့ မချည်ထားဘူး။

လက်ရှိရွေးထားတဲ့ stack: **React/TypeScript UI + Go API/agent + PostgreSQL + Docker + Traefik**။ Source code build အတွက် BuildKit၊ Railpack ပါဝင်ပြီးဖြစ်သည်။

Public customer signup၊ billing၊ arbitrary customer code၊ autoscaling၊ Kubernetes၊ multi-region နဲ့ multi-node scheduling တွေကို ပထမ release scope ထဲ မထည့်သေးဘူး။

## 2. အခု ဘယ်ရောက်နေပြီလဲ

**[Alpha `0.1.0-alpha.4`](https://github.com/MinnKhantThuu/cloudrail/releases/tag/v0.1.0-alpha.4) ကို public release တင်ပြီး၊ CI/runtime/update/rollback၊ independent-host recovery နဲ့ package provenance စစ်အောင်ထားသည်။**

| အပိုင်း | လက်ရှိအခြေအနေ |
| --- | --- |
| Phase 0–3 — flow, workspace, node/jobs | Code + local verification ပြီး |
| Phase 4 — GitHub source builds | Public Dockerfile/Railpack live builds ပြီး; real GitHub App delivery pending |
| Phase 5 — operating features | Code + local DB/volume/HTTPS tests ပြီး; clean VPS/ACME pending |
| Phase 6 — AWS pilot | Template + read-only account/catalog planner ပြီး; AWS profile/domain မရှိသေး |
| Phase 7 — release preparation | Source + Linux amd64/arm64 archives, CI definitions, docs ပြီး; final local checks အောင် |
| UX | Desktop/mobile automated checks + visual inspection ပြီး; owner final review pending |
| Git / hosting | **[MinnKhantThuu/cloudrail](https://github.com/MinnKhantThuu/cloudrail) public source တင်ပြီး**; alpha.4 release တင်ပြီး; AWS deployment မရှိသေး |

Code ရေးပြီးတာကို public VPS/AWS verification ပြီးတယ်လို့ မရေတွက်ပါ။ Phase 6 account-bound အဆင့်ရောက်ပြီးနောက် အဲဒီ access မလိုတဲ့ Phase 7 packaging ကို ဆက်လုပ်ထားသည်။ Phase အရေအတွက်နဲ့ completion percentage မတွက်ပါ။

## 3. Project flow ကို နားလည်ဖို့

### အဓိကအရာတွေ

| အမည် | ဘာကိုဆိုလိုတာလဲ | ဥပမာ |
| --- | --- | --- |
| Workspace | ကိုယ်စီမံမယ့် project တွေစုထားတဲ့နေရာ | Personal workspace |
| Server / Node | Application containers တွေ တကယ် run မယ့် Linux machine | AWS ပေါ်က VPS တစ်လုံး |
| Project | ဆက်စပ်တဲ့ services တွေစုထားတဲ့နေရာ | My Shop |
| Environment | Project တစ်ခုရဲ့ သီးခြား configuration/deployment နေရာ | staging / production |
| Service | Deploy လုပ်မယ့် application တစ်ခု | web / api |
| Deployment | Service တစ်ခုကို version တစ်ခုအဖြစ်တင်တဲ့မှတ်တမ်း | api version အသစ်တင်ခြင်း |
| Database service | Persistent data နဲ့ backup လိုတဲ့ service အမျိုးအစား | PostgreSQL — Phase 5 တွင်ပါဝင်ပြီး |

အခု code မှာ single owner account၊ secure session၊ environment create/switch ပါလာပြီ။ Platform PostgreSQL က platform records သိမ်းဖို့ဖြစ်သည်။ Application PostgreSQL service များကို သီးခြား private network/volume ဖြင့်ဖန်တီးနိုင်သည်။

### နောက်ဆုံးရချင်တဲ့ user flow

**ပထမဆုံးတစ်ကြိမ်:**

`Linux VPS ပြင်ဆင် → Cloudrail install → owner account setup → server health စစ် → GitHub ချိတ်`

**Application အသစ်တင်ချိန်:**

`New project → environment ရွေး → New service → GitHub repo/branch ရွေး → variables ဖြည့် → Deploy → build/logs ကြည့် → HTTPS URL ဖွင့်`

**နေ့စဉ် update လုပ်ချိန်:**

`Code push → build → candidate container → readiness စစ် → traffic ပြောင်း → deployment history သိမ်း`

Build/readiness မအောင်ရင် current release ကိုဆက်သုံးပြီး failure reason ပြမယ်။ Version ဟောင်းပြန်တင်ရင် deployment အသစ်တစ်ခုအဖြစ် မှတ်တမ်းတင်မယ်။ Database schema/data ကိုတော့ image rollback နဲ့ အလိုအလျောက်ပြန်မပြောင်းဘူး။

### အခု တကယ်သုံးလို့ရတဲ့ local flow

`Docker stack → owner setup/login → project/environment → HTTP service → GitHub source or pinned image → variables → build/deploy → localhost URL`

`PostgreSQL service → app binding → redeploy app → backup → empty target restore`

Public HTTPS installer/config ရှိပြီး local TLS proof ရှိသည်။ Real domain ACME issuance နဲ့ AWS flow ကို target account/domain ရမှ ဆက်စမ်းနိုင်မည်။

### System ထဲမှာ တာဝန်ခွဲထားပုံ

1. **React dashboard** — user လုပ်ချင်တဲ့ action ကိုလက်ခံပြီး status/logs ပြမယ်။
2. **Go API + PostgreSQL** — project/service records၊ configuration၊ deployment jobs/history သိမ်းမယ်။
3. **Go agent** — Linux node ပေါ် Docker container တွေစ/ရပ်၊ readiness စစ်၊ route update လုပ်မယ်။
4. **Traefik** — app URL ကလာတဲ့ traffic ကို active container ဆီပို့မယ်။ User traffic က Go API ကို ဖြတ်သန်းစရာမလိုဘူး။
5. **BuildKit / Railpack** — Source code ကနေ image ထုတ်ပြီး local registry ကိုတင်မယ်။

## 4. Phase အစဉ်နဲ့ လက်ရှိ status

| Phase | ရည်ရွယ်ချက် | အဆုံးမှာ user မြင်ရမယ့်ရလဒ် | Status |
| --- | --- | --- | --- |
| **0 — Scope & flow** | ဘာဆောက်မယ်၊ ဘယ်လိုသုံးမယ်၊ ဘယ်အစဉ်လိုက်သွားမယ် ရှင်းလင်းခြင်း | Roadmap၊ screen map၊ current/target flow | **အစဉ်အတိုင်း ဆက်လုပ်ရန် အတည်ပြုပြီး** |
| **1 — Local deployment prototype** | Image deployment နဲ့ failed replacement ကိုသက်သေပြခြင်း | Local image တင်/ဖွင့်၊ logs/history ကြည့်နိုင် | **Code + local verification ပြီး** |
| **2 — Workspace & core UX** | နေ့စဉ်သုံးမယ့် screens နဲ့ settings ကိုအခြေခံချခြင်း | Login၊ projects/environments/services၊ variables၊ restart/stop | **Code + local verification ပြီး; user UX review pending** |
| **3 — Reliable node & jobs** | Restart/disconnect/overlap ဖြစ်ချိန် state မှန်အောင်လုပ်ခြင်း | Node online/offline၊ recoverable jobs၊ ရှင်းလင်းတဲ့ failure state | **Code + local failure/recovery checks ပြီး** |
| **4 — GitHub deployment** | Repo ကနေ app တင်လို့ရအောင်လုပ်ခြင်း | Repo/branch ရွေး၊ push-to-deploy၊ build logs | **Local code + public GitHub build proof ပြီး; live App install/webhook pending** |
| **5 — VPS operating features** | Public URL နဲ့ persistent app တွေကိုထိန်းနိုင်အောင်လုပ်ခြင်း | Installer၊ domain/HTTPS၊ metrics၊ volumes၊ DB backup/restore | **Code + local verification ပြီး; clean public VPS/ACME pending** |
| **6 — AWS pilot** | ကိုယ်ပိုင် application တစ်ခုနဲ့တကယ်သုံးစမ်းခြင်း | AWS ပေါ် end-to-end deployed app နဲ့ measured cost | **Preparation ပြီး; real pilot pending account/domain** |
| **7 — Open-source beta** | တခြားသူ install/update လုပ်လို့ရတဲ့ release ပြင်ခြင်း | Versioned release၊ docs၊ clean install/upgrade proof | **Public source/docs တင်ပြီး; hosted CI evidence ရရှိ; public VPS/beta gates pending** |

**လက်ရှိ Phase 7 local alpha packaging/verification ပြီးထားသည်။ Phase 6 real AWS pilot ကို credentials/domain မရှိသေးသဖြင့် pending ထားသည်။**

## 5. Phase တစ်ခုချင်းစီရဲ့ အလုပ်နဲ့ ပြီးဆုံးစံ

### Phase 0 — Scope & project flow

- [x] Product ရည်ရွယ်ချက်၊ ပထမ release scope နဲ့ မပါသေးမယ့်အပိုင်းတွေ ရေးခြင်း။
- [x] Current flow နဲ့ target flow ခွဲပြခြင်း။
- [x] ရှိပြီးသား code ကို phase/status နဲ့ ပြန်တွဲပြခြင်း။
- [x] Phase အစဉ်၊ deliverables နဲ့ exit criteria ရေးခြင်း။
- [x] User က 2026-09-08 တွင် phase အစဉ်အတိုင်း ပြီးသည်အထိဆက်လုပ်ရန် အတည်ပြုသည်။ UX အပြီးသတ်အတည်ပြုချက်အဖြစ် မယူဆရ။

**Exit:** ဘယ် phase က ဘာပေးမယ်၊ အခုဘယ်ရောက်နေတယ်၊ နောက်လုပ်မယ့်အပိုင်းကဘာလဲကို နှစ်ဖက်လုံးရှင်းလင်းနေခြင်း။ Plan တင်ပြထားတာကို user review ပြီးပြီလို့ မမှတ်တမ်းတင်ရ။

### Phase 1 — Local deployment prototype — implementation ပြီး

- [x] Go API/agent၊ PostgreSQL schema၊ local Compose stack။
- [x] Project/service create၊ public image digest validation။
- [x] Persisted jobs/events၊ image pull/container start။
- [x] Readiness check၊ Traefik traffic switch၊ failed candidate cleanup။
- [x] Basic dashboard၊ deployment history၊ bounded runtime logs၊ image redeploy။
- [x] Go/race tests၊ frontend build၊ real Docker acceptance၊ desktop/mobile browser journey။

**Exit:** App A serving ဖြစ်နေချိန် replacement B readiness/pull ပျက်ရင် A ဆက်ဖွင့်ရ။ Healthy B အောင်ရင် B ဆီ traffic ပြောင်းရ။ Agent/API restart အတွက် သတ်မှတ်ထားတဲ့ local cases အောင်ရ။

Evidence: [M1 verification](verification.md) နဲ့ [အသေးစိတ် technical scope](milestone-1.md)။ Phase 1 baseline ကို ယခင် run တွင်စစ်ပြီး ယခု implementation တွင် current owner/mTLS/jobs flow နှင့် real Docker acceptance ပြန်အောင်ထားသည်။ Final UX approval၊ public VPS readiness နဲ့ AWS proof အဖြစ် မရေတွက်ရ။

### Phase 2 — Workspace & core UX

**2.1 Screen/interaction design**

Screen map ကို အရင်သတ်မှတ်မယ်: owner setup/login → project list → project overview → service detail → deployment detail; သီးခြား server/settings pages။ Empty/loading/error/success states နဲ့ desktop/mobile behavior ပါသတ်မှတ်မယ်။ ရှိပြီးသား UI ကို starting prototype အဖြစ်ယူမယ်။

**2.2 Accounts & environments**

Admin token login ကို owner account + secure session flow နဲ့ပြောင်းမယ်။ `staging`/`production` ကို create/switch လုပ်နိုင်အောင် environment model နဲ့ service configuration ခွဲမယ်။ Single owner scope ကိုဆက်ထိန်းမယ်။

**2.3 Service configuration & actions**

Encrypted runtime variables၊ port/readiness settings၊ restart/stop၊ historical image redeploy တို့ကို API နဲ့ UI အပြည့်ချိတ်မယ်။ Variable value ကို logs/API response တွေမှာ မတော်တဆမပေါ်အောင်စစ်မယ်။

**Exit:** User တစ်ယောက်က owner setup ကစပြီး project/environment/service ဆောက်၊ variables နဲ့ image deploy၊ restart/stop/redeploy လုပ်နိုင်ရ။ Staging ပြင်တာ production ကိုမထိရ။ Screen flow ကို user review လုပ်ပြီး usability ပြင်ဆင်ချက်တွေပြီးရ။

### Phase 3 — Reliable node & deployment jobs

**3.1 Node identity:** enrollment၊ certificate authentication/mTLS၊ heartbeat၊ online/offline status နဲ့ revoke flow။ Single execution node scope ဖြစ်နေဆဲ။

**3.2 Job correctness:** bounded retries/backoff၊ timeout၊ cancellation semantics၊ duplicate request/command protection၊ per-service serialization၊ stale command မactivate နိုင်အောင် generation checks။

**3.3 Recovery:** Node reconnect/restart မှာ stored state နဲ့ container/proxy state ပြန်ညှိခြင်း။ Control-plane backup/restore စမ်းခြင်း။ Platform update/migration recovery အခြေခံ။

**Exit:** Network disconnect၊ duplicated delivery၊ API/agent restart နဲ့ overlapping deployment tests မှာ obsolete release မactive ရ၊ job ကအကန့်အသတ်မဲ့မစောင့်ရ၊ ရှင်းလင်းတဲ့ recoverable/failed state ရရ။ Control-plane backup ကို သီးခြား clean environment မှာ restore လုပ်ရ။

### Phase 4 — GitHub → build → deploy

**4.1 GitHub connection:** GitHub App install၊ repo/branch selection၊ webhook signature validation နဲ့ delivery deduplication။ Callback testing အတွက် လိုအပ်တဲ့ temporary test endpoint ရှိရင် အဲဒီ scope ကိုရှင်းပြမယ်။

**4.2 Dockerfile builds:** Exact commit SHA ကို checkout၊ bounded BuildKit build၊ image registry/digest၊ live build logs၊ existing deployment pipeline ဆက်ခြင်း။ Build-time secrets ကို image layers ထဲမထည့်ရ။

**4.3 Automatic builds:** Supported sample applications အတွက် Railpack detection၊ build/start overrides နဲ့ monorepo root setting။ Supported stacks ကိုစမ်းထားတဲ့ matrix နဲ့ဖော်ပြမယ်။

**Exit:** GitHub repo ရွေးပြီး first deploy ရရ၊ configured branch push က deploy တစ်ခုပဲဖန်တီးရ၊ build fail ရင် current app ဆက်အလုပ်လုပ်ရ။ Dockerfile app နဲ့ supported Dockerfile မပါတဲ့ sample တစ်ခုစီ live proof ရရ။

### Phase 5 — VPS ပေါ် နေ့စဉ်သုံးနိုင်ခြင်း

**5.1 Installation & networking:** Supported clean Linux VPS ပေါ် installer၊ owner bootstrap၊ port/DNS preflight၊ custom domain၊ automatic HTTPS နဲ့ certificate renewal/error flow။

**5.2 Operational controls:** CPU/RAM/disk metrics၊ resource limits UI၊ image/log retention၊ disk-pressure behavior၊ ongoing service health နဲ့ backup status။

**5.3 Persistent workloads:** Volume attach/persistence၊ PostgreSQL service template တစ်မျိုး၊ private connection variables၊ app/database backups နဲ့ tested restore flow။ Database ကို HTTP app ရဲ့ replacement strategy အတိုင်း မပြောင်းတင်ရ။

**Exit:** Clean VPS မှာ docs အတိုင်းတင်လို့ရ၊ real domain HTTPS အလုပ်လုပ်ရ၊ app redeploy/reboot ပြီး data မပျောက်ရ၊ database backup ကိုအသစ်တစ်နေရာမှာ restore လုပ်ပြီး data စစ်လို့ရ။ Destructive data actions ကိုသီးခြားရှင်းပြရ။

### Phase 6 — AWS pilot

**6.1 Deployment preparation:** Measured workload အပေါ် Lightsail/EC2 ရွေး၊ target region၊ disk၊ registry၊ backups နဲ့ monthly cost estimate သတ်မှတ်။ Account/resource access နဲ့ billable scope လိုတဲ့အချက်တွေကို ဒီအဆင့်ရောက်မှ ဖြည့်မယ်။

**6.2 Real deployment:** AWS node install၊ domain/TLS၊ repo connection၊ user ရွေးတဲ့ ကိုယ်ပိုင် application တစ်ခုတင်။ Provider-specific IAM/registry/backup integration လိုတာကိုသာထည့်မယ်။

**6.3 Pilot validation:** Push update၊ failed deployment၊ recovery၊ reboot၊ backup/restore နဲ့ အချိန်ကာလသတ်မှတ်ထားတဲ့ resource/cost observation။ Existing production traffic ပြောင်းမယ့် scope ကိုအဲဒီအချိန်မှာ သီးခြားသတ်မှတ်မယ်။

**Exit:** AWS ပေါ် app ကိုတကယ်ဖွင့်ပြီးအထက်ပါ operations စမ်းထားရ။ Estimated cost နဲ့ actual usage evidence ခွဲတင်ပြရ။ Local tests ကို AWS proof လို့မသတ်မှတ်ရ။

### Phase 7 — Open-source beta release

**7.1 Packaging:** Versioned release images/binaries၊ reproducible build/CI၊ upgrade/rollback path။

**7.2 Documentation:** Quickstart၊ architecture၊ supported stacks/limits၊ troubleshooting၊ contributing/security reporting နဲ့ license/dependency notices။

**7.3 Independent install:** Development machine state ကိုမမှီခိုတဲ့ clean server တစ်လုံးမှာ install → deploy → update → recover စမ်းခြင်း။ Final UX walkthrough၊ known limitations၊ beta release notes။

**Exit:** တခြားသူက docs အတိုင်း fresh install/update လုပ်နိုင်တဲ့ evidence ရရ။ User သတ်မှတ်တဲ့ public repository/release destination နဲ့ publishing scope ရှိမှ အပြင်ကိုတင်မယ်။

## 6. အခုကစပြီး တစ်ဆင့်ချင်းသွားမယ့်စည်းမျဉ်း

1. **Phase မစခင်** phase/substep၊ လုပ်မယ့်အရာ၊ user မြင်ရမယ့်ရလဒ်နဲ့ exit criteria ကိုအရင်ပြမယ်။
2. **တစ်ကြိမ်မှာ သတ်မှတ်ထားတဲ့ phase တစ်ခုပဲလုပ်မယ်။** Dependency fix လိုရင် ဘာကြောင့်ပါလာလဲ ရှင်းပြမယ်။ Later-phase feature ကိုမသိမသာမထည့်ရ။
3. **Phase အဆုံးမှာ** code ပြီးတာ၊ test/live evidence၊ user UX review၊ မပြီးသေးတာနဲ့ နောက်အဆင့်ကိုခွဲတင်ပြမယ်။
4. Test pass တာနဲ့ phase နောက်တစ်ခုကို အလိုအလျောက်ကူးမသွားရ။ User ကဆက်လုပ်ဖို့ပေးထားတဲ့ scope အတိုင်းဆက်မယ်; authorization ရှိပြီးသားအရာအတွက် ခွင့်ပြုချက်ထပ်မတောင်းရ။
5. Phase scope ပြောင်းရင် ဒီဖိုင်ကိုအရင် update လုပ်ပြီး အစဉ်ပြောင်းသွားတာကို user ဆီရှင်းပြမယ်။

Progress report ပုံစံ:

```text
Current phase:
ဒီ phase ရဲ့ရည်ရွယ်ချက်:
ပြီးသွားတာ:
စစ်ပြီးတဲ့ evidence:
မပြီးသေးတာ / review လိုတာ:
နောက်လုပ်မယ့် substep:
```

## 7. လက်ရှိ checkpoint

- **Current:** Phase 7.3 alpha delivery is verified; continuation is **blocked on target account/domain/pilot inputs and owner UX acceptance**, not marked complete. Alpha.4 source and four verified release assets are public at `302aec491fc33fd8a0ef5216b33856d899e5cb5f`; both [main CI](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34213340630) and [tag CI](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34213842668) passed all five jobs. Independent-host recovery and fixture-TLS dashboard routing are proven; real public-host installation/ACME and AWS/GitHub pilot gates below remain unproven.
- **Phase 0:** User approved sequential continuation through all phases.
- **Phase 2 evidence:** Go race tests + vet, frontend build, real Docker acceptance, Chrome desktop/mobile journey passed (31.9s). See [Phase 2 flow](phase-2-flow.md).
- **UX:** Automated browser verification and agent visual inspection; user final UX review pending.
- **Local owner handoff:** Login correction passed fresh registration (3.7s) and the full dashboard/deploy/browser journey (32.4s). The assistant-created acceptance owner was backed up and removed, retaining projects/apps, so the user can create their own account at localhost:8080. Do not run acceptance helpers that create an owner on this handed-over installation; use disposable CI installations for further registration/maintenance tests.
- **Remote:** Public source committed/pushed to [MinnKhantThuu/cloudrail](https://github.com/MinnKhantThuu/cloudrail), main branch. Private vulnerability reporting enabled. No AWS resources, public application domain or installed GitHub App; [v0.1.0-alpha.4](https://github.com/MinnKhantThuu/cloudrail/releases/tag/v0.1.0-alpha.4) is published with all four verified CI assets. Alpha.3 notes link to its public-dashboard recovery repair.
- **Phase 3 evidence:** mTLS heartbeat/revoke, dedup, cancellation, agent/container restart recovery, isolated PostgreSQL stale-attempt/retry tests and independent backup restore passed. See [Phase 3 flow](phase-3-flow.md).
- **Phase 4 evidence:** Public GitHub Dockerfile + Railpack builds each served real HTTP via registry digests; failed build preserved traffic. Browser Source/GitHub desktop/mobile passed. Signed replay/stale/cancellation/retry DB tests passed. A transient local disk-full failure was resolved with Cloudrail-only build-cache cleanup and the DB test rerun passed.
- **Account gap:** GitHub App installation/delivered webhook and AWS/domain are pending user account details.
- **Phase 5 evidence:** PostgreSQL and HTTP volume persistence/backup/restore passed, including nonempty restore rejection; actual Docker limits/metrics and domain route passed. Local HTTPS with a trusted test certificate passed. Database UI journey passed (13.1s). Ubuntu installer syntax/public Compose validation passed; clean host + ACME issuance pending.
- **Phase 6 preparation:** Lightsail CloudFormation template + read-only account/catalog planner and cost/runbook complete. CLI response shape checked; authenticated validation/provisioning pending.
- **Phase 7 preparation:** Versioned source and Linux amd64/arm64 archives, checksums/notices, CI definitions, current quickstart/API/architecture/security/contributing/update/recovery docs. Final Go race/vet + isolated DB tests + real Docker acceptance passed after dependency fixes. All 3 Chrome desktop/mobile journeys passed (42.8s). Archive checksums/notices and deterministic source/credential-exclusion checks passed; [Hosted CI run 34201850332](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34201850332) passed checks and runtime jobs on a fresh Ubuntu 24.04 runner. This verifies the local Compose quickstart on Linux amd64; public installer/ACME remain pending. Independent-host cold recovery subsequently passed in run 34211555644. The subsequent [alpha.2 CI run](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34205416387) passed all three jobs, including the old-public-runtime → alpha.2 upgrade/rollback rehearsal with continuous app HTTP traffic and encrypted configuration preservation.
- **Next external inputs:** AWS profile/region + DNS domain + application GitHub App/repository + selected pilot application. Existing credentials must stay outside chat. See [AWS runbook](aws-pilot.md), [release gates](release.md), [current verification](verification-current.md).

## 8. Completion-goal work queue — 2026-09-08

0. [x] Coolify-like first-owner registration/login flow; desktop/mobile and CI verified. User UX acceptance remains separate.
1. [x] Pending-job/concurrent-operator guards, verified rollback metadata and incompatible image-rollback rejection.
2. [x] Fresh Linux Compose install → deployment → update → rollback → continued operation and backup restore in hosted CI.
3. [x] Publish versioned alpha artifacts with checksums and verified build provenance.
4. [x] Cold host backup/recovery and a disposable independent-host rehearsal passed, including application state beyond the control database. Final alpha.4 CI/package verification also passed, including dashboard-route regeneration after restore.
5. [ ] Validate public installer/networking and complete the AWS/GitHub/domain pilot when account access is supplied.
6. [ ] Record final owner UX acceptance and real pilot evidence; never count missing external proof as complete.

This queue completes existing Phase 5–7 gates; it does not add multi-tenancy, Kubernetes or unrelated features.

External gate check after alpha.3 publication: AWS CLI is installed but `aws configure list-profiles` returned no configured profiles. No target domain/GitHub App/pilot inputs or owner UX acceptance have been supplied. No AWS infrastructure or real public application was created by this release.

## 9. Completion audit and external block — 2026-09-08

The preceding alpha.3 and alpha.4 goal turns completed release/recovery work while recording the same missing account/domain/pilot inputs. The next continuation revalidated that condition: AWS CLI has zero profiles, STS returns no credentials, no AWS pilot plan exists, the local `github_app` table contains no configuration, and dashboard/app domains plus the public-installation marker are absent. These were read-only checks; owner credentials/data were not changed. Both alpha.4 workflow runs are now terminal and successful.

| Required outcome | Evidence / remaining gap | State |
| --- | --- | --- |
| Public source, README/flow/usage and versioned alpha | Public alpha.4 tag/assets, 167 source entries matching Git, checksums/provenance and anonymous download; current docs pushed | Verified |
| Independent install, update, rollback and recovery | Main/tag CI: clean Ubuntu Compose journey, cross-version maintenance, two-host data recovery and fixture-TLS dashboard routing | Verified within the recorded Linux/fixture scope |
| Clean public VPS installer, real DNS/HTTPS and reboot recovery | No target public host/domain; fixture certificates do not prove ACME or public secure-cookie login | Pending target access |
| Real GitHub App push-to-deploy | No configured App/pilot repository supplied; signed local tests/public source builds are narrower evidence | Pending owner App/repository |
| AWS pilot, real app flow, off-server recovery and 48-hour measured operation/cost | STS has no credentials; no account/region selection or deployed pilot | Pending AWS access and pilot choice |
| Final owner UX walkthrough | Automated/visual checks exist, but no owner acceptance was supplied | Pending owner review |

Resume with a configured AWS CLI profile and region, dashboard/wildcard app domain, the owner's GitHub App/pilot repository and selected app flow. First produce the read-only account/catalog plan, then proceed through the existing AWS runbook. This blocked state does not claim production/beta readiness or an external security audit.
