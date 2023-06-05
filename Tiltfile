local('kubectl apply --server-side -k config/tilt/base')

k8s_yaml(kustomize('config/tilt/manager'))

docker_build(
    'weaveworks/tf-controller',
    context='.',
    dockerfile='Dockerfile',
)

docker_build(
    'weaveworks/branch-based-planner',
    context='.',
    dockerfile='planner.Dockerfile',
)

custom_build(
    'localhost:5000/weaveworks/tf-runner',
    'make docker-dev-runner RUNNER_IMG=localhost:5000/weaveworks/tf-runner TAG=$EXPECTED_TAG',
    deps=['runner/', 'runner.Dockerfile'],
)

### this is a group of resources that are deployed together
k8s_yaml(
    'config/tilt/test/tf-dev-subject.yaml',
)
k8s_kind('Terraform', image_json_path='{.spec.runnerPodTemplate.spec.image}', pod_readiness='ignore')

k8s_resource(
  objects=['helloworld:GitRepository:flux-system','helloworld-tf:Secret:flux-system'],
  workload='helloworld-tf',
  extra_pod_selectors={'instance': 'helloworld-tf'},
  pod_readiness='ignore',
)
