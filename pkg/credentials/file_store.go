// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package credentials

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	corev1api "k8s.io/api/core/v1"
	kbclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/velero/pkg/util/filesystem"
	"github.com/vmware-tanzu/velero/pkg/util/kube"
)

// FileStore defines operations for interacting with credentials
// that are stored on a file system.
type FileStore interface {
	// Path returns a path on disk where the secret key defined by
	// the given selector is serialized.
	Path(selector *corev1api.SecretKeySelector) (string, error)
}

type namespacedFileStore struct {
	client    kbclient.Client
	namespace string
	fsRoot    string
	fs        filesystem.Interface
}

// NewNamespacedFileStore returns a FileStore which can interact with credentials
// for the given namespace and will store them under the given fsRoot.
func NewNamespacedFileStore(client kbclient.Client, namespace string, fsRoot string, fs filesystem.Interface) (FileStore, error) {
	fsNamespaceRoot := filepath.Join(fsRoot, namespace)

	if err := fs.MkdirAll(fsNamespaceRoot, 0755); err != nil {
		return nil, err
	}

	return &namespacedFileStore{
		client:    client,
		namespace: namespace,
		fsRoot:    fsNamespaceRoot,
		fs:        fs,
	}, nil
}

// Path returns a path on disk where the secret key defined by
// the given selector is serialized.
func (n *namespacedFileStore) Path(selector *corev1api.SecretKeySelector) (string, error) {
	creds, err := kube.GetSecretKey(n.client, n.namespace, selector)
	if err != nil {
		return "", errors.Wrap(err, "unable to get key for secret")
	}

	keyFilePath := filepath.Join(n.fsRoot, fmt.Sprintf("%s-%s", selector.Name, selector.Key))

	file, err := n.fs.OpenFile(keyFilePath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return "", errors.Wrap(err, "unable to open credentials file for writing")
	}

	if _, err := file.Write(creds); err != nil {
		return "", errors.Wrap(err, "unable to write credentials to store")
	}

	if err := file.Close(); err != nil {
		return "", errors.Wrap(err, "unable to close credentials file")
	}

	return keyFilePath, nil
}
