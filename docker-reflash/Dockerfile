# syntax=docker/dockerfile:1

FROM debian:latest
RUN apt update && apt install -y sudo systemctl debootstrap xz-utils u-boot-tools fdisk cpio e2fsprogs
WORKDIR /app
COPY . .
CMD ["./mkimage.sh"]
