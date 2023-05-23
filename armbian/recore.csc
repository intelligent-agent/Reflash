# Allwinner A64 quad core 1GB RAM SoC GBE for 3D printers
BOARD_NAME="Iagent Recore"
BOARDFAMILY="sun50iw1"
BOOTCONFIG="recore_defconfig"
KERNEL_TARGET="legacy,current"
FULL_DESKTOP="no"
MODULES="g_serial"
BOOT_LOGO="yes"

function post_family_config__shrink_atf() {
    echo "Shrink ATF"
    declare -g ATF_TARGET_MAP="PLAT=$ATF_PLAT DEBUG=0 SUNXI_PSCI_USE_SCPI=0 bl31;;build/$ATF_PLAT/release/bl31.bin"

    echo "Compile without SCP binary"
    UBOOT_TARGET_MAP="SCP=/dev/null;;u-boot-sunxi-with-spl.bin"
}
