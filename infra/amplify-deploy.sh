#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TF_DIR="$ROOT/infra/terraform"
AWS_REGION="${AWS_REGION:-us-east-1}"
AMPLIFY_APP_NAME="${AMPLIFY_APP_NAME:-entropy}"
AMPLIFY_BRANCH="${AMPLIFY_BRANCH:-production}"
: "${API_HOST:?Set API_HOST to the HTTPS backend DNS name}"

for command in aws curl jq npm terraform zip; do
  command -v "$command" >/dev/null || {
    echo "Required command not found: $command" >&2
    exit 1
  }
done

COGNITO_USER_POOL_ID="$(terraform -chdir="$TF_DIR" output -raw cognito_user_pool_id)"
COGNITO_CLIENT_ID="$(terraform -chdir="$TF_DIR" output -raw cognito_client_id)"

VITE_API_URL="https://$API_HOST" \
VITE_WS_URL="wss://$API_HOST/ws" \
VITE_COGNITO_USER_POOL_ID="$COGNITO_USER_POOL_ID" \
VITE_COGNITO_CLIENT_ID="$COGNITO_CLIENT_ID" \
VITE_COGNITO_REGION="$AWS_REGION" \
  npm --prefix "$ROOT/frontend" run build

app_id="$(aws amplify list-apps --region "$AWS_REGION" \
  --query "apps[?name=='$AMPLIFY_APP_NAME'].appId | [0]" --output text)"
if [[ "$app_id" == "None" || -z "$app_id" ]]; then
  app_id="$(aws amplify create-app \
    --region "$AWS_REGION" \
    --name "$AMPLIFY_APP_NAME" \
    --platform WEB \
    --query app.appId --output text)"
fi

branch_name="$(aws amplify get-branch \
  --region "$AWS_REGION" \
  --app-id "$app_id" \
  --branch-name "$AMPLIFY_BRANCH" \
  --query branch.branchName --output text 2>/dev/null || true)"
if [[ "$branch_name" != "$AMPLIFY_BRANCH" ]]; then
  aws amplify create-branch \
    --region "$AWS_REGION" \
    --app-id "$app_id" \
    --branch-name "$AMPLIFY_BRANCH" >/dev/null
fi

archive="$(mktemp -t entropy-amplify.XXXXXX.zip)"
trap 'rm -f "$archive"' EXIT
(
  cd "$ROOT/frontend/dist"
  zip -qr "$archive" .
)

deployment="$(aws amplify create-deployment \
  --region "$AWS_REGION" \
  --app-id "$app_id" \
  --branch-name "$AMPLIFY_BRANCH")"
upload_url="$(jq -r '.zipUploadUrl' <<<"$deployment")"
job_id="$(jq -r '.jobId' <<<"$deployment")"
curl --fail --silent --show-error -T "$archive" "$upload_url"
aws amplify start-deployment \
  --region "$AWS_REGION" \
  --app-id "$app_id" \
  --branch-name "$AMPLIFY_BRANCH" \
  --job-id "$job_id" >/dev/null

domain="$(aws amplify get-app \
  --region "$AWS_REGION" \
  --app-id "$app_id" \
  --query app.defaultDomain --output text)"
echo "Amplify deployment started: https://$AMPLIFY_BRANCH.$domain"
