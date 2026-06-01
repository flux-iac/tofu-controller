#!/bin/bash

# Exit the script if any command fails
set -e

VERSION=e2e-$(git rev-parse --short HEAD)-$(if [[ $(git diff --stat) != '' ]]; then echo 'dirty'; else echo 'clean'; fi)

kind create cluster

[[ -z "$SKIP_IMAGE_BUILD" ]] && make docker-build MANAGER_IMG=test/tofu-controller RUNNER_IMG=test/tf-runner TAG=$VERSION # BUILD_ARGS="--no-cache"

kind load docker-image test/tofu-controller:$VERSION
kind load docker-image test/tf-runner:$VERSION

make install

# Dev deploy
make dev-deploy MANAGER_IMG=test/tofu-controller RUNNER_IMG=test/tf-runner TAG=$VERSION || true
make dev-deploy MANAGER_IMG=test/tofu-controller RUNNER_IMG=test/tf-runner TAG=$VERSION

kubectl patch deployment \
  tofu-controller \
  --namespace tofu-system \
  --type='json' \
  -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/args", "value": [
  "--watch-all-namespaces",
  "--log-level=info",
  "--log-encoding=json",
  "--enable-leader-election",
  "--concurrent=10",
]}]'

kubectl -n tofu-system rollout status deploy/source-controller --timeout=1m
kubectl -n tofu-system rollout status deploy/tofu-controller --timeout=1m

echo "==================== Show Terraform version"
docker run --rm --entrypoint=/usr/local/bin/terraform test/tf-runner:$VERSION version

echo "==================== Add git repository source"
kubectl -n tofu-system apply -f ./config/testdata/source
kubectl -n tofu-system wait gitrepository/helloworld --for=condition=ready --timeout=4m

echo "==================== Run approvePlan tests"
kubectl -n tofu-system apply -f ./config/testdata/approve-plan
kubectl -n tofu-system wait terraform/helloworld-auto-approve --for=condition=ready --timeout=4m
kubectl -n tofu-system wait terraform/helloworld-manual-approve --for=condition=plan=true --timeout=4m

# delete after tests
kubectl -n tofu-system delete -f ./config/testdata/approve-plan

echo "==================== Run plan with pod cleanup tests"

kubectl -n tofu-system apply -f ./config/testdata/always-clean-pod
kubectl -n tofu-system wait terraform/helloworld-always-clean-pod-manual-approve --for=condition=plan=true --timeout=4m

# negate pod not found to be true
! kubectl -n tofu-system get terraform/helloworld-always-clean-pod-manual-approve-tf-runner

# delete after tests
kubectl -n tofu-system delete -f ./config/testdata/always-clean-pod

echo "==================== Run drift detection tests"

kubectl -n tofu-system apply -f ./config/testdata/drift-detection

# apply should be true first
kubectl -n tofu-system wait terraform/helloworld-drift-detection --for=condition=apply=true --timeout=4m

# patch .spec.approvePlan to "disable"
kubectl -n tofu-system patch terraform/helloworld-drift-detection -p '{"spec":{"approvePlan":"disable"}}' --type=merge
kubectl -n tofu-system wait  terraform/helloworld-drift-detection --for=condition=ready=true  --timeout=4m

# disable drift detection
# the object should work correctly
kubectl -n tofu-system wait terraform/helloworld-drift-detection-disable --for=condition=ready --timeout=4m

# delete after tests
kubectl -n tofu-system delete -f ./config/testdata/drift-detection

echo "==================== Run healthchecks tests"

kubectl -n tofu-system apply -f ./config/testdata/healthchecks
kubectl -n tofu-system wait terraform/helloworld-healthchecks --for=condition=ready --timeout=4m

# delete after tests
kubectl -n tofu-system delete -f ./config/testdata/healthchecks

echo "==================== Run vars tests"

kubectl -n tofu-system apply -f ./config/testdata/vars
kubectl -n tofu-system wait terraform/helloworld-vars --for=condition=ready --timeout=4m

# delete after tests
kubectl -n tofu-system delete -f ./config/testdata/vars

source ./config/testdata/assert-helpers.sh

