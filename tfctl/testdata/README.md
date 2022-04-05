Terraform will complain if the plan file was created using a different version of Terraform.

To re-generate the plan execute the following:

terraform plan -out plan && gzip plan
