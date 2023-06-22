local('kubectl apply --server-side -k config/tilt/base')

k8s_yaml(kustomize('config/tilt/manager'))
k8s_yaml(kustomize('config/tilt/bbp'))

docker_build(
    'weaveworks/tf-controller',
    context='.',
    dockerfile='Dockerfile',
)

docker_build(
    'weaveworks/tf-runner',
    context='.',
    dockerfile='runner.Dockerfile',
)

docker_build(
    'weaveworks/branch-based-planner',
    context='.',
    dockerfile='planner.Dockerfile',
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
