# Getting started with Netdata Cloud On-Prem
Helm chart for the Netdata Cloud On-Prem installation on Kubernetes is available at ECR registry.
ECR registry is private, so you need to login first. Credentials are sent by our Product team. If you do not have them, please contact our marketing team - info@netdata.cloud.

Firstly credentials must be set for authentication with AWS's ECR:
```bash
export AWS_ACCESS_KEY_ID=<your_secret_id>
export AWS_SECRET_ACCESS_KEY=<your_secret_key>
```
Than login with helm:
```bash
aws ecr get-login-password --region us-east-1 | helm registry login --username AWS --password-stdin 362923047827.dkr.ecr.us-east-1.amazonaws.com/netdata-cloud-onprem
```

After this step you should be able to add the repository to your helm or just pull the helm chart:
```bash
helm pull oci://362923047827.dkr.ecr.us-east-1.amazonaws.com/netdata-cloud-onprem
```

Further instructions on how to proceed with the installation are located in the `README.md` file in the chart directory.