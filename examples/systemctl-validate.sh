#!/bin/bash
# assumes run as root
if systemctl list-units | grep -i failed; then
  exit 1
fi
exit 0
