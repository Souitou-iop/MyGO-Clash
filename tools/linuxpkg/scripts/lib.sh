# Shared by the maintainer scripts of the deb, rpm and Arch packages (see
# scripts.go, which puts it in front of each script). POSIX sh. Every step is
# best effort: a package must install and remove even where systemd, the
# desktop tools or the app's service are missing.
#
# The app's privileged service (TUN mode) is made by the app, not the package:
# the app copies its executable to /usr/local/lib/<name>/<name>-service and
# writes /etc/systemd/system/<name>-service.service, as `<name> service
# install` does. So the package only removes it when it is removed, and
# refreshes that copy when the package is upgraded.

name='@NAME@'
svc="$name-service"
unit="/etc/systemd/system/$svc.service"
app="/opt/$name/$name"

have_systemd() {
  [ -d /run/systemd/system ] && command -v systemctl > /dev/null 2>&1
}

# service_cleanup removes what the service leaves, whatever of it is still
# there: what the app's own uninstall does, for when that cannot run.
service_cleanup() {
  if [ -f "$unit" ] && have_systemd; then
    systemctl disable --now "$svc.service" > /dev/null 2>&1 || true
  fi
  rm -f "$unit"
  if have_systemd; then
    systemctl daemon-reload > /dev/null 2>&1 || true
  fi
  rm -f "/usr/local/lib/$name/$svc" "/run/$svc.sock"
  rmdir "/usr/local/lib/$name" > /dev/null 2>&1 || true
  rm -rf "/run/$svc" "/var/lib/$svc"
}

# service_remove stops, disables and removes the service, for the removal of
# the package (never for an upgrade). It touches nothing else: running
# instances of the app, and what the users' sessions set up, are left alone.
service_remove() {
  if [ -f "$unit" ] && [ -x "$app" ]; then
    "$app" service uninstall --name "$name" > /dev/null 2>&1 || true
  fi
  service_cleanup
}

# service_refresh, after an upgrade has put the new files in place, installs
# the new executable as the service and restarts it, if the service was
# running: its executable is a copy of the app's, so it would keep running the
# old version. The service stays as it was for whoever it serves, which the
# unit records. A stopped or absent service is left alone: the app offers to
# reinstall an outdated one.
service_refresh() {
  [ -f "$unit" ] && [ -x "$app" ] && have_systemd || return 0
  systemctl is-active --quiet "$svc.service" > /dev/null 2>&1 || return 0
  owner=$(sed -n 's/^ExecStart=.* --owner "\([^"]*\)".*$/\1/p' "$unit" | head -n 1)
  case "$owner" in
    '' | *[!0-9]*) return 0 ;;
  esac
  if ! "$app" service install --name "$name" --owner "$owner" > /dev/null 2>&1; then
    echo "$name: could not restart its service on the new version; reinstall the service from the app's settings" >&2
  fi
  return 0
}

# refresh_caches updates the desktop, icon and MIME databases that the app's
# menu entry, icons and clash:// links are looked up in.
refresh_caches() {
  if command -v update-desktop-database > /dev/null 2>&1; then
    update-desktop-database -q /usr/share/applications > /dev/null 2>&1 || true
  fi
  if [ -d /usr/share/icons/hicolor ] && command -v gtk-update-icon-cache > /dev/null 2>&1; then
    gtk-update-icon-cache -q -t -f /usr/share/icons/hicolor > /dev/null 2>&1 || true
  fi
  if [ -d /usr/share/mime/packages ] && command -v update-mime-database > /dev/null 2>&1; then
    update-mime-database /usr/share/mime > /dev/null 2>&1 || true
  fi
  return 0
}

# fix_permissions makes the executable what the app's service expects to copy.
fix_permissions() {
  if [ -f "$app" ]; then
    chmod 0755 "$app" > /dev/null 2>&1 || true
  fi
  return 0
}
