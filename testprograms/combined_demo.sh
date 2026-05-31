#!/bin/bash
echo "=== Combined User/Group/Umask Demo ==="
echo "Initial state:"
echo "  UID: $(id -u), GID: $(id -g)"
echo "  Umask: $(umask)"
echo ""

umask 022
touch /tmp/combined_step1
ls -ln /tmp/combined_step1
echo ""

umask 077
touch /tmp/combined_step2
ls -ln /tmp/combined_step2
echo ""

su - nobody -c "
  umask 022
  touch /tmp/combined_step3_nobody
  ls -ln /tmp/combined_step3_nobody
" 2>/dev/null || echo "su to nobody failed"
echo ""

touch /etc/combined_test 2>&1 | head -1
echo ""
