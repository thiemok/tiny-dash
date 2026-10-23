#!/bin/sh
# Wrapper to launch Chrome installed via flatpak.
# chromedp calls this as if it were the chrome binary, passing flags directly.
# The fontconfig override (aliased text, see fontconfig/fonts.conf) is mounted
# read-only into the sandbox, matching the Docker image.
#
# --die-with-parent: chromedp's Pdeathsig only reaches the first bwrap that
# `flatpak run` execs into; without this the sandboxed Chrome is orphaned
# whenever the API server dies without a graceful shutdown.
#
# --user-data-dir: chromedp creates a fresh /tmp/chromedp-runner* profile per
# run, but the flatpak's /tmp is private ($XDG_RUNTIME_DIR/.flatpak/...), so
# chromedp only removes the empty host dir and the real profile (~130 MB of
# downloaded components) piles up in tmpfs. Chrome honours the last
# --user-data-dir, so this reuses one on-disk profile instead.
fc_dir=$(cd "$(dirname "$0")/../fontconfig" && pwd)
profile_dir="$HOME/.var/app/com.google.Chrome/cache/tiny-dash-chromedp"
exec flatpak run --die-with-parent --filesystem="$fc_dir:ro" --env=FONTCONFIG_FILE="$fc_dir/fonts.conf" \
	--command=chrome com.google.Chrome "$@" --user-data-dir="$profile_dir"
