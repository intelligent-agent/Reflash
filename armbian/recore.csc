# Allwinner A64 quad core 1GB RAM SoC GBE
BOARD_NAME="Recore"
BOARDFAMILY="sun50iw1"
BOOTCONFIG="recore_defconfig"
KERNEL_TARGET="legacy"
FULL_DESKTOP="no"
MODULES="g_serial"
PACKAGE_LIST_BOARD="python3-flask python3-requests pv xz-utils avahi-daemon unzip nginx gunicorn expect iptables dnsmasq-base"
ENABLE_EXTENSIONS=reflash