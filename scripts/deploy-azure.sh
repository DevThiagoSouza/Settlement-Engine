#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
TERRAFORM_DIR="$ROOT_DIR/infra/azure"

if [ ! -f "$ROOT_DIR/.env" ]; then
  printf '%s\n' 'Arquivo .env não encontrado.' >&2
  exit 1
fi

set -a
. "$ROOT_DIR/.env"
set +a

: "${AZURE_SUBSCRIPTION_ID:?Defina AZURE_SUBSCRIPTION_ID no .env}"
: "${DB_PASSWORD:?Defina DB_PASSWORD no .env}"
: "${JWT_SECRET:?Defina JWT_SECRET no .env}"
az account set --subscription "$AZURE_SUBSCRIPTION_ID"

export TF_VAR_subscription_id="$AZURE_SUBSCRIPTION_ID"
export TF_VAR_location="${AZURE_LOCATION:-eastus}"
export TF_VAR_resource_group_name="${AZURE_RESOURCE_GROUP_NAME:-settlement-rg}"
export TF_VAR_name_prefix="${AZURE_NAME_PREFIX:-settlement}"
export TF_VAR_container_image="${CONTAINER_IMAGE:-settlement-api:latest}"
export TF_VAR_postgres_admin_username="${DB_USER:-settlement_admin}"
export TF_VAR_postgres_admin_password="$DB_PASSWORD"
export TF_VAR_postgres_database_name="${DB_NAME:-settlement_engine}"
export TF_VAR_environment="${APP_ENV:-production}"
export TF_VAR_jwt_secret="$JWT_SECRET"

terraform -chdir="$TERRAFORM_DIR" init
terraform -chdir="$TERRAFORM_DIR" apply -target=azurerm_container_registry.main -auto-approve

ACR_NAME=$(terraform -chdir="$TERRAFORM_DIR" output -raw container_registry_name)
az acr build --registry "$ACR_NAME" --image "settlement-api:${APP_VERSION:-latest}" "$ROOT_DIR"

export TF_VAR_container_image="$ACR_NAME.azurecr.io/settlement-api:${APP_VERSION:-latest}"
terraform -chdir="$TERRAFORM_DIR" apply -auto-approve
terraform -chdir="$TERRAFORM_DIR" output