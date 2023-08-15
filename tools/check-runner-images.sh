#!/bin/bash

# Define the base image name and tag prefix
IMAGE_NAME="ghcr.io/weaveworks/tf-runner"
VERSION="${1}" # Assuming the desired version is passed as an argument to this script

# Versions of Terraform to check
TF_VERSIONS=(1.0.11 1.1.9 1.2.9 1.3.9 1.4.6 1.5.5)

# Loop over each Terraform version
for TF_VERSION in "${TF_VERSIONS[@]}"; do
    IMAGE_TAG="${VERSION}-tf-${TF_VERSION}"
    FULL_IMAGE_NAME="${IMAGE_NAME}:${IMAGE_TAG}"

    echo "Checking image ${FULL_IMAGE_NAME}..."

    # Pull the Docker image
    docker pull "${FULL_IMAGE_NAME}"

    # Check the Terraform version inside the Docker image
    ACTUAL_TF_VERSION=$(docker run --entrypoint=/bin/sh --rm "${FULL_IMAGE_NAME}" -c "terraform version" | head -n 1)

    # Verify if the version inside the Docker image matches the expected version
    if [[ "${ACTUAL_TF_VERSION}" == *"Terraform v${TF_VERSION}"* ]]; then
        echo "Image ${FULL_IMAGE_NAME} has the correct Terraform version: ${TF_VERSION}."
    else
        echo "ERROR: Image ${FULL_IMAGE_NAME} has Terraform version ${ACTUAL_TF_VERSION}, expected ${TF_VERSION}."
    fi
done
