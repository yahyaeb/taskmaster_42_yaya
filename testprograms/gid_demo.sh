#!/bin/bash
echo "=== GID Demonstration ==="
echo "My groups: $(groups)"
echo "My primary GID: $(id -g)"
echo ""

touch /tmp/gid_test_file
ls -ln /tmp/gid_test_file

echo ""

if chgrp staff /tmp/gid_test_file 2>/dev/null; then
  ls -ln /tmp/gid_test_file
else
  echo "Cannot chgrp to staff (not in that group)"
fi

echo ""

if newgrp staff 2>/dev/null << 'INNER'
  echo "Now in group staff"
  echo "Primary GID: $(id -g)"
  touch /tmp/staff_file
  ls -ln /tmp/staff_file
INNER
then
  echo "newgrp succeeded"
else
  echo "newgrp failed (staff group may not exist or you lack permission)"
fi
