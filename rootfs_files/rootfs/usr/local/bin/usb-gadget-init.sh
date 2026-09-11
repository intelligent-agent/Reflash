#!/bin/bash

GADGET_DIR="/sys/kernel/config/usb_gadget/g1"

case "$1" in
    start)
        # Pulls in libcomposite + u_serial and registers the usb_gadget configfs subsystem.
        modprobe usb_f_acm || { echo "modprobe usb_f_acm failed" >&2; exit 1; }

        # configfs registration is asynchronous; wait for the gadget subsystem to appear.
        for i in $(seq 1 50); do
            [ -d /sys/kernel/config/usb_gadget ] && break
            sleep 0.1
        done
        [ -d /sys/kernel/config/usb_gadget ] || { echo "usb_gadget configfs unavailable" >&2; exit 1; }

        # Bind to the first available UDC (musb-hdrc.4.auto on the A64).
        UDC_NAME="$(ls /sys/class/udc 2>/dev/null | head -1)"
        [ -n "$UDC_NAME" ] || { echo "no UDC available" >&2; exit 1; }

        mkdir -p $GADGET_DIR
        cd $GADGET_DIR

        echo 0x1d6b > idVendor
        echo 0x0104 > idProduct
        echo 0x0200 > bcdUSB

        mkdir -p strings/0x409
        echo "0123456789" > strings/0x409/serialnumber
        echo "Iagent" > strings/0x409/manufacturer
        echo "Recore USB Serial" > strings/0x409/product

        # Two ACM functions, in this order: ports are handed out in the order
        # the functions are linked into the config, so acm.usb0 is ttyGS0 and
        # acm.usb1 is ttyGS1, which the host sees as /dev/ttyACM0 and ACM1.
        # ttyGS0 carries the login getty and ttyGS1 the Reflash control
        # protocol - a getty and the protocol cannot share one tty, so they get
        # one each rather than the protocol taking the only USB console.
        mkdir -p functions/acm.usb0
        mkdir -p functions/acm.usb1
        mkdir -p configs/c.1/strings/0x409
        echo "Config 1: Serial" > configs/c.1/strings/0x409/configuration

        # Link function to config
        ln -s functions/acm.usb0 configs/c.1/ 2>/dev/null
        ln -s functions/acm.usb1 configs/c.1/ 2>/dev/null

        # Bind to hardware
        echo "$UDC_NAME" > UDC
        ;;
    stop)
        # Do not unbind on the way down. The point of the teardown is to release
        # the UDC so the service can be restarted at runtime; a reboot resets the
        # controller anyway, so on shutdown it buys nothing - and it is the one
        # step that can hang the board.
        #
        # `echo "" > UDC` unbinds through gserial_disconnect() while the ttyGS0
        # getty and Reflash itself may still be closing their ports. Lose that
        # race and the writing task parks in uninterruptible sleep: no panic,
        # TimeoutStopSec cannot kill it, "reboot: Restarting system" never
        # prints, and the board stays powered and hung with the console already
        # gone. Measured at roughly one reboot in ten on a 0484 loop; the nine
        # that worked cleared this same step in ~0.55s.
        #
        # The ordering drop-in on serial-getty@ttyGS0 closes the race for the
        # runtime path. This closes it for shutdown, where the unbind is pure
        # risk with no benefit.
        if [ "$(systemctl is-system-running 2>/dev/null)" = "stopping" ]; then
            echo "system is shutting down - leaving the gadget bound" >&2
            exit 0
        fi

        if [ -d "$GADGET_DIR" ]; then
            cd $GADGET_DIR
            echo "" > UDC
            rm -f configs/c.1/acm.usb0
            rm -f configs/c.1/acm.usb1
            [ -d "configs/c.1/strings/0x409" ] && rmdir configs/c.1/strings/0x409
            [ -d "configs/c.1" ] && rmdir configs/c.1
            [ -d "functions/acm.usb0" ] && rmdir functions/acm.usb0
            [ -d "functions/acm.usb1" ] && rmdir functions/acm.usb1
            [ -d "strings/0x409" ] && rmdir strings/0x409
            cd ..
            rmdir g1
        fi
        ;;
esac
