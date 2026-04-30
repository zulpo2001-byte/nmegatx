#!/bin/bash
set -e

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo ""
echo "================================="
echo "      NME v9 一键部署脚本        "
echo "================================="
echo ""

# 交互：填写域名
read -p "请输入你的域名（如 pay.example.com，不含 https://）: " DOMAIN
if [ -z "$DOMAIN" ]; then
  echo -e "${RED}错误：域名不能为空${NC}"
  exit 1
fi

# 随机生成
DB_NAME="nme_$(cat /dev/urandom | tr -dc 'a-z0-9' | head -c 8)"
DB_PASSWORD=$(cat /dev/urandom | tr -dc 'A-Za-z0-9' | head -c 24)
JWT_SECRET=$(cat /dev/urandom | tr -dc 'A-Za-z0-9' | head -c 48)

echo ""
echo -e "${GREEN}✔ 正在生成配置...${NC}"

cat > .env <<EOF
APP_ENV=prod
APP_PORT=8080
APP_BASE_URL=https://${DOMAIN}

JWT_SECRET=${JWT_SECRET}
JWT_EXPIRES_HOURS=168
JWT_REFRESH_DAYS=30

DB_HOST=postgres
DB_PORT=5432
DB_USER=nme
DB_PASSWORD=${DB_PASSWORD}
DB_NAME=${DB_NAME}
DB_SSLMODE=disable

REDIS_ADDR=redis:6379
REDIS_PASSWORD=
REDIS_DB=0

ASYNQ_CONCURRENCY=20

HMAC_WINDOW_SECONDS=300
CORS_ALLOW_ORIGINS=https://${DOMAIN}

SEED_ADMIN_EMAIL=admin@zulpo.com
SEED_ADMIN_PASSWORD=AA123456
SEED_USER_EMAIL=user@zulpo.com
SEED_USER_PASSWORD=AA123456
SEED_API_KEY=ak_demo_seed_001
SEED_API_SECRET=sk_demo_seed_001
SEED_USER_API_KEY=ak_demo_user_001
SEED_USER_API_SECRET=sk_demo_user_001
EOF

echo -e "${GREEN}✔ .env 已生成${NC}"

# 清理旧容器和数据卷
echo -e "${YELLOW}► 清理旧容器和数据卷...${NC}"
docker compose -f docker-compose.prod.yml down -v 2>/dev/null || true

# 启动容器
echo -e "${YELLOW}► 构建并启动容器...${NC}"
docker compose -f docker-compose.prod.yml up -d --build

# 等待 postgres 就绪
echo -e "${YELLOW}► 等待数据库就绪...${NC}"
MAX_WAIT=60
COUNT=0
until docker compose -f docker-compose.prod.yml exec -T postgres pg_isready -U nme -d "${DB_NAME}" > /dev/null 2>&1; do
  COUNT=$((COUNT+1))
  if [ $COUNT -ge $MAX_WAIT ]; then
    echo -e "${RED}错误：数据库启动超时，请检查日志：docker compose -f docker-compose.prod.yml logs postgres${NC}"
    exit 1
  fi
  sleep 1
done
echo -e "${GREEN}✔ 数据库已就绪${NC}"

# 执行迁移
echo -e "${YELLOW}► 执行数据库迁移...${NC}"
docker compose -f docker-compose.prod.yml exec -T app ./nme-migrate
echo -e "${GREEN}✔ 数据库迁移完成${NC}"

# 写入初始数据
echo -e "${YELLOW}► 写入初始数据...${NC}"
docker compose -f docker-compose.prod.yml exec -T app ./nme-seed
echo -e "${GREEN}✔ 初始数据写入完成${NC}"

echo ""
echo "================================="
echo -e "${GREEN}        部署完成！${NC}"
echo "================================="
echo ""
echo "  域名:       https://${DOMAIN}"
echo "  管理员账号: admin@zulpo.com"
echo "  管理员密码: AA123456"
echo "  数据库名:   ${DB_NAME}"
echo "  API 端口:   8080"
echo ""
echo "  宝塔反向代理配置："
echo "  目标地址:   http://127.0.0.1:8080"
echo "  静态目录:   $(pwd)/frontend"
echo ""
echo -e "${YELLOW}  ⚠ 请在宝塔完成反向代理配置后再访问${NC}"
echo ""
