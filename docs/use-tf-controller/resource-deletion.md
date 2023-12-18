# Resource Deletion Dependencies in Terraform Controller

This document discusses potential difficulties you may encounter when deleting Terraform resources 
through the Terraform Controller and the necessary components to facilitate a smooth deletion process.

## Source Object

The source object (e.g., GitRepository or OCIRepository) is a critical component of the Terraform resource deletion process. 
This object houses the Terraform source files (.tf files) that describe the configuration of the infrastructure resources.

During the deletion process, the Terraform Controller uses these source files to conduct a re-planning operation. 
This operation is instrumental to deleting the Terraform Custom Resource (CR).

However, if the source object is unavailable or has been deleted, the re-planning operation fails. 
As a result, the Terraform Controller cannot locate the resource state, 
leading to an infinite deletion attempt cycle, commonly known as a looping process.

If the source object was deleted and the deletion of a Terraform resource is
stuck, clearing the `finalizers` let the system delete the resource skipping
the finalizer of the Terraform Controller.

1. Suspend the `kustomization`:
   ```
   flux suspend kustomization tf-stk
   ```
2. Patch the Terraform resource:
   ```
   kubectl patch terraforms.infra.contrib.fluxcd.io \
     -n stk helloworld \
     -p '{"metadata":{"finalizers":null}}' \
     --type=merge
   ```

As it skips the finalizer of the Terraform controller, any cleanup the
controller would do will be skipped too.

## Role Bindings

Role bindings assign permissions to Terraform runners, allowing them to execute operations within the Kubernetes cluster.
These bindings define the actions that the Terraform runners are authorized to carry out.

If role bindings are missing or misconfigured, 
the Terraform runners may lack the necessary permissions to execute the deletion process, causing the process to fail.

## Secrets and ConfigMaps

Before initiating the resource deletion process, 
the Terraform Controller leverages Secrets and ConfigMaps to generate a complete source before planning. 
Secrets store confidential data like API keys or passwords, while ConfigMaps hold configuration data in a key-value format.

Should any of these components be missing or misconfigured, the Terraform Controller may fail to generate an accurate deletion plan, 
which could impede the resource deletion process.

## Troubleshooting

To prevent the aforementioned issues, ensure the availability and proper configuration of the source object, 
role bindings, and Secrets and ConfigMaps during the deletion process.

As of now, we are actively working to address these limitations in the Terraform Controller. 
We appreciate your patience and welcome any feedback to help enhance the Terraform Controller's performance.
