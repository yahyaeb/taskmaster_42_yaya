cat > setup.sh << 'EOF'
#!/bin/bash
USER_HOME=$(eval echo ~$USER)
WORKDIR="$USER_HOME/Documents/taskmaster_42"

sed "s|/home/yel-bouk/Documents/taskmaster_42|$WORKDIR|g" config.yml.template > config.yml
echo "config.yml generated for user $USER at $WORKDIR"
EOF
chmod +x setup.sh