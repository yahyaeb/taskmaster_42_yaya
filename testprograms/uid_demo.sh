#!/bin/bash
echo "=== UID Demonstration ==="
echo "Running as: $(whoami)"
echo "My UID is: $(id -u)"
echo "My GID is: $(id -g)"
echo ""

touch /tmp/uid_test_file
ls -ln /tmp/uid_test_file

echo ""

echo "Attempting su - nobody..."
if su - nobody -c "
  echo 'Now running as: \$(whoami)'
  echo 'UID: \$(id -u), GID: \$(id -g)'
  touch /tmp/nobody_file
  ls -ln /tmp/nobody_file
" 2>/dev/null; then
  echo "su succeeded"
else
  echo "su failed (user nobody may not exist or has no shell)"
fi
