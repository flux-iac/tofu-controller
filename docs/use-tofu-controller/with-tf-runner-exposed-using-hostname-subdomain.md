# Use tofu-controller with Terraform Runners **exposed via hostname/subdomain** 

Tofu-controller uses the Controller/Runner architecture. The Controller acts as a client, and talks to each Runner's Pod via gRPC over port 30000.

Tofu-controller must thus be able to reliably connect to each Runner's pod regardless of the cluster network topology.

## The Default Runner DNS resolution

By default, tofu-controller fetches the Runner's pod IP address after it is instantiated (e.g. `1.2.3.4`).

It then transforms the IP address into its `IP-based pod DNS A record` (e.g. `1.2.3.4.<namespace>.pod.<cluster-domain>`) which is used to connect to the Runner pod using gRPC protocol.

In standard Kubernetes cluster deployment, `IP-based pod DNS resolution` is usually provided by [Coredns](https://coredns.io/) and especially the `pods` option of the [Kubernetes plugin](https://coredns.io/plugins/kubernetes/#syntax).

```
cluster.local {
    kubernetes {
        pods verified
    }
}
```

IMPORTANT: The gRPC communication between tofu-controller and Runner's pod is secured with mTLS. tofu-controller generates a valid wildcard TLS certificate for `*.<namespace>.pod.<cluster-domain>` hosts on the Runner's namespace. The Runner's pod present this certificate during TLS handshake with tofu-controller. 

## Hostname/Subdomain Runner DNS resolution

The default configuration described above works for standard Kubernetes deployments. It does not work however when the cluster DNS provider do not support `IP-based pod DNS resolution`. This is the case for `GCP Cloud DNS` for example.

For such setup, you can switch the DNS resolution mode to [Hostname/Subdomain](https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/#pod-s-hostname-and-subdomain-fields). Enabling this option will :

- Create a `Headless service` named `tf-runner` in each allowed namespace

```yaml hl_lines="4-5,8-10"
apiVersion: v1
kind: Service
metadata:
  name: tf-runner
  namespace: hello-world
spec:
  clusterIP: None
  ports:
  - name: grpc
    port: 30000
  selector:
    app.kubernetes.io/created-by: tofu-controller
    app.kubernetes.io/name: tf-runner
```

- Set Runner's pod spec with `hostname: <terraform_object_name>` and `subdomain: tf-runner`

```yaml hl_lines="12-13"
apiVersion: v1
kind: Pod
  labels:
    app.kubernetes.io/created-by: tofu-controller
    app.kubernetes.io/instance: tf-runner-3ac83e0f
    app.kubernetes.io/name: tf-runner
    infra.contrib.fluxcd.io/terraform: hello-world
    tf.weave.works/tls-secret-name: terraform-runner.tls-1693866794
  name: helloworld-tf-runner
  namespace: hello-world
spec:
  hostname: helloworld
  subdomain: tf-runner
  containers:
  - args:
    - --grpc-port
    - "30000"
    - --tls-secret-name
    - terraform-runner.tls-1693866794
    - --grpc-max-message-size
    - "4"
    image: ghcr.io/flux-iac/tf-runner:v0.16.0-rc.4
    name: tf-runner
    ports:
    - containerPort: 30000
      name: grpc
    resources:
      limits:
        cpu: 500m
        ephemeral-storage: 1Gi
        memory: 2Gi
    securityContext:
      allowPrivilegeEscalation: false
      capabilities:
        drop:
        - ALL
      readOnlyRootFilesystem: true
      runAsNonRoot: true
      runAsUser: 65532
      seccompProfile:
        type: RuntimeDefault
  preemptionPolicy: PreemptLowerPriority
  priority: 0
  schedulerName: gke.io/optimize-utilization-scheduler
  securityContext:
    seccompProfile:
      type: RuntimeDefault
  serviceAccountName: tf-runner
```

The Runner's pod can then be targeted by tofu-controller using `<terraform_object_name>.tf-runner.<namespace>.svc.<cluster-domain> (helloworld.tf-runner.hello-world.svc.cluster.local)` as per Kubernetes specification instead of `IP-based pod DNS resolution`.

The switch is performed by setting the following _Helm value_ `usePodSubdomainResolution: true` or running directly tofu-controller with the option `--use-pod-subdomain-resolution=true`

IMPORTANT: The gRPC communication between tofu-controller and Runner's pod is secured with mTLS. tofu-controller generates a valid wildcard TLS certificate for `*.<namespace>.pod.<cluster-domain>` and `*.tf-runner.<namespace>.svc.<cluster-domain>` hosts on the Runner's namespace. The Runner's pod present this certificate during TLS handshake with tofu-controller. 
