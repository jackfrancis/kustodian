#!/bin/bash
# assumes run as root
sed -i "s|--node-status-update-frequency=10s|--node-status-update-frequency=1m|g" /etc/default/kubelet
systemctl restart kubelet
