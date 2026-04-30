# NME SaaS v9.1 (Go)

Go 重构版支付中控，包含 Gin + GORM + PostgreSQL + Redis + Asynq。

## 环境要求

- Docker >= 20.x
- Docker Compose >= 2.x

## 快速部署

```bash
# 1. 解压项目到服务器
tar -xzf nme_v9.tar.gz
cd nme_v9_modified

# 2. 添加执行权限
chmod +x deploy.sh

# 3. 运行一键部署脚本
./deploy.sh
```

脚本会自动完成：
- 交互式填写域名
- 随机生成数据库名、数据库密码、JWT Secret
- 写入 `.env` 配置文件
- 构建镜像并启动容器
- 等待数据库就绪
- 执行数据库迁移
- 写入初始数据

## 默认账号

| 角色 | 邮箱 | 密码 |
|------|------|------|
| 管理员 | admin@zulpo.com | AA123456 |
| 普通用户 | user@zulpo.com | AA123456 |

> ⚠️ 生产环境请部署完成后立即修改密码！

## 反向代理配置

API 服务运行在宿主机 `8080` 端口，静态前端文件在项目 `frontend/` 目录下。

### 宝塔面板配置步骤

1. **网站** → **添加站点** → 填入域名
2. 进入站点设置 → **反向代理** → **添加反向代理**
   - 目标 URL：`http://127.0.0.1:8080`
   - 代理目录：`/api/`
3. 再添加一条反向代理：
   - 目标 URL：`http://127.0.0.1:8080`
   - 代理目录：`/pay/result`
4. **网站根目录** 设置为项目下的 `frontend/` 文件夹
5. 进入站点设置 → **伪静态** → 添加以下规则：
   ```nginx
   try_files $uri $uri/ /dashboard.html;
   ```
6. 反向代理 Header 配置（在自定义配置或反向代理高级设置中添加）：
   ```nginx
   proxy_set_header Host $host;
   proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
   proxy_set_header X-Forwarded-Proto $scheme;
   proxy_http_version 1.1;
   ```
7. **SSL** → 申请 Let's Encrypt 证书 → 开启强制 HTTPS

## 容器管理

```bash
# 查看运行状态
docker compose -f docker-compose.prod.yml ps

# 查看日志
docker compose -f docker-compose.prod.yml logs app --tail=50
docker compose -f docker-compose.prod.yml logs worker --tail=50

# 重启服务
docker compose -f docker-compose.prod.yml restart app

# 停止所有服务
docker compose -f docker-compose.prod.yml down

# 更新部署（代码更新后）
docker compose -f docker-compose.prod.yml up -d --build
```

## 服务说明

| 服务 | 说明 | 端口 |
|------|------|------|
| app | HTTP API 服务（Gin） | 8080 |
| worker | 异步任务 Worker（Asynq） | — |
| postgres | 主数据库（PostgreSQL 16） | — |
| redis | 缓存与任务队列（Redis 7） | — |

## 注意事项

- `GET /pay/result` 仅用于结果展示，不作为支付成功凭证
- 订单终态以服务端回调为准
- 网关签名说明见：`docs/api-signing.md`
- 认证接口：
  - `POST /api/auth/login`
  - `POST /api/auth/refresh`
  - `POST /api/auth/logout`
