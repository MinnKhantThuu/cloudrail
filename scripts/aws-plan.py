#!/usr/bin/env python3
"""Read the selected AWS account/catalog and write a reviewable Lightsail stack plan.
Never creates resources. Account/profile and SSH access are explicit inputs.
"""
import argparse,datetime,ipaddress,json,pathlib,subprocess
p=argparse.ArgumentParser(description=__doc__)
p.add_argument('--profile',required=True);p.add_argument('--region',default='ap-southeast-1')
p.add_argument('--ssh-cidr',required=True);p.add_argument('--key-pair',required=True)
p.add_argument('--name',default='cloudrail-pilot');p.add_argument('--output',default='.data/aws-plan')
a=p.parse_args();network=ipaddress.ip_network(a.ssh_cidr,strict=False)
if network.version!=4 or network.prefixlen<24:p.error('Use a restricted IPv4 SSH range (/24 or narrower).')
def aws(*args):return json.loads(subprocess.check_output(['aws','--profile',a.profile,'--region',a.region,'--output','json','--no-cli-pager',*args]))
identity=aws('sts','get-caller-identity')
bundles=aws('lightsail','get-bundles')['bundles']
options=[b for b in bundles if b.get('isActive') and b.get('ramSizeInGb')==4 and 'LINUX_UNIX' in b.get('supportedPlatforms',[]) and 'ipv6' not in b['bundleId'].lower()]
if not options:raise SystemExit('No active 4 GB Linux IPv4 bundle found in this region.')
bundle=min(options,key=lambda b:b['price'])
blueprints=aws('lightsail','get-blueprints')['blueprints']
blueprint=next((b for b in blueprints if b.get('isActive') and b['blueprintId']=='ubuntu_24_04'),None)
if not blueprint:raise SystemExit('Ubuntu 24.04 blueprint is unavailable; verify the supported catalog before proceeding.')
keys=aws('lightsail','get-key-pairs')['keyPairs']
if not any(k['name']==a.key_pair for k in keys):raise SystemExit('The selected Lightsail key pair does not exist in this region.')
regions=aws('lightsail','get-regions','--include-availability-zones')['regions']
region=next(r for r in regions if r['name']==a.region);zone=next(z['zoneName'] for z in region['availabilityZones'] if z.get('state')=='available')
root=pathlib.Path(__file__).resolve().parents[1];template=json.loads((root/'deploy/aws/lightsail.json').read_text())
parameters=[{'ParameterKey':k,'ParameterValue':str(v)} for k,v in {'InstanceName':a.name,'BundleId':bundle['bundleId'],'BlueprintId':blueprint['blueprintId'],'AvailabilityZone':zone,'SSHKeyPair':a.key_pair,'SSHCidr':str(network)}.items()]
out=pathlib.Path(a.output);out.mkdir(parents=True,exist_ok=True)
(out/'parameters.json').write_text(json.dumps(parameters,indent=2)+'\n')
(out/'plan.json').write_text(json.dumps({'createdAt':datetime.datetime.now(datetime.timezone.utc).isoformat(),'account':identity['Account'],'profile':a.profile,'region':a.region,'instanceName':a.name,'bundle':bundle,'blueprint':blueprint['blueprintId'],'sshCidr':str(network),'keyPair':a.key_pair,'baseMonthlyUSD':bundle['price'],'extraCosts':'snapshots, transfer overage, taxes and domain registration are additional','resourcesCreated':False},indent=2)+'\n')
print(f"Plan saved to {out}; base instance price ${bundle['price']}/month. No resources created.")
print('Review the plan and template before provisioning. Actual deployment and installation are separate steps.')
