#!/bin/bash
PATH=$PATH:/usr/local/sbin:/usr/sbin:/sbin
DEBIAN_FRONTEND=noninteractive
dnf upgrade -y -x kube* 
