# qucheng-operator

## 安装

```bash
git clone https://github.com/easysoft/qucheng-operator.git
cd qucheng-operator
kubectl create ns cne-system

# helm 方式安装
helm install -n cne-system qucheng-operator config/charts/cne-operator

# yaml 方式安装
kubectl apply -f hack/deploy/deploy.yaml
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
# 运行
make run
```
