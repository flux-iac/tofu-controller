# Least Required Permissions For Git Providers

## GitHub

### Fine-Grained Personal Access Token

For public repositories, it's sufficient to enable `Public Repositories`, without
any additional permissions.

For private repositories the following permissions are required:

* `Issues` with Read and Write access. This is required to list and read
  comments for commands, and to create comments with the Plan output.
* `Pull requests` with Read-Only access. This is required to check Pull Request
  changes.
* `Metadata` with Read-only access. This is automatically marked as "mandatory"
  because of the permissions listed above.
