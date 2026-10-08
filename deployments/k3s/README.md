# K3s 双环境部署说明

`dev` 与 `prod` 两套环境部署在同一个 K3s 集群上，通过 namespace 隔离，
共用 `base/` 里的一份模板，只有「环境变量」和「入口域名」不同。

```text
base/            两份环境共用的模板（不要写死域名和环境专属值）
overlays/dev/    开发环境 → namespace: pet-dev
overlays/prod/   生产环境 → namespace: pet-prod
```

## 资源开销（部署前先确认集群扛得住）

| 组件 | request | limit |
| --- | --- | --- |
| pet-api（单套） | 512Mi | 1Gi |
| postgres（单套） | 256Mi | 512Mi |
| **两套合计** | **约 1.5Gi** | **约 3Gi** |

API 进程实测常驻内存 550~600 MB（两份 gse 中文分词词典），**不能把 request 调低**，
否则会 OOM 反复重启。集群可用内存建议 ≥ 4Gi。

## 一、前置准备

1. 修改两个 overlay 里的域名，把 `example.com` 换成你的真实域名：
   - `overlays/dev/20-patch-config.yaml` → `PUBLIC_BASE_URL`
   - `overlays/dev/70-ingress.yaml` → `host` 与 `tls.hosts`
   - `overlays/prod/20-patch-config.yaml` → `PUBLIC_BASE_URL`
   - `overlays/prod/70-ingress.yaml` → `host` 与 `tls.hosts`

2. 两个域名都要提前加入小程序后台的 request 合法域名白名单。
   白名单**每月只能修改 5 次**，务必一次配齐。

## 二、构建并分发镜像

```bash
cd /Users/zyj/Developer/miniprogram/pet

docker build -f deployments/docker/Dockerfile -t pet-api:latest .
docker build -f deployments/docker/Dockerfile.migrate -t pet-migrate:latest .
```

K3s 用 containerd，没有私有仓库时可用本地导入：

```bash
docker save pet-api:latest -o pet-api.tar
docker save pet-migrate:latest -o pet-migrate.tar

sudo k3s ctr images import pet-api.tar
sudo k3s ctr images import pet-migrate.tar
```

> 导入后用 `sudo k3s crictl images | grep pet` 确认。
> manifests 里 `imagePullPolicy: IfNotPresent`，不会去远端拉取。
> 若用私有镜像仓库，记得给两个 namespace 都配 imagePullSecrets。

## 三、创建密钥（两个 namespace 各一份，勿提交 Git）

```bash
NS_AND_ENV=(pet-dev pet-prod)

for ns in "${NS_AND_ENV[@]}"; do
  kubectl create namespace $ns --dry-run=client -o yaml | kubectl apply -f -

  kubectl -n $ns create secret generic pet-api-secret \
    --from-literal=JWT_SECRET="$(openssl rand -hex 32)" \
    --from-literal=WECHAT_APP_ID="wx你的AppID" \
    --from-literal=WECHAT_APP_SECRET="你的AppSecret" \
    --from-literal=POSTGRES_PASSWORD="强密码" \
    --dry-run=client -o yaml | kubectl apply -f -

  # 通配符证书可两个 namespace 共用同一份；单域名证书则各自替换
  kubectl -n $ns create secret tls pet-tls \
    --cert=你的证书.pem --key=你的证书.key \
    --dry-run=client -o yaml | kubectl apply -f -
done
```

## 四、部署

**顺序很重要：迁移 Job 必须在 PostgreSQL 就绪之后再跑。**

```bash
cd /Users/zyj/Developer/miniprogram/pet/deployments/k3s

# 但 base 里的 Job 会随 kustomize 一起被创建，先只应用基础资源
kubectl apply -k overlays/dev
kubectl apply -k overlays/prod
```

若 Job 因数据库未就绪而失败，删掉重跑即可：

```bash
kubectl -n pet-dev delete job pet-migrate && kubectl apply -k overlays/dev
kubectl -n pet-prod delete job pet-migrate && kubectl apply -k overlays/prod
```

确认 54 个 migration 全部成功：

```bash
kubectl -n pet-dev logs job/pet-migrate
kubectl -n pet-prod logs job/pet-migrate
```

## 五、验证

```bash
kubectl -n pet-dev get pods,svc,ingress
kubectl -n pet-prod get pods,svc,ingress

kubectl -n pet-dev port-forward svc/pet-api 8081:8080
curl -i http://127.0.0.1:8081/healthz
```

健康检查端点是 `/healthz`，**不带 `/api/v1` 前缀**。

## 六、两套环境的差异

| | dev | prod |
| --- | --- | --- |
| namespace | `pet-dev` | `pet-prod` |
| APP_ENV | `development` | `production` |
| 订阅消息 state | `developer`（正式版收不到） | `formal`（正常送达） |
| 数据库 | 独立实例 | 独立实例 |

**两个后端必须用不同数据库**：同一个 AppID 下，体验版和正式版拿到的是同一个 openid，
共用数据库会导致测试数据污染生产数据。当前配置已按 namespace 做到物理隔离。

APP_ENV=development 时会额外开启 FastPath 与调试日志，且不强制校验 AppSecret 等；
生产环境请务必保持 `production`。

## 七、后续迁移腾讯云

prod 环境迁移时**保持域名不变**，只把 DNS 指向新服务器，因此：

- 小程序后台白名单不用改；
- 前端 `TARO_APP_API_BASE_URL` 不用改、不用重新发版；
- 只需迁移数据库数据 + `pet-uploads` 卷里的图片。

届时本目录下的 prod overlay 停用，改用 Nginx/Docker Compose 承接。