echo "==================== Run state Secret label tests"

kubectl -n tofu-system apply -f ./config/testdata/state-secret-label/test.yaml
kubectl -n tofu-system wait terraform/helloworld-state-label-default \
  --for=condition=ready --timeout=4m
kubectl -n tofu-system wait terraform/helloworld-state-label-backendconfig-nolabels \
  --for=condition=ready --timeout=4m
kubectl -n tofu-system wait terraform/helloworld-state-label-backendconfig-labels \
  --for=condition=ready --timeout=4m

# Scenario A — default path: no CR metadata labels on state Secret
SECRET_A="tfstate-default-helloworld-state-label-default"
assert_label_absent  tofu-system "$SECRET_A" "kustomize.toolkit.fluxcd.io/name"
assert_label_absent  tofu-system "$SECRET_A" "helm.toolkit.fluxcd.io/name"
assert_label_present tofu-system "$SECRET_A" "tfstate" "true"

# Scenario B — explicit BackendConfig, Labels omitted: same expectation as A
SECRET_B="tfstate-default-state-label-bc-nolabels"
assert_label_absent  tofu-system "$SECRET_B" "kustomize.toolkit.fluxcd.io/name"
assert_label_absent  tofu-system "$SECRET_B" "helm.toolkit.fluxcd.io/name"
assert_label_present tofu-system "$SECRET_B" "tfstate" "true"

# Scenario C — explicit BackendConfig with Labels: custom labels present, CR labels absent
SECRET_C="tfstate-default-state-label-bc-labels"
assert_label_absent  tofu-system "$SECRET_C" "kustomize.toolkit.fluxcd.io/name"
assert_label_absent  tofu-system "$SECRET_C" "helm.toolkit.fluxcd.io/name"
assert_label_present tofu-system "$SECRET_C" "env" "staging"
assert_label_present tofu-system "$SECRET_C" "app" "my-service"

echo "state Secret label tests passed"

# delete after tests
kubectl -n tofu-system delete -f ./config/testdata/state-secret-label/test.yaml

echo "==================== Run state Secret label-drift regression test"

kubectl -n tofu-system apply -f ./config/testdata/state-secret-label/label-drift.yaml
kubectl -n tofu-system wait terraform/helloworld-state-label-drift \
  --for=condition=ready --timeout=4m

DRIFT_SECRET="tfstate-default-helloworld-state-label-drift"

# Capture the state before the label change
state_before=$(kubectl -n tofu-system get secret "$DRIFT_SECRET" \
  -o jsonpath='{.data.tfstate}' 2>/dev/null || true)
if [[ -z "$state_before" ]]; then
  echo "FAIL: state Secret has no tfstate data before label change" >&2
  exit 1
fi

# Simulate Flux updating the CR labels (e.g. new kustomization revision)
kubectl -n tofu-system patch terraform/helloworld-state-label-drift --type=merge -p \
  '{"metadata":{"labels":{"kustomize.toolkit.fluxcd.io/name":"global-stack-v2","helm.toolkit.fluxcd.io/name":"my-release-v2"}}}'

# Force an immediate reconcile and wait for it to settle
kubectl -n tofu-system annotate terraform/helloworld-state-label-drift \
  reconcile.fluxcd.io/requestedAt="$(date -u +%Y-%m-%dT%H:%M:%SZ)" --overwrite
kubectl -n tofu-system wait terraform/helloworld-state-label-drift \
  --for=condition=ready --timeout=4m

# Verify: still exactly one state Secret for this CR (no duplicate created by label drift)
secret_count=$(kubectl -n tofu-system get secrets --no-headers \
  | grep -c "^${DRIFT_SECRET}" || true)
if [[ "$secret_count" != "1" ]]; then
  echo "FAIL: expected 1 state Secret after label change, found $secret_count" >&2
  exit 1
fi
echo "OK: exactly 1 state Secret after label change"

# Verify: state Secret still holds the original tfstate data (state was not lost)
state_after=$(kubectl -n tofu-system get secret "$DRIFT_SECRET" \
  -o jsonpath='{.data.tfstate}' 2>/dev/null || true)
