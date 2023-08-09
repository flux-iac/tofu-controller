load('ext://restart_process', 'docker_build_with_restart')
load('ext://helm_remote', 'helm_remote')
load('ext://secret', 'secret_from_dict')
load('ext://namespace', 'namespace_create', 'namespace_inject')

namespace       = "flux-system"
tfNamespace     = "terraform"
buildSHA        = str(local('git rev-parse --short HEAD')).rstrip('\n')
buildVersionRef = str(local('git rev-list --tags --max-count=1')).rstrip('\n')
buildVersion    = str(local("git describe --tags ${buildVersionRef}")).rstrip('\n')

if os.path.exists('Tiltfile.local'):
   include('Tiltfile.local')

namespace_create(tfNamespace)

# Download chart deps
local_resource("helm-dep-update", "helm dep update charts/tf-controller", trigger_mode=TRIGGER_MODE_MANUAL, auto_init=True)

# Define resources
k8s_resource('chart-tf-controller',
  labels=["deployments"],
  new_name='controller')

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
  objects=['helloworld:GitRepository:terraform','helloworld-tf:Secret:terraform','helloworld-tf:Terraform:terraform'],
  extra_pod_selectors={'instance': 'helloworld-tf'},
  new_name="helloworld-tf",
  pod_readiness='ignore',
  labels=["resources"],
)

# Images
docker_build(
  "ghcr.io/weaveworks/tf-controller",
  "",
  dockerfile="Dockerfile",
  build_args={
    'BUILD_SHA': buildSHA,
    'BUILD_VERSION': buildVersion,
  })

# There are no resources using this image when tilt starts, but we still need
# this image.
update_settings(suppress_unused_image_warnings=["ghcr.io/weaveworks/tf-runner"])
docker_build(
  'ghcr.io/weaveworks/tf-runner',
  '',
  dockerfile='runner.Dockerfile',
  build_args={
    'BUILD_SHA': buildSHA,
    'BUILD_VERSION': buildVersion,
  })
