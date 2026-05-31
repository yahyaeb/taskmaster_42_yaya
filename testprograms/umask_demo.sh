#!/bin/bash
echo "=== Umask Demonstration ==="
echo "Current umask: $(umask)"
echo ""

umask 022
touch /tmp/umask_022_file
ls -ln /tmp/umask_022_file
echo ""

umask 077
touch /tmp/umask_077_file
ls -ln /tmp/umask_077_file
echo ""

umask 000
touch /tmp/umask_000_file
ls -ln /tmp/umask_000_file
