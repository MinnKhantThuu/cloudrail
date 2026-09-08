import hashlib,json,pathlib,subprocess,time,urllib.request
from test_client import Client,ROOT
c=Client();c.login()
p=c.json('/api/projects',{'name':'VPS operations '+str(int(time.time()))},expected=201)
result={'project':p}
def wait(fn,label,timeout=100):
 end=time.monotonic()+timeout
 while time.monotonic()<end:
  v=fn()
  if v:return v
  time.sleep(.5)
 raise AssertionError('Timed out: '+label)
def active(s):
 state=c.json('/api/state');v=next(x for x in state['services'] if x['id']==s['id'])
 if not v['activeId']:
  ds=[d for d in state['deployments'] if d['serviceId']==s['id']]
  if ds and ds[0]['status']=='failed':raise AssertionError(ds[0]['error'])
  return None
 return next(d for d in state['deployments'] if d['id']==v['activeId'])
def action(s,kind,backup=None):
 a=c.json('/api/services/'+s['id']+'/actions',{'kind':kind,**({'backupId':backup} if backup else {})},expected=202)
 def done():
  v=next(x for x in c.json('/api/state')['actions'] if x['id']==a['id'])
  if v['status']=='failed':raise AssertionError(v['error'])
  return v if v['status']=='done' else None
 wait(done,kind);return a
def pg(d,sql):return subprocess.check_output(['docker','exec','cloudrail-app-'+d['id'],'psql','-U','app','-d','app','-Atc',sql]).decode().strip()
db=c.json('/api/projects/'+p['id']+'/databases',{'name':'database'},expected=201);d=wait(lambda:active(db),'PostgreSQL template')
pg(d,"CREATE TABLE proof(id integer primary key, note text); INSERT INTO proof VALUES(1,'persistent-data');")
network=json.loads(subprocess.check_output(['docker','inspect','cloudrail-app-'+d['id'],'--format','{{json .NetworkSettings.Networks}}']))
assert len(network)==1 and next(iter(network)).startswith('cloudrail-env-')
assert not json.loads(subprocess.check_output(['docker','inspect','cloudrail-app-'+d['id'],'--format','{{json .HostConfig.PortBindings}}']))
print('PASS PostgreSQL template runs on private environment network with no public port',flush=True)
# Replace the database container with the pinned image; only one writer may run.
replacement=c.json('/api/services/'+db['id']+'/deployments',{'image':d['image'],'port':5432,'healthPath':'/'},expected=202)
def replaced():
 current=active(db)
 return current if current and current['id']==replacement['id'] else None
d=wait(replaced,'stateful replacement');assert pg(d,'SELECT note FROM proof WHERE id=1')=='persistent-data'
print('PASS PostgreSQL data survives stop-before-start replacement',flush=True)
app=c.json('/api/projects/'+p['id']+'/services',{'name':'application'},expected=201)
c.json('/api/services/'+db['id']+'/bindings',{'targetServiceId':app['id'],'variableName':'DATABASE_URL'})
assert 'DATABASE_URL' in c.json('/api/services/'+app['id']+'/variables')['names']
c.json('/api/projects/'+p['id']+'/environments',{'name':'staging'},expected=201)
other=c.json('/api/projects/'+p['id']+'/services',{'name':'isolated','environment':'staging'},expected=201)
c.json('/api/services/'+db['id']+'/bindings',{'targetServiceId':other['id'],'variableName':'DATABASE_URL'},expected=400)
print('PASS private database binding is hidden and rejects another environment',flush=True)
a=action(db,'backup');backup=next(b for b in c.json('/api/backups') if b['id']==a['id'])
req=urllib.request.Request('http://127.0.0.1:8080/api/backups/'+backup['id']+'/download')
with c.opener.open(req) as r:dump=r.read();assert hashlib.sha256(dump).hexdigest()==backup['checksum']
restored=c.json('/api/projects/'+p['id']+'/databases',{'name':'database-restored'},expected=201);rd=wait(lambda:active(restored),'empty restore target')
action(restored,'restore',backup['id']);assert pg(rd,'SELECT note FROM proof WHERE id=1')=='persistent-data'
print('PASS database backup download checksum and empty-target restore preserve data',flush=True)
# Refuse overwrite of nonempty target.
a=c.json('/api/services/'+restored['id']+'/actions',{'kind':'restore','backupId':backup['id']},expected=202)
wait(lambda:next(x for x in c.json('/api/state')['actions'] if x['id']==a['id'])['status']=='failed','nonempty restore rejection')
assert pg(rd,'SELECT count(*) FROM proof')=='1'
result.update({'database':db,'restored':restored,'app':app,'backup':backup})
(ROOT/'.data/phase5-result.json').write_text(json.dumps(result,indent=2))
print('PASS nonempty target restore rejected without changing data',flush=True)
