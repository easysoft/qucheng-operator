#!/usr/bin/env bash

go mod tidy
go mod vendor
retVal=$?
if [ $retVal -ne 0 ]; then
    exit $retVal
fi

set -e
TMP_DIR=$(mktemp -d)
mkdir -p "${TMP_DIR}"/src/gitlab.zcorp.cc/pangu/cne-operator
cp -r ./{apis,hack,vendor,go.mod} "${TMP_DIR}"/src/gitlab.zcorp.cc/pangu/cne-operator

(cd "${TMP_DIR}"/src/gitlab.zcorp.cc/pangu/cne-operator; \
    GOPATH=${TMP_DIR} GO111MODULE=off /bin/bash vendor/k8s.io/code-generator/generate-groups.sh all \
    gitlab.zcorp.cc/pangu/cne-operator/pkg/client gitlab.zcorp.cc/pangu/cne-operator/apis "qucheng:v1beta1" -h ./hack/boilerplate.go.txt ;
    )

rm -rf ./pkg/client/{clientset,informers,listers}
tree "${TMP_DIR}"/src/gitlab.zcorp.cc/pangu/cne-operator/pkg/client/
mv "${TMP_DIR}"/src/gitlab.zcorp.cc/pangu/cne-operator/pkg/client/* ./pkg/client/
rm -rf ${TMP_DIR}