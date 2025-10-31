# ------------------------------------------------------------------
# Forkbomb AVDCTL Base Config Template
# AGPL-3.0-or-later © 2025 Forkbomb B.V.
# ------------------------------------------------------------------
AvdId=base-a35
PlayStore.enabled=true

# --- System image / ABI ---
image.sysdir.1=/opt/android-sdk/system-images/android-35/google_apis_playstore/x86_64
abi.type=x86_64
hw.cpu.arch=x86_64
hw.cpu.ncore=4
hw.ramSize=4096
vm.heapSize=384
sdcard.size=256M

# --- Display & sensors (headless friendly) ---
hw.lcd.width=1080
hw.lcd.height=2400
hw.lcd.density=420
hw.gpu.mode=swiftshader_indirect
hw.gpu.enabled=yes
hw.keyboard=yes
hw.battery=yes
hw.accelerometer=no
hw.sensors.orientation=no

# --- Boot behaviour ---
QuickBoot.mode=disabled
snapshot.present=false
fastboot.forceColdBoot=yes
userdata.useQcow2=yes

# --- Networking ---
hw.gps=no
hw.wifi=yes
hw.ril=no
dns.server=8.8.8.8

# --- Misc ---
hw.audioInput=no
hw.audioOutput=no
showDeviceFrame=no
skin.dynamic=yes
