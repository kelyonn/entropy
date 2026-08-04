#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TF_DIR="$ROOT/infra/terraform"
AWS_REGION="${AWS_REGION:-us-east-1}"
IMAGE_TAG="${IMAGE_TAG:-$(git -C "$ROOT" rev-parse --short=12 HEAD)}"
: "${API_HOST:?Set API_HOST to the DNS name for the backend ALB}"
: "${ACM_CERTIFICATE_ARN:?Set ACM_CERTIFICATE_ARN to a certificate covering API_HOST}"

for command in aws docker helm jq kubectl terraform python3; do
  command -v "$command" >/dev/null || {
    echo "Required command not found: $command" >&2
    exit 1
  }
done

CLUSTER_NAME="$(terraform -chdir="$TF_DIR" output -raw eks_cluster_name)"
S3_BUCKET="$(terraform -chdir="$TF_DIR" output -raw s3_bucket)"
COGNITO_USER_POOL_ID="$(terraform -chdir="$TF_DIR" output -raw cognito_user_pool_id)"
COGNITO_CLIENT_ID="$(terraform -chdir="$TF_DIR" output -raw cognito_client_id)"
ECR_JSON="$(terraform -chdir="$TF_DIR" output -json ecr_repository_urls)"
ROLE_JSON="$(terraform -chdir="$TF_DIR" output -json service_role_arns)"
SECRET_JSON="$(terraform -chdir="$TF_DIR" output -json secret_arns)"

SESSION_MANAGER_ROLE_ARN="$(jq -r '.session_manager' <<<"$ROLE_JSON")"
AI_EXPLAINER_ROLE_ARN="$(jq -r '.ai_explainer' <<<"$ROLE_JSON")"
LOAD_BALANCER_ROLE_ARN="$(jq -r '.load_balancer_controller' <<<"$ROLE_JSON")"
FLUENT_BIT_ROLE_ARN="$(jq -r '.fluent_bit' <<<"$ROLE_JSON")"

SESSION_MANAGER_IMAGE="$(jq -r '."session-manager"' <<<"$ECR_JSON"):$IMAGE_TAG"
AIRLOCK_IMAGE="$(jq -r '.airlock' <<<"$ECR_JSON"):$IMAGE_TAG"
AI_EXPLAINER_IMAGE="$(jq -r '."ai-explainer"' <<<"$ECR_JSON"):$IMAGE_TAG"
EBPF_MONITOR_IMAGE="$(jq -r '."ebpf-monitor"' <<<"$ECR_JSON"):$IMAGE_TAG"
REGISTRY="${SESSION_MANAGER_IMAGE%%/*}"

aws ecr get-login-password --region "$AWS_REGION" |
  docker login --username AWS --password-stdin "$REGISTRY"

build_and_push() {
  local image="$1"
  local dockerfile="$2"
  docker buildx build \
    --platform linux/amd64 \
    --file "$ROOT/$dockerfile" \
    --tag "$image" \
    --push \
    "$ROOT"
}

build_and_push "$SESSION_MANAGER_IMAGE" Dockerfile.session-manager
build_and_push "$AIRLOCK_IMAGE" Dockerfile.airlock
build_and_push "$AI_EXPLAINER_IMAGE" Dockerfile.ai-explainer
build_and_push "$EBPF_MONITOR_IMAGE" Dockerfile.ebpf-monitor

aws eks update-kubeconfig --region "$AWS_REGION" --name "$CLUSTER_NAME"

helm repo add eks https://aws.github.io/eks-charts
helm repo add metrics-server https://kubernetes-sigs.github.io/metrics-server/
helm repo update

kubectl create serviceaccount aws-load-balancer-controller \
  --namespace kube-system \
  --dry-run=client -o yaml |
  kubectl apply -f -
kubectl annotate serviceaccount aws-load-balancer-controller \
  --namespace kube-system \
  "eks.amazonaws.com/role-arn=$LOAD_BALANCER_ROLE_ARN" \
  --overwrite

helm upgrade --install aws-load-balancer-controller eks/aws-load-balancer-controller \
  --namespace kube-system \
  --set "clusterName=$CLUSTER_NAME" \
  --set serviceAccount.create=false \
  --set serviceAccount.name=aws-load-balancer-controller
helm upgrade --install metrics-server metrics-server/metrics-server \
  --namespace kube-system

kubectl create namespace amazon-cloudwatch --dry-run=client -o yaml | kubectl apply -f -
kubectl create serviceaccount aws-for-fluent-bit \
  --namespace amazon-cloudwatch \
  --dry-run=client -o yaml |
  kubectl apply -f -
kubectl annotate serviceaccount aws-for-fluent-bit \
  --namespace amazon-cloudwatch \
  "eks.amazonaws.com/role-arn=$FLUENT_BIT_ROLE_ARN" \
  --overwrite
helm upgrade --install aws-for-fluent-bit eks/aws-for-fluent-bit \
  --namespace amazon-cloudwatch \
  --set serviceAccount.create=false \
  --set serviceAccount.name=aws-for-fluent-bit \
  --set cloudWatch.enabled=true \
  --set "cloudWatch.region=$AWS_REGION" \
  --set cloudWatch.logGroupName=/aws/eks/entropy/application \
  --set cloudWatch.autoCreateGroup=false

DATABASE_URL="$(aws secretsmanager get-secret-value \
  --secret-id "$(jq -r '.database_url' <<<"$SECRET_JSON")" \
  --query SecretString --output text)"
REDIS_URL="$(aws secretsmanager get-secret-value \
  --secret-id "$(jq -r '.redis_url' <<<"$SECRET_JSON")" \
  --query SecretString --output text)"

kubectl create namespace entropy --dry-run=client -o yaml | kubectl apply -f -
kubectl create secret generic entropy-secrets \
  --namespace entropy \
  --from-literal="database_url=$DATABASE_URL" \
  --from-literal="redis_url=$REDIS_URL" \
  --dry-run=client -o yaml |
  kubectl apply -f -

export AWS_REGION S3_BUCKET COGNITO_USER_POOL_ID COGNITO_CLIENT_ID
export SESSION_MANAGER_ROLE_ARN AI_EXPLAINER_ROLE_ARN
export SESSION_MANAGER_IMAGE AIRLOCK_IMAGE AI_EXPLAINER_IMAGE EBPF_MONITOR_IMAGE
export API_HOST ACM_CERTIFICATE_ARN

rendered="$(mktemp)"
trap 'rm -f "$rendered"' EXIT
python3 - "$ROOT/k8s/deployment.yaml" "$rendered" <<'PY'
import os
import string
import sys

source, destination = sys.argv[1:3]
with open(source, encoding="utf-8") as handle:
    template = string.Template(handle.read())
with open(destination, "w", encoding="utf-8") as handle:
    handle.write(template.substitute(os.environ))
PY

kubectl apply -f "$rendered"
kubectl rollout status deployment/session-manager -n entropy --timeout=5m
kubectl rollout status deployment/airlock -n entropy --timeout=5m
kubectl rollout status deployment/ai-explainer -n entropy --timeout=5m
kubectl rollout status daemonset/ebpf-monitor -n entropy --timeout=5m
kubectl get ingress entropy -n entropy
