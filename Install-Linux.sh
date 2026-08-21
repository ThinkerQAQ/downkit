#!/usr/bin/env sh
set -eu

host_name='com.downkit.bridge'
extension_id='hfjpenjlamneepmigemmmiibebpokmik'
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)

case "$(uname -m)" in
  x86_64|amd64) architecture='amd64' ;;
  aarch64|arm64) architecture='arm64' ;;
  *)
    printf 'DownKit 不支持当前 CPU 架构：%s\n' "$(uname -m)" >&2
    exit 1
    ;;
esac

bridge_path="$script_dir/downkit-linux-$architecture"
if [ ! -f "$bridge_path" ]; then
  printf '找不到 Bridge：%s\n' "$bridge_path" >&2
  exit 1
fi

chmod +x "$bridge_path"
if [ -f "$script_dir/tools/ffmpeg-slim" ]; then
  chmod +x "$script_dir/tools/ffmpeg-slim"
fi

escaped_bridge_path=$(printf '%s' "$bridge_path" | sed 's/\\/\\\\/g; s/"/\\"/g')
manifest=$(printf '%s\n' \
  '{' \
  "  \"name\": \"$host_name\"," \
  '  "description": "DownKit local bridge",' \
  "  \"path\": \"$escaped_bridge_path\"," \
  '  "type": "stdio",' \
  '  "allowed_origins": [' \
  "    \"chrome-extension://$extension_id/\"" \
  '  ]' \
  '}')

installed=0
for native_dir in \
  "$HOME/.config/google-chrome/NativeMessagingHosts" \
  "$HOME/.config/chromium/NativeMessagingHosts" \
  "$HOME/.config/microsoft-edge/NativeMessagingHosts"
do
  mkdir -p "$native_dir"
  manifest_path="$native_dir/$host_name.json"
  printf '%s\n' "$manifest" > "$manifest_path"
  chmod 600 "$manifest_path"
  printf '已注册：%s\n' "$manifest_path"
  installed=$((installed + 1))
done

printf 'DownKit Linux 安装完成，共写入 %s 个浏览器清单。\n' "$installed"
printf '请在 Chrome 或 Edge 的扩展管理页加载本目录下的 extension 文件夹。\n'
