#!/bin/bash
dnf upgrade -y -x kube*
touch /var/run/reboot-required
