import json, subprocess, time, urllib.request, urllib.error
from test_client import Client, ENV
c=Client();c.login()
def sql(query):
 return subprocess.check_output(['docker','exec','cloudrail-postgres-1','psql','-U','cloudrail','-d','cloudrail','-Atc',query]).decode().strip()
def dep(id):return next(d for d in c.json('/api/state')['deployments'] if d['id']==id)
def wait(fn,label,timeout=60):
 end=time.monotonic()+timeout
 while time.monotonic()<end:
  if fn():return
  time.sleep(.5)
 raise AssertionError(label)
wait(lambda:c.json('/api/node')['online'],'node heartbeat')
Client().json('/internal/claim',{},expected=401,headers={'Authorization':'Bearer '+ENV['CLOUDRAIL_AGENT_TOKEN']})
print('PASS enrolled certificate heartbeat and plaintext RPC rejection',flush=True)
p=c.json('/api/projects',{'name':'Phase 3 '+str(int(time.time()))},expected=201)
s=c.json('/api/projects/'+p['id']+'/services',{'name':'reliable-api'},expected=201)
image=json.loads(subprocess.check_output(['docker','image','inspect','traefik/whoami:v1.11.0','--format','{{json .RepoDigests}}']))[0]
path='/api/services/'+s['id']+'/deployments';spec={'image':image,'port':80,'healthPath':'/'}
a=c.json(path,spec,expected=202,headers={'Idempotency-Key':'first'})
assert c.json(path,spec,expected=202,headers={'Idempotency-Key':'first'})['id']==a['id']
c.json(path,{**spec,'port':81},expected=409,headers={'Idempotency-Key':'first'})
wait(lambda:dep(a['id'])['status']=='active','initial activation')
def route():
 try:
  with urllib.request.urlopen(urllib.request.Request('http://127.0.0.1:8088/',headers={'Host':s['host']}),timeout=3) as r:return r.headers.get('X-Cloudrail-Deployment')==a['id']
 except (OSError,urllib.error.URLError):return False
b=c.json(path,{**spec,'port':1},expected=202)
wait(lambda:dep(b['id'])['status']=='checking','candidate readiness')
c.json('/api/deployments/'+b['id']+'/cancel',{},expected=202)
wait(lambda:dep(b['id'])['status']=='failed','cancel cleanup')
assert route()
print('PASS request deduplication, mismatch conflict, running cancellation preserves traffic',flush=True)
subprocess.run(['docker','restart','cloudrail-agent-1'],check=True,stdout=subprocess.DEVNULL)
# Stop a desired-running container out of band; the next reconciliation must repair it.
subprocess.run(['docker','stop','cloudrail-app-'+a['id']],check=True,stdout=subprocess.DEVNULL)
wait(route,'active container restart and route reconciliation')
print('PASS agent restart reconciles stopped active container',flush=True)
c.json('/api/node/revoke',{})
try:
 assert c.json('/api/node')['revoked'] and not c.json('/api/node')['online']
 observed=sql('SELECT last_seen::text FROM nodes')
 time.sleep(6)
 assert sql('SELECT last_seen::text FROM nodes')==observed
 assert route()
 print('PASS revocation blocks heartbeat while existing application serves',flush=True)
finally:
 # This test revokes and restores only the local test installation identity.
 sql('UPDATE nodes SET revoked=false')
wait(lambda:c.json('/api/node')['online'],'node resumes after test reset')
print('Phase 3 runtime acceptance complete',flush=True)
