# Binary Vulnerability Management

This document describes the security scanning approach for tofu-controller runner images that support both OpenTofu and Terraform binaries.

## Overview

Starting with version v0.16.0, tofu-controller provides separate runner image variants:
- **OpenTofu variant** (default): Contains only the OpenTofu binary
- **Terraform variant**: Contains only the Terraform binary for backward compatibility

This separation allows for proper security scanning without false positives.

## Image Variants

### OpenTofu Images (Default)

**Images**:
- `ghcr.io/flux-iac/tf-runner:latest`
- `ghcr.io/flux-iac/tf-runner:v{VERSION}`
- `ghcr.io/flux-iac/tf-runner-azure:latest`
- `ghcr.io/flux-iac/tf-runner-azure:v{VERSION}`

**Binary**: `/usr/local/bin/tofu` (OpenTofu v1.11.2)

**Security Posture**:
- ✅ No CVE exceptions for Terraform (binary not present)
- ✅ All security scans must pass without skip rules
- ✅ Clean security scan results
- ✅ Recommended for all new deployments

**Scanning Configuration**:
```yaml
- name: Run Trivy vulnerability scanner on runner image (OpenTofu)
  uses: aquasecurity/trivy-action@...
  with:
    image-ref: 'ghcr.io/flux-iac/tf-runner:latest'
    format: 'table'
    exit-code: '1'
    ignore-unfixed: true
    vuln-type: 'os,library'
    severity: 'CRITICAL,HIGH'
    # NO skip-files - scan must be clean
```

### Terraform Images (Legacy)

**Images**:
- `ghcr.io/flux-iac/tf-runner:latest-terraform`
- `ghcr.io/flux-iac/tf-runner:v{VERSION}-terraform`
- `ghcr.io/flux-iac/tf-runner-azure:latest-terraform`
- `ghcr.io/flux-iac/tf-runner-azure:v{VERSION}-terraform`

**Binary**: `/usr/local/bin/terraform` (Terraform v1.14.3)

**Security Posture**:
- ⚠️ Skip rules applied for known Terraform CVEs
- ⚠️ Provided for backward compatibility only
- ⚠️ Scheduled for deprecation in future release
- ⚠️ Use only if OpenTofu is not an option

**Scanning Configuration**:
```yaml
- name: Run Trivy vulnerability scanner on runner image (Terraform)
  uses: aquasecurity/trivy-action@...
  with:
    image-ref: 'ghcr.io/flux-iac/tf-runner:latest-terraform'
    format: 'table'
    exit-code: '1'
    ignore-unfixed: true
    vuln-type: 'os,library'
    severity: 'CRITICAL,HIGH'
    skip-files: '/usr/local/bin/terraform' # Skip Terraform binary CVEs
```

## Security Scanning Process

### Automated Scanning

Security scans run automatically in CI/CD via GitHub Actions:

1. **On Every Build** (`.github/workflows/build-and-publish.yaml`):
   - Not currently included, but can be added

2. **On Schedule** (`.github/workflows/scan.yaml`):
   - Weekly scheduled scans using Trivy
   - Separate scan jobs for each image variant
   - Exit code 1 on HIGH or CRITICAL vulnerabilities

3. **On Release** (`.github/workflows/release.yaml`):
   - Images scanned before signing
   - Both variants must pass their respective scans

### Manual Scanning

To scan images manually:

```bash
# Scan OpenTofu image (should be clean)
trivy image ghcr.io/flux-iac/tf-runner:latest

# Scan Terraform image (will show Terraform CVEs)
trivy image ghcr.io/flux-iac/tf-runner:latest-terraform
```

## Known Vulnerabilities

### Terraform Binary CVEs

The Terraform binary has known CVEs that are accepted for the `-terraform` tagged images:

- **CVE-YYYY-XXXX**: Description (if applicable)
- These CVEs are skipped only for Terraform variant images
- OpenTofu images do not have these CVEs (different binary)

