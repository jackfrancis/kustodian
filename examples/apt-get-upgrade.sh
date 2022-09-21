#!/bin/bash
# assumes run as root
export DEBIAN_FRONTEND=noninteractive
export PATH="$PATH:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
wait_for_apt_locks() {
  while fuser /var/lib/dpkg/lock /var/lib/apt/lists/lock /var/cache/apt/archives/lock >/dev/null 2>&1; do
    echo 'Waiting for release of apt locks'
    sleep 3
  done
}
wait_for_apt_locks
apt-get update
wait_for_apt_locks
apt-mark unhold $(apt-mark showhold | grep -v 'kube')
apt-get upgrade -y
wait_for_apt_locks
apt-get upgrade -y $(apt-mark showmanual | grep -v 'kube')
