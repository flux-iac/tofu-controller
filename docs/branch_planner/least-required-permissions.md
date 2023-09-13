# Least Required Permissions For Git Providers

## GitHub

### Fine-Grained Personal Access Token

For public repositories, it's sufficient to enable `Public Repositories`, without
any additional permissions.

For private repositories the following permissions are required:

* `Pull requests` with Read-Write access. This is required to check Pull Request
  changes, list comments, and create or update comments.
* `Metadata` with Read-only access. This is automatically marked as "mandatory"
  because of the permissions listed above.
