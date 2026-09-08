import json,pathlib,subprocess,time,urllib.request
from test_client import Client,ROOT
c=Client();c.login()
p=c.json('/api/projects',{'name':'Source build demo '+str(int(time.time()))},expected=201)
results={'project':p,'services':[]}
def wait_build(service,id):
 end=time.monotonic()+1000;status=None
 while time.monotonic()<end:
  b=next(b for b in c.json('/api/services/'+service['id']+'/builds') if b['id']==id)
  if b['status']!=status:print(service['name'],b['status'],flush=True);status=b['status']
  if b['status'] in ('failed','cancelled'):
   (ROOT/'.data/failed-build.log').write_text(b['logs'])
   raise AssertionError(b['error']+' (see .data/failed-build.log)')
  if b['status']=='succeeded' and b['deploymentId']:
   d=next(d for d in c.json('/api/state')['deployments'] if d['id']==b['deploymentId'])
   if d['status']=='failed':raise AssertionError(d['error'])
   if d['status']=='active':return b,d
  time.sleep(2)
 raise AssertionError('Build/deploy timed out')
for name,repo,branch,builder,port in [('dockerfile-api','traefik/whoami','master','dockerfile',80),('railpack-api','railwayapp-templates/expressjs','main','railpack',3333)]:
 s=c.json('/api/projects/'+p['id']+'/services',{'name':name},expected=201)
 source={'repository':repo,'installation':0,'branch':branch,'root':'','builder':builder,'dockerfile':'Dockerfile','buildCommand':'','startCommand':'','port':port,'healthPath':'/','autoDeploy':False}
 c.json('/api/services/'+s['id']+'/source',source,method='PUT')
 b=c.json('/api/services/'+s['id']+'/builds',{},expected=202,headers={'Idempotency-Key':'source-first'})
 finished,d=wait_build(s,b['id'])
 with urllib.request.urlopen(urllib.request.Request('http://127.0.0.1:8088/',headers={'Host':s['host']})) as r:
  assert r.status==200 and r.headers.get('X-Cloudrail-Deployment')==d['id']
 assert '@sha256:' in finished['image'] and len(finished['commit'])==40
 results['services'].append({'service':s,'source':source,'build':finished,'deployment':d})
 (ROOT/'.data/phase4-result.json').write_text(json.dumps(results,indent=2))
 print('PASS',builder,'exact GitHub commit → BuildKit → registry digest → live HTTP',flush=True)
# A source build failure must preserve an already serving release.
s=results['services'][0]['service'];source={**results['services'][0]['source'],'dockerfile':'Missing.Dockerfile'}
c.json('/api/services/'+s['id']+'/source',source,method='PUT');b=c.json('/api/services/'+s['id']+'/builds',{},expected=202)
for _ in range(90):
 b=next(x for x in c.json('/api/services/'+s['id']+'/builds') if x['id']==b['id'])
 if b['status']=='failed':break
 time.sleep(1)
assert b['status']=='failed'
with urllib.request.urlopen(urllib.request.Request('http://127.0.0.1:8088/',headers={'Host':s['host']})) as r:assert r.status==200
c.json('/api/services/'+s['id']+'/source',results['services'][0]['source'],method='PUT')
print('PASS build failure preserves current application; source restored',flush=True)
