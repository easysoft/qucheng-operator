# qucheng-operator

## 安装

```bash
git clone https://github.com/easysoft/qucheng-operator.git
cd qucheng-operator
kubectl create ns cne-system
helm install -n cne-system qucheng-operator config/charts/cne-operator
```
