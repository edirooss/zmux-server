#!/bin/bash
set -e
systemctl disable zmux-server || true
systemctl stop zmux-server || true
systemctl daemon-reload
