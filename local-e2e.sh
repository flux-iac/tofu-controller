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
