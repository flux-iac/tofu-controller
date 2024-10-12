# 3. Workspace BLOB Caching

* Status: [ **proposed** | rejected | accepted | deprecated ]
* Date: 2023-09-20 
* Authors: @chanwit
* Deciders: TBD 

## Context

The Tofu-Controller currently faces challenges related to the deletion of Terraform resources.
These problems span across three categories:

1. Single object deletion,
2. Resources with dependencies deletion, and
3. Namespace deletion.

These problems must be fixed in the above order as (2) and (3) require single object deletion to be resolved first.

Deleting a single TF object can sometimes be obstructed because it's tied to other resources like Source objects, Secrets, and ConfigMaps. If we try to remove it without deleting these resources, the TF object gets stuck in an inconsistent state, making it harder for users to manage their infrastructure smoothly.
Therefore, the Tofu-Controller is being enhanced to address this problem more efficiently, using the contents of generated Workspace BLOBs. Each BLOB contains all necessary information from the associated Source, Secrets, and ConfigMaps to ensure that Tofu-Controller finalization procedures can delete objects correctly.

Currently, the Tofu-Controller downloads a Source BLOB and pushes it to a tf-runner. The tf-runner processes this BLOB to create a Workspace file system. It generates a backend configuration file, variable files, and other necessary files for the Workspace file system, using data from associated Secrets and ConfigMaps. This newly created Workspace file system is then compressed, sent back to the Tofu-Controller, and stored as a Workspace BLOB in the controller's storage.
A caching mechanism for these BLOBs is essential to fixing the single TF object deletion process.

## Decision

1. **BLOB Creation and Storage**
   * A gRPC function named `CreateWorkspaceBlob` will be invoked by the Tofu-Controller 
     to tell tf-runner to compress the Workspace file system into a tar.gz BLOB, which is then retrieved back to the controller.
   * The caching mechanism will be executed right before the Terraform Initialization step, ensuring that the latest and most relevant data is used.
   * Each Workspace Blob will be cached on the Tofu-Controller's local disk, using the UUID of the Terraform object as the filename,`${uuid}.tar.gz`.
   * To reduce the risk of unauthorized access to the cache entries, and cache collisions, the cache file will be deleted after the finalization process is complete.
2. **Persistence** 
   * [The persistence mechanism used by the Source Controller](https://fluxcd.io/flux/installation/configuration/vertical-scaling/#persistent-storage-for-flux-internal-artifacts) will be adopted for the Tofu-Controller's persistence volume.
3. **BLOB Encryption**
   * The encryption and decryption of the BLOBs will be tasked to the runner, with the controller solely responsible for storing encrypted BLOBs.
   * Each namespace will require a service account, preferably named "tf-runner".
   * The token of this service account, which is natively supported by Kubernetes, will serve as the most appropriate encryption key because it's stored in a Secret, access to which can be controlled by RBAC. Storing it in a Secret also allows the key to be rotated.
4. **Security Measures (Based on STRIDE Analysis)**
   * **Spoofing:** Implement Kubernetes RBAC for access restrictions and use mutual authentication for gRPC communications.
   * **Tampering:** Use checksums for integrity verification and 0600 permissions to write-protect local disk storage.
   * **Repudiation:** Ensure strong logging and auditing mechanisms for tracking activities.
   * **Information Disclosure:** Utilize robust encryption algorithms, rotate encryption keys periodically, and secure service account tokens.
   * **Denial of Service:** Monitor storage space and automate cleanup processes.
   * **Elevation of Privilege:** Minimize permissions associated with service account tokens.
5. **First MVP & Future Planning**
   * For the initial MVP, the default pod local volume will be used.
   * Since a controller restart will erase the BLOB cache, consideration for using persistent volumes should be made for subsequent versions.

## Consequence

1. With the implementation of this architecture:
   * Single object deletions will succeed in circumstances in which they previously got stuck.
   * Security measures will ensure the safety of the new Workspace BLOB storage mechanics, minimizing potential risks.
2. Using the default pod local volume might limit storage capabilities and risk data loss upon controller restart. This warrants the need for considering persistent volumes in future versions.
3. Encryption and security measures will demand regular maintenance and monitoring, especially concerning key rotations and integrity checks.
