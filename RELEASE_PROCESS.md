# Release Management Process

This markdown file documents the current release process for the Tofu Controller, and it's associated components (tf-runner, tfctl, etc). This process is identical for release candidates and for stable releases.

If any step is unclear, please reach out to an existing [maintainer](MAINTAINERS).

1. Create a new release branch

    ```console
    git switch -c release/v0.16.0
    ```

2. Bump statically defined versions across the repository.

    ```console
    bash ./tools/bump-version.sh v0.16.0
    ```

    At this point, it is worthwhile performing a cursory review of the repository to ensure there are no missed version references.

3. Update the `CHANGELOG.md` file. At the moment, this is done manually but you can generate a helper template with:

    ```console
    git log --first-parent v0.16.0-rc.8..v0.16.0 --merges --pretty='- %s ([%h](https://github.com/flux-iac/tofu-controller/commit/%H))'
    ```

4. Arrange to have your release branch merged and commited to the `main` branch.

5. Once merged, create and push a release tag.

   This will kick off the release workflow, which will build, sign and publish the various docker images.

   ```console
    git tag -a v0.16.0 -m "v0.16.0"
    git push origin v0.16.0
   ```

6. At this point, monitor the release workflow and ensure that no steps fail.

7. Once the release workflow has finished, you can then publish the Helm Chart.

    To do this, manually run the `helm-release` workflow. You can validate that the Helm Chart has been released by:

    ```console
    $ helm repo add tofu-controller https://flux-iac.github.io/tofu-controller
    "tofu-controller" has been added to your repositories

    $ helm repo update tofu-controller
    Hang tight while we grab the latest from your chart repositories...
    ...Successfully got an update from the "tofu-controller" chart repository
    Update Complete. ⎈Happy Helming!⎈

    $ helm search repo tofu-controller -l --version 0.16.0
    NAME                           	CHART VERSION	APP VERSION 	DESCRIPTION
    tofu-controller/tofu-controller	0.16.0  	v0.16.0	The Helm chart for Weave GitOps Terraform Contr...
    ```
