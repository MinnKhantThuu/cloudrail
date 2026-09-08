"""Local TLS routing proof using an explicitly trusted test certificate; not ACME proof."""
import json,pathlib,subprocess,time,urllib.request,ssl
from test_client import ROOT
result=json.loads((ROOT/'.data/phase4-result.json').read_text())
d=result['services'][0]['deployment'];directory=ROOT/'.data/tls-lab';directory.mkdir(mode=0o700,exist_ok=True)
subprocess.run(['openssl','req','-x509','-newkey','rsa:2048','-nodes','-keyout',str(directory/'key.pem'),'-out',str(directory/'cert.pem'),'-days','1','-subj','/CN=localhost','-addext','subjectAltName=DNS:localhost'],check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
config={'tls':{'certificates':[{'certFile':'/lab/cert.pem','keyFile':'/lab/key.pem'}]},'http':{'routers':{'proof':{'rule':'Host(`localhost`)','entryPoints':['websecure'],'service':'proof','tls':{}}},'services':{'proof':{'loadBalancer':{'servers':[{'url':'http://cloudrail-app-'+d['id']+':80'}]}}}}}
(directory/'routes.yaml').write_text(json.dumps(config))
name='cloudrail-tls-verification'
subprocess.run(['docker','run','--rm','-d','--name',name,'--network','cloudrail-apps','--memory','128m','-v',str(directory)+':/lab:ro','-p','127.0.0.1:18443:443','traefik:v3.5.2','--entrypoints.websecure.address=:443','--providers.file.filename=/lab/routes.yaml','--log.level=WARN'],check=True,stdout=subprocess.DEVNULL)
try:
 context=ssl.create_default_context(cafile=str(directory/'cert.pem'))
 for _ in range(30):
  try:
   req=urllib.request.Request('https://localhost:18443/',headers={'Host':'localhost'})
   with urllib.request.urlopen(req,context=context,timeout=3) as r:assert r.status==200 and b'Hostname:' in r.read()
   print('PASS TLS certificate validation and Traefik HTTPS application routing');break
  except OSError:time.sleep(.5)
 else:raise AssertionError('TLS proxy did not become ready')
finally:subprocess.run(['docker','rm','-f',name],check=True,stdout=subprocess.DEVNULL)
