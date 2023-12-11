#!/usr/bin/env bash

VERSION=$1

yq e -i ".version=\"${VERSION#v}\"" charts/tf-controller/Chart.yaml
yq e -i ".appVersion=\"$VERSION\"" charts/tf-controller/Chart.yaml
yq e -i ".image.tag=\"$VERSION\"" charts/tf-controller/values.yaml
yq e -i ".runner.image.tag=\"$VERSION\"" charts/tf-controller/values.yaml
yq e -i ".images[0].newTag=\"$VERSION\"" config/manager/kustomization.yaml
yq e -i ".images[0].newTag=\"$VERSION\"" config/branch-planner/kustomization.yaml

# bump version in docs/release.yaml

# Without Branch Planner
yq e -i "(select(di == 1).spec.chart.spec.version) = \">=${VERSION#v}\"" docs/release.yaml
yq e -i "(select(di == 1).spec.values.image.tag) = \"$VERSION\"" docs/release.yaml
yq e -i "(select(di == 1).spec.values.runner.image.tag) = \"$VERSION\"" docs/release.yaml

# With Branch Planner
yq e -i "(select(di == 1).spec.chart.spec.version) = \">=${VERSION#v}\"" docs/branch-planner/release.yaml
yq e -i "(select(di == 1).spec.values.image.tag) = \"$VERSION\"" docs/branch-planner/release.yaml
yq e -i "(select(di == 1).spec.values.runner.image.tag) = \"$VERSION\"" docs/branch-planner/release.yaml
yq e -i "(select(di == 1).spec.values.branchPlanner.image.tag) = \"$VERSION\"" docs/branch-planner/release.yaml
