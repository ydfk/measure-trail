#!/usr/bin/env sh
set -eu

output_path="${1:-.env.production}"
public_base_url="${MEASURETRAIL_PUBLIC_BASE_URL:-https://measure-trail.ydfk.site}"
default_username="${MEASURETRAIL_DEFAULT_USERNAME:-admin}"
default_password="${MEASURETRAIL_DEFAULT_PASSWORD:-}"
ios_app_id="${MEASURETRAIL_IOS_APP_ID:-}"

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

if [ -z "$ios_app_id" ]; then
  echo "请通过 MEASURETRAIL_IOS_APP_ID 提供 Apple Team ID.Bundle ID" >&2
  exit 1
fi
case "$ios_app_id" in
  *.*) ;;
  *)
    echo "MEASURETRAIL_IOS_APP_ID 必须使用 Apple Team ID.Bundle ID 格式" >&2
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

random_base64() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 32 | tr -d '\n'
  else
    dd if=/dev/urandom bs=32 count=1 2>/dev/null | base64 | tr -d '\n'
  fi
}

access_secret=$(random_hex)
refresh_secret=$(random_hex)
if [ -z "$default_password" ]; then
  default_password=$(random_hex | cut -c 1-24)
fi
passkey_encryption_key=$(random_base64)

umask 077
cat > "$output_path" <<EOF
MEASURETRAIL_ENV=production
MEASURETRAIL_PUBLIC_BASE_URL=${public_base_url%/}
MEASURETRAIL_DEFAULT_USERNAME=$default_username
MEASURETRAIL_DEFAULT_PASSWORD=$default_password
MEASURETRAIL_JWT_ACCESS_SECRET=$access_secret
MEASURETRAIL_JWT_REFRESH_SECRET=$refresh_secret
MEASURETRAIL_PASSKEY_CREDENTIAL_ENCRYPTION_KEY=$passkey_encryption_key
MEASURETRAIL_IOS_APP_ID=$ios_app_id
EOF

chmod 600 "$output_path"
echo "已生成生产环境文件：$output_path"
echo "默认用户名：${default_username}；密码仅保存在该文件中。"
