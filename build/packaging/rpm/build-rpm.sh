#!/usr/bin/env bash

set -x

echo "running rpm build"

SOURCE_DIR=/opt/package-build/source

pushd ${SOURCE_DIR}
rpmbuild --define "_version 1.1" --define "_source_dir ${SOURCE_DIR}" \
  -ba --build-in-place build/packaging/rpm/google-guest-agent.spec

popd
