# make docker-build
make release-manifests
yq -i e '.spec.template.spec.containers[0].image="ghcr.io/weaveworks/tf-controller:latest"' ./config/release/tf-controller.deployment.yaml

kind create cluster

flux install

kubectl apply -f ./config/release/tf-controller.crds.yaml
kubectl apply -f ./config/release/tf-controller.rbac.yaml
kubectl apply -f ./config/release/tf-controller.deployment.yaml

NS="flux-system"
kubectl -n $NS apply -f ./config/testdata/source
kubectl -n $NS wait gitrepository/helloworld --for=condition=ready --timeout=4m

kubectl -n $NS apply -f ./config/testdata/approve-plan
kubectl -n $NS wait terraform/helloworld-auto-approve --for=condition=ready --timeout=4m
kubectl -n $NS wait terraform/helloworld-manual-approve --for=condition=plan=true --timeout=4m