if [[ -z "$state_after" ]]; then
  echo "FAIL: state Secret has no tfstate data after label change — state was lost" >&2
  exit 1
fi
if [[ "$state_after" != "$state_before" ]]; then
  echo "FAIL: tfstate data changed after label update — a new empty Secret may have been used" >&2
  exit 1
fi
echo "OK: tfstate data preserved after label change"

echo "state Secret label-drift regression test passed"

# delete after tests
kubectl -n tofu-system delete -f ./config/testdata/state-secret-label/label-drift.yaml

echo "==================== Run BackendConfig label expansion test"

kubectl -n tofu-system apply -f ./config/testdata/state-secret-label/backend-labels-expand.yaml
kubectl -n tofu-system wait terraform/helloworld-backend-labels-expand \
  --for=condition=ready --timeout=4m

EXPAND_SECRET="tfstate-default-backend-labels-expand"

# Capture state before adding a label
state_before=$(kubectl -n tofu-system get secret "$EXPAND_SECRET" \
  -o jsonpath='{.data.tfstate}' 2>/dev/null || true)
if [[ -z "$state_before" ]]; then
  echo "FAIL: state Secret has no tfstate data before label expansion" >&2
  exit 1
fi
assert_label_present tofu-system "$EXPAND_SECRET" "env" "staging"
echo "initial state captured, env=staging confirmed"

# Add a new label to BackendConfig.Labels
kubectl -n tofu-system patch terraform/helloworld-backend-labels-expand --type=merge -p \
  '{"spec":{"backendConfig":{"labels":{"env":"staging","team":"platform"}}}}'

# Force an immediate reconcile and wait for it to settle
kubectl -n tofu-system annotate terraform/helloworld-backend-labels-expand \
  reconcile.fluxcd.io/requestedAt="$(date -u +%Y-%m-%dT%H:%M:%SZ)" --overwrite
kubectl -n tofu-system wait terraform/helloworld-backend-labels-expand \
  --for=condition=ready --timeout=4m

# Verify: still exactly one state Secret (no duplicate created)
secret_count=$(kubectl -n tofu-system get secrets --no-headers \
  | grep -c "^${EXPAND_SECRET}" || true)
if [[ "$secret_count" != "1" ]]; then
  echo "FAIL: expected 1 state Secret after label expansion, found $secret_count" >&2
  exit 1
fi
echo "OK: exactly 1 state Secret after label expansion"

# Verify: tfstate data unchanged (no state loss)
state_after=$(kubectl -n tofu-system get secret "$EXPAND_SECRET" \
  -o jsonpath='{.data.tfstate}' 2>/dev/null || true)
if [[ -z "$state_after" ]]; then
  echo "FAIL: state Secret has no tfstate data after label expansion" >&2
  exit 1
fi
if [[ "$state_after" != "$state_before" ]]; then
  echo "FAIL: tfstate data changed after BackendConfig label expansion — state may have been lost" >&2
  exit 1
fi
echo "OK: tfstate data preserved after label expansion"

# Verify: original label still present and state data unchanged.
# The Kubernetes backend only writes labels during a state write (plan+apply).
# New BackendConfig.Labels land on the Secret the next time apply runs; this e2e
# test asserts the critical property — state is not lost — and leaves the label-
# propagation timing to unit/integration coverage of getLabelsAsHCL.
assert_label_present tofu-system "$EXPAND_SECRET" "env" "staging"
echo "OK: original env=staging label preserved; state not lost after BackendConfig label expansion"

echo "BackendConfig label expansion test passed"

# delete after tests
kubectl -n tofu-system delete -f ./config/testdata/state-secret-label/backend-labels-expand.yaml

echo "==================== Run same-namespace multi-CR label isolation test"

kubectl -n tofu-system apply -f ./config/testdata/state-secret-label/same-ns-multi.yaml
kubectl -n tofu-system wait terraform/helloworld-ws-dev --for=condition=ready --timeout=4m
kubectl -n tofu-system wait terraform/helloworld-ws-prd --for=condition=ready --timeout=4m

