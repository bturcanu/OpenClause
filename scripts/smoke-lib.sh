#!/usr/bin/env bash

ensure_setup_initialized() {
  local base_url="${1:?base URL is required}"
  local admin_email="${2:?admin email is required}"
  local admin_password="${3:?admin password is required}"
  local org_name="${4:-OpenClause Local Org}"
  local first_tenant_name="${5:-OpenClause Local Tenant}"

  local initialized
  initialized="$(curl -fsS "${base_url}/setup/status" | jq -er '.initialized')"

  case "$initialized" in
    true)
      return 0
      ;;
    false)
      curl -fsS -X POST "${base_url}/setup/initialize" \
        -H 'Content-Type: application/json' \
        --data "$(jq -nc \
          --arg org_name "$org_name" \
          --arg email "$admin_email" \
          --arg password "$admin_password" \
          --arg first_tenant_name "$first_tenant_name" \
          '{org_name:$org_name,email:$email,password:$password,first_tenant_name:$first_tenant_name}')" >/dev/null
      ;;
    *)
      echo "unexpected setup status response from ${base_url}/setup/status: ${initialized}" >&2
      return 1
      ;;
  esac
}

login_console_admin() {
  local base_url="${1:?base URL is required}"
  local admin_email="${2:?admin email is required}"
  local admin_password="${3:?admin password is required}"

  curl -fsS -X POST "${base_url}/auth/login" \
    -H 'Content-Type: application/json' \
    --data "$(jq -nc \
      --arg email "$admin_email" \
      --arg password "$admin_password" \
      '{email:$email,password:$password}')" \
    | jq -er '.token'
}
