load('ext://restart_process', 'docker_build_with_restart')
load('ext://helm_remote', 'helm_remote')
load('ext://secret', 'secret_from_dict')
load('ext://namespace', 'namespace_create', 'namespace_inject')

namespace        = "flux-system"
tfNamespace      = "terraform"
buildSHA         = str(local('git rev-parse --short HEAD')).rstrip('\n')
buildVersionRef  = str(local('git rev-list --tags --max-count=1')).rstrip('\n')
buildVersion     = str(local("git describe --tags ${buildVersionRef}")).rstrip('\n')
LIBCRYPTO_VERSION = "3.1.4-r5"

if os.path.exists('Tiltfile.local'):
   include('Tiltfile.local')

namespace_create(tfNamespace)

# Download chart deps
local_resource("helm-dep-update", "helm dep update charts/tf-controller", trigger_mode=TRIGGER_MODE_MANUAL, auto_init=True)

# Define resources
k8s_resource('chart-tf-controller',
  labels=["deployments"],
  new_name='controller')

k8s_resource('chart-tf-controller-branch-planner',
  labels=["deployments"],
  new_name='branch-planner')

helm_values = ['config/tilt/helm/dev-values.yaml']
if os.path.exists('config/tilt/helm/dev-values-local.yaml'):
   helm_values.append('config/tilt/helm/dev-values-local.yaml')

k8s_yaml(helm(
   "charts/tf-controller",
   namespace=namespace,
   values=helm_values,
))

# Add Example
k8s_yaml("./config/tilt/test/tf-dev-subject.yaml")
k8s_resource(
  objects=['helloworld:GitRepository:terraform','helloworld-tf:Secret:terraform'],
  extra_pod_selectors={'instance': 'helloworld-tf'},
  new_name="helloworld-tf-runner",
  pod_readiness='ignore',
  labels=["resources"],
)

# Add Secrets
if not os.getenv('GITHUB_TOKEN'):
   fail("You need to set GITHUB_TOKEN in your terminal before running this")

k8s_yaml(namespace_inject(secret_from_dict("bbp-token", inputs = {
    'token' : os.getenv('GITHUB_TOKEN')
}), namespace))

# Add configMap
k8s_yaml(namespace_inject("./config/tilt/configMap.yaml", namespace))

local_resource(
  'manager-compile',
  'CGO_ENABLED=0 GOOS=linux GOARCH=$(go env GOARCH) go build -o bin/tofu-controller ./cmd/manager',
  deps=[
    'api/',
    'tfctl/',
    'cmd/manager/',
    'controllers/',
    'mtls/',
    'runner/',
    'internal/',
    'utils/',
    'go.mod',
    'go.sum'
  ],
  labels = ['native-processes'],
)

# Images
docker_build(
  "ghcr.io/flux-iac/tofu-controller",
  "",
  dockerfile="Dockerfile.dev",
  build_args={
    'BUILD_SHA': buildSHA,
    'BUILD_VERSION': buildVersion,
    'LIBCRYPTO_VERSION': LIBCRYPTO_VERSION,
  }
)

local_resource(
  'branch-planner-compile',
  'CGO_ENABLED=0 GOOS=linux GOARCH=$(go env GOARCH) go build -o bin/branch-planner ./cmd/branch-planner',
  deps=[
    'api/',
    'tfctl/',
    'cmd/branch-planner/',
    'internal/',
    'utils/',
    'go.mod',
    'go.sum'
  ],
  labels = ['native-processes'],
)

docker_build(
  "ghcr.io/flux-iac/branch-planner",
  "",
  dockerfile="planner.Dockerfile.dev",
  build_args={
    'BUILD_SHA': buildSHA,
    'BUILD_VERSION': buildVersion,
    'LIBCRYPTO_VERSION': LIBCRYPTO_VERSION,
  }
)


local_resource(
  'runner-compile',
  'CGO_ENABLED=0 GOOS=linux GOARCH=$(go env GOARCH) go build -o bin/tf-runner ./cmd/runner/main.go',
  deps=[
    'api/',
    'tfctl/',
    'cmd/runner',
    'controllers/',
    'mtls/',
    'runner/',
    'internal/',
    'utils/',
    'go.mod',
    'go.sum'
  ],
  labels = ['native-processes'],
)
k8s_kind('Terraform', image_json_path='{.spec.runnerPodTemplate.spec.image}')
docker_build(
  'ghcr.io/flux-iac/tf-runner',
  '',
  dockerfile='runner.Dockerfile.dev',
  build_args={
    'LIBCRYPTO_VERSION': LIBCRYPTO_VERSION,
  }
)
