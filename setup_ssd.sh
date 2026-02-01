#!/bin/bash

# Configuration
UUID="6EE5-D7D2"
MOUNT_POINT="/mnt/ssd"
FSTAB_LINE="UUID=$UUID  $MOUNT_POINT  exfat  defaults,uid=1000,gid=1000,umask=000  0  0"

echo "--- SSD Permanent Auto-Mount & API Configuration Setup ---"

# 1. Create mount directory
echo "[1/5] Creating mount point at $MOUNT_POINT..."
sudo mkdir -p $MOUNT_POINT

# 2. Check if already in fstab to avoid duplicates
if grep -q "$UUID" /etc/fstab; then
    echo "[!] UUID $UUID is already registered in /etc/fstab."
else
    echo "[2/5] Adding entry to /etc/fstab..."
    echo "$FSTAB_LINE" | sudo tee -a /etc/fstab
fi

# 3. Unmount old locations and mount to the new folder
echo "[3/5] Mounting SSD to new location..."
sudo umount /run/media/roniserv/V-GEN* 2>/dev/null
sudo mount -a

if mountpoint -q $MOUNT_POINT; then
    echo "OK: SSD is now mounted at $MOUNT_POINT"
else
    echo "ERROR: Failed to mount SSD. Please check if the device is connected."
    exit 1
fi

# 4. Automatically update .env file
echo "[4/5] Updating .env file..."
if [ -f ".env" ]; then
    sed -i "s|HOST_PATH_SSD=.*|HOST_PATH_SSD=$MOUNT_POINT|g" .env
    sed -i "s|ssd:[^,]*|ssd:$MOUNT_POINT|g" .env
    echo "OK: .env successfully updated."
else
    echo "WARN: .env file not found in this directory."
fi

# 5. Restart server
echo "[5/5] Restarting Docker containers..."
docker-compose down && docker-compose up -d

echo "--- FINISHED ---"
echo "Your SSD is now locked to $MOUNT_POINT and will persist across reboots."
