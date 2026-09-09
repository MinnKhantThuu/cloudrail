# Cloudrail အသုံးပြုလမ်းညွှန်

Cloudrail က ကိုယ်ပိုင် Linux server ပေါ်မှာ application တွေတင်ဖို့ open-source deployment platform ဖြစ်ပါတယ်။ Dashboard ကနေ project၊ environment၊ service တွေခွဲပြီး app တင်တာ၊ logs ကြည့်တာ၊ PostgreSQL/Redis data service နဲ့ persistent storage စီမံတာတွေ လုပ်နိုင်ပါတယ်။

**လက်ရှိက alpha ဖြစ်ပါတယ်။** Owner တစ်ယောက်၊ server တစ်လုံးနဲ့ ကိုယ်ယုံကြည်ရတဲ့ repositories တွေအတွက် ရည်ရွယ်ထားပါတယ်။ Public source တင်ထားတာနဲ့ production verification ပြီးတယ်လို့ မဆိုလိုပါဘူး။ ဘယ်အပိုင်းတွေစမ်းပြီး၊ ဘာတွေကျန်သေးလဲကို [Roadmap](ROADMAP.md) နဲ့ [Verification](verification-current.md) မှာကြည့်နိုင်ပါတယ်။

## ၁။ Local မှာ စတင်သုံးခြင်း

Docker Engine API 1.47+၊ Docker Compose၊ Git နဲ့ OpenSSL လိုပါတယ်။ Docker အတွက် RAM အနည်းဆုံး 4 GB၊ disk လွတ်နေရာ အနည်းဆုံး 8 GB ထားပါ။ Source build တွေလုပ်ဖို့ 20 GB လွတ်နေရာရှိရင် ပိုအဆင်ပြေပါတယ်။ Mac သုံးရင် Docker Desktop ဒါမှမဟုတ် Colima ကို အရင်ဖွင့်ပါ။

```sh
git clone --branch v0.1.0-alpha.4 --depth 1 https://github.com/MinnKhantThuu/cloudrail.git
cd cloudrail
bash scripts/dev-up.sh
```

