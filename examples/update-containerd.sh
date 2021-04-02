#!/bin/bash
# assumes run as root
PATH=$PATH:/usr/local/sbin:/usr/sbin:/sbin
DEBIAN_FRONTEND=noninteractive
apt-get upgrade moby-containerd -y
systemctl restart kubelet
