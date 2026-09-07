#!/usr/bin/env sh
set -eu

output_path="${1:-.env.production}"
public_base_url="${MEASURETRAIL_PUBLIC_BASE_URL:-https://measure-api.ydfk.site}"
default_username="${MEASURETRAIL_DEFAULT_USERNAME:-admin}"

if [ -e "$output_path" ]; then
  echo "目标文件已存在，未覆盖：$output_path" >&2
  exit 1
fi

case "$public_base_url" in
  https://*) ;;
  *)
    echo "生产地址必须以 https:// 开头" >&2
    exit 1
    ;;
esac

random_hex() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32
  else
    LC_ALL=C od -An -N32 -tx1 /dev/urandom | tr -d ' \n'
  fi
}

access_secret=$(random_hex)
refresh_secret=$(random_hex)
default_password=$(random_hex | cut -c 1-24)

umask 077
cat > "$output_path" <<EOF
MEASURETRAIL_ENV=production
MEASURETRAIL_PUBLIC_BASE_URL=${public_base_url%/}
MEASURETRAIL_CORS_ORIGINS=${public_base_url%/}
MEASURETRAIL_DEFAULT_USERNAME=$default_username
MEASURETRAIL_DEFAULT_PASSWORD=$default_password
MEASURETRAIL_JWT_ISSUER=measuretrail
MEASURETRAIL_JWT_AUDIENCE=measuretrail-ios
MEASURETRAIL_JWT_ACCESS_SECRET=$access_secret
MEASURETRAIL_JWT_REFRESH_SECRET=$refresh_secret
EOF

chmod 600 "$output_path"
echo "已生成生产环境文件：$output_path"
echo "默认用户名：${default_username}；随机密码仅保存在该文件中。"
