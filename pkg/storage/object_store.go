// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package storage

import (
	"context"
	"github.com/easysoft/qucheng-operator/pkg/logging"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const ()

type objectStorage struct {
	ctx      context.Context
	AbsPath  string
	client   *minio.Client
	bucket   string
	location string
}

func NewObjectStorage(ctx context.Context, endpoint, accessKey, secretKey, location, bucket string) (Storage, error) {
	var client *minio.Client
	var err error

	// velero's endpoint style allow contain schema, we need parse the real host
	ep, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}

	if strings.HasSuffix(ep.Host, "aliyuncs.com") {
		// aliyun should use ssl connection,
		// other wise the upload will cause a `Aws MultiChunkedEncoding is not supported` error
		if client, err = minio.New(ep.Host, &minio.Options{
			Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
			Secure: true,
		}); err != nil {
			return nil, err
		}
	} else if strings.HasSuffix(ep.Host, "myqcloud.com") {
		// tencent endpoint should remove the bucket prefix,
		// and use dns lookup style.
		// Then the saved object file path will not start with a bucket's name directory.
		host := ep.Host
		if strings.HasPrefix(ep.Host, bucket) {
			host = ep.Host[len(bucket)+1 : len(ep.Host)]
		}
		if client, err = minio.New(host, &minio.Options{
			Creds:        credentials.NewStaticV4(accessKey, secretKey, ""),
			Secure:       true,
			BucketLookup: minio.BucketLookupDNS,
		}); err != nil {
			return nil, err
		}
	} else {
		useSSL := true
		if ep.Scheme == "http" {
			useSSL = false
		}
		if client, err = minio.New(ep.Host, &minio.Options{
			Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
			Secure: useSSL,
			Region: location,
		}); err != nil {
			return nil, err
		}
	}

	// check bucket exists. If not, try to create.
	exist, err := client.BucketExists(ctx, bucket)
	if !exist {
		err = client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: location})
		if err != nil {
			return nil, err
		}
	}
	return &objectStorage{
		ctx:    ctx,
		client: client,
		bucket: bucket,
	}, nil
}

func (o *objectStorage) PutBackup(path string, fd *os.File) (int64, error) {
	fd.Seek(0, 0)
	stat, _ := fd.Stat()
	logging.DefaultLogger().Infof("file size %d", stat.Size())
	info, err := o.client.PutObject(o.ctx, o.bucket, path, fd, stat.Size(), minio.PutObjectOptions{})
	if err != nil {
		return stat.Size(), err
	}
	return info.Size, nil
}

func (o *objectStorage) PullBackup(path string) (*os.File, error) {
	frames := strings.Split(path, "/")
	localPath := filepath.Join("/tmp", frames[len(frames)-1])
	if err := o.client.FGetObject(o.ctx, o.bucket, path, localPath, minio.GetObjectOptions{}); err != nil {
		return nil, err
	}
	fd, err := os.Open(localPath)
	return fd, err
}

func (o *objectStorage) Kind() Kind {
	return KindObjectSystem
}
