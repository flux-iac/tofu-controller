# Use tofu-controller with IPv6 addresses.

Tofu-controller uses pod IPv4 address to generate the in-cluster hostname to
communicate with the runner instance. This logic fails when the runner pod has
IPv6 address instead of IPv4.

The tofu-controller has a flag to use pod subdomain resolution instead of an IP
address, with that enabled the controller will use cluster subdomains, and it
works with IPv6 addresses as the resolution is happening at cluster level.

To enable this feature, you can set `usePodSubdomainResolution` to `true` in the
Helm values file:

```yaml
usePodSubdomainResolution: true
```
