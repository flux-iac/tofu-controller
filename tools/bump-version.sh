#!/usr/bin/env bash

VERSION=$1

yq e -i ".appVersion=\"$VERSION\"" charts/tf-controller/Chart.yaml
yq e -i ".image.tag=\"$VERSION\"" charts/tf-controller/values.yaml
yq e -i ".runner.image.tag=\"$VERSION\"" charts/tf-controller/values.yaml
yq e -i ".images[0].newTag=\"$VERSION\"" config/manager/kustomization.yaml
yq e -i ".images[0].newTag=\"$VERSION\"" config/bbp/kustomization.yaml
