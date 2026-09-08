import json,subprocess,time,urllib.request
from test_client import Client,ROOT
c=Client();c.login();data=json.loads((ROOT/'.data/phase5-result.json').read_text());p=data['project'];app=data['app']
def wait(fn,label):
 for _ in range(180):
  v=fn()
  if v:return v
  time.sleep(.5)
 raise AssertionError(label)
def deployment(id):return next(x for x in c.json('/api/state')['deployments'] if x['id']==id)
def deploy(s,port=80):
 d=c.json('/api/services/'+s['id']+'/deployments',{'image':image,'port':port,'healthPath':'/'},expected=202)
 def ready():
  v=deployment(d['id'])
  if v['status']=='failed':raise AssertionError(v['error'])
  return v if v['status']=='active' else None
 return wait(ready,'application deployment')
def action(s,kind,backup=None):
 a=c.json('/api/services/'+s['id']+'/actions',{'kind':kind,**({'backupId':backup} if backup else {})},expected=202)
 def done():
  v=next(x for x in c.json('/api/state')['actions'] if x['id']==a['id'])
  if v['status']=='failed':raise AssertionError(v['error'])
  return v if v['status']=='done' else None
 wait(done,kind);return a
image=json.loads(subprocess.check_output(['docker','image','inspect','traefik/whoami:v1.11.0','--format','{{json .RepoDigests}}']))[0]
c.json('/api/services/'+app['id']+'/settings',{'memoryMB':128,'cpuMillis':500,'mountPath':'/data'},method='PUT')
d=deploy(app)
config=json.loads(subprocess.check_output(['docker','inspect','cloudrail-app-'+d['id'],'--format','{{json .HostConfig}}']))
assert config['Memory']==128*1048576 and config['NanoCpus']==500000000
fixture=ROOT/'.data/volume-proof.txt';fixture.write_text('volume-persistence-proof')
subprocess.run(['docker','cp',str(fixture),'cloudrail-app-'+d['id']+':/data/proof.txt'],check=True)
d=deploy(app)
out=ROOT/'.data/volume-readback.txt';subprocess.run(['docker','cp','cloudrail-app-'+d['id']+':/data/proof.txt',str(out)],check=True);assert out.read_text()==fixture.read_text()
c.json('/api/services/'+app['id']+'/settings',{'memoryMB':128,'cpuMillis':500,'mountPath':'/other'},method='PUT',expected=400)
print('PASS actual resource limits and persistent volume replacement; detach rejected',flush=True)
metrics=wait(lambda:c.json('/api/services/'+app['id']+'/metrics').get('memoryBytes'),'actual container metrics');assert metrics>0
host='volume-proof.localhost';c.json('/api/services/'+app['id']+'/domain',{'host':host},method='PUT')
def route():
 try:
  with urllib.request.urlopen(urllib.request.Request('http://127.0.0.1:8088/',headers={'Host':host})) as r:return r.status==200
 except OSError:return False
wait(route,'domain change');print('PASS observed container metrics and primary-domain route change',flush=True)
action(app,'stop');backup=action(app,'backup')
target=c.json('/api/projects/'+p['id']+'/services',{'name':'volume-restored'},expected=201)
c.json('/api/services/'+target['id']+'/settings',{'memoryMB':128,'cpuMillis':500,'mountPath':'/restored'},method='PUT');td=deploy(target);action(target,'stop');action(target,'restore',backup['id'])
subprocess.run(['docker','cp','cloudrail-app-'+td['id']+':/restored/proof.txt',str(out)],check=True);assert out.read_text()==fixture.read_text()
action(app,'start');action(target,'start');data['volumeTarget']=target;(ROOT/'.data/phase5-result.json').write_text(json.dumps(data,indent=2))
print('PASS stopped-volume backup and empty-volume restore, including a different mount path',flush=True)
