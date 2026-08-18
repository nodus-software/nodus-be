#!/usr/bin/env bash

set -Eeuo pipefail

IMAGE_REF="${1:?usage: deploy.sh IMAGE_REF HEALTH_URL}"
readonly HEALTH_URL="${2:?usage: deploy.sh IMAGE_REF HEALTH_URL}"
readonly DEPLOY_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly COMPOSE_FILE="${DEPLOY_DIR}/compose.production.yml"
readonly LOCK_FILE="${DEPLOY_DIR}/deploy.lock"
readonly EXPECTED_IMAGE_PATTERN='^ghcr\.io/nodus-software/nodus-be@sha256:[0-9a-f]{64}$'

if [[ ! "${IMAGE_REF}" =~ ${EXPECTED_IMAGE_PATTERN} ]]; then
  echo "refusing mutable or unexpected image reference: ${IMAGE_REF}" >&2
  exit 1
fi

if [[ ! "${HEALTH_URL}" =~ ^https:// ]]; then
  echo "HEALTH_URL must use HTTPS" >&2
  exit 1
fi

if [[ ! -f "${DEPLOY_DIR}/.env" ]]; then
  echo "missing host-managed environment file: ${DEPLOY_DIR}/.env" >&2
  exit 1
fi

if [[ ! -f "${DEPLOY_DIR}/alloy.env" ]]; then
  echo "missing host-managed Alloy environment file: ${DEPLOY_DIR}/alloy.env" >&2
  exit 1
fi

exec 9>"${LOCK_FILE}"
if ! flock -n 9; then
  echo "another deployment is already running" >&2
  exit 1
fi

compose=(docker compose --project-directory "${DEPLOY_DIR}" --file "${COMPOSE_FILE}")
export IMAGE_REF

current_container="$("${compose[@]}" ps --quiet api 2>/dev/null || true)"
previous_ref=""
if [[ -n "${current_container}" ]]; then
  previous_ref="$(docker inspect --format '{{.Config.Image}}' "${current_container}" 2>/dev/null || true)"
fi

wait_for_container() {
  local attempts=30
  local attempt
  local container_id
  local status
  container_id="$("${compose[@]}" ps --quiet api)"

  for ((attempt = 1; attempt <= attempts; attempt++)); do
    status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "${container_id}" 2>/dev/null || true)"
    case "${status}" in
      healthy)
        return 0
        ;;
      exited|dead|unhealthy)
        docker logs --tail 100 "${container_id}" >&2 || true
        return 1
        ;;
    esac
    sleep 2
  done

  docker logs --tail 100 "${container_id}" >&2 || true
  return 1
}

wait_for_alloy() {
  local attempts=10
  local attempt
  local container_id
  local status
  container_id="$("${compose[@]}" ps --quiet alloy)"

  for ((attempt = 1; attempt <= attempts; attempt++)); do
    status="$(docker inspect --format '{{.State.Status}}' "${container_id}" 2>/dev/null || true)"
    case "${status}" in
      running)
        return 0
        ;;
      exited|dead)
        docker logs --tail 100 "${container_id}" >&2 || true
        return 1
        ;;
    esac
    sleep 1
  done

  docker logs --tail 100 "${container_id}" >&2 || true
  return 1
}

rollback() {
  if [[ -z "${previous_ref}" || "${previous_ref}" == "${IMAGE_REF}" ]]; then
    echo "no previous application image is available for rollback" >&2
    return 1
  fi

  echo "rolling the application back to ${previous_ref}" >&2
  export IMAGE_REF="${previous_ref}"
  "${compose[@]}" up --detach --no-deps --force-recreate api
  wait_for_container
}

echo "pulling ${IMAGE_REF}"
docker pull "${IMAGE_REF}"

echo "pulling Grafana Alloy"
"${compose[@]}" pull alloy

echo "validating Grafana Alloy configuration"
"${compose[@]}" run --rm --no-deps alloy validate --stability.level=experimental /etc/alloy/config.alloy

echo "starting Grafana Alloy"
"${compose[@]}" up --detach --no-deps --force-recreate alloy
if ! wait_for_alloy; then
  echo "deployment failed Grafana Alloy startup verification" >&2
  exit 1
fi

echo "applying database migrations"
"${compose[@]}" --profile tools run --rm migrate

echo "starting the API"
"${compose[@]}" up --detach --no-deps --force-recreate api

if ! wait_for_container; then
  rollback || true
  echo "deployment failed container health verification" >&2
  exit 1
fi

if ! curl --fail --silent --show-error --retry 5 --retry-delay 2 --max-time 10 "${HEALTH_URL}" >/dev/null; then
  rollback || true
  echo "deployment failed public health verification" >&2
  exit 1
fi

echo "deployment of ${IMAGE_REF} succeeded"
docker image prune --all --force \
  --filter 'label=org.opencontainers.image.source=https://github.com/nodus-software/nodus-be' >/dev/null
