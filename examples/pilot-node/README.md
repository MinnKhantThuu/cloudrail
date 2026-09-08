# Cloudrail pilot Node app

This dependency-free HTTP service is the public-host push-to-deploy fixture. Configure a Cloudrail service with:

- repository: `MinnKhantThuu/cloudrail`
- branch: `main`
- root directory: `examples/pilot-node`
- builder: Railpack
- port: `3000`
- health path: `/health`
- automatic deploy: enabled

The initial `pilot-v1` response establishes the baseline. A later committed release-string change is used to prove that one signed GitHub push delivery creates one build/deployment.
