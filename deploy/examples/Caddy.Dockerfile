# 构建含 caddy-ratelimit 插件的自定义 Caddy 镜像。
# 用法（在项目根执行）：
#   docker build -t caddy-rl -f deploy/examples/Caddy.Dockerfile .
#   docker run -d --name caddy-rl --network host \
#     -v $(pwd)/deploy/examples/Caddyfile:/etc/caddy/Caddyfile \
#     caddy-rl
# Caddy 自动签发 pub-demo.lanpang.top 的 TLS 证书（需 80/443 可达 + DNS 已解析）。

FROM caddy:2-builder AS builder
RUN xcaddy build --with github.com/mholt/caddy-ratelimit

FROM caddy:2-alpine
COPY --from=builder /usr/bin/caddy /usr/bin/caddy
COPY deploy/examples/Caddyfile /etc/caddy/Caddyfile
