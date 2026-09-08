import json
import subprocess
import time
import urllib.request
import urllib.error
from test_client import Client, ENV

client=Client();owner=client.login()
client.json('/auth/setup', {'token':ENV['CLOUDRAIL_ADMIN_TOKEN'], **owner}, expected=409)
client.json('/api/projects', {'name':'Forbidden'}, expected=403, headers={'Origin':'https://evil.example'})
assert client.json('/auth/status')['email']==owner['email']
print('PASS owner bootstrap is one-time; session and origin protection')
p=client.json('/api/projects',{'name':'Phase 2 '+str(int(time.time()))},expected=201)
client.json('/api/projects/'+p['id']+'/environments',{'name':'staging'},expected=201)
services=[client.json('/api/projects/'+p['id']+'/services',{'name':'api','environment':env},expected=201) for env in ('production','staging')]
assert services[0]['id']!=services[1]['id']
image=json.loads(subprocess.check_output(['docker','image','inspect','traefik/whoami:v1.11.0','--format','{{json .RepoDigests}}']))[0]
secret='phase2-private-'+str(int(time.time()))
for svc in services:
 value=secret+'-'+svc['environment']
 client.json('/api/services/'+svc['id']+'/variables/APP_PRIVATE',{'value':value},method='PUT')
 names=client.json('/api/services/'+svc['id']+'/variables');assert names=={'names':['APP_PRIVATE']}
def wait(kind,id,wanted):
 for _ in range(180):
  state=client.json('/api/state');entry=next(x for x in state[kind] if x['id']==id)
  if entry['status']==wanted:return entry
  if entry['status']=='failed':raise AssertionError(entry['error'])
  time.sleep(.5)
 raise AssertionError('Timed out '+id)
for svc in services:
 d=client.json('/api/services/'+svc['id']+'/deployments',{'image':image,'port':80,'healthPath':'/'},expected=202)
 wait('deployments',d['id'],'active');svc['deployment']=d
 runtime_env=json.loads(subprocess.check_output(['docker','inspect','cloudrail-app-'+d['id'],'--format','{{json .Config.Env}}']))
 assert 'APP_PRIVATE='+secret+'-'+svc['environment'] in runtime_env
assert secret not in json.dumps(client.json('/api/state'))
stored=subprocess.check_output(['docker','exec','cloudrail-postgres-1','psql','-U','cloudrail','-d','cloudrail','-Atc',"SELECT encode(ciphertext,'escape') FROM service_variables"])
assert secret.encode() not in stored
print('PASS environments isolate identical service names and encrypted runtime variables')
def get(svc):
 req=urllib.request.Request('http://127.0.0.1:8088/',headers={'Host':svc['host']})
 try:
  with urllib.request.urlopen(req,timeout=5) as r:return r.status
 except urllib.error.HTTPError as e:return e.code
for action in ('stop','start','restart'):
 a=client.json('/api/services/'+services[1]['id']+'/actions',{'kind':action},expected=202);wait('actions',a['id'],'done')
 assert get(services[0])==200
 assert get(services[1])==(404 if action=='stop' else 200)
print('PASS stop/start/restart change staging only; production remains serving')
client.json('/auth/logout',{})
client.json('/api/state',expected=401)
print('PASS logout invalidates the session; Phase 2 acceptance complete')