ပထမဆုံး run ချိန်မှာ images တွေ build လုပ်ရလို့ အချိန်ယူနိုင်ပါတယ်။ ပြီးရင် [localhost:8080](http://localhost:8080) ကိုဖွင့်ပါ။

**Create your account** မှာ ကိုယ့် email၊ password နဲ့ Confirm password ဖြည့်ပြီး **Create account** နှိပ်ပါ။ Workspace ထဲ တန်းဝင်သွားမယ်။ Token ရှာဖြည့်စရာမလိုပါ။ နောက်တစ်ခါဝင်ရင် email/password နဲ့ **Sign in** ပဲလုပ်ရပါတယ်။ Password ကို အနည်းဆုံး 12 characters သုံးပါ (အများဆုံး 72 bytes)။ မျက်လုံးပုံ **Show** ကိုနှိပ်ပြီး password စစ်ကြည့်နိုင်ပါတယ်။

Public VPS မှာ install ပြီးတာနဲ့ account ကို ချက်ချင်းဖွင့်ပါ။ ပထမဆုံး account ဖွင့်သူက installation owner ဖြစ်ပြီး၊ အဲဒီနောက် account အသစ်ထပ်ဖွင့်လို့ မရတော့ပါ။ `.env` ထဲက server secrets နဲ့ encryption key တွေကို GitHub မတင်ဘဲ ကိုယ်ပိုင်လုံခြုံတဲ့နေရာမှာ backup ထားပါ။

## ၂။ Project flow ကို နားလည်ထားရန်

```text
Project ဖန်တီး
  → Environment ရွေး
  → Canvas ပေါ် Resource ထည့်
  → GitHub / Docker / Empty source ရွေး
  → Web / Worker / Cron workload ရွေး
  → Variables ဖြည့်
  → Build / deploy
  → Readiness စစ်
  → URL ဖွင့်ပြီး အသုံးပြု
```

| အမည် | ဆိုလိုတာ |
| --- | --- |
| Project | ဆက်စပ်တဲ့ applications တွေ စုထားတဲ့နေရာ၊ ဥပမာ Shop |
| Environment | Configuration နဲ့ deployment ခွဲထားတဲ့နေရာ၊ ဥပမာ staging / production |
| HTTP service | Website၊ API စတဲ့ application တစ်ခု |
| Worker service | Public URL မရှိတဲ့ background process |
| Cron service | UTC schedule နဲ့ command တစ်ကြိမ်စီ run မယ့် job |
| PostgreSQL service | Private network ထဲမှာသုံးတဲ့ database |
| Redis service | Cache၊ queue နဲ့ key-value data အတွက် private data service |
| Bucket | ရှိပြီးသား S3-compatible object storage ကို application နဲ့ချိတ်ရန် |
| Build | Source commit တစ်ခုကနေ image ထုတ်ခြင်း |
| Deployment | Image တစ်ခုကို configuration snapshot နဲ့ run ဖို့ကြိုးစားမှု |

Staging အသစ်ဖန်တီးတာက production data ကို အလိုအလျောက်ကူးပေးတာ မဟုတ်ပါဘူး။ အဲဒီ environment ထဲမှာ လိုတဲ့ services နဲ့ variables ကို သီးခြားထည့်ရပါတယ်။

## ၃။ ပထမ app ကို image နဲ့တင်ကြည့်ခြင်း

```sh
docker pull traefik/whoami:v1.11.0
docker image inspect traefik/whoami:v1.11.0 --format '{{index .RepoDigests 0}}'
```

1. Dashboard မှာ project တစ်ခုဆောက်ပြီး `production` ကိုရွေးပါ။
2. **New resource → Docker Image** ကိုရွေး၊ workload ကို **Web / API** ထားပြီး `hello` လို့နာမည်ပေးပါ။
3. **Deploy** ကိုနှိပ်ပြီး command ကရတဲ့ `repository@sha256:...` ကိုထည့်ပါ။
4. Port ကို **80**၊ readiness path ကို **/** ထားပါ။
5. **Active** ဖြစ်ရင် service URL ကိုဖွင့်ပါ။

ဒီ port က container အတွင်း app နားထောင်နေတဲ့ port ဖြစ်ရပါတယ်။ ကိုယ့် app ရေးတဲ့အခါ `0.0.0.0` မှာ listen လုပ်ထားဖို့လိုပါတယ်။ Local URL က `http://<service-id>.localhost:8088` ပုံစံဖြစ်ပါတယ်။

## ၄။ GitHub repo ကနေတင်ခြင်း

HTTP service ရဲ့ **Source** tab ကိုဖွင့်ပါ။ Public repo ဆို **Public repository** ကိုရွေးပြီး `owner/repository`၊ branch နဲ့ root directory ထည့်ပါ။ Root က repo တစ်ခုလုံးဆို `.` ဖြစ်ပါတယ်။

- Dockerfile ပါရင် **Dockerfile** builder ကိုရွေးပါ။
- Dockerfile မပါရင် **Railpack** ကိုသုံးနိုင်ပါတယ်။ Stack တိုင်းကို စမ်းပြီးသားတော့ မဟုတ်ပါဘူး။
- App port နဲ့ readiness path သတ်မှတ်ပြီး source ကို save လုပ်ပါ။
- **Build & deploy** ကိုနှိပ်ပြီး Build history၊ Deployments နဲ့ logs ကိုကြည့်ပါ။

Private repo နဲ့ push တိုင်း auto deploy လိုရင် ကိုယ်ပိုင် GitHub App ဖန်တီးပြီးချိတ်ရပါမယ်။ လိုတဲ့ permissions၊ webhook နဲ့ installation အဆင့်တွေကို [GitHub App guide](source-builds.md#github-app) မှာရေးထားပါတယ်။ Cloudrail source repo ကို GitHub တင်ထားတာနဲ့ ကိုယ့် apps တွေရဲ့ auto deploy ချိတ်ပြီးသား မဖြစ်ပါဘူး။

## ၅။ Worker၊ Cron နဲ့ deployment commands

Public URL မလိုတဲ့ long-running process ဆို **New resource → Background Worker** ကိုရွေးပါ။ Schedule နဲ့ command run မယ်ဆို **Cron Job** ကိုရွေးပြီး **Settings** ထဲမှာ five-field UTC cron expression သိမ်းပြီးမှ deploy လုပ်ပါ။ Cron run တစ်ခုချင်း logs နဲ့ exit status သိမ်းထားပြီး service တစ်ခုတည်းမှာ run နှစ်ခုမထပ်အောင်ထိန်းထားပါတယ်။

**Settings → Deploy commands & recovery** မှာ start command override၊ pre-deploy command/timeout နဲ့ restart policy သတ်မှတ်နိုင်ပါတယ်။ Pre-deploy က image pull ပြီး၊ candidate မစခင် သီးခြား container ထဲ run တယ်; service volume မတပ်ပါဘူး။ Fail/timeout ဖြစ်ရင် လက်ရှိ active release ကိုဆက်ထားပါတယ်။ Settings ပြောင်းပြီးရင် deploy အသစ်လုပ်မှအသက်ဝင်ပါမယ်။

## ၆။ Variables နဲ့ update လုပ်ခြင်း

Service ရဲ့ **Variables** tab မှာ `DATABASE_URL`၊ `PORT` စတာတွေထည့်နိုင်ပါတယ်။ သိမ်းပြီးတဲ့ secret value ကို UI ကပြန်မဖော်ပြပါဘူး။ ပြောင်းချင်ရင် value အသစ်ထည့်ရပါတယ်။

Variables နဲ့ resource settings ပြင်ပြီးရင် **deploy အသစ်လုပ်ပါ**။ Restart က လက်ရှိ deployment ရဲ့ configuration ကိုပဲပြန်သုံးပါတယ်။ Runtime variables တွေကို build အတွင်း မပို့ပါဘူး။ Private package build secrets ကို ဒီ alpha မှာ မပံ့ပိုးသေးပါဘူး။

## ၇။ Database ချိတ်ခြင်း

1. Application နဲ့ project/environment တူတဲ့နေရာမှာ **New resource → PostgreSQL** ဖန်တီးပါ။
2. Active ဖြစ်ရင် database ရဲ့ **Settings** ကိုဖွင့်ပါ။
3. **Connect an application** မှာ app ကိုရွေးပြီး `DATABASE_URL` variable အဖြစ် save လုပ်ပါ။
4. အဲဒီ app ကို redeploy လုပ်ပါ။

Database မှာ public port မဖွင့်ထားပါဘူး။ Password ကိုလည်း dashboard မှာ မဖော်ပြပါဘူး။ Environment မတူတဲ့ app ကို တိုက်ရိုက်ချိတ်တာကို ပိတ်ထားပါတယ်။

## ၈။ Backup နဲ့ restore

PostgreSQL ရဲ့ **Settings → Create backup** ကနေ dump ထုတ်ပြီး download လုပ်နိုင်ပါတယ်။ Server ပြင်ပက လုံခြုံတဲ့နေရာမှာ copy သိမ်းပါ။ Restore လုပ်ဖို့ database အသစ်တစ်ခုဖန်တီးပြီး backup ကိုရွေးပါ။ Data ရှိပြီးသား database ကို overwrite လုပ်တာကို ပိတ်ထားပါတယ်။

### Redis ထည့်ပြီး application နဲ့ချိတ်ရန်

1. Application ရှိတဲ့ project/environment ထဲမှာ **New resource → Redis** ကိုရွေးပါ။
2. နာမည်ပေးပြီး create လုပ်ရုံနဲ့ Redis 8.2.2၊ generated password နဲ့ `/data` volume ကို Cloudrail က အလိုအလျောက် deploy လုပ်ပေးပါတယ်။
3. Redis node ကိုဖွင့်ပြီး **Settings → Connect an application** မှာ target application ကိုရွေးပါ။ Variable ကို `REDIS_URL` အတိုင်းထားပါ။
4. Application ကို redeploy လုပ်ရင် private Redis connection ကိုရပါပြီ။ Secret တန်ဖိုးကို UI/API က ပြန်မပြပါဘူး။

Redis data က container stop နဲ့ agent restart ပြီးလည်း volume ထဲမှာဆက်ရှိပါတယ်။ Backup/restore လုပ်မယ်ဆို Redis ကိုအရင် Stop လုပ်ပါ။ Version တူတဲ့ Redis target ဗလာထဲပဲ restore ဝင်ပြီး key ရှိပြီးသား target ကို data မဖျက်ဘဲ reject လုပ်ပါတယ်။

### S3-compatible bucket ချိတ်ရန်

Storage provider ဘက်မှာ bucket နဲ့ access credential ကိုအရင်ဖန်တီးထားပါ။ Cloudrail က AWS S3၊ Cloudflare R2၊ Backblaze B2၊ MinIO လို S3-compatible service ကို application နဲ့ချိတ်ပေးတာဖြစ်ပြီး remote bucket ကိုဖန်တီးတာ၊ object ဖျက်တာ မလုပ်ပါဘူး။

1. Application ရှိတဲ့ project/environment ထဲမှာ **New resource → S3-compatible Bucket** ကိုရွေးပါ။
2. Display name၊ provider endpoint origin၊ region၊ remote bucket name နဲ့ access credential ထည့်ပါ။ MinIO လို provider က path-style access လိုရင် checkbox ကိုဖွင့်ပါ။
3. Bucket node ကိုဖွင့်ပြီး **Connect an application** မှာ web application ကိုရွေးပါ။ Prefix ကို `UPLOADS` လို နာမည်ပေးပါ။
4. Application ကို redeploy လုပ်ပါ။ Deployment အသစ်မှာ `UPLOADS_BUCKET`၊ `UPLOADS_ENDPOINT`၊ `UPLOADS_REGION`၊ `UPLOADS_ACCESS_KEY_ID`၊ `UPLOADS_SECRET_ACCESS_KEY` နဲ့ `UPLOADS_FORCE_PATH_STYLE` variables ရပါမယ်။

Credential ကို Cloudrail က encrypt လုပ်သိမ်းပြီး UI/API မှာ secret ပြန်မဖော်ပြပါဘူး။ Rotate လုပ်ရင် provider ဘက်က key အသစ်ကိုအရင်ဖွင့်၊ Cloudrail မှာ **Rotate credentials** လုပ်၊ ချိတ်ထားတဲ့ application အားလုံးကို redeploy လုပ်ပြီးမှ key အဟောင်းကို provider ဘက်မှာ revoke လုပ်ပါ။ **Disconnect** က saved variables နဲ့ canvas edge ကိုဖယ်ပေးတယ်; လက်ရှိ run နေတဲ့ container ကနေဖယ်ဖို့ redeploy ထပ်လုပ်ရပါတယ်။ Provider key နဲ့ remote objects ကို Cloudrail က မဖျက်ပါဘူး။

HTTP service ရဲ့ volume ကို backup လုပ်ဖို့ service ကိုအရင် Stop လုပ်ပါ။ Restore target ကလည်း stopped ဖြစ်ပြီး workload type တူကာ volume ဗလာဖြစ်ရပါတယ်။ Volume export အရွယ်အစားက 1 GB အထိဖြစ်ပါတယ်။

UI က လက်ရှိ installation ထဲမှာကျန်နေတဲ့ backups ကို restore လုပ်ပေးတာဖြစ်ပါတယ်။ အပြင်က dump တစ်ခု upload လုပ်ပြီး restore လုပ်တဲ့ UI မပါသေးပါဘူး။ Server တစ်ခုလုံး recovery အတွက် database dump အပြင် encryption key၊ node identity၊ registry နဲ့ app volumes တွေပါလိုပါတယ်။ [Operations guide](vps-operations.md#back-up-and-restore) ကိုဖတ်ပါ။

Alpha.3 မှာ server တစ်ခုလုံးအတွက် cold backup / host restore command ပါဝင်ပါတယ်။ Backup ယူနေချိန် apps တွေ ခဏရပ်မယ်။ Backup directory အပြည့်ကို server ပြင်ပမှာ encrypt လုပ်ပြီးသိမ်းပါ။ Restore က မူလ server ကိုရပ်ထားပြီး version/architecture တူတဲ့ Docker host ဗလာတစ်လုံးပေါ်မှာလုပ်ရပါတယ်။ လက်ရှိ server ကို overwrite လုပ်တဲ့ command မဟုတ်ပါဘူး။ [Host recovery အဆင့်ဆင့်](host-recovery.md) ကိုလိုက်ပါ။

## ၉။ Deploy ပျက်သွားလျှင်

Failed deployment ရဲ့ error နဲ့ logs ကိုအရင်ကြည့်ပါ။ Stateless app အသစ် readiness မအောင်ရင် အရင် release ကို ဆက်သုံးနိုင်အောင်လုပ်ထားပါတယ်။ Persistent service ဆို writer နှစ်ခုမဖြစ်အောင် container အဟောင်းကိုရပ်ပြီးမှ အသစ်ကိုစပါတယ်၊ ဒါကြောင့် ခဏပြတ်နိုင်ပါတယ်။

History က image အဟောင်းကို redeploy လုပ်ရင် deployment အသစ်တစ်ခုဖြစ်ပါတယ်။ **Database migration နဲ့ data ပြောင်းလဲမှုတွေကို image rollback က နောက်ပြန်မပြင်ပေးပါဘူး။** ပြဿနာအလိုက် [Troubleshooting](troubleshooting.md) မှာကြည့်ပါ။

## ၁၀။ VPS / Linode ပေါ်တင်ခြင်း

Local preview ကို public ဖွင့်ရုံနဲ့ VPS installation မပြီးပါဘူး။ Domain၊ wildcard DNS၊ ports 80/443၊ HTTPS နဲ့ backup တွေပါပြင်ဖို့ [VPS install guide](vps-operations.md) အတိုင်းလိုက်ပါ။

ပထမ public test ကို Linode ပေါ်မှာလုပ်ရန် [Linode pilot guide](linode-pilot.md) အတိုင်း SSH၊ DNS၊ firewall၊ install နဲ့ live checks ကိုအစဉ်လိုက်သွားပါမယ်။ AWS က မဖြစ်မနေမလိုဘဲ နောက်ပိုင်း provider comparison အတွက် [AWS pilot guide](aws-pilot.md) ကိုဆက်ထားပါတယ်။
