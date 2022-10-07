# Weave GitOps Terraform Controller

![Version: 0.8.1](https://img.shields.io/badge/Version-0.8.1-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v0.13.0-rc.2](https://img.shields.io/badge/AppVersion-v0.13.0--rc.2-informational?style=flat-square)

The Helm chart for Weave GitOps Terraform Controller

## Installation

Before using TF-controller, you have to install Flux by using either `flux install` or `flux bootstrap` command.
After that you can install TF-controller manually with Helm by:

```shell
# Add tf-controller helm repository
helm repo add tf-controller https://weaveworks.github.io/tf-controller/

# Install tf-controller
helm upgrade -i tf-controller tf-controller/tf-controller \
    --namespace flux-system
```

## Configuration

The following table lists the configurable parameters of the TF-controller chart and their default values.

__Note__: If you need to use the `imagePullSecrets` it would be best to set `serviceAccount.create: true` and `runner.serviceAccount.create: true`

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` | Affinity properties for the TF-Controller deployment |
| caCertValidityDuration | string | `"168h0m"` | Argument for `--ca-cert-validity-duration` (Controller) |
| certRotationCheckFrequency | string | `"30m0s"` | Argument for `--cert-rotation-check-frequency` (Controller) |
| certValidityDuration | string | `"6h0m"` | Argument for `--cert-validity-duration` (Controller) |
| concurrency | int | `24` | Concurrency of the controller (Controller) |
| eksSecurityGroupPolicy | object | `{"create":false,"ids":[]}` | Create an AWS EKS Security Group Policy with the supplied Security Group IDs [See](https://docs.aws.amazon.com/eks/latest/userguide/security-groups-for-pods.html#deploy-securitygrouppolicy) |
| eksSecurityGroupPolicy.create | bool | `false` | Create the EKS SecurityGroupPolicy |
| eksSecurityGroupPolicy.ids | list | `[]` | List of AWS Security Group IDs |
| eventsAddress | string | `"http://notification-controller.flux-system.svc.cluster.local./"` | Argument for `--events-addr` (Controller). The event address, default to the address of the Notification Controller |
| extraEnv | object | `{}` | Additional container environment variables. |
| fullnameOverride | string | `""` | Provide a fullname |
| image.pullPolicy | string | `"IfNotPresent"` | Controller image pull policy |
| image.repository | string | `"ghcr.io/weaveworks/tf-controller"` | Controller image repository |
| image.tag | string | `.Chart.AppVersion` | Overrides the image tag whose default is the chart appVersion. |
| imagePullSecrets | list | `[]` | Controller image pull secret |
| installCRDs | bool | `true` | If `true`, install CRDs as part of the helm installation |
| logLevel | string | `"info"` | Level of logging of the controller (Controller) |
| nameOverride | string | `""` | Provide a name |
| nodeSelector | object | `{}` | Node Selector properties for the TF-Controller deployment |
| podAnnotations | object | `{}` | Additional pod annotations |
| podLabels | object | `{}` | Additional pod labels |
| podSecurityContext | object | `{"fsGroup":1337}` | Pod-level security context |
| priorityClassName | string | `""` | PriorityClassName property for the TF-Controller deployment |
| rbac.create | bool | `true` | If `true`, create and use RBAC resources |
| replicaCount | int | `1` | Number of TF-Controller pods to deploy, more than one is not desirable. |
| resources | object | `{"limits":{"cpu":"1000m","memory":"1Gi"},"requests":{"cpu":"200m","memory":"64Mi"}}` | Resource limits and requests |
| runner | object | `{"creationTimeout":"5m0s","grpc":{"maxMessageSize":4},"image":{"repository":"ghcr.io/weaveworks/tf-runner","tag":"v0.13.0-rc.2"},"serviceAccount":{"annotations":{},"create":true,"name":""}}` | Runner-specific configurations |
| runner.creationTimeout | string | `"5m0s"` | Timeout for runner-creation (Controller) |
| runner.grpc.maxMessageSize | int | `4` | Maximum GRPC message size (Controller) |
| runner.image.repository | string | `"ghcr.io/weaveworks/tf-runner"` | Runner image repository |
| runner.image.tag | string | `.Chart.AppVersion` | Runner image tag |
| runner.serviceAccount.annotations | object | `{}` | Additional runner service Account annotations |
| runner.serviceAccount.create | bool | `true` | If `true`, create a new runner service account |
| runner.serviceAccount.name | string | `""` | Runner service account to be used |
| securityContext | object | `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"readOnlyRootFilesystem":true,"runAsNonRoot":true,"runAsUser":65532,"seccompProfile":{"type":"RuntimeDefault"}}` | Container-level security context |
| serviceAccount.annotations | object | `{}` | Additional Service Account annotations |
| serviceAccount.create | bool | `true` | If `true`, create a new service account |
| serviceAccount.name | string | tf-controller | Service account to be used |
| tolerations | list | `[]` | Tolerations properties for the TF-Controller deployment |
| volumeMounts | list | `[]` | Volume mounts properties for the TF-Controller deployment |
| volumes | list | `[]` | Volumes properties for the TF-Controller deployment |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.11.0](https://github.com/norwoodj/helm-docs/releases/v1.11.0)