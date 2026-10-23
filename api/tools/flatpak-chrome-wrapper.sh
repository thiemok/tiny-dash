#!/bin/sh
# Wrapper to launch Chrome installed via flatpak.
# chromedp calls this as if it were the chrome binary, passing flags directly.
# The fontconfig override (aliased text, see fontconfig/fonts.conf) is mounted
# read-only into the sandbox, matching the Docker image.
fc_dir=$(cd "$(dirname "$0")/../fontconfig" && pwd)
exec flatpak run --filesystem="$fc_dir:ro" --env=FONTCONFIG_FILE="$fc_dir/fonts.conf" --command=chrome com.google.Chrome "$@"
