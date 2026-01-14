# ------------------------------------------------------------------
# Forkbomb AVDCTL Production-Safe Config Template
# Copyright (C) 2025 Forkbomb B.V.
# License: AGPL-3.0-only
# ------------------------------------------------------------------
# This template is used when AVDCTL_CONFIG_TEMPLATE is set.
# Otherwise, base AVD's config.ini is used and sanitized.
# ------------------------------------------------------------------

PlayStore.enabled=no
abi.type=x86_64
avd.ini.encoding=UTF-8
disk.cachePartition=yes
disk.cachePartition.size=66MB
disk.systemPartition.size=0
disk.vendorPartition.size=0

# ------------------------------------------------------------------
# BOOT SETTINGS - Cold boot only, no snapshots
# ------------------------------------------------------------------
QuickBoot.mode=disabled
snapshot.present=false
fastboot.forceColdBoot=yes
firstboot.bootFromDownloadableSnapshot=no
firstboot.bootFromLocalSnapshot=no
firstboot.saveToLocalSnapshot=no

# ------------------------------------------------------------------
# STORAGE - Use raw IMG format for compatibility
# ------------------------------------------------------------------
userdata.useQcow2=no
hw.useext4=yes

# ------------------------------------------------------------------
# HARDWARE - Pixel 6 profile
# ------------------------------------------------------------------
hw.accelerometer=yes
hw.accelerometer_uncalibrated=yes
hw.arc=no
hw.arc.autologin=no
hw.audioInput=yes
hw.audioOutput=yes
hw.battery=yes
hw.camera.back=emulated
hw.camera.front=none
hw.cpu.arch=x86_64
hw.cpu.ncore=4
hw.dPad=no
hw.device.hash2=MD5:2016577e1656e8e7c2adb0fac972beea
hw.device.manufacturer=Google
hw.device.name=pixel_6

# ------------------------------------------------------------------
# DISPLAY SETTINGS
# ------------------------------------------------------------------
hw.lcd.backlight=yes
hw.lcd.circular=false
hw.lcd.density=420
hw.lcd.depth=16
hw.lcd.height=2400
hw.lcd.vsync=60
hw.lcd.width=1080
hw.initialOrientation=portrait
showDeviceFrame=yes

# Multi-display settings (disabled)
hw.display1.density=0
hw.display1.flag=0
hw.display1.height=0
hw.display1.width=0
hw.display1.xOffset=-1
hw.display1.yOffset=-1
hw.display2.density=0
hw.display2.flag=0
hw.display2.height=0
hw.display2.width=0
hw.display2.xOffset=-1
hw.display2.yOffset=-1
hw.display3.density=0
hw.display3.flag=0
hw.display3.height=0
hw.display3.width=0
hw.display3.xOffset=-1
hw.display3.yOffset=-1
hw.displayRegion.0.1.height=0
hw.displayRegion.0.1.width=0
hw.displayRegion.0.1.xOffset=-1
hw.displayRegion.0.1.yOffset=-1
hw.displayRegion.0.2.height=0
hw.displayRegion.0.2.width=0
hw.displayRegion.0.2.xOffset=-1
hw.displayRegion.0.2.yOffset=-1
hw.displayRegion.0.3.height=0
hw.displayRegion.0.3.width=0
hw.displayRegion.0.3.xOffset=-1
hw.displayRegion.0.3.yOffset=-1
hw.hotplug_multi_display=no
hw.multi_display_window=no

# ------------------------------------------------------------------
# GPU SETTINGS - Software rendering for headless
# ------------------------------------------------------------------
hw.gltransport=pipe
hw.gltransport.asg.dataRingSize=32768
hw.gltransport.asg.writeBufferSize=1048576
hw.gltransport.asg.writeStepSize=4096
hw.gltransport.drawFlushInterval=800
hw.gpu.enabled=no
hw.gpu.mode=auto

# ------------------------------------------------------------------
# INPUT DEVICES
# ------------------------------------------------------------------
hw.keyboard=no
hw.keyboard.charmap=qwerty2
hw.keyboard.lid=yes
hw.mainKeys=no
hw.screen=multi-touch
hw.touchpad0=no
hw.touchpad0.height=400
hw.touchpad0.width=600
hw.trackBall=no
hw.rotaryInput=no

# ------------------------------------------------------------------
# CONNECTIVITY
# ------------------------------------------------------------------
hw.gps=yes
hw.gsmModem=yes

# ------------------------------------------------------------------
# SENSORS
# ------------------------------------------------------------------
hw.gyroscope=yes
hw.sensor.hinge=no
hw.sensor.hinge.count=0
hw.sensor.hinge.fold_to_displayRegion.0.1_at_posture=1
hw.sensor.hinge.resizable.config=1
hw.sensor.hinge.sub_type=0
hw.sensor.hinge.type=0
hw.sensor.roll=no
hw.sensor.roll.count=0
hw.sensor.roll.resize_to_displayRegion.0.1_at_posture=6
hw.sensor.roll.resize_to_displayRegion.0.2_at_posture=6
hw.sensor.roll.resize_to_displayRegion.0.3_at_posture=6
hw.sensors.gyroscope_uncalibrated=yes
hw.sensors.heading=no
hw.sensors.heart_rate=no
hw.sensors.humidity=yes
hw.sensors.light=yes
hw.sensors.magnetic_field=yes
hw.sensors.magnetic_field_uncalibrated=yes
hw.sensors.orientation=yes
hw.sensors.pressure=yes
hw.sensors.proximity=yes
hw.sensors.rgbclight=no
hw.sensors.temperature=yes
hw.sensors.wrist_tilt=no

# ------------------------------------------------------------------
# SDCARD - Required for video recording
# ------------------------------------------------------------------
hw.sdCard=yes
sdcard.size=512 MB

# ------------------------------------------------------------------
# MEMORY
# ------------------------------------------------------------------
hw.ramSize=6144M
vm.heapSize=768M

# ------------------------------------------------------------------
# SYSTEM IMAGE PATH - Update this for your setup
# ------------------------------------------------------------------
image.sysdir.1=system-images/android-35/google_apis_playstore/x86_64/

# ------------------------------------------------------------------
# KERNEL & RUNTIME
# ------------------------------------------------------------------
kernel.newDeviceNaming=autodetect
kernel.supportsYaffs2=autodetect
runtime.network.latency=none
runtime.network.speed=full

# ------------------------------------------------------------------
# TAGS
# ------------------------------------------------------------------
tag.display=Google Play
tag.id=google_apis_playstore
tag.ids=google_apis_playstore

# ------------------------------------------------------------------
# TEST/DEBUG FLAGS
# ------------------------------------------------------------------
test.delayAdbTillBootComplete=0
test.monitorAdb=0
test.quitAfterBootTimeOut=-1
