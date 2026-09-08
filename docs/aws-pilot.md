# AWS pilot runbook

Status, 2026-09-08: preparation only. No configured AWS profile or credentials were available; STS returned `NoCredentials`. No resource has been provisioned. CloudFormation JSON and CLI shapes can be checked locally, but authenticated template validation, regional catalog selection, DNS/ACME, reboot and billing observation remain pending.

## Proposed capacity and cost

Start with a **4 GB Linux Lightsail instance**, Ubuntu 24.04 and one static public IPv4. Singapore (`ap-southeast-1`) is a provisional region, pending the owner's choice. This keeps one-node operations simple and avoids adding load balancers, NAT gateways or Kubernetes to the pilot.

AWS's published Linux public-IPv4 4 GB bundle is **$24/month**, with 2 vCPUs and 80 GB SSD. Snapshot storage is separately listed at **$0.05/GB-month**. Domain registration, applicable transfer overage and taxes are additional; regional allowances must be checked in the selected catalog. These are estimates, not measured bills. [AWS Lightsail pricing](https://aws.amazon.com/lightsail/pricing/).

Local idle Docker measurements recorded about 211 MB across API, agent, control database, proxy, registry and BuildKit. That excludes applications and build peaks. It does not justify running builds on a 512 MB VPS; BuildKit alone is allowed 2 GB and application limits consume additional capacity.

## Create a read-only plan

Configure an AWS CLI profile using your normal authenticated workflow. Keep access keys out of chat/source control. Select an existing **Lightsail** SSH key pair in the target region whose private key you hold.

```sh
python3 scripts/aws-plan.py --profile YOUR_PROFILE --region ap-southeast-1 \
  --ssh-cidr YOUR_IP/32 --key-pair YOUR_LIGHTSAIL_KEY
```

The script reads account identity, active bundles, Ubuntu blueprints, key pairs and availability zones. It saves `.data/aws-plan/plan.json` and `parameters.json`; it does not create resources. Review the account, region, actual catalog price and SSH range.

## Provision after reviewing the plan

Run from the release root, replacing profile/region consistently:

```sh
aws --profile YOUR_PROFILE --region ap-southeast-1 cloudformation validate-template \
  --template-body file://deploy/aws/lightsail.json

aws --profile YOUR_PROFILE --region ap-southeast-1 cloudformation create-stack \
  --stack-name cloudrail-pilot --template-body file://deploy/aws/lightsail.json \
  --parameters file://.data/aws-plan/parameters.json

aws --profile YOUR_PROFILE --region ap-southeast-1 cloudformation wait stack-create-complete \
  --stack-name cloudrail-pilot

aws --profile YOUR_PROFILE --region ap-southeast-1 cloudformation describe-stacks \
  --stack-name cloudrail-pilot --query 'Stacks[0].Outputs'
```

Creation is billable. The template retains the instance and static IP on stack deletion/replacement to protect data. **Deleting the stack does not stop charges or remove retained resources.** To decommission, first export/verify backups and then explicitly delete the retained instance and release its static IP in Lightsail. Snapshots have their own retention and billing.

The template permits public TCP 80/443 and restricts SSH to the chosen CIDR; database, registry and agent ports are not opened. Its user data only prepares OS packages. Wait for `cloud-init status --wait` over SSH, configure dashboard and wildcard app A records to the output IPv4, copy the reviewed source release, then follow [VPS installation](vps-operations.md).

## Pilot exit checklist

- [ ] Record account, region, bundle, stack ID and public IP without publishing credentials.
- [ ] Fresh Ubuntu install and owner setup succeed over a valid public HTTPS certificate.
- [ ] Install the owner's GitHub App; a signed real push creates one build/deployment.
- [ ] Deploy the owner's selected application and verify its important user flow.
- [ ] Failed build/readiness leaves the previous release serving.
- [ ] Reboot the VPS; node identity, routes, data and application recover.
- [ ] Export a backup off-server; restore into a separate empty target and verify data.
- [ ] Perform an update and recovery rehearsal using the release runbook.
- [ ] Observe at least 48 hours including one build/update; record RAM/CPU/disk, transfer, uptime and costs. A monthly projection is separate from the actual billed amount.

Until these pass, this is a locally verified alpha, not an AWS-verified beta. Existing production DNS/traffic is outside this pilot until its destination and cutover are specified.

Template references: [Lightsail instance](https://docs.aws.amazon.com/AWSCloudFormation/latest/TemplateReference/aws-resource-lightsail-instance.html), [port properties](https://docs.aws.amazon.com/AWSCloudFormation/latest/TemplateReference/aws-properties-lightsail-instance-port.html), [static IP](https://docs.aws.amazon.com/AWSCloudFormation/latest/TemplateReference/aws-resource-lightsail-staticip.html).
