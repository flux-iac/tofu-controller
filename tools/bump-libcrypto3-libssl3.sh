#!/usr/bin/bash

files=(
  Makefile
  Tiltfile
  .github/workflows/release-runners.yaml
  .github/workflows/release.yaml
  .github/workflows/build-and-publish.yaml
)

old=$(grep "LIBCRYPTO_VERSION ?= " Makefile | cut -d ' ' -f 3)
new=${1:-}


if [ "${old}" == "" -o "${new}" == "" ]; then
  echo "$0 <new version>"
  exit 1
fi

echo " --> Old version: ${old}"
echo " --> New version: ${new}"

echo ""
echo "Press ^C to exit or any key to continue..."
read

for f in "${files[@]}"; do
  sed -i "s/${old}/${new}/g" "${f}"
done
