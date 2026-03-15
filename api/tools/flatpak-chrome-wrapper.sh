#!/bin/sh
# Wrapper to launch Chrome installed via flatpak.
# chromedp calls this as if it were the chrome binary, passing flags directly.
exec flatpak run --command=chrome com.google.Chrome "$@"
