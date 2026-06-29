#!/usr/bin/env bash
set -euo pipefail
SSH="ssh -o ConnectTimeout=5 root@${1:?Usage: $0 <board-ip>}"
$SSH "journalctl -u app --no-pager -l -n 30 | grep -iE 'display|fb|backlight|error|WARN'"
