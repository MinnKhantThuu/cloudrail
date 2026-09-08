# Cloudrail အသုံးပြုလမ်းညွှန်

Cloudrail က ကိုယ်ပိုင် Linux server ပေါ်မှာ application တွေတင်ဖို့ open-source deployment platform ဖြစ်ပါတယ်။ Dashboard ကနေ project၊ environment၊ service တွေခွဲပြီး app တင်တာ၊ logs ကြည့်တာ၊ PostgreSQL database နဲ့ backups စီမံတာတွေ လုပ်နိုင်ပါတယ်။

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
  → Service ထည့်
  → GitHub source သို့မဟုတ် container image သတ်မှတ်
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
| PostgreSQL service | Private network ထဲမှာသုံးတဲ့ database |
| Build | Source commit တစ်ခုကနေ image ထုတ်ခြင်း |
| Deployment | Image တစ်ခုကို configuration snapshot နဲ့ run ဖို့ကြိုးစားမှု |

Staging အသစ်ဖန်တီးတာက production data ကို အလိုအလျောက်ကူးပေးတာ မဟုတ်ပါဘူး။ အဲဒီ environment ထဲမှာ လိုတဲ့ services နဲ့ variables ကို သီးခြားထည့်ရပါတယ်။

## ၃။ ပထမ app ကို image နဲ့တင်ကြည့်ခြင်း

```sh
docker pull traefik/whoami:v1.11.0
docker image inspect traefik/whoami:v1.11.0 --format '{{index .RepoDigests 0}}'
```

1. Dashboard မှာ project တစ်ခုဆောက်ပြီး `production` ကိုရွေးပါ။
2. **New service → Application** ကိုရွေးပြီး `hello` လို့နာမည်ပေးပါ။
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

## ၅။ Variables နဲ့ update လုပ်ခြင်း

Service ရဲ့ **Variables** tab မှာ `DATABASE_URL`၊ `PORT` စတာတွေထည့်နိုင်ပါတယ်။ သိမ်းပြီးတဲ့ secret value ကို UI ကပြန်မဖော်ပြပါဘူး။ ပြောင်းချင်ရင် value အသစ်ထည့်ရပါတယ်။

Variables နဲ့ resource settings ပြင်ပြီးရင် **deploy အသစ်လုပ်ပါ**။ Restart က လက်ရှိ deployment ရဲ့ configuration ကိုပဲပြန်သုံးပါတယ်။ Runtime variables တွေကို build အတွင်း မပို့ပါဘူး။ Private package build secrets ကို ဒီ alpha မှာ မပံ့ပိုးသေးပါဘူး။

## ၆။ Database ချိတ်ခြင်း

1. Application နဲ့ project/environment တူတဲ့နေရာမှာ **New service → PostgreSQL database** ဖန်တီးပါ။
2. Active ဖြစ်ရင် database ရဲ့ **Settings** ကိုဖွင့်ပါ။
3. **Connect an application** မှာ app ကိုရွေးပြီး `DATABASE_URL` variable အဖြစ် save လုပ်ပါ။
4. အဲဒီ app ကို redeploy လုပ်ပါ။

Database မှာ public port မဖွင့်ထားပါဘူး။ Password ကိုလည်း dashboard မှာ မဖော်ပြပါဘူး။ Environment မတူတဲ့ app ကို တိုက်ရိုက်ချိတ်တာကို ပိတ်ထားပါတယ်။

## ၇။ Backup နဲ့ restore

PostgreSQL ရဲ့ **Settings → Create backup** ကနေ dump ထုတ်ပြီး download လုပ်နိုင်ပါတယ်။ Server ပြင်ပက လုံခြုံတဲ့နေရာမှာ copy သိမ်းပါ။ Restore လုပ်ဖို့ database အသစ်တစ်ခုဖန်တီးပြီး backup ကိုရွေးပါ။ Data ရှိပြီးသား database ကို overwrite လုပ်တာကို ပိတ်ထားပါတယ်။

HTTP service ရဲ့ volume ကို backup လုပ်ဖို့ service ကိုအရင် Stop လုပ်ပါ။ Restore target ကလည်း stopped ဖြစ်ပြီး volume ဗလာဖြစ်ရပါတယ်။ Volume export အရွယ်အစားက 1 GB အထိဖြစ်ပါတယ်။

UI က လက်ရှိ installation ထဲမှာကျန်နေတဲ့ backups ကို restore လုပ်ပေးတာဖြစ်ပါတယ်။ အပြင်က dump တစ်ခု upload လုပ်ပြီး restore လုပ်တဲ့ UI မပါသေးပါဘူး။ Server တစ်ခုလုံး recovery အတွက် database dump အပြင် encryption key၊ node identity၊ registry နဲ့ app volumes တွေပါလိုပါတယ်။ [Operations guide](vps-operations.md#back-up-and-restore) ကိုဖတ်ပါ။

Alpha.3 မှာ server တစ်ခုလုံးအတွက် cold backup / host restore command ပါဝင်ပါတယ်။ Backup ယူနေချိန် apps တွေ ခဏရပ်မယ်။ Backup directory အပြည့်ကို server ပြင်ပမှာ encrypt လုပ်ပြီးသိမ်းပါ။ Restore က မူလ server ကိုရပ်ထားပြီး version/architecture တူတဲ့ Docker host ဗလာတစ်လုံးပေါ်မှာလုပ်ရပါတယ်။ လက်ရှိ server ကို overwrite လုပ်တဲ့ command မဟုတ်ပါဘူး။ [Host recovery အဆင့်ဆင့်](host-recovery.md) ကိုလိုက်ပါ။

## ၈။ Deploy ပျက်သွားလျှင်

Failed deployment ရဲ့ error နဲ့ logs ကိုအရင်ကြည့်ပါ။ Stateless app အသစ် readiness မအောင်ရင် အရင် release ကို ဆက်သုံးနိုင်အောင်လုပ်ထားပါတယ်။ Persistent service ဆို writer နှစ်ခုမဖြစ်အောင် container အဟောင်းကိုရပ်ပြီးမှ အသစ်ကိုစပါတယ်၊ ဒါကြောင့် ခဏပြတ်နိုင်ပါတယ်။

History က image အဟောင်းကို redeploy လုပ်ရင် deployment အသစ်တစ်ခုဖြစ်ပါတယ်။ **Database migration နဲ့ data ပြောင်းလဲမှုတွေကို image rollback က နောက်ပြန်မပြင်ပေးပါဘူး။** ပြဿနာအလိုက် [Troubleshooting](troubleshooting.md) မှာကြည့်ပါ။

## ၉။ VPS / AWS ပေါ်တင်ခြင်း

Local preview ကို public ဖွင့်ရုံနဲ့ VPS installation မပြီးပါဘူး။ Domain၊ wildcard DNS၊ ports 80/443၊ HTTPS နဲ့ backup တွေပါပြင်ဖို့ [VPS install guide](vps-operations.md) အတိုင်းလိုက်ပါ။

AWS သုံးမယ်ဆို [AWS pilot guide](aws-pilot.md) မှာ account/region ရွေးခြင်း၊ cost estimate၊ template နဲ့ live checks တွေရှိပါတယ်။ Plan ထုတ်တဲ့ script က read-only ဖြစ်ပြီး infrastructure template ကို apply လုပ်မှ billable resources ဖန်တီးပါတယ်။
