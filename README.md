# qucheng-operator

## 安装

```bash
git clone https://github.com/easysoft/qucheng-operator.git
cd qucheng-operator
kubectl create ns cne-system
helm install -n cne-system qucheng-operator config/charts/cne-operator
```

## 本地开发

- go,推荐go1.18
- docker
- kubectl
- helm
- kind,推荐最新版本

```
# kind部署集群
bash hack/kind/setup.sh
# gen crd
make local-crd
kubetl apply -f hack/deploy/crd.yaml
```
