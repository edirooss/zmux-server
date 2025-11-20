#!/bin/bash
set -e
systemctl daemon-reload
systemctl enable zmux-server
systemctl restart zmux-server || true