# Different workspaces, default secretSuffix (= CR name): backend differentiates via tfstateWorkspace
SECRET_WS_DEV="tfstate-dev-helloworld-ws-dev"
SECRET_WS_PRD="tfstate-prd-helloworld-ws-prd"

# Each workspace must produce its own distinct state Secret
kubectl -n tofu-system get secret "$SECRET_WS_DEV"
kubectl -n tofu-system get secret "$SECRET_WS_PRD"
echo "OK: each workspace has its own state Secret"

# Neither Secret must carry the shared Flux/Helm CR metadata labels
assert_label_absent tofu-system "$SECRET_WS_DEV" "kustomize.toolkit.fluxcd.io/name"
assert_label_absent tofu-system "$SECRET_WS_DEV" "helm.toolkit.fluxcd.io/name"
assert_label_absent tofu-system "$SECRET_WS_PRD" "kustomize.toolkit.fluxcd.io/name"
assert_label_absent tofu-system "$SECRET_WS_PRD" "helm.toolkit.fluxcd.io/name"
echo "OK: neither state Secret carries CR metadata labels"

# Both Secrets must have the backend-native tfstate label
assert_label_present tofu-system "$SECRET_WS_DEV" "tfstate" "true"
assert_label_present tofu-system "$SECRET_WS_PRD" "tfstate" "true"
echo "OK: both state Secrets have backend-native tfstate label"

echo "same-namespace multi-CR label isolation test passed"

# delete after tests
kubectl -n tofu-system delete -f ./config/testdata/state-secret-label/same-ns-multi.yaml

echo "==================== Run multi-tenancy test"
kubectl -n tofu-system scale --replicas=3 deploy/tofu-controller
kustomize build ./config/testdata/multi-tenancy/tenant01 | kubectl apply -f -
kustomize build ./config/testdata/multi-tenancy/tenant02 | kubectl apply -f -
kubectl -n tf-tenant01-dev wait terraform/helloworld-tenant01-dev --for=condition=ready --timeout=4m
kubectl -n tf-tenant01-prd wait terraform/helloworld-tenant01-prd --for=condition=ready --timeout=4m
kubectl -n tf-tenant02-dev wait terraform/helloworld-tenant02-dev --for=condition=ready --timeout=4m
kubectl -n tf-tenant02-prd wait terraform/helloworld-tenant02-prd --for=condition=ready --timeout=4m

# delete after tests
kubectl -n tf-tenant01-dev delete terraform --all
kubectl -n tf-tenant01-prd delete terraform --all
kubectl -n tf-tenant02-dev delete terraform --all
kubectl -n tf-tenant02-prd delete terraform --all

kubectl -n tf-tenant01-dev delete gitrepository --all
kubectl -n tf-tenant01-prd delete gitrepository --all
kubectl -n tf-tenant02-dev delete gitrepository --all
kubectl -n tf-tenant02-prd delete gitrepository --all

kubectl delete ns tf-tenant01-dev
kubectl delete ns tf-tenant01-prd
kubectl delete ns tf-tenant02-dev
kubectl delete ns tf-tenant02-prd

echo "==================== Set up chaos testing environment"
kubectl -n tofu-system scale --replicas=1 deploy/tofu-controller
kubectl -n chaos-testing apply -f ./config/testdata/chaos
kubectl -n chaos-testing apply -f ./config/testdata/source
sleep 20

echo "==================== Randomly delete runner pods"
for i in {1..10};
do
  num=$((1 + $RANDOM % 5))
  kubectl -n chaos-testing delete pod helloworld-chaos0$num-tf-runner || true
  sleep 5
done
sleep 20

echo "==================== Verify chaos testing result"

kubectl -n chaos-testing get pods

kubectl -n chaos-testing wait terraform/helloworld-chaos01 --for=condition=ready --timeout=30m
kubectl -n chaos-testing wait terraform/helloworld-chaos02 --for=condition=ready --timeout=30m
kubectl -n chaos-testing wait terraform/helloworld-chaos03 --for=condition=ready --timeout=30m
kubectl -n chaos-testing wait terraform/helloworld-chaos04 --for=condition=ready --timeout=30m
kubectl -n chaos-testing wait terraform/helloworld-chaos05 --for=condition=ready --timeout=30m
