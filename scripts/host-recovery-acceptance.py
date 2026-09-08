#!/usr/bin/env python3
"""Disposable runner only; prepare/verify state around an independent Docker daemon restore."""
import hashlib
import json
import os
import pathlib
import subprocess
import sys
import time
import urllib.request
from test_client import Client, ROOT

if os.environ.get('CLOUDRAIL_DISPOSABLE_TEST') != '1':
    raise SystemExit('Requires an explicitly disposable test installation')
c=Client();c.login()

def command(*args):
    return subprocess.check_output(args,text=True).strip()

def wait(fn,label):
    end=time.monotonic()+120
    while time.monotonic()<end:
        result=fn()
        if result:return result
        time.sleep(.5)
    raise AssertionError('Timed out: '+label)

def current(service):
    state=c.json('/api/state')
    s=next(s for s in state['services'] if s['id']==service)
    return s,next((d for d in state['deployments'] if d['id']==s['activeId']),None)

def deploy(service,image):
    d=c.json('/api/services/'+service+'/deployments',{'image':image,'port':80,'healthPath':'/'},expected=202)
    def complete():
        item=next(v for v in c.json('/api/state')['deployments'] if v['id']==d['id'])
        if item['status']=='failed':raise AssertionError(item['error'])
        return item if item['status']=='active' else None
    return wait(complete,'new deployment')

def env_hash(deployment):
    env=json.loads(command('docker','inspect','cloudrail-app-'+deployment,'--format','{{json .Config.Env}}'))
    value=next(v for v in env if v.startswith('DATABASE_URL='))
    return hashlib.sha256(value.encode()).hexdigest()

def route(host):
    try:
        with urllib.request.urlopen(urllib.request.Request('http://127.0.0.1:8088/',headers={'Host':host}),timeout=3) as r:
            return r.status==200 and 'Hostname:' in r.read().decode()
    except OSError:return False

if sys.argv[1]=='prepare':
    result=json.loads((ROOT/'.data/phase5-result.json').read_text())
    app=result['app'];tag='127.0.0.1:5001/cloudrail/'+app['id']+':host-recovery'
    command('docker','tag','traefik/whoami:v1.11.0',tag)
    command('docker','push',tag)
    image=next(ref for ref in json.loads(command('docker','image','inspect',tag,'--format','{{json .RepoDigests}}')) if ref.startswith('127.0.0.1:5001/'))
    d=deploy(app['id'],image)
    stopped=result['volumeTarget']['id']
    action=c.json('/api/services/'+stopped+'/actions',{'kind':'stop'},expected=202)
    wait(lambda:next(a for a in c.json('/api/state')['actions'] if a['id']==action['id'])['status']=='done','stop fixture')
    # Exercise optional certificate-volume retention without claiming real ACME proof.
    command('docker','volume','create','--label','com.docker.compose.project=cloudrail','--label','com.docker.compose.volume=certificates','cloudrail_certificates')
    command('docker','run','--rm','--network','none','--mount','type=volume,src=cloudrail_certificates,dst=/volume','--entrypoint','sh','cloudrail-agent','-c',"printf certificate-storage-fixture > /volume/fixture")
    state=c.json('/api/state')
    fixture={'projectIDs':sorted(p['id'] for p in state['projects']),'serviceIDs':sorted(s['id'] for s in state['services']),
             'app':app['id'],'image':image,'bindingSHA256':env_hash(d['id']),'database':result['database']['id'],
             'restoredDatabase':result['restored']['id'],'backup':result['backup'],'stopped':stopped,
             'sourceDockerID':command('docker','info','--format','{{.ID}}')}
    (ROOT/'.data/host-fixture.json').write_text(json.dumps(fixture));(ROOT/'.data/host-fixture.json').chmod(0o600)
    print('PASS fixture contains a retained registry image, database rows, app volume, backup download, hidden binding and stopped service')
elif sys.argv[1]=='verify':
    fixture=json.loads((ROOT/'.data/host-fixture.json').read_text())
    assert command('docker','info','--format','{{.ID}}')!=fixture['sourceDockerID']
    state=c.json('/api/state')
    assert sorted(p['id'] for p in state['projects'])==fixture['projectIDs']
    assert sorted(s['id'] for s in state['services'])==fixture['serviceIDs']
    app,d=current(fixture['app']);wait(lambda:route(app['host']),'restored application route')
    assert env_hash(d['id'])==fixture['bindingSHA256']
    output=ROOT/'.data/restored-proof.txt'
    command('docker','cp','cloudrail-app-'+d['id']+':/data/proof.txt',str(output))
    assert output.read_text()=='volume-persistence-proof'
    for service in (fixture['database'],fixture['restoredDatabase']):
        _,db=current(service)
        name='cloudrail-app-'+db['id']
        wait(lambda:json.loads(command('docker','inspect',name,'--format','{{json .State}}')).get('Health',{}).get('Status')=='healthy','restored PostgreSQL')
        assert command('docker','exec',name,'psql','-U','app','-d','app','-Atc','SELECT note FROM proof WHERE id=1')=='persistent-data'
        assert not json.loads(command('docker','inspect',name,'--format','{{json .HostConfig.PortBindings}}'))
    stopped,_=current(fixture['stopped']);assert stopped['desiredState']=='stopped'
    existing=json.loads(command('docker','ps','-a','--filter','name=cloudrail-app-'+stopped['activeId'],'--format','{{json .State}}') or '"absent"')
    assert existing in ('absent','exited','created')
    backup=fixture['backup']
    with c.opener.open('http://127.0.0.1:8080/api/backups/'+backup['id']+'/download') as response:
        assert hashlib.sha256(response.read()).hexdigest()==backup['checksum']
    assert command('docker','run','--rm','--network','none','--mount','type=volume,src=cloudrail_certificates,dst=/volume,readonly','--entrypoint','cat','cloudrail-agent','/volume/fixture')=='certificate-storage-fixture'
    assert c.json('/api/node')['online']
    new=deploy(app['id'],fixture['image']);assert env_hash(new['id'])==fixture['bindingSHA256']
    wait(lambda:route(app['host']),'new deployment after host recovery')
    print('PASS independent Docker host: owner/session, identity, projects, DB rows, private networks, app volume, registry image, backups, certificate storage and encrypted binding restored; stopped service stays stopped; new deployment serves HTTP')
else:raise SystemExit('Use prepare or verify')