### Exclusion Rationale

**Why we skip Terraform binary CVEs**:
1. Terraform binary is provided "as-is" from HashiCorp
2. We cannot patch the binary ourselves
3. Users requiring Terraform accept these known risks
4. Alternative (OpenTofu) is available and recommended
5. Terraform support is temporary for backward compatibility

**Why we don't skip OpenTofu binary CVEs**:
1. OpenTofu is the default and recommended binary
2. All CVEs must be addressed promptly
3. No exceptions allowed for the default path
4. Ensures highest security posture for new users

## External Security Policies

### Security Office Integration

For organizations with external security scanning:

**OpenTofu Images**:
- No exceptions needed in external security policies
- Should pass all organizational security gates
- No binary-specific CVE allowlists required

**Terraform Images**:
- May require exceptions in external security policies
- Exception scope: Only `/usr/local/bin/terraform` binary
- Exception tags: Images tagged with `-terraform` suffix only
- Exception rationale: Documented in this file

### Example Exception Policy

For security teams managing allowlists:

```yaml
# Example policy for Terraform variant images
exceptions:
  - image_pattern: "ghcr.io/flux-iac/tf-runner:*-terraform"
    binary_path: "/usr/local/bin/terraform"
    reason: "Legacy Terraform support for backward compatibility"
    expiration: "2026-12-31"  # Review annually

  - image_pattern: "ghcr.io/flux-iac/tf-runner-azure:*-terraform"
    binary_path: "/usr/local/bin/terraform"
    reason: "Legacy Terraform support for backward compatibility"
    expiration: "2026-12-31"  # Review annually
```

## Migration Guidance

### From Terraform to OpenTofu

Organizations using Terraform variant images should migrate to OpenTofu:

1. **Test in non-production**:
   ```yaml
   runner:
     image:
       tag: "v0.16.0-rc.7"  # OpenTofu variant (no suffix)
   ```

2. **Verify compatibility**:
   - Test all Terraform configurations
   - Check for any OpenTofu-specific behavior
   - Review state file compatibility

3. **Update production**:
   - Update Helm values to remove `-terraform` suffix
   - Monitor for issues
   - Document any required changes

4. **Benefits**:
   - ✅ No security scan exceptions needed
   - ✅ Clean vulnerability reports
   - ✅ Better security posture
   - ✅ Future-proof (Terraform support will be deprecated)

### Deprecation Timeline

**Terraform support deprecation plan**:

| Phase | Timeline | Status |
|-------|----------|--------|
| Introduce dual binary support | v0.16.0 | ✅ Complete |
| Encourage OpenTofu migration | v0.16.0 - v0.18.0 | 🔄 Current |
| Announce Terraform deprecation | v0.18.0 | 📅 Planned |
| Stop building Terraform variants | v0.20.0+ | 📅 Future |

## Compliance

### SOC 2 / ISO 27001

For organizations requiring compliance certifications:

- **OpenTofu images**: Fully compliant, no exceptions
- **Terraform images**: May require documented risk acceptance
- **Recommendation**: Use OpenTofu variant for compliance-critical deployments

### Risk Assessment

| Image Variant | Security Risk | Operational Risk | Recommendation |
|--------------|---------------|------------------|----------------|
| OpenTofu (default) | ✅ Low | ✅ Low | ✅ Use for all new deployments |
| Terraform (-terraform) | ⚠️ Medium | ✅ Low | ⚠️ Use only for migration period |

## Contact

For security concerns or questions:
- GitHub Issues: https://github.com/flux-iac/tofu-controller/issues
- Security Email: security@flux-iac.io (if available)

## References

- [Trivy Documentation](https://aquasecurity.github.io/trivy/)
- [OpenTofu Releases](https://github.com/opentofu/opentofu/releases)
- [Terraform Releases](https://github.com/hashicorp/terraform/releases)
- [CVE Database](https://cve.mitre.org/)
