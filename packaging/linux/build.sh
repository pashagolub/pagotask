#!/bin/sh
# Packages a built Linux binary as a .deb and a tarball.
#   packaging/linux/build.sh <binary> <version> <outdir>
set -eu
bin=$1
version=$2
out=$3
here=$(cd "$(dirname "$0")" && pwd)
arch=amd64

root=$(mktemp -d)
chmod 755 "$root"
trap 'rm -rf "$root"' EXIT
install -Dm755 "$bin" "$root/usr/bin/pagotask"
install -Dm644 "$here/pagotask.desktop" "$root/usr/share/applications/pagotask.desktop"
install -Dm644 "$here/pagotask.png" "$root/usr/share/icons/hicolor/256x256/apps/pagotask.png"
install -Dm644 "$here/../../LICENSE" "$root/usr/share/doc/pagotask/copyright"
mkdir -p "$root/DEBIAN"
cat > "$root/DEBIAN/control" <<EOF
Package: pagotask
Version: $version
Architecture: $arch
Maintainer: Pavlo Golub <9463113+pashagolub@users.noreply.github.com>
Depends: libgtk-4-1, libwebkitgtk-6.0-4
Recommends: x11-utils, gnome-keyring
Section: utils
Priority: optional
Homepage: https://github.com/pashagolub/pagotask
Description: Quick capture for Google Tasks
 A tray app: a keyboard shortcut opens a small popup, prefilled from the
 page or window in front, and Enter saves the task to Google Tasks.
EOF
mkdir -p "$out"
dpkg-deb --root-owner-group --build "$root" "$out/pagotask_${version}_${arch}.deb"

tdir=$(mktemp -d)
mkdir "$tdir/pagotask"
cp "$bin" "$tdir/pagotask/pagotask"
cp "$here/pagotask.desktop" "$here/pagotask.png" "$here/../../LICENSE" "$tdir/pagotask/"
tar -C "$tdir" -czf "$out/pagotask_${version}_linux_${arch}.tar.gz" pagotask
rm -rf "$tdir"
