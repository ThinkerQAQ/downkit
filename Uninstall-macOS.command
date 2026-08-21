#!/bin/sh
set -eu

host_name='com.downkit.bridge'
removed=0

for native_dir in \
  "$HOME/Library/Application Support/Google/Chrome/NativeMessagingHosts" \
  "$HOME/Library/Application Support/Chromium/NativeMessagingHosts" \
  "$HOME/Library/Application Support/Microsoft Edge/NativeMessagingHosts"
do
  manifest_path="$native_dir/$host_name.json"
  if [ -f "$manifest_path" ]; then
    rm -f -- "$manifest_path"
    printf '已移除：%s\n' "$manifest_path"
    removed=$((removed + 1))
  fi
done

printf 'DownKit macOS 卸载完成，共移除 %s 个浏览器清单。\n' "$removed"
printf '如不再使用，请同时从浏览器中移除 DownKit 扩展。\n'
