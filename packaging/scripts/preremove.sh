#!/bin/bash
set -e

systemctl stop    reddit-monitor 2>/dev/null || true
systemctl disable reddit-monitor 2>/dev/null || true
